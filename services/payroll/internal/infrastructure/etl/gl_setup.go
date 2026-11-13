package etl

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// TableGlSetup is the legacy GL account setup (11§4.1, 1.338 rows, no PK).
const TableGlSetup = "MGT_PAYROLL_SETUP"

// glValidFrom is the start of history for migrated GL setup rows (11§4.5 step 6).
const glValidFrom = 201712

// Legacy MPS_JENIS values and derived line kinds.
const (
	glJenisBalance = "EMPLOYEE BALANCE"
	glKindComp     = "COMPONENT"
	glKindBalance  = "EMPLOYEE_BALANCE"
)

// GlSetupOptions controls the GL setup load. ExpectRows = 0 means unchecked (production: 1338).
type GlSetupOptions struct {
	CompanyID  int16
	ExpectRows int
}

type glKey struct {
	division, journal, group, cost string
	sysid                          int64
	kind                           string
}

type glRow struct {
	key             glKey
	main, sub, drcr string
	desc            string
	subFromEmp      bool
	legacy          map[string]string
	srcLine         int
}

var glCostGroups = map[string]string{"MGT": "MGT", "MB": "MB", "CONTRACT": "CONTRACT"}

// glJournalType maps MPS_TYPE (GAJI/THR/Bonus) to pgs_journal_type; "" = unknown.
func glJournalType(s string) string {
	switch strings.ToUpper(s) {
	case "GAJI":
		return "GAJI"
	case "THR":
		return "THR"
	case "BONUS":
		return "BONUS"
	}
	return ""
}

// GlAccount normalizes MPS_AKUN / MPS_MAIN / MPS_SUB (11§4.5 step 1): "main-sub" in MPS_AKUN is split,
// otherwise MPS_MAIN/MPS_SUB are used (MPS_AKUN alone counts as main). ok=false when no main account remains.
func GlAccount(akun, main, sub string) (m, s string, ok bool) {
	akun, main, sub = strings.TrimSpace(akun), strings.TrimSpace(main), strings.TrimSpace(sub)
	if i := strings.Index(akun, "-"); i >= 0 {
		m, s = strings.TrimSpace(akun[:i]), strings.TrimSpace(akun[i+1:])
		return m, s, m != ""
	}
	if main == "" {
		main = akun
	}
	return main, sub, main != ""
}

// parseGlRow validates one source row; the string is the exception text ("" = ok).
func parseGlRow(r []string, ix map[string]int, sysids map[sysKey]int64) (glRow, string) {
	c := func(n string) string { return r[ix[n]] }
	var g glRow
	g.key.division = strings.ToUpper(c("MPS_DIVISION"))
	if g.key.division != "SOLO" && g.key.division != "JAKARTA" {
		return g, "MPS_DIVISION tidak dikenal: " + fmt.Sprintf("%q", c("MPS_DIVISION"))
	}
	if g.key.journal = glJournalType(c("MPS_TYPE")); g.key.journal == "" {
		return g, fmt.Sprintf("MPS_TYPE %q bukan GAJI/THR/Bonus", c("MPS_TYPE"))
	}
	if g.key.group = c("MPS_JABATAN"); g.key.group == "" {
		return g, "MPS_JABATAN kosong"
	}
	if d := strings.ToUpper(c("MPS_DEPT")); d != "" {
		cg, ok := glCostGroups[d]
		if !ok {
			return g, fmt.Sprintf("MPS_DEPT %q bukan MGT/MB/Contract", c("MPS_DEPT"))
		}
		g.key.cost = cg
	}
	if g.drcr = strings.ToUpper(c("MPS_DRCR")); g.drcr != "D" && g.drcr != "C" {
		return g, fmt.Sprintf("MPS_DRCR %q bukan D/C", c("MPS_DRCR"))
	}
	var ok bool
	if g.main, g.sub, ok = GlAccount(c("MPS_AKUN"), c("MPS_MAIN"), c("MPS_SUB")); !ok {
		return g, "akun tidak terurai (MPS_AKUN/MPS_MAIN kosong)"
	}
	g.desc = c("MPS_DESCRIPTION")
	jenis := strings.ToUpper(c("MPS_JENIS"))
	if jenis == glJenisBalance {
		g.key.kind, g.subFromEmp = glKindBalance, true
		return g, ""
	}
	g.key.kind = glKindComp
	typ := map[string]string{"TUNJANGAN": typeTunjangan, "POTONGAN": typePotongan}[jenis]
	if typ == "" {
		return g, fmt.Sprintf("MPS_JENIS %q bukan Tunjangan/Potongan/Employee Balance", c("MPS_JENIS"))
	}
	var no int
	if _, err := fmt.Sscanf(c("MPS_NO"), "%d", &no); err != nil {
		return g, fmt.Sprintf("MPS_NO %q bukan angka", c("MPS_NO"))
	}
	sys, found := sysids[sysKey{typ, no}]
	if !found {
		return g, fmt.Sprintf("komponen %s %d (%s) tanpa peta sysid di pyr_component_legacy_map", jenis, no, g.key.division)
	}
	g.key.sysid = sys
	return g, ""
}

// glGroupMap is the legacy Level_Status CASE (SALARY001) as level → gl_group rows per division. Driver in dept HRMC
// becomes "Driver Contract" in the legacy CASE; that depends on the department, so it is resolved by the report, not here.
func glGroupMap() map[string]map[string]string {
	solo := map[string]string{"Executive": "Executive"}
	jkt := map[string]string{
		"Staff HO": "Staff HO", "Office Boy": "Staff HO", "Driver HO": "Driver HO",
		"Staff Bandung": "Staff Bandung", "OB Bandung": "Staff Bandung", "Driver Bandung": "Staff Bandung",
		"Solo Asmen": "Solo Asmen",
	}
	for _, r := range LevelRules() {
		if _, isJkt := jkt[r.Level]; !isJkt && r.Level != "Executive" {
			solo[r.Level] = "Operator"
		}
	}
	return map[string]map[string]string{"SOLO": solo, "JAKARTA": jkt}
}

// RunGlSetup loads MGT_PAYROLL_SETUP into pyr_gl_setup (+ seeds pyr_gl_group_map) in ONE transaction (11§4.5).
// Rows that cannot be loaded (unknown division/type/dept/drcr, unparsable account, component without sysid map) and
// duplicate keys with differing account/drcr (the legacy table has no PK) are reported as Exceptions and not loaded;
// identical duplicates collapse into one row (Warnings). Idempotent: upsert on the unique index, pgs_legacy_row = source row.
//
//nolint:gocognit,gocyclo // linear read -> classify -> upsert pipeline, same shape as RunThrLegacy
func RunGlSetup(ctx context.Context, db *sql.DB, src Source, o GlSetupOptions) (TableReport, error) {
	t, err := src.Read(TableGlSetup)
	if err != nil {
		return TableReport{}, err
	}
	rep := TableReport{Table: t.Name, SrcRows: len(t.Rows), Checksum: t.Checksum()}
	if o.ExpectRows > 0 && rep.SrcRows != o.ExpectRows {
		return rep, fmt.Errorf("%s: %d baris, diharapkan %d", TableGlSetup, rep.SrcRows, o.ExpectRows)
	}
	ix, err := bindCols(t, []string{"MPS_JABATAN", "MPS_NO", "MPS_JENIS", "MPS_DEPT", "MPS_AKUN", "MPS_MAIN", "MPS_SUB", "MPS_DRCR", "MPS_DESCRIPTION", "MPS_TYPE", "MPS_DIVISION"})
	if err != nil {
		return rep, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return rep, err
	}
	defer func() { _ = tx.Rollback() }() //nolint:errcheck // no-op after Commit
	sysids := map[string]map[sysKey]int64{}
	groups := make([]glRow, 0, len(t.Rows))
	byKey := map[glKey][]glRow{}
	for i, r := range t.Rows {
		div := strings.ToUpper(r[ix["MPS_DIVISION"]])
		if _, ok := sysids[div]; !ok && (div == "SOLO" || div == "JAKARTA") {
			if sysids[div], err = glSysids(ctx, tx, o.CompanyID, div); err != nil {
				return rep, err
			}
		}
		g, exc := parseGlRow(r, ix, sysids[div])
		if exc != "" {
			rep.Exceptions = append(rep.Exceptions, fmt.Sprintf("%s baris %d: %s", t.Name, i+2, exc))
			continue
		}
		g.srcLine = i + 2
		g.legacy = map[string]string{}
		for j, c := range t.Cols {
			g.legacy[c] = r[j]
		}
		byKey[g.key] = append(byKey[g.key], g)
		groups = append(groups, g)
	}
	loaded := 0
	seen := map[glKey]bool{}
	for _, g := range groups {
		if seen[g.key] {
			continue
		}
		seen[g.key] = true
		dups := byKey[g.key]
		if conflict := glConflict(dups); conflict != "" {
			for _, d := range dups {
				rep.Exceptions = append(rep.Exceptions, fmt.Sprintf("%s baris %d: duplikat kunci %s dengan akun/drcr berbeda (%s)", t.Name, d.srcLine, glKeyText(d.key), conflict))
			}
			continue
		}
		if len(dups) > 1 {
			rep.Warnings = append(rep.Warnings, fmt.Sprintf("%d baris identik digabung: %s", len(dups), glKeyText(g.key)))
		}
		if err := upsertGl(ctx, tx, o.CompanyID, g); err != nil {
			return rep, fmt.Errorf("baris %d: %w", g.srcLine, err)
		}
		loaded += len(dups)
	}
	rep.DstRows = loaded
	if err := loadGlGroupMap(ctx, tx, o.CompanyID, &rep); err != nil {
		return rep, err
	}
	if err := tx.Commit(); err != nil {
		return rep, err
	}
	return rep, nil
}

func glKeyText(k glKey) string {
	return fmt.Sprintf("%s/%s/%s/cost=%q/sysid=%d/%s", k.division, k.journal, k.group, k.cost, k.sysid, k.kind)
}

// glConflict reports the first account/drcr difference among rows sharing one key.
func glConflict(rows []glRow) string {
	for _, r := range rows[1:] {
		if r.main != rows[0].main || r.sub != rows[0].sub || r.drcr != rows[0].drcr {
			return fmt.Sprintf("%s-%s %s vs %s-%s %s", rows[0].main, rows[0].sub, rows[0].drcr, r.main, r.sub, r.drcr)
		}
	}
	return ""
}

func glSysids(ctx context.Context, tx *sql.Tx, company int16, division string) (map[sysKey]int64, error) {
	rows, err := tx.QueryContext(ctx, `SELECT lower(pclm_legacy_type::text), pclm_legacy_id, pclm_sysid FROM payroll.pyr_component_legacy_map
WHERE pclm_company_id=$1 AND pclm_legacy_division=$2`, company, division)
	if err != nil {
		return nil, err
	}
	defer closeQuiet(rows)
	m := map[sysKey]int64{}
	for rows.Next() {
		var k sysKey
		var s int64
		if err := rows.Scan(&k.typ, &k.id, &s); err != nil {
			return nil, err
		}
		m[k] = s
	}
	return m, rows.Err()
}

func upsertGl(ctx context.Context, tx *sql.Tx, company int16, g glRow) error {
	legacy, err := json.Marshal(g.legacy)
	if err != nil {
		return err
	}
	var cost, sysid, sub any
	if g.key.cost != "" {
		cost = g.key.cost
	}
	if g.key.sysid != 0 {
		sysid = g.key.sysid
	}
	if g.sub != "" {
		sub = g.sub
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO payroll.pyr_gl_setup (pgs_company_id, pgs_division, pgs_journal_type, pgs_gl_group, pgs_cost_group, pgs_component_sysid,
  pgs_line_kind, pgs_account_main, pgs_account_sub, pgs_sub_from_employee, pgs_drcr, pgs_description, pgs_valid_from, pgs_legacy_row)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14::jsonb)
ON CONFLICT (pgs_company_id, pgs_division, pgs_journal_type, pgs_gl_group, coalesce(pgs_cost_group,'*'), coalesce(pgs_component_sysid,0), pgs_line_kind, pgs_valid_from)
DO UPDATE SET pgs_account_main=EXCLUDED.pgs_account_main, pgs_account_sub=EXCLUDED.pgs_account_sub,
  pgs_sub_from_employee=EXCLUDED.pgs_sub_from_employee, pgs_drcr=EXCLUDED.pgs_drcr,
  pgs_description=EXCLUDED.pgs_description, pgs_legacy_row=EXCLUDED.pgs_legacy_row`,
		company, g.key.division, g.key.journal, g.key.group, cost, sysid, g.key.kind, g.main, sub, g.subFromEmp, g.drcr, g.desc, glValidFrom, string(legacy))
	return err
}

// loadGlGroupMap seeds pyr_gl_group_map and warns when a mapped gl_group has no setup row for its division.
func loadGlGroupMap(ctx context.Context, tx *sql.Tx, company int16, rep *TableReport) error {
	for div, m := range glGroupMap() {
		for level, grp := range m {
			if _, err := tx.ExecContext(ctx, `
INSERT INTO payroll.pyr_gl_group_map (pggm_company_id, pggm_division, pggm_level, pggm_gl_group) VALUES ($1,$2,$3,$4)
ON CONFLICT (pggm_company_id, pggm_division, pggm_level) DO UPDATE SET pggm_gl_group=EXCLUDED.pggm_gl_group`,
				company, div, level, grp); err != nil {
				return fmt.Errorf("gl_group_map %s/%s: %w", div, level, err)
			}
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT m.pggm_division, m.pggm_gl_group FROM payroll.pyr_gl_group_map m
WHERE m.pggm_company_id=$1 AND NOT EXISTS (SELECT 1 FROM payroll.pyr_gl_setup s WHERE s.pgs_company_id=m.pggm_company_id
  AND s.pgs_division=m.pggm_division AND s.pgs_gl_group=m.pggm_gl_group) ORDER BY 1,2`, company)
	if err != nil {
		return err
	}
	defer closeQuiet(rows)
	for rows.Next() {
		var d, g string
		if err := rows.Scan(&d, &g); err != nil {
			return err
		}
		rep.Warnings = append(rep.Warnings, fmt.Sprintf("gl_group %q (%s) ada di group_map tetapi tanpa baris setup GL", g, d))
	}
	return rows.Err()
}
