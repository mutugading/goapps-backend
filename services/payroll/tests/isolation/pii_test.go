package isolation

// I21 (09§6.5, D-17): PII columns are bytea ciphertext and payroll_ro cannot decrypt them,
// because the key lives only in the application config (never in the DB). I22/I23 need the
// API layer and are deferred to B-API-RUN / B-IAM1.

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

const (
	i21Key   = "i21-test-enc-key-0123456789abcdef"
	i21NPWP  = "99.888.777.6-543.000"
	i21KTP   = "3374019999990001"
	i21Bank  = "0987654321"
	i21NIK   = "I21-PII"
	i21Wrong = "not-the-key"
)

// roTx runs q on a payroll_ro session scoped to company 1 SOLO (RLS needs a scope even to read).
func roQuery(t *testing.T, ro *sql.DB, q string, args []any, dest ...any) error {
	t.Helper()
	ctx := context.Background()
	tx, err := ro.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }() //nolint:errcheck // read-only tx
	if _, err := tx.ExecContext(ctx, `SELECT set_config('app.scopes', '1:SOLO', true)`); err != nil {
		t.Fatal(err)
	}
	return tx.QueryRowContext(ctx, q, args...).Scan(dest...)
}

func TestI21_PIIEncryptedAndROCannotDecrypt(t *testing.T) {
	t.Cleanup(func() {
		if _, err := ownDB.Exec(`DELETE FROM payroll.pyr_employee WHERE pe_company_id=1 AND pe_nik=$1`, i21NIK); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	})
	// 1. Schema: every PII column is bytea and no plaintext twin exists.
	var nBytea int
	if err := ownDB.QueryRow(`SELECT count(*) FROM information_schema.columns WHERE table_schema='payroll'
	  AND data_type='bytea' AND ((table_name='pyr_employee' AND column_name IN ('pe_npwp_enc','pe_ktp_enc','pe_bank_acnt_enc'))
	   OR (table_name='pyr_company' AND column_name='pco_npwp_enc'))`).Scan(&nBytea); err != nil {
		t.Fatal(err)
	}
	if nBytea != 4 {
		t.Fatalf("I21: %d of 4 PII columns are bytea", nBytea)
	}
	var plain string
	if err := ownDB.QueryRow(`SELECT coalesce(string_agg(table_name||'.'||column_name, ','), '') FROM information_schema.columns
	  WHERE table_schema='payroll' AND column_name IN ('pe_npwp','pe_ktp','pe_bank_acnt','pco_npwp')`).Scan(&plain); err != nil {
		t.Fatal(err)
	}
	if plain != "" {
		t.Fatalf("I21: plaintext PII column(s): %s", plain)
	}

	// 2. Store ciphertext (as the application does, key as bind parameter).
	if _, err := ownDB.Exec(`INSERT INTO payroll.pyr_employee (pe_company_id, pe_nik, pe_division, pe_nama, pe_orion_sub_acnt,
	  pe_npwp_enc, pe_ktp_enc, pe_bank_acnt_enc)
	  VALUES (1, $1, 'SOLO', 'I21 Employee', 'x', payroll.pgp_sym_encrypt($2::text,$5::text),
	          payroll.pgp_sym_encrypt($3::text,$5::text), payroll.pgp_sym_encrypt($4::text,$5::text))`,
		i21NIK, i21NPWP, i21KTP, i21Bank, i21Key); err != nil {
		t.Fatal(err)
	}

	// 3. payroll_ro sees only ciphertext, and nothing it can call reveals the plaintext without the key.
	ro := openAs(t, "payroll_ro", "payroll_ro123", "payroll")
	var npwp, ktp, bank []byte
	if err := roQuery(t, ro, `SELECT pe_npwp_enc, pe_ktp_enc, pe_bank_acnt_enc FROM payroll.pyr_employee WHERE pe_nik=$1`,
		[]any{i21NIK}, &npwp, &ktp, &bank); err != nil {
		t.Fatalf("I21: ro read: %v", err)
	}
	for name, c := range map[string][]byte{"npwp": npwp, "ktp": ktp, "bank": bank} {
		if len(c) == 0 {
			t.Fatalf("I21: %s ciphertext empty", name)
		}
		for _, p := range []string{i21NPWP, i21KTP, i21Bank} {
			if bytes.Contains(c, []byte(p)) {
				t.Errorf("I21: %s ciphertext contains plaintext %q", name, p)
			}
		}
	}
	for label, key := range map[string]string{"wrong key": i21Wrong, "empty key": ""} {
		var out sql.NullString
		err := roQuery(t, ro, `SELECT payroll.pgp_sym_decrypt(pe_bank_acnt_enc, $2::text) FROM payroll.pyr_employee WHERE pe_nik=$1`,
			[]any{i21NIK, key}, &out)
		var pe *pgconn.PgError
		if !errors.As(err, &pe) || pe.Code != "39000" {
			t.Errorf("I21: decrypt with %s: want SQLSTATE 39000, got %v (value %q)", label, err, out.String)
		}
	}
	var out sql.NullString
	if err := roQuery(t, ro, `SELECT payroll.pgp_sym_decrypt(pe_bank_acnt_enc, NULL::text) FROM payroll.pyr_employee WHERE pe_nik=$1`,
		[]any{i21NIK}, &out); err != nil || out.Valid {
		t.Errorf("I21: decrypt with NULL key = %q, %v; want NULL", out.String, err)
	}

	// 4. The key is nowhere in the DB (D-17): settings, per-role settings and function bodies.
	var hits int
	if err := roQuery(t, ro, `SELECT
	  (SELECT count(*) FROM pg_settings WHERE setting LIKE '%'||$1||'%')
	+ (SELECT count(*) FROM pg_db_role_setting s, unnest(s.setconfig) c WHERE c LIKE '%'||$1||'%')
	+ (SELECT count(*) FROM pg_proc WHERE prosrc LIKE '%'||$1||'%')`, []any{i21Key}, &hits); err != nil {
		t.Fatal(err)
	}
	if hits != 0 {
		t.Errorf("I21: encryption key found in %d catalog place(s)", hits)
	}
	if strings.Contains(strings.ToLower(i21Key), "payroll") {
		t.Fatal("test key must not look like a real one")
	}
}

func TestI22_I23_DeferredToAPI(t *testing.T) {
	t.Skip("I22 (JWT payroll_scope -> 403 DIVISION_FORBIDDEN) needs the run API: B-API-RUN; " +
		"I23 (nominal masked without payroll.salary.view) needs the permission in IAM: B-IAM1")
}
