package run

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/engine"
)

// T016c: the tax inputs of a run, read from the DETAIL of ACTIVE runs (ENGINE and LEGACY alike).
// Input.Thr stays empty until B-THR: neither the THR/Bonus of earlier months (YTD.Gross/TaxPaid,
// YTDCompany.ThrBonus) nor the THR of the run month is loaded yet.

// ErrYTDNPWPConflict rejects a run whose YTD chain mixes two NPWPs (13§3.3).
var ErrYTDNPWPConflict = errors.New("run: YTD_NPWP_CONFLICT: pe_prev_employee_id chain has a different NPWP")

// ErrYTDOutOfScope rejects a v2 run whose YTD would miss placements in divisions outside the scope.
var ErrYTDOutOfScope = errors.New("run: YTD_PLACEMENT_OUT_OF_SCOPE: YTD placement in a division outside app.scopes")

const (
	// CodeThrNotLoaded warns that a December run has no THR/Bonus in YTD/YTDCompany yet (B-THR).
	CodeThrNotLoaded = "THR_NOT_LOADED"
	// CodeYTDOutOfScope flags a placement of the tax subject in a division outside app.scopes whose
	// YTD is not summed (v1 warning, v2 error; T016e).
	CodeYTDOutOfScope = "YTD_PLACEMENT_OUT_OF_SCOPE"
	// CodeYTDNoNPWP warns that the YTD of an employee without NPWP follows pe_prev_employee_id (13§3.3).
	CodeYTDNoNPWP = "YTD_NO_NPWP" //nolint:gosec // issue code, not a credential

	ytdChainDepth = 20 // guard against a cyclic pe_prev_employee_id chain
)

// taxInput is the per-employee tax part of engine.Input.
type taxInput struct {
	ytd        engine.YTDTax
	ytdCompany engine.YTDCompanyTax
	taxAdj     engine.TaxAdj
	basis      *decimal.Decimal
	prevBasis  *decimal.Decimal
	locked     *decimal.Decimal
	lockedYYMM int
}

// loadTax reads the YTD and TaxAdj (December only) and the P0 basis of the rule version.
func loadTax(ctx context.Context, tx *sql.Tx, q Query, legacy bool, basisPeriod int) (map[string]*taxInput, []Issue, error) {
	out := map[string]*taxInput{}
	get := func(nik string) *taxInput {
		if out[nik] == nil {
			out[nik] = &taxInput{}
		}
		return out[nik]
	}
	var issues []Issue
	if q.YYMM%100 == 12 {
		var err error
		if issues, err = loadYTD(ctx, tx, q, get); err != nil {
			return nil, nil, err
		}
		scopeIssues, err := ytdOutOfScope(ctx, tx, q, legacy)
		if err != nil {
			return nil, nil, err
		}
		issues = append(issues, scopeIssues...)
		issues = append(issues, Issue{Code: CodeThrNotLoaded,
			Message: "THR/Bonus Jan–Des belum masuk YTD PPh tahunan dan P5 (menunggu B-THR)"})
		if err := loadTaxAdj(ctx, tx, q, get); err != nil {
			return nil, nil, err
		}
	}
	var err error
	switch {
	case !legacy:
		err = loadSubsidyLock(ctx, tx, q, get)
	case basisPeriod > 0:
		err = loadSubsidyBasisV1(ctx, tx, q, basisPeriod, get)
	}
	if err != nil {
		return nil, nil, err
	}
	return out, issues, nil
}

// apply copies the tax part into in.
func (t *taxInput) apply(in *engine.Input) {
	if t == nil {
		return
	}
	in.YTD, in.YTDCompany, in.TaxAdj = t.ytd, t.ytdCompany, t.taxAdj
	in.Subsidy.Basis, in.Subsidy.PrevBasis = t.basis, t.prevBasis
	in.Subsidy.Locked, in.Subsidy.LockedYYMM = t.locked, t.lockedYYMM
}

// ytdSQL aggregates January up to the previous period per target NIK over every placement of the
// same tax subject in the company (13§3.3, D-N4): the pe_prev_employee_id chain plus every placement
// sharing a pe_npwp_hash of that chain. Two NPWPs in one chain is a conflict. Visible placements only: RLS limits them to the run scope, so a
// placement in a division outside the scope is not summed (legacy sums per NIK + division too).
// Compositions (D-29 P1, D-31 P5):
//
//	YTD.Gross    = Σ tax=2 tax_paid_company=N id<>75 − Σ P4, P8   (+ THR/Bonus: B-THR)
//	YTD.Deduct   = Σ P14, P18, P22, P23
//	YTD.TaxPaid  = Σ P1 + Σ addition P7                          (+ PPh THR/Bonus: B-THR)
//	YTDCompany   = Gross Σ tax=2 · CompanyPaid Σ tax=2 tax_paid_company=Y · Deduct Σ P4, P14, P18 · P5Paid Σ P5
const ytdSQL = `WITH RECURSIVE tgt AS (
  SELECT pe_nik nik, pe_id id, pe_npwp_hash h FROM payroll.pyr_employee
  WHERE pe_company_id = $1 AND pe_division = $2 AND pe_sts_payroll = 2 AND ($5::text[] = '{}' OR pe_nik = ANY($5::text[]))
), chain AS (
  SELECT t.nik, t.id, e.pe_prev_employee_id prev, 1 depth
  FROM tgt t JOIN payroll.pyr_employee e ON e.pe_id = t.id
  UNION ALL
  SELECT c.nik, e.pe_id, e.pe_prev_employee_id, c.depth + 1
  FROM chain c JOIN payroll.pyr_employee e ON e.pe_id = c.prev AND e.pe_company_id = $1
  WHERE c.depth < $6
), hashes AS (
  SELECT DISTINCT c.nik, e.pe_npwp_hash h FROM chain c JOIN payroll.pyr_employee e ON e.pe_id = c.id
  WHERE e.pe_npwp_hash IS NOT NULL
), mem AS (
  SELECT c.nik, e.pe_nik m_nik, e.pe_division m_div FROM chain c JOIN payroll.pyr_employee e ON e.pe_id = c.id
  UNION
  SELECT x.nik, e.pe_nik, e.pe_division
  FROM hashes x JOIN payroll.pyr_employee e ON e.pe_company_id = $1 AND e.pe_npwp_hash = x.h
), det AS (
  SELECT m.nik, d.psd_type::text typ, d.psd_id id, COALESCE(d.psd_tax, 0) = 2 tax,
         COALESCE(d.psd_tax_paid_company, 'N') = 'Y' tpc, d.psd_amount amt
  FROM mem m
  JOIN payroll.pyr_run r ON r.pr_company_id = $1 AND r.pr_division = m.m_div AND r.pr_status = 'ACTIVE'
   AND r.pr_parent_run_id IS NULL AND r.pr_yymm BETWEEN $3 AND $4
  JOIN payroll.pyr_salary_detail d ON d.psd_run_id = r.pr_id AND d.psd_nik = m.m_nik
), agg AS (
  SELECT nik,
    COALESCE(SUM(amt) FILTER (WHERE tax AND NOT tpc AND id <> 75), 0)
      - COALESCE(SUM(amt) FILTER (WHERE typ = 'Potongan' AND id IN (4, 8)), 0) gross,
    COALESCE(SUM(amt) FILTER (WHERE typ = 'Potongan' AND id IN (14, 18, 22, 23)), 0) deduct,
    COALESCE(SUM(amt) FILTER (WHERE typ = 'Potongan' AND id = 1), 0) p1,
    COALESCE(SUM(amt) FILTER (WHERE tax), 0) c_gross,
    COALESCE(SUM(amt) FILTER (WHERE tax AND tpc), 0) c_paid,
    COALESCE(SUM(amt) FILTER (WHERE typ = 'Potongan' AND id IN (4, 14, 18)), 0) c_deduct,
    COALESCE(SUM(amt) FILTER (WHERE typ = 'Potongan' AND id = 5), 0) p5
  FROM det GROUP BY nik
), p7 AS (
  SELECT m.nik, SUM(a.pa_amount) amt
  FROM mem m JOIN payroll.pyr_addition a ON a.pa_company_id = $1 AND a.pa_division = m.m_div AND a.pa_nik = m.m_nik
   AND a.pa_type = 'Potongan' AND a.pa_id = 7 AND a.pa_yymm BETWEEN $3 AND $4
  GROUP BY m.nik
)
SELECT t.nik, t.h IS NULL, (SELECT count(*) FROM hashes x WHERE x.nik = t.nik) > 1,
  COALESCE(a.gross, 0)::text, COALESCE(a.deduct, 0)::text, (COALESCE(a.p1, 0) + COALESCE(p7.amt, 0))::text,
  COALESCE(a.c_gross, 0)::text, COALESCE(a.c_paid, 0)::text, COALESCE(a.c_deduct, 0)::text, COALESCE(a.p5, 0)::text
FROM tgt t LEFT JOIN agg a ON a.nik = t.nik LEFT JOIN p7 ON p7.nik = t.nik
ORDER BY t.nik`

// loadYTD fills YTD and YTDCompany (1 query). A NPWP conflict fails the load (13§3.3).
func loadYTD(ctx context.Context, tx *sql.Tx, q Query, get func(string) *taxInput) ([]Issue, error) {
	year := q.YYMM / 100 * 100
	rows, err := tx.QueryContext(ctx, ytdSQL, q.CompanyID, q.Division, year+1, q.YYMM-1, nikArray(q.NIKs), ytdChainDepth)
	if err != nil {
		return nil, fmt.Errorf("load YTD: %w", err)
	}
	defer closeRows(rows)
	var issues []Issue
	var conflicts []string
	for rows.Next() {
		var nik string
		var noNPWP, conflict bool
		var v [7]string
		if err := rows.Scan(&nik, &noNPWP, &conflict, &v[0], &v[1], &v[2], &v[3], &v[4], &v[5], &v[6]); err != nil {
			return nil, fmt.Errorf("scan YTD: %w", err)
		}
		d, err := decimals(v[:])
		if err != nil {
			return nil, fmt.Errorf("YTD %s: %w", nik, err)
		}
		t := get(nik)
		t.ytd = engine.YTDTax{Gross: d[0], Deduct: d[1], TaxPaid: d[2]}
		t.ytdCompany = engine.YTDCompanyTax{Gross: d[3], CompanyPaid: d[4], Deduct: d[5], P5Paid: d[6]}
		if conflict {
			conflicts = append(conflicts, nik)
		}
		if noNPWP {
			issues = append(issues, Issue{NIK: nik, Code: CodeYTDNoNPWP,
				Message: "NPWP kosong; YTD digabung lewat rantai pe_prev_employee_id, lengkapi NPWP"})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(conflicts) > 0 {
		return nil, fmt.Errorf("%w: %s", ErrYTDNPWPConflict, strings.Join(conflicts, ", "))
	}
	return issues, nil
}

// ytdOutOfScope detects placements outside app.scopes through the SECURITY DEFINER function
// fn_ytd_out_of_scope (counts only, no amounts). v1 warns per NIK, v2 fails the load (T016e).
func ytdOutOfScope(ctx context.Context, tx *sql.Tx, q Query, legacy bool) ([]Issue, error) {
	year := q.YYMM / 100 * 100
	rows, err := tx.QueryContext(ctx,
		`SELECT out_nik, out_placements FROM payroll.fn_ytd_out_of_scope($1, $2, $3, $4, $5::text[], $6) ORDER BY 1`,
		q.CompanyID, q.Division, year+1, q.YYMM-1, nikArray(q.NIKs), ytdChainDepth)
	if err != nil {
		return nil, fmt.Errorf("load YTD out of scope: %w", err)
	}
	defer closeRows(rows)
	var issues []Issue
	var niks []string
	for rows.Next() {
		var nik string
		var n int
		if err := rows.Scan(&nik, &n); err != nil {
			return nil, fmt.Errorf("scan YTD out of scope: %w", err)
		}
		niks = append(niks, nik)
		issues = append(issues, Issue{NIK: nik, Code: CodeYTDOutOfScope,
			Message: fmt.Sprintf("%d penempatan di divisi di luar scope tidak masuk YTD; tambahkan scope divisi itu", n)})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if !legacy && len(niks) > 0 {
		return nil, fmt.Errorf("%w: %s", ErrYTDOutOfScope, strings.Join(niks, ", "))
	}
	return issues, nil
}

// loadTaxAdj reads pyr_tax_adj of the period (1 query): Tunjangan → Earn, Potongan → Deduct.
func loadTaxAdj(ctx context.Context, tx *sql.Tx, q Query, get func(string) *taxInput) error {
	rows, err := tx.QueryContext(ctx, `SELECT pta_nik,
  COALESCE(SUM(pta_amount) FILTER (WHERE pta_type = 'Tunjangan'), 0)::text,
  COALESCE(SUM(pta_amount) FILTER (WHERE pta_type = 'Potongan'), 0)::text
FROM payroll.pyr_tax_adj
WHERE pta_company_id = $1 AND pta_division = $2 AND pta_yymm = $3 AND ($4::text[] = '{}' OR pta_nik = ANY($4::text[]))
GROUP BY pta_nik`, q.CompanyID, q.Division, q.YYMM, nikArray(q.NIKs))
	if err != nil {
		return fmt.Errorf("load tax adj: %w", err)
	}
	defer closeRows(rows)
	for rows.Next() {
		var nik string
		var v [2]string
		if err := rows.Scan(&nik, &v[0], &v[1]); err != nil {
			return fmt.Errorf("scan tax adj: %w", err)
		}
		d, err := decimals(v[:])
		if err != nil {
			return fmt.Errorf("tax adj %s: %w", nik, err)
		}
		get(nik).taxAdj = engine.TaxAdj{Earn: d[0], Deduct: d[1]}
	}
	return rows.Err()
}

// loadSubsidyBasisV1 reads the legacy P0 basis (D-30, 1 query): Σ DETAIL Tunjangan with
// tax_subsidy = Y of the basis period (Basis) and of the latest period after it and before the run
// that has any DETAIL of the NIK (PrevBasis). The run period itself is never read: a re-run of the
// basis period falls back to its current lines. No row = nil; the engine treats nil and 0 alike.
func loadSubsidyBasisV1(ctx context.Context, tx *sql.Tx, q Query, basisPeriod int, get func(string) *taxInput) error {
	rows, err := tx.QueryContext(ctx, `WITH det AS (
  SELECT d.psd_nik nik, r.pr_yymm yymm, d.psd_amount amt,
    d.psd_type = 'Tunjangan' AND COALESCE(c.pc_subsidy_eligible, cc.pc_subsidy_eligible, false) sub
  FROM payroll.pyr_run r
  JOIN payroll.pyr_salary_detail d ON d.psd_run_id = r.pr_id
  LEFT JOIN payroll.pyr_component c ON c.pc_sysid = d.psd_component_sysid
  LEFT JOIN payroll.pyr_component cc ON d.psd_component_sysid IS NULL AND cc.pc_company_id = $1
   AND cc.pc_code = CASE d.psd_type WHEN 'Tunjangan' THEN 'T' ELSE 'P' END || lpad(d.psd_id::text, 3, '0')
  WHERE r.pr_company_id = $1 AND r.pr_division = $2 AND r.pr_status = 'ACTIVE' AND r.pr_parent_run_id IS NULL
    AND r.pr_yymm >= $3 AND r.pr_yymm < $4 AND ($5::text[] = '{}' OR d.psd_nik = ANY($5::text[]))
), prev AS (
  SELECT nik, MAX(yymm) yymm FROM det WHERE yymm > $3 GROUP BY nik
)
SELECT d.nik, (SUM(d.amt) FILTER (WHERE d.yymm = $3 AND d.sub))::text, (SUM(d.amt) FILTER (WHERE d.yymm = p.yymm AND d.sub))::text
FROM det d LEFT JOIN prev p ON p.nik = d.nik
GROUP BY d.nik`, q.CompanyID, q.Division, basisPeriod, q.YYMM, nikArray(q.NIKs))
	if err != nil {
		return fmt.Errorf("load subsidy basis: %w", err)
	}
	defer closeRows(rows)
	for rows.Next() {
		var nik string
		var basis, prev sql.NullString
		if err := rows.Scan(&nik, &basis, &prev); err != nil {
			return fmt.Errorf("scan subsidy basis: %w", err)
		}
		t := get(nik)
		if t.basis, err = decimalPtr(basis); err != nil {
			return fmt.Errorf("subsidy basis %s: %w", nik, err)
		}
		if t.prevBasis, err = decimalPtr(prev); err != nil {
			return fmt.Errorf("subsidy prev basis %s: %w", nik, err)
		}
	}
	return rows.Err()
}

// loadSubsidyLock reads the rule v2 lock (D-30, 1 query): the earliest P0 line of the tax year,
// before the run period, whose basis source is LOCK_NOW (the runner keeps it in psd_meta, T017).
// LEGACY runs carry no meta, so a mid-year cut-over has no lock yet (D-38).
func loadSubsidyLock(ctx context.Context, tx *sql.Tx, q Query, get func(string) *taxInput) error {
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT ON (d.psd_nik) d.psd_nik, r.pr_yymm, d.psd_meta->>$6
FROM payroll.pyr_run r JOIN payroll.pyr_salary_detail d ON d.psd_run_id = r.pr_id
WHERE r.pr_company_id = $1 AND r.pr_division = $2 AND r.pr_status = 'ACTIVE' AND r.pr_parent_run_id IS NULL
  AND r.pr_yymm BETWEEN $3 AND $4 AND d.psd_type = 'Potongan' AND d.psd_id = 0
  AND d.psd_meta->>$7 = $8 AND ($5::text[] = '{}' OR d.psd_nik = ANY($5::text[]))
ORDER BY d.psd_nik, r.pr_yymm`, q.CompanyID, q.Division, q.YYMM/100*100+1, q.YYMM-1, nikArray(q.NIKs),
		engine.MetaSubsidyBasis, engine.MetaSubsidySource, engine.SubsidySrcLockNow)
	if err != nil {
		return fmt.Errorf("load subsidy lock: %w", err)
	}
	defer closeRows(rows)
	for rows.Next() {
		var nik string
		var yymm int
		var basis sql.NullString
		if err := rows.Scan(&nik, &yymm, &basis); err != nil {
			return fmt.Errorf("scan subsidy lock: %w", err)
		}
		v, err := decimalPtr(basis)
		if err != nil {
			return fmt.Errorf("subsidy lock %s: %w", nik, err)
		}
		if v != nil {
			t := get(nik)
			t.locked, t.lockedYYMM = v, yymm
		}
	}
	return rows.Err()
}

func decimals(ss []string) ([]decimal.Decimal, error) {
	out := make([]decimal.Decimal, len(ss))
	for i, s := range ss {
		d, err := decimal.NewFromString(s)
		if err != nil {
			return nil, err
		}
		out[i] = d
	}
	return out, nil
}

func decimalPtr(s sql.NullString) (*decimal.Decimal, error) {
	if !s.Valid {
		return nil, nil //nolint:nilnil // NULL sum = no DETAIL row
	}
	d, err := decimal.NewFromString(s.String)
	if err != nil {
		return nil, err
	}
	return &d, nil
}
