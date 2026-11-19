package etl

import (
	"context"
	"database/sql"
	"fmt"
	"maps"
	"slices"
	"strings"
)

// TableDepartment is the Orion department master whose DEPT_FIELD_01 holds the cost group (T028q, D-63).
const TableDepartment = "FM_DEPARTMENT"

// CostGroupOptions controls the pe_cost_group load. ExpectRows = 0 means unchecked; it counts the FM_DEPARTMENT rows
// that carry a DEPT_FIELD_01.
type CostGroupOptions struct {
	CompanyID  int16
	ExpectRows int
	// AllowExceptions commits even when rows are exceptions (T033h2); default: report them and roll back.
	AllowExceptions bool
}

var validCostGroups = map[string]bool{costGroupMGT: true, "MB": true, costGroupContract: true}

// RunCostGroup loads FM_DEPARTMENT.DEPT_FIELD_01 (MGT|MB|CONTRACT) into pyr_employee.pe_cost_group per employee dept
// (pe_dept_id = DEPT_CODE) in ONE transaction. DEPT_FIELD_01 values outside the three groups and dept codes that
// disagree between rows are Exceptions (never a silent pick); employees whose dept has no group are Warnings and keep
// pe_cost_group NULL (report gl then rejects them with GL_COSTGROUP_MISSING). Idempotent. Only active-or-not is not
// filtered: every employee of the company is updated, so a re-run also corrects a changed dept group.
func RunCostGroup(ctx context.Context, db *sql.DB, src Source, o CostGroupOptions) (TableReport, error) {
	t, err := src.Read(TableDepartment)
	if err != nil {
		return TableReport{}, err
	}
	rep := TableReport{Table: t.Name, SrcRows: len(t.Rows), Checksum: t.Checksum()}
	ix, err := bindCols(t, []string{"DEPT_CODE", "DEPT_FIELD_01"})
	if err != nil {
		return rep, err
	}
	groups, unset, selected := parseCostGroups(t, ix, &rep)
	if o.ExpectRows > 0 && selected != o.ExpectRows {
		return rep, fmt.Errorf("%s: %d baris ber-DEPT_FIELD_01, diharapkan %d", TableDepartment, selected, o.ExpectRows)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return rep, err
	}
	defer func() { _ = tx.Rollback() }() //nolint:errcheck // no-op after Commit
	n, err := setCostGroups(ctx, tx, o.CompanyID, groups)
	if err != nil {
		return rep, err
	}
	rep.DstRows += n
	n, err = unsetCostGroups(ctx, tx, o.CompanyID, unset)
	if err != nil {
		return rep, err
	}
	rep.DstRows += n
	if err := warnAppCostGroups(ctx, tx, o.CompanyID, unset, &rep); err != nil {
		return rep, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT coalesce(pe_dept_id,''), count(*) FROM payroll.pyr_employee
WHERE pe_company_id=$1 AND pe_cost_group IS NULL GROUP BY 1 ORDER BY 1`, o.CompanyID)
	if err != nil {
		return rep, err
	}
	defer closeQuiet(rows)
	for rows.Next() {
		var dept string
		var n int
		if err = rows.Scan(&dept, &n); err != nil {
			return rep, err
		}
		rep.Warnings = append(rep.Warnings, fmt.Sprintf("dept %q: %d karyawan tanpa cost group (dept tidak ada/kosong di FM_DEPARTMENT)", dept, n))
	}
	if err = rows.Err(); err != nil {
		return rep, err
	}
	if len(rep.Exceptions) > 0 && !o.AllowExceptions {
		return rep, fmt.Errorf("%s: %d exception, tidak di-commit (perbaiki sumber atau -allow-exceptions)", TableDepartment, len(rep.Exceptions))
	}
	return rep, tx.Commit()
}

// setCostGroups writes the group of every dept in groups (sorted, deterministic) and returns the employees changed.
func setCostGroups(ctx context.Context, tx *sql.Tx, company int16, groups map[string]string) (int, error) {
	total := 0
	for _, code := range slices.Sorted(maps.Keys(groups)) {
		res, err := tx.ExecContext(ctx, `UPDATE payroll.pyr_employee SET pe_cost_group=$3, pe_version=pe_version+1, updated_by='ETL', updated_at=now()
WHERE pe_company_id=$1 AND pe_dept_id=$2 AND pe_cost_group IS DISTINCT FROM $3`, company, code, groups[code])
		if err != nil {
			return 0, fmt.Errorf("dept %s: %w", code, scrub(err))
		}
		n, _ := res.RowsAffected() //nolint:errcheck // lib/pq-style drivers always support it
		total += int(n)
	}
	return total, nil
}

// unsetCostGroups clears the group of depts that are blank, invalid or conflicting in the source (T033h2): keeping the
// old group would post the GL to a cost group the source no longer states (report gl then flags GL_COSTGROUP_MISSING).
// Only rows ETL wrote last (updated_by='ETL') are cleared; a row HR edited in the app keeps its value (warning below).
func unsetCostGroups(ctx context.Context, tx *sql.Tx, company int16, depts []string) (int, error) {
	if len(depts) == 0 {
		return 0, nil
	}
	res, err := tx.ExecContext(ctx, `UPDATE payroll.pyr_employee SET pe_cost_group=NULL, pe_version=pe_version+1, updated_by='ETL', updated_at=now()
WHERE pe_company_id=$1 AND pe_dept_id = ANY($2::text[]) AND pe_cost_group IS NOT NULL AND updated_by='ETL'`, company, depts)
	if err != nil {
		return 0, fmt.Errorf("kosongkan cost group: %w", scrub(err))
	}
	n, _ := res.RowsAffected() //nolint:errcheck // lib/pq-style drivers always support it
	return int(n), nil
}

// warnAppCostGroups reports employees of depts the source no longer groups whose cost group was set in the app (kept).
func warnAppCostGroups(ctx context.Context, tx *sql.Tx, company int16, depts []string, rep *TableReport) error {
	if len(depts) == 0 {
		return nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT pe_dept_id, count(*) FROM payroll.pyr_employee
WHERE pe_company_id=$1 AND pe_dept_id = ANY($2::text[]) AND pe_cost_group IS NOT NULL GROUP BY 1 ORDER BY 1`, company, depts)
	if err != nil {
		return err
	}
	defer closeQuiet(rows)
	for rows.Next() {
		var dept string
		var n int
		if err := rows.Scan(&dept, &n); err != nil {
			return err
		}
		rep.Warnings = append(rep.Warnings, fmt.Sprintf("dept %q: %d karyawan tetap ber-cost group (diubah lewat aplikasi), sumber tidak lagi memberi group", dept, n))
	}
	return rows.Err()
}

// parseCostGroups maps DEPT_CODE → group from the rows that carry DEPT_FIELD_01 (CR/LF stripped, upper-cased), and
// lists the dept codes whose group must be cleared: blank in the source (and no row with a group), unknown or conflicting.
func parseCostGroups(t *Table, ix map[string]int, rep *TableReport) (groups map[string]string, unset []string, selected int) {
	groups = map[string]string{}
	bad, blank := map[string]bool{}, map[string]bool{}
	for i, r := range t.Rows {
		code := strings.TrimSpace(r[ix["DEPT_CODE"]])
		g := strings.ToUpper(strings.TrimSpace(strings.NewReplacer("\r", "", "\n", "").Replace(r[ix["DEPT_FIELD_01"]])))
		if g == "" {
			if code != "" {
				blank[code] = true
			}
			continue
		}
		selected++
		where := fmt.Sprintf("%s baris %d dept %q", t.Name, i+2, code)
		switch {
		case code == "":
			rep.Exceptions = append(rep.Exceptions, where+": DEPT_CODE kosong")
		case !validCostGroups[g]:
			bad[code] = true
			rep.Exceptions = append(rep.Exceptions, fmt.Sprintf("%s: DEPT_FIELD_01 %q bukan MGT/MB/CONTRACT", where, g))
		case groups[code] != "" && groups[code] != g:
			bad[code] = true
			rep.Exceptions = append(rep.Exceptions, fmt.Sprintf("%s: group %q berbeda dengan baris lain (%q)", where, g, groups[code]))
		default:
			groups[code] = g
		}
	}
	for c := range bad {
		delete(groups, c)
	}
	for c := range blank {
		if groups[c] == "" {
			bad[c] = true
		}
	}
	unset = slices.Sorted(maps.Keys(bad))
	return groups, unset, selected
}

// Cost-group and GL level names repeated across the ETL setup tables.
const (
	costGroupMGT      = "MGT"
	costGroupContract = "CONTRACT"
	levelExecutive    = "Executive"
	levelStaffHO      = "Staff HO"
	levelStaffBandung = "Staff Bandung"
)
