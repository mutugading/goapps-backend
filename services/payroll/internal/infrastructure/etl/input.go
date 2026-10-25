package etl

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"

	"github.com/rs/zerolog/log"
)

// Legacy rule & input tables (T009c, 08§2). STATIC_VALUE is IM_VS_STATIC_VALUE (PAYROLLSETUP, EMPTAX).
const (
	TableStaticValue    = "IM_VS_STATIC_VALUE"
	TableAttdDays       = "MGT_PAYROLL_ATTD_DAYS"
	TableSubsidiPPh     = "MGT_PAYROLL_SUBSIDI_PPH"
	TableTaxAdj         = "MGT_PAYROLL_TAX_ADJ"
	TableHoliday        = "MGT_PAYROLL_HOLIDAY"
	TableAddition       = "MGT_PAYROLL_ADDITION"
	TableAbnormalPremi  = "MGT_PAYROLL_ABNORMAL_PREMI"
	TableBatalPremi     = "MGT_PAYROLL_BATAL_PREMI"
	LegacyAdditionLabel = "LEGACY" // pyr_addition.pa_source baris hasil ETL (08§2)
	legacyRuleSetID     = 1
)

// InputOptions controls one rule & input load.
type InputOptions struct {
	CompanyID int16
	// Expect* = 0 means unchecked; production sizes per 08§2.
	ExpectAddition, ExpectAbnormal int
}

// ruleConstants are the legacy magic numbers kept as rule_set 1 constants (06§1 DM/HM/BD, 06§4 BPJS limits).
// They do not exist in an Oracle table, so they are seeded from the spec (D8: same values).
var ruleConstants = []struct {
	key, division string
	value         string
}{
	{"DAYS_MONTH", "", "30"}, {"HOURS_MONTH", "", "173"}, {"BERAS_DAYS", "", "25"},
	{"KESMIN", "JAKARTA", "5729876"}, {"KESMAX", "JAKARTA", "12000000"}, {"NAKMIN", "JAKARTA", "5729876"},
	{"PENMIN", "JAKARTA", "5729876"}, {"PENMAX", "JAKARTA", "11086300"},
	{"KESMIN", "SOLO", "2592154"}, {"KESMAX", "SOLO", "12000000"}, {"NAKMIN", "SOLO", "2592154"},
	{"PENMIN", "SOLO", "2592154"}, {"PENMAX", "SOLO", "11086300"},
}

// reqDate parses a mandatory date cell with the shared parseDate (employee.go); empty is an error.
func reqDate(col, v string) (string, error) {
	d, err := parseDate(v)
	if err != nil || d == "" {
		return "", fmt.Errorf("%s %q bukan tanggal", col, v)
	}
	return d, nil
}

// rowLoader writes one source row. A non-empty exception string means "not loaded, reported".
type rowLoader func(r []string, idx map[string]int, line int) (exception string, err error)

// loadTable runs fn over every row of src and counts loaded rows (08§6 row count + checksum).
func loadTable(src *Table, cols []string, fn rowLoader) (TableReport, error) {
	rep := TableReport{Table: src.Name, SrcRows: len(src.Rows), Checksum: src.Checksum()}
	idx, err := bindCols(src, cols)
	if err != nil {
		return rep, err
	}
	for i, r := range src.Rows {
		exc, err := fn(r, idx, i+2)
		if err != nil {
			return rep, fmt.Errorf("%s baris %d: %w", src.Name, i+2, err)
		}
		if exc != "" {
			rep.Exceptions = append(rep.Exceptions, fmt.Sprintf("%s baris %d: %s", src.Name, i+2, exc))
			continue
		}
		rep.DstRows++
	}
	return rep, nil
}

func atoi(s string) (int, error) { return strconv.Atoi(s) }

// LoadStaticValues fills pyr_static_value for rule_set 1 from IM_VS_STATIC_VALUE.
func LoadStaticValues(ctx context.Context, tx *sql.Tx, src *Table) (TableReport, error) {
	return loadTable(src, []string{"VSSV_VS_CODE", "VSSV_CODE", "VSSV_NAME", "VSSV_SHORT_NAME", "VSSV_FIELD_01", "VSSV_FIELD_02"},
		func(r []string, ix map[string]int, _ int) (string, error) {
			_, err := tx.ExecContext(ctx, `
INSERT INTO payroll.pyr_static_value (psv_rule_set_id, psv_vs_code, psv_code, psv_name, psv_short_name, psv_field_01, psv_field_02)
VALUES ($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT (psv_rule_set_id, psv_vs_code, psv_code) DO UPDATE SET psv_name=EXCLUDED.psv_name,
  psv_short_name=EXCLUDED.psv_short_name, psv_field_01=EXCLUDED.psv_field_01, psv_field_02=EXCLUDED.psv_field_02`,
				legacyRuleSetID, r[ix["VSSV_VS_CODE"]], r[ix["VSSV_CODE"]], nullable(r[ix["VSSV_NAME"]]),
				nullable(r[ix["VSSV_SHORT_NAME"]]), nullable(r[ix["VSSV_FIELD_01"]]), nullable(r[ix["VSSV_FIELD_02"]]))
			return "", err
		})
}

// LoadRuleConstants upserts rule_set 1 (legacy, rules v1) and its constants; returns the constant count.
func LoadRuleConstants(ctx context.Context, tx *sql.Tx, company int16) (TableReport, error) {
	rep := TableReport{Table: "pyr_rule_constant", Checksum: "-"}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO payroll.pyr_rule_set (prs_id, prs_name, prs_effective_from, prs_legacy_compat, prs_status)
VALUES ($1,'Legacy v1 (paritas Oracle)',201712,true,'ACTIVE') ON CONFLICT (prs_id) DO NOTHING`, legacyRuleSetID); err != nil {
		return rep, err
	}
	for _, c := range ruleConstants {
		var div any
		if c.division != "" {
			div = c.division
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO payroll.pyr_rule_constant (prc_rule_set_id, prc_company_id, prc_key, prc_value, prc_division)
VALUES ($1,$2,$3,$4::numeric,$5)
ON CONFLICT (prc_rule_set_id, prc_company_id, prc_key, COALESCE(prc_division,'*')) DO UPDATE SET prc_value=EXCLUDED.prc_value`,
			legacyRuleSetID, company, c.key, c.value, div); err != nil {
			return rep, fmt.Errorf("rule_constant %s: %w", c.key, err)
		}
	}
	rep.SrcRows = len(ruleConstants)
	err := tx.QueryRowContext(ctx, `SELECT count(*) FROM payroll.pyr_rule_constant WHERE prc_rule_set_id=$1 AND prc_company_id=$2`,
		legacyRuleSetID, company).Scan(&rep.DstRows)
	return rep, err
}

// LoadAttdDays fills pyr_attd_days (MPAD_*).
func LoadAttdDays(ctx context.Context, tx *sql.Tx, src *Table, company int16) (TableReport, error) {
	return loadTable(src, []string{"MPAD_YYMM", "MPAD_DIVISION", "MPAD_DAYS"}, func(r []string, ix map[string]int, _ int) (string, error) {
		days, err := atoi(r[ix["MPAD_DAYS"]])
		if err != nil {
			return "", fmt.Errorf("MPAD_DAYS %q", r[ix["MPAD_DAYS"]])
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO payroll.pyr_attd_days (pad_company_id, pad_yymm, pad_division, pad_days) VALUES ($1,$2,$3,$4)
ON CONFLICT (pad_company_id, pad_yymm, pad_division) DO UPDATE SET pad_days=EXCLUDED.pad_days`,
			company, r[ix["MPAD_YYMM"]], r[ix["MPAD_DIVISION"]], days)
		return "", err
	})
}

// LoadSubsidiPPh fills pyr_subsidi_pph (MPSP_*).
func LoadSubsidiPPh(ctx context.Context, tx *sql.Tx, src *Table, company int16) (TableReport, error) {
	return loadTable(src, []string{"MPSP_YYMM", "MPSP_DIVISION", "MPSP_DEPT", "MPSP_YN", "MPSP_LIMIT_AMT"}, func(r []string, ix map[string]int, _ int) (string, error) {
		_, err := tx.ExecContext(ctx, `
INSERT INTO payroll.pyr_subsidi_pph (psp_company_id, psp_yymm, psp_division, psp_dept, psp_yn, psp_limit_amt) VALUES ($1,$2,$3,$4,$5,$6::numeric)
ON CONFLICT (psp_company_id, psp_yymm, psp_division, psp_dept) DO UPDATE SET psp_yn=EXCLUDED.psp_yn, psp_limit_amt=EXCLUDED.psp_limit_amt`,
			company, r[ix["MPSP_YYMM"]], r[ix["MPSP_DIVISION"]], r[ix["MPSP_DEPT"]], nullable(r[ix["MPSP_YN"]]), nullable(r[ix["MPSP_LIMIT_AMT"]]))
		return "", err
	})
}

// LoadTaxAdj fills pyr_tax_adj (MPTA_*).
func LoadTaxAdj(ctx context.Context, tx *sql.Tx, src *Table, company int16) (TableReport, error) {
	return loadTable(src, []string{"MPTA_YYMM", "MPTA_DIVISION", "MPTA_NIK", "MPTA_NAMA", "MPTA_TYPE", "MPTA_REASON", "MPTA_AMOUNT"}, func(r []string, ix map[string]int, _ int) (string, error) {
		_, err := tx.ExecContext(ctx, `
INSERT INTO payroll.pyr_tax_adj (pta_company_id, pta_yymm, pta_division, pta_nik, pta_type, pta_nama, pta_reason, pta_amount)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8::numeric)
ON CONFLICT (pta_company_id, pta_yymm, pta_division, pta_nik, pta_type) DO UPDATE SET pta_nama=EXCLUDED.pta_nama,
  pta_reason=EXCLUDED.pta_reason, pta_amount=EXCLUDED.pta_amount`,
			company, r[ix["MPTA_YYMM"]], r[ix["MPTA_DIVISION"]], r[ix["MPTA_NIK"]], r[ix["MPTA_TYPE"]], nullable(r[ix["MPTA_NAMA"]]),
			nullable(r[ix["MPTA_REASON"]]), nullable(r[ix["MPTA_AMOUNT"]]))
		return "", err
	})
}

// LoadHoliday fills pyr_holiday. The table has a surrogate id, so idempotency is by (date, descr, status, year).
func LoadHoliday(ctx context.Context, tx *sql.Tx, src *Table) (TableReport, error) {
	rep, err := loadTable(src, []string{"MPH_DATE", "MPH_DESCR", "MPH_STATUS", "MPH_YEAR"}, func(r []string, ix map[string]int, _ int) (string, error) {
		d, err := reqDate("MPH_DATE", r[ix["MPH_DATE"]])
		if err != nil {
			return "", err
		}
		var year any
		if v := r[ix["MPH_YEAR"]]; v != "" {
			if year, err = atoi(v); err != nil {
				return "", fmt.Errorf("MPH_YEAR %q", v)
			}
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO payroll.pyr_holiday (phd_date, phd_descr, phd_status, phd_year)
SELECT $1,$2,$3,$4 WHERE NOT EXISTS (SELECT 1 FROM payroll.pyr_holiday WHERE phd_date=$1
  AND phd_descr IS NOT DISTINCT FROM $2::varchar AND phd_status IS NOT DISTINCT FROM $3::varchar AND phd_year IS NOT DISTINCT FROM $4::int)`,
			d, nullable(r[ix["MPH_DESCR"]]), nullable(r[ix["MPH_STATUS"]]), year)
		return "", err
	})
	if err != nil {
		return rep, err
	}
	err = tx.QueryRowContext(ctx, `SELECT count(*) FROM payroll.pyr_holiday`).Scan(&rep.DstRows)
	return rep, err
}

// LoadAddition fills pyr_addition with source LEGACY (MPA_*). Closed-period rows are fine: the guard
// (04§9) only fires for non-ETL changes of CLOSED periods that exist, and ETL runs before period import.
func LoadAddition(ctx context.Context, tx *sql.Tx, src *Table, company int16) (TableReport, error) {
	return loadTable(src, []string{"MPA_YYMM", "MPA_NIK", "MPA_NAMA", "MPA_COMPNT_TYPE", "MPA_COMPNT_ID", "MPA_COMPNT_NAME",
		"MPA_COMPNT_STS", "MPA_COMPNT_TAX", "MPA_AMOUNT", "MPA_REMARKS", "MPA_DIVISION"}, func(r []string, ix map[string]int, _ int) (string, error) {
		id, err := atoi(r[ix["MPA_COMPNT_ID"]])
		if err != nil {
			return "", fmt.Errorf("MPA_COMPNT_ID %q", r[ix["MPA_COMPNT_ID"]])
		}
		var tax any
		if v := r[ix["MPA_COMPNT_TAX"]]; v != "" {
			if tax, err = atoi(v); err != nil {
				return "", fmt.Errorf("MPA_COMPNT_TAX %q", v)
			}
		}
		amount := r[ix["MPA_AMOUNT"]]
		if amount == "" {
			amount = "0" // pa_amount NOT NULL; legacy kolom nullable
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO payroll.pyr_addition (pa_company_id, pa_yymm, pa_nik, pa_division, pa_type, pa_id, pa_nama, pa_compnt_name, pa_compnt_sts,
  pa_compnt_tax, pa_amount, pa_remarks, pa_source)
VALUES ($1,$2,$3,$4,$5::payroll.comp_type,$6,$7,$8,$9,$10,$11::numeric,$12,$13)
ON CONFLICT (pa_company_id, pa_yymm, pa_nik, pa_type, pa_id, pa_division) DO UPDATE SET pa_nama=EXCLUDED.pa_nama,
  pa_compnt_name=EXCLUDED.pa_compnt_name, pa_compnt_sts=EXCLUDED.pa_compnt_sts, pa_compnt_tax=EXCLUDED.pa_compnt_tax,
  pa_amount=EXCLUDED.pa_amount, pa_remarks=EXCLUDED.pa_remarks`,
			company, r[ix["MPA_YYMM"]], r[ix["MPA_NIK"]], r[ix["MPA_DIVISION"]], compType(r[ix["MPA_COMPNT_TYPE"]]), id,
			nullable(r[ix["MPA_NAMA"]]), nullable(r[ix["MPA_COMPNT_NAME"]]), nullable(r[ix["MPA_COMPNT_STS"]]), tax, amount,
			nullable(r[ix["MPA_REMARKS"]]), LegacyAdditionLabel)
		return "", err
	})
}

// LoadAbnormalPremi fills pyr_abnormal_premi (MPAP_*).
func LoadAbnormalPremi(ctx context.Context, tx *sql.Tx, src *Table, company int16) (TableReport, error) {
	return loadTable(src, []string{"MPAP_YYMM", "MPAP_NIK", "MPAP_DATE", "MPAP_CODE", "MPAP_DIVISION", "MPAP_NAMA", "MPAP_DEPT"}, func(r []string, ix map[string]int, _ int) (string, error) {
		d, err := reqDate("MPAP_DATE", r[ix["MPAP_DATE"]])
		if err != nil {
			return "", err
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO payroll.pyr_abnormal_premi (pap_company_id, pap_yymm, pap_nik, pap_date, pap_code, pap_division, pap_nama, pap_dept)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT (pap_company_id, pap_yymm, pap_nik, pap_date, pap_code, pap_division) DO UPDATE SET pap_nama=EXCLUDED.pap_nama, pap_dept=EXCLUDED.pap_dept`,
			company, r[ix["MPAP_YYMM"]], r[ix["MPAP_NIK"]], d, r[ix["MPAP_CODE"]], r[ix["MPAP_DIVISION"]],
			nullable(r[ix["MPAP_NAMA"]]), nullable(r[ix["MPAP_DEPT"]]))
		return "", err
	})
}

// LoadBatalPremi fills pyr_batal_premi (MPBP_*).
func LoadBatalPremi(ctx context.Context, tx *sql.Tx, src *Table) (TableReport, error) {
	return loadTable(src, []string{"MPBP_CODE", "MPBP_BERAS", "MPBP_ABNORMAL"}, func(r []string, ix map[string]int, _ int) (string, error) {
		_, err := tx.ExecContext(ctx, `
INSERT INTO payroll.pyr_batal_premi (pbp_code, pbp_beras, pbp_abnormal) VALUES ($1,$2,$3)
ON CONFLICT (pbp_code) DO UPDATE SET pbp_beras=EXCLUDED.pbp_beras, pbp_abnormal=EXCLUDED.pbp_abnormal`,
			r[ix["MPBP_CODE"]], nullable(r[ix["MPBP_BERAS"]]), nullable(r[ix["MPBP_ABNORMAL"]]))
		return "", err
	})
}

// RunInputs loads the rule & input tables in ONE transaction and logs each table in pyr_migration_log.
func RunInputs(ctx context.Context, db *sql.DB, src Source, o InputOptions) ([]TableReport, error) {
	read := func(names ...string) (map[string]*Table, error) {
		out := map[string]*Table{}
		for _, n := range names {
			t, err := src.Read(n)
			if err != nil {
				return nil, err
			}
			out[n] = t
		}
		return out, nil
	}
	tb, err := read(TableStaticValue, TableAttdDays, TableSubsidiPPh, TableTaxAdj, TableHoliday, TableAddition, TableAbnormalPremi, TableBatalPremi)
	if err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			log.Warn().Err(rbErr).Msg("etl: rollback failed")
		}
	}()

	steps := []func() (TableReport, error){
		func() (TableReport, error) { return LoadRuleConstants(ctx, tx, o.CompanyID) }, // rule_set 1 dulu (FK)
		func() (TableReport, error) { return LoadStaticValues(ctx, tx, tb[TableStaticValue]) },
		func() (TableReport, error) { return LoadAttdDays(ctx, tx, tb[TableAttdDays], o.CompanyID) },
		func() (TableReport, error) { return LoadSubsidiPPh(ctx, tx, tb[TableSubsidiPPh], o.CompanyID) },
		func() (TableReport, error) { return LoadTaxAdj(ctx, tx, tb[TableTaxAdj], o.CompanyID) },
		func() (TableReport, error) { return LoadHoliday(ctx, tx, tb[TableHoliday]) },
		func() (TableReport, error) { return LoadAddition(ctx, tx, tb[TableAddition], o.CompanyID) },
		func() (TableReport, error) { return LoadAbnormalPremi(ctx, tx, tb[TableAbnormalPremi], o.CompanyID) },
		func() (TableReport, error) { return LoadBatalPremi(ctx, tx, tb[TableBatalPremi]) },
	}
	reps := make([]TableReport, 0, len(steps))
	for _, s := range steps {
		r, err := s()
		if err != nil {
			return reps, err
		}
		if err := logReport(ctx, tx, o.CompanyID, r); err != nil {
			return append(reps, r), err
		}
		reps = append(reps, r)
	}
	if err := checkExpected(reps, o); err != nil {
		return reps, err
	}
	return reps, tx.Commit()
}

// checkExpected rejects a source whose size differs from the production figure given in o (0 = unchecked).
func checkExpected(reps []TableReport, o InputOptions) error {
	want := map[string]int{TableAddition: o.ExpectAddition, TableAbnormalPremi: o.ExpectAbnormal}
	for _, r := range reps {
		if n := want[r.Table]; n > 0 && r.SrcRows != n {
			return fmt.Errorf("%s: %d baris, diharapkan %d", r.Table, r.SrcRows, n)
		}
	}
	return nil
}
