package run

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/engine"
)

// closeRows is a deferred helper that logs a Close error instead of dropping it.
func closeRows(rows *sql.Rows) {
	if err := rows.Close(); err != nil {
		log.Warn().Err(err).Msg("failed to close rows")
	}
}

// snapRow is the frozen recap of one NIK (REKAP + ADD + SCH rows of the snapshot).
type snapRow struct {
	att       engine.AttRekap
	add       engine.AttRekapAdd
	sch       engine.SchRekap
	premiFail int
	berasFail int
}

func nikArray(niks []string) string {
	if len(niks) == 0 {
		return "{}"
	}
	esc := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	q := make([]string, len(niks))
	for i, n := range niks {
		q[i] = `"` + esc.Replace(n) + `"`
	}
	return "{" + strings.Join(q, ",") + "}"
}

// loadWorkdays reads pyr_attd_days (1 query).
func loadWorkdays(ctx context.Context, tx *sql.Tx, q Query) (*int, error) {
	var d int
	err := tx.QueryRowContext(ctx, `SELECT pad_days FROM payroll.pyr_attd_days
 WHERE pad_company_id = $1 AND pad_division = $2 AND pad_yymm = $3`, q.CompanyID, q.Division, q.YYMM).Scan(&d)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s %d", ErrMissingAttdDays, q.Division, q.YYMM)
	}
	if err != nil {
		return nil, fmt.Errorf("load attd days: %w", err)
	}
	return &d, nil
}

// loadEmployees reads the active (sts_payroll = 2) employees of the division (1 query).
func loadEmployees(ctx context.Context, tx *sql.Tx, q Query) ([]engine.Employee, error) {
	rows, err := tx.QueryContext(ctx, `SELECT pe_nik, pe_nama, COALESCE(pe_level,''), COALESCE(pe_tax_id,''),
  COALESCE(pe_dept_id,''), pe_npwp_enc IS NOT NULL, pe_birth_date, pe_date_join
FROM payroll.pyr_employee
WHERE pe_company_id = $1 AND pe_division = $2 AND pe_sts_payroll = 2 AND ($3::text[] = '{}' OR pe_nik = ANY($3::text[]))
ORDER BY pe_nik`, q.CompanyID, q.Division, nikArray(q.NIKs))
	if err != nil {
		return nil, fmt.Errorf("load employees: %w", err)
	}
	defer closeRows(rows)
	var out []engine.Employee
	for rows.Next() {
		var e engine.Employee
		var birth, join sql.NullTime
		if err := rows.Scan(&e.NIK, &e.Name, &e.Level, &e.TaxStatus, &e.Dept, &e.HasNPWP, &birth, &join); err != nil {
			return nil, fmt.Errorf("scan employee: %w", err)
		}
		e.BirthDate, e.JoinDate = timePtr(birth), timePtr(join)
		out = append(out, e)
	}
	return out, rows.Err()
}

func timePtr(t sql.NullTime) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}

// loadComponents reads the attached components (1 query): frz_flag = 2 inside the validity
// window, plus frozen P4/P8/P14/P18/P22/P23 (D-28d, paritas).
func loadComponents(ctx context.Context, tx *sql.Tx, q Query) (map[string][]engine.EmpComponent, error) {
	rows, err := tx.QueryContext(ctx, `SELECT e.pec_nik, c.pc_type::text, c.pc_code, COALESCE(e.pec_amount,0)::text,
  COALESCE(e.pec_sts, c.pc_default_sts), COALESCE(e.pec_tax,0) = 2, COALESCE(e.pec_tax_paid_company,'N') = 'Y',
  c.pc_subsidy_eligible, e.pec_frz_flag <> 2
FROM payroll.pyr_employee_component e
JOIN payroll.pyr_employee p ON p.pe_company_id = e.pec_company_id AND p.pe_nik = e.pec_nik AND p.pe_division = e.pec_division
JOIN payroll.pyr_component c ON c.pc_sysid = e.pec_component_sysid
WHERE e.pec_company_id = $1 AND e.pec_division = $2 AND p.pe_sts_payroll = 2
  AND (e.pec_valid_from IS NULL OR e.pec_valid_from <= $3) AND (e.pec_valid_to IS NULL OR e.pec_valid_to >= $3)
  AND (e.pec_frz_flag = $5 OR (c.pc_type = 'Potongan' AND c.pc_code IN ('P004','P008','P014','P018','P022','P023')))
  AND ($4::text[] = '{}' OR e.pec_nik = ANY($4::text[]))
ORDER BY e.pec_nik, c.pc_type, c.pc_code`, q.CompanyID, q.Division, q.YYMM, nikArray(q.NIKs), frzActive)
	if err != nil {
		return nil, fmt.Errorf("load components: %w", err)
	}
	defer closeRows(rows)
	out := map[string][]engine.EmpComponent{}
	for rows.Next() {
		var nik, typ, code, amount, sts string
		var c engine.EmpComponent
		if err := rows.Scan(&nik, &typ, &code, &amount, &sts, &c.Tax, &c.TaxPaidCompany, &c.TaxSubsidy, &c.Frozen); err != nil {
			return nil, fmt.Errorf("scan component: %w", err)
		}
		var err error
		if c.Key, err = parseCode(typ, code); err != nil {
			return nil, err
		}
		if c.Amount, err = decimal.NewFromString(amount); err != nil {
			return nil, fmt.Errorf("component %s %s: %w", nik, code, err)
		}
		c.Sts = sts
		out[nik] = append(out[nik], c)
	}
	return out, rows.Err()
}

// parseCode maps (pc_type, pc_code 'T001') to the legacy key.
func parseCode(typ, code string) (engine.CompKey, error) {
	t, err := typeOf(typ)
	if err != nil {
		return engine.CompKey{}, err
	}
	n, err := strconv.Atoi(strings.TrimLeft(code, "TPtp"))
	if err != nil || len(code) < 2 {
		return engine.CompKey{}, fmt.Errorf("%w: %q", ErrBadComponentCode, code)
	}
	return engine.CompKey{Type: t, ID: n}, nil
}

// loadAdditions reads pyr_addition (1 query).
func loadAdditions(ctx context.Context, tx *sql.Tx, q Query) (map[string]map[engine.CompKey][]engine.Addition, error) {
	rows, err := tx.QueryContext(ctx, `SELECT pa_nik, pa_type::text, pa_id, pa_amount::text, COALESCE(pa_remarks,''), pa_source
FROM payroll.pyr_addition
WHERE pa_company_id = $1 AND pa_division = $2 AND pa_yymm = $3 AND ($4::text[] = '{}' OR pa_nik = ANY($4::text[]))
ORDER BY pa_nik, pa_type, pa_id`, q.CompanyID, q.Division, q.YYMM, nikArray(q.NIKs))
	if err != nil {
		return nil, fmt.Errorf("load additions: %w", err)
	}
	defer closeRows(rows)
	out := map[string]map[engine.CompKey][]engine.Addition{}
	for rows.Next() {
		var nik, typ, amount string
		var id int
		var a engine.Addition
		if err := rows.Scan(&nik, &typ, &id, &amount, &a.Remark, &a.Source); err != nil {
			return nil, fmt.Errorf("scan addition: %w", err)
		}
		t, err := typeOf(typ)
		if err != nil {
			return nil, err
		}
		if a.Amount, err = decimal.NewFromString(amount); err != nil {
			return nil, fmt.Errorf("addition %s: %w", nik, err)
		}
		if out[nik] == nil {
			out[nik] = map[engine.CompKey][]engine.Addition{}
		}
		k := engine.CompKey{Type: t, ID: id}
		out[nik][k] = append(out[nik][k], a)
	}
	return out, rows.Err()
}

// loadSnapshot reads the newest locked attendance snapshot of the period (2 queries).
// Keys of pasr_data (MPAR_DAY_WORK, MPARA_SHIFT, ...) become lower case without prefix
// (day_work, shift), the names the engine reads. Missing recap = absent from the map (D2).
func loadSnapshot(ctx context.Context, tx *sql.Tx, q Query) (map[string]snapRow, error) {
	var id int64
	err := tx.QueryRowContext(ctx, `SELECT pas_snapshot_id FROM payroll.pyr_att_snapshot
 WHERE pas_company_id = $1 AND pas_division = $2 AND pas_yymm = $3 AND pas_locked
 ORDER BY pas_snapshot_id DESC LIMIT 1`, q.CompanyID, q.Division, q.YYMM).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s %d", ErrNoSnapshot, q.Division, q.YYMM)
	}
	if err != nil {
		return nil, fmt.Errorf("find snapshot: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `SELECT pasr_kind, pasr_nik, pasr_data::text, pasr_premi_fail, pasr_beras_fail
FROM payroll.pyr_att_snapshot_row WHERE pasr_snapshot_id = $1 AND ($2::text[] = '{}' OR pasr_nik = ANY($2::text[]))
ORDER BY pasr_nik, pasr_kind`, id, nikArray(q.NIKs))
	if err != nil {
		return nil, fmt.Errorf("load snapshot rows: %w", err)
	}
	defer closeRows(rows)
	out := map[string]snapRow{}
	for rows.Next() {
		var kind, nik, data string
		var premi, beras sql.NullInt32
		if err := rows.Scan(&kind, &nik, &data, &premi, &beras); err != nil {
			return nil, fmt.Errorf("scan snapshot row: %w", err)
		}
		r := out[nik]
		if err := r.apply(kind, data); err != nil {
			return nil, fmt.Errorf("snapshot %s %s: %w", nik, kind, err)
		}
		if kind == "REKAP" {
			r.premiFail, r.berasFail = int(premi.Int32), int(beras.Int32)
		}
		out[nik] = r
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// A recap exists only with a REKAP row; ADD/SCH alone leave Att nil (legacy D2).
	for nik, r := range out {
		if r.att == nil {
			delete(out, nik)
		}
	}
	return out, nil
}

// apply folds one snapshot row into the recap of its NIK.
func (r *snapRow) apply(kind, data string) error {
	dec := json.NewDecoder(strings.NewReader(data))
	dec.UseNumber()
	var raw map[string]any
	if err := dec.Decode(&raw); err != nil {
		return err
	}
	switch kind {
	case "REKAP":
		m, err := recapColumns(raw, "MPAR_")
		r.att = m
		return err
	case "ADD":
		m, err := recapColumns(raw, "MPARA_")
		r.add = m
		return err
	case "SCH":
		sch, ok := raw["SCHEDULES"].(map[string]any)
		if !ok && raw["SCHEDULES"] != nil {
			return fmt.Errorf("%w: SCHEDULES", ErrBadSnapshotValue)
		}
		m := engine.SchRekap{}
		for code, v := range sch {
			d, ok, err := toDecimal(v)
			if err != nil {
				return err
			}
			if ok {
				m[code] = d
			}
		}
		r.sch = m
	}
	return nil
}

// recapColumns lower-cases the column names and drops the prefix; null values are absent
// (the engine reads an absent column as zero, legacy NVL).
func recapColumns(raw map[string]any, prefix string) (engine.AttRekap, error) {
	m := engine.AttRekap{}
	for k, v := range raw {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		d, ok, err := toDecimal(v)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", k, err)
		}
		if ok {
			m[strings.ToLower(strings.TrimPrefix(k, prefix))] = d
		}
	}
	return m, nil
}

func toDecimal(v any) (decimal.Decimal, bool, error) {
	switch x := v.(type) {
	case nil:
		return decimal.Zero, false, nil
	case json.Number:
		d, err := decimal.NewFromString(x.String())
		return d, err == nil, err
	case string:
		d, err := decimal.NewFromString(x)
		if err != nil {
			return decimal.Zero, false, fmt.Errorf("%w: %q", ErrBadSnapshotValue, x)
		}
		return d, true, nil
	}
	return decimal.Zero, false, fmt.Errorf("%w: %v", ErrBadSnapshotValue, v)
}

// loadGapokHolders returns the NIKs with a T1 row in pyr_employee_component, whatever its frz
// flag or validity (05§7: JAKARTA beras has no frz condition, like legacy).
func loadGapokHolders(ctx context.Context, tx *sql.Tx, q Query) (map[string]bool, error) {
	rows, err := tx.QueryContext(ctx, `SELECT e.pec_nik FROM payroll.pyr_employee_component e
JOIN payroll.pyr_component c ON c.pc_sysid = e.pec_component_sysid
WHERE e.pec_company_id = $1 AND e.pec_division = $2 AND c.pc_code = 'T001'
  AND ($3::text[] = '{}' OR e.pec_nik = ANY($3::text[]))`, q.CompanyID, q.Division, nikArray(q.NIKs))
	if err != nil {
		return nil, fmt.Errorf("load T1 holders: %w", err)
	}
	defer closeRows(rows)
	out := map[string]bool{}
	for rows.Next() {
		var nik string
		if err := rows.Scan(&nik); err != nil {
			return nil, fmt.Errorf("scan T1 holder: %w", err)
		}
		out[nik] = true
	}
	return out, rows.Err()
}
