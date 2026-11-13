package etl

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

// TableDepartment is the Orion department master whose DEPT_FIELD_01 holds the cost group (T028q, D-63).
const TableDepartment = "FM_DEPARTMENT"

// CostGroupOptions controls the pe_cost_group load. ExpectRows = 0 means unchecked; it counts the FM_DEPARTMENT rows
// that carry a DEPT_FIELD_01.
type CostGroupOptions struct {
	CompanyID  int16
	ExpectRows int
}

var validCostGroups = map[string]bool{"MGT": true, "MB": true, "CONTRACT": true}

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
	groups, selected := parseCostGroups(t, ix, &rep)
	if o.ExpectRows > 0 && selected != o.ExpectRows {
		return rep, fmt.Errorf("%s: %d baris ber-DEPT_FIELD_01, diharapkan %d", TableDepartment, selected, o.ExpectRows)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return rep, err
	}
	defer func() { _ = tx.Rollback() }() //nolint:errcheck // no-op after Commit
	codes := make([]string, 0, len(groups))
	for c := range groups {
		codes = append(codes, c)
	}
	sort.Strings(codes)
	for _, code := range codes {
		res, uerr := tx.ExecContext(ctx, `UPDATE payroll.pyr_employee SET pe_cost_group=$3, updated_by='ETL', updated_at=now()
WHERE pe_company_id=$1 AND pe_dept_id=$2 AND pe_cost_group IS DISTINCT FROM $3`, o.CompanyID, code, groups[code])
		if uerr != nil {
			return rep, fmt.Errorf("dept %s: %w", code, scrub(uerr))
		}
		n, _ := res.RowsAffected() //nolint:errcheck // lib/pq-style drivers always support it
		rep.DstRows += int(n)
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
	return rep, tx.Commit()
}

// parseCostGroups maps DEPT_CODE → group from the rows that carry DEPT_FIELD_01 (CR/LF stripped, upper-cased).
func parseCostGroups(t *Table, ix map[string]int, rep *TableReport) (map[string]string, int) {
	groups := map[string]string{}
	bad := map[string]bool{}
	selected := 0
	for i, r := range t.Rows {
		code := strings.TrimSpace(r[ix["DEPT_CODE"]])
		g := strings.ToUpper(strings.TrimSpace(strings.NewReplacer("\r", "", "\n", "").Replace(r[ix["DEPT_FIELD_01"]])))
		if g == "" {
			continue
		}
		selected++
		where := fmt.Sprintf("%s baris %d dept %q", t.Name, i+2, code)
		switch {
		case code == "":
			rep.Exceptions = append(rep.Exceptions, where+": DEPT_CODE kosong")
		case !validCostGroups[g]:
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
	return groups, selected
}
