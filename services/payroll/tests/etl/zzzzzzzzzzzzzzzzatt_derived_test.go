package etl

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/recon"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/etl"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/postgres"
)

// T021e: ATT_DERIVED option A (05§5). Period 203911 = 2039-10-26 .. 2039-11-25 (31 days).
const derivedYM = 203911

func seedDerivedMasters(t *testing.T) {
	t.Helper()
	comp := func(nik, code string, frz int, validTo string) string {
		return fmt.Sprintf(`INSERT INTO payroll.pyr_employee_component (pec_company_id, pec_nik, pec_division, pec_component_sysid, pec_sts, pec_tax, pec_amount, pec_frz_flag, pec_valid_to)
		 SELECT 1, '%s', 'SOLO', c.pc_sysid, 'F', 2, 100000, %d, %s FROM payroll.pyr_component c WHERE c.pc_company_id = 1 AND c.pc_code = '%s'`, nik, frz, validTo, code)
	}
	for _, q := range []string{
		`INSERT INTO payroll.pyr_component (pc_company_id, pc_code, pc_type, pc_name, pc_status, pc_valid_from) VALUES
		  (1,'T006','Tunjangan','OT in work','ACTIVE',202001), (1,'T034','Tunjangan','Shift N','ACTIVE',202001),
		  (1,'T035','Tunjangan','Shift S','ACTIVE',202001) ON CONFLICT (pc_company_id, pc_code) DO NOTHING`,
		`INSERT INTO payroll.pyr_employee (pe_company_id, pe_nik, pe_division, pe_nama, pe_level, pe_tax_id, pe_sts_payroll, pe_orion_sub_acnt) VALUES
		  (1,'DV-A','SOLO','A','Driver','TK',2,'DV-A'), (1,'DV-B','SOLO','B','Staff','TK',2,'DV-B'), (1,'DV-C','SOLO','C','Driver','TK',2,'DV-C'),
		  (1,'DV-D','SOLO','D','Driver','TK',2,'DV-D'), (1,'DV-E','SOLO','E','Driver','TK',2,'DV-E')`,
		comp("DV-A", "T034", 2, "NULL"), comp("DV-A", "T006", 2, "NULL"), // shift + otiw
		comp("DV-B", "T035", 2, "NULL"),                                  // shift (staff), no T6
		comp("DV-D", "T034", 1, "NULL"), comp("DV-D", "T006", 1, "NULL"), // frozen: not eligible (view: frz = 2)
		comp("DV-E", "T034", 2, "203910"), comp("DV-E", "T006", 2, "203910"), // validity ended before the period
	} {
		if _, err := ownDB.Exec(q); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}
}

// derivedDays: 31 days; days 0–4 Shift with hours, day 5 Shift with NULL hours, days 6–7 OT-in-work 'No'.
func derivedDays(nik string) []etl.DailyRow {
	out := make([]etl.DailyRow, 31)
	for i := range out {
		out[i] = etl.DailyRow{WorkDate: time.Date(2039, 10, 26+i, 0, 0, 0, 0, time.UTC), NIK: nik, Division: "SOLO",
			ScheduleCode: "NW", WorkHours: nd2("8"), SchType: "Normal", OtInwork: "Yes"}
	}
	for i := 0; i <= 5; i++ {
		out[i].ScheduleCode, out[i].SchType = "S1", "Shift"
	}
	out[5].WorkHours = nd2("") // actual_hour NULL: not a shift day
	out[6].OtInwork, out[7].OtInwork = "No", "No"
	out[8].SchType, out[8].OtInwork = "", "" // schedule not in HM_SCHEDULE: NULL type/flag
	return out
}

// derivedRekap: day_work 20, dayoff 4, cuti 1, alpa 1, ijin 0, dirumahkan NULL → Σ = 26; otiw_decr = 2 + 31 − 26 = 7.
func derivedRekap(nik, shift, otiw string) etl.RekapRow {
	r := rrow(derivedYM, nik, "SOLO", "20")
	for k, v := range map[string]string{"dayoff": "4", "cuti": "1", "alpa": "1", "ijin": "0", "shift": shift, "otiw_decr": otiw} {
		r.Nums[k] = nd2(v)
	}
	return r
}

func TestAttDerived(t *testing.T) {
	seedDerivedMasters(t)
	var daily []etl.DailyRow
	for _, n := range []string{"DV-A", "DV-B", "DV-C", "DV-D", "DV-E"} {
		daily = append(daily, derivedDays(n)...)
	}
	f := &fakeAtt{
		rekap: []etl.RekapRow{
			derivedRekap("DV-A", "5", "7"), // equal to derived
			derivedRekap("DV-B", "4", "0"), // shift differs (+1)
			derivedRekap("DV-C", "", ""),   // no T34/T35/T6: derived 0, MPAR NULL = 0
			derivedRekap("DV-D", "5", "7"), // frozen components: derived 0 → both differ
			derivedRekap("DV-E", "0", "0"), // validity ended: derived 0
			derivedRekap("DV-F", "3", "1"), // no daily rows: not checked (D-60)
		},
		daily: daily,
	}
	syncAll(t, f, derivedYM, "SOLO")
	if got := scalar(t, `SELECT count(*) FILTER (WHERE psad_sch_type='Shift')||'/'||count(*) FILTER (WHERE psad_ot_inwork='No')||'/'||
		count(*) FILTER (WHERE psad_sch_type IS NULL AND psad_ot_inwork IS NULL) FROM payroll_stg.pyr_stg_att_daily WHERE psad_nik='DV-A'`); got != "6/2/1" {
		t.Fatalf("staging sch_type/ot_inwork %s", got)
	}
	s := snapOne(t, derivedYM, "SOLO")

	t.Run("snapshot keys (MPAR_* unchanged)", func(t *testing.T) {
		for nik, want := range map[string]string{"DV-A": "5/7/5/7", "DV-B": "5/0/4/0", "DV-C": "0/0/null/null", "DV-D": "0/0/5/7", "DV-E": "0/0/0/0"} {
			got := scalar(t, fmt.Sprintf(`SELECT concat_ws('/', pasr_data->>'derived_shift', pasr_data->>'derived_otiw_decr',
				coalesce(pasr_data->>'MPAR_SHIFT','null'), coalesce(pasr_data->>'MPAR_OTIW_DECR','null'))
				FROM payroll.pyr_att_snapshot_row WHERE pasr_snapshot_id=%d AND pasr_kind='REKAP' AND pasr_nik='%s'`, s.SnapshotID, nik))
			if got != want {
				t.Errorf("%s derived_shift/derived_otiw/MPAR_SHIFT/MPAR_OTIW = %s, want %s", nik, got, want)
			}
		}
	})

	t.Run("NIK without daily rows: no derived keys", func(t *testing.T) {
		if got := scalar(t, fmt.Sprintf(`SELECT (pasr_data ? 'derived_shift')::text||'/'||(pasr_data->>'MPAR_SHIFT') FROM payroll.pyr_att_snapshot_row
			WHERE pasr_snapshot_id=%d AND pasr_kind='REKAP' AND pasr_nik='DV-F'`, s.SnapshotID)); got != "false/3" {
			t.Fatalf("DV-F %s", got)
		}
	})

	t.Run("attChanged ignores derived keys (no false INPUT_ATT)", func(t *testing.T) {
		// A LEGACY-format copy of the synced snapshot: same MPAR_*, no derived keys.
		var legacy int64
		if err := ownDB.QueryRow(`INSERT INTO payroll.pyr_att_snapshot (pas_company_id, pas_yymm, pas_division, pas_source_batch_ids, pas_checksum, created_by)
			VALUES (1, $1, 'SOLO', '{}', 'x', 'test') RETURNING pas_snapshot_id`, derivedYM).Scan(&legacy); err != nil {
			t.Fatal(err)
		}
		if _, err := ownDB.Exec(`INSERT INTO payroll.pyr_att_snapshot_row (pasr_snapshot_id, pasr_kind, pasr_nik, pasr_data, pasr_premi_fail, pasr_beras_fail)
			SELECT $1, pasr_kind, pasr_nik, pasr_data - 'derived_shift' - 'derived_otiw_decr', pasr_premi_fail, pasr_beras_fail
			FROM payroll.pyr_att_snapshot_row WHERE pasr_snapshot_id = $2`, legacy, s.SnapshotID); err != nil {
			t.Fatal(err)
		}
		raw, err := sql.Open("pgx", appDSN)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = raw.Close() }()
		var changed map[string]bool
		if err := postgres.NewDBFromSQL(raw).ScopedTx(asScopes(t, "SOLO"), func(tx *sql.Tx) error {
			changed, err = recon.LoadAttChanged(context.Background(), tx, legacy, s.SnapshotID)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if len(changed) != 0 {
			t.Fatalf("derived keys counted as INPUT_ATT: %v", changed)
		}
	})

	t.Run("recon reads MPAR vs derived as payroll_app", func(t *testing.T) {
		raw, err := sql.Open("pgx", appDSN)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = raw.Close() }()
		var snap int64
		var rows []recon.AttDerivedRow
		if err := postgres.NewDBFromSQL(raw).ScopedTx(asScopes(t, "SOLO"), func(tx *sql.Tx) error {
			snap, rows, err = recon.LoadAttDerived(context.Background(), tx, 1, "SOLO", derivedYM)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if snap != s.SnapshotID || len(rows) != 10 { // DV-F has no derived keys
			t.Fatalf("snapshot %d (want %d), rows %d", snap, s.SnapshotID, len(rows))
		}
		rep := recon.Diff(recon.DiffInput{Derived: rows})
		// DV-B shift, DV-D shift + otiw. DV-C NULL vs 0 and DV-E 0 vs 0 match.
		if n := rep.Summary.Categories[recon.CatAttDerived]; n != 3 {
			t.Fatalf("ATT_DERIVED = %d: %+v", n, rep.AttDerived)
		}
		if d := rep.AttDerived[0]; d.NIK != "DV-B" || d.Field != recon.DerivedShift || d.Delta.String() != "1" {
			t.Errorf("first diff %+v", d)
		}
	})

	t.Run("no synced snapshot: not checked", func(t *testing.T) {
		raw, err := sql.Open("pgx", appDSN)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = raw.Close() }()
		if err := postgres.NewDBFromSQL(raw).ScopedTx(asScopes(t, "SOLO"), func(tx *sql.Tx) error {
			snap, rows, err := recon.LoadAttDerived(context.Background(), tx, 1, "SOLO", derivedYM+1)
			if err == nil && (snap != 0 || rows != nil) {
				err = fmt.Errorf("want none, got %d %d rows", snap, len(rows))
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
	})
}
