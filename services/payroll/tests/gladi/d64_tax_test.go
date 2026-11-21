package gladi

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

// Independent recomputation of the PPh invariants (06§6.1–6.2 with the v1 composition of D-29/D-56). It reads the DB with its
// own SQL per NIK (no loader code); placements merged through pe_prev_employee_id / shared NPWP are reported apart (chain).

// decSQL: one row per NIK of the division with the 202612 run, YTD Jan–Nov per NIK and the current-month parts.
const decSQL = `WITH cur AS (
  SELECT r.pr_id FROM payroll.pyr_run r WHERE r.pr_company_id=$1 AND r.pr_division=$2 AND r.pr_yymm=$3 AND r.pr_status='ACTIVE' AND r.pr_parent_run_id IS NULL
), ytd_runs AS (
  SELECT r.pr_id FROM payroll.pyr_run r WHERE r.pr_company_id=$1 AND r.pr_division=$2 AND r.pr_yymm BETWEEN $3/100*100+1 AND $3-1
   AND r.pr_status='ACTIVE' AND r.pr_parent_run_id IS NULL
), y AS (
  SELECT d.psd_nik nik,
    COALESCE(SUM(d.psd_amount) FILTER (WHERE COALESCE(d.psd_tax,0)=2 AND COALESCE(d.psd_tax_paid_company,'N')<>'Y' AND d.psd_id<>75), 0)
      - COALESCE(SUM(d.psd_amount) FILTER (WHERE d.psd_type='Potongan' AND d.psd_id IN (4,8)), 0) gross,
    COALESCE(SUM(d.psd_amount) FILTER (WHERE d.psd_type='Potongan' AND d.psd_id IN (14,18,22,23)), 0) deduct,
    COALESCE(SUM(d.psd_amount) FILTER (WHERE d.psd_type='Potongan' AND d.psd_id=1), 0) p1
  FROM payroll.pyr_salary_detail d WHERE d.psd_run_id IN (SELECT pr_id FROM ytd_runs) GROUP BY 1
), p7 AS (
  SELECT pa_nik nik, SUM(pa_amount) amt FROM payroll.pyr_addition WHERE pa_company_id=$1 AND pa_division=$2 AND pa_type='Potongan' AND pa_id=7
   AND pa_yymm BETWEEN $3/100*100+1 AND $3-1 GROUP BY 1
), thr AS (
  SELECT td_nik nik,
    COALESCE(SUM(td_amount) FILTER (WHERE td_jenis='Tunjangan' AND td_amount<>0), 0) gross,
    COALESCE(SUM(td_amount) FILTER (WHERE td_jenis='Potongan' AND (td_no=1 OR (td_type='Bonus' AND td_no=3))), 0) paid
  FROM payroll.v_pyr_thr_detail WHERE td_division=$2 AND is_closed AND td_type IN ('Thr','Bonus') AND td_yymm BETWEEN $3/100*100+1 AND $3-1 GROUP BY 1
), c AS (
  SELECT d.psd_nik nik,
    bool_or(COALESCE(d.psd_tax,0)=2 AND NOT (d.psd_type='Potongan' AND d.psd_id IN (0,1,5))) OR bool_or(d.psd_type='Potongan' AND d.psd_id IN (4,8)) has_gross,
    COALESCE(SUM(d.psd_amount) FILTER (WHERE COALESCE(d.psd_tax,0)=2 AND d.psd_id<>75 AND NOT (d.psd_type='Potongan' AND d.psd_id IN (0,1,5))), 0)
      - COALESCE(SUM(d.psd_amount) FILTER (WHERE d.psd_type='Potongan' AND d.psd_id IN (4,8)), 0) gross,
    COALESCE(SUM(d.psd_amount) FILTER (WHERE d.psd_type='Potongan' AND d.psd_id IN (14,18,22,23)), 0) deduct,
    bool_or(d.psd_type='Potongan' AND d.psd_id=1) has_p1,
    (array_agg(d.psd_amount::text) FILTER (WHERE d.psd_type='Potongan' AND d.psd_id=1))[1] p1
  FROM payroll.pyr_salary_detail d WHERE d.psd_run_id IN (SELECT pr_id FROM cur) GROUP BY 1
), frz AS (
  -- Frozen P14/P18 (frz_flag <> 2) still feed the annual deduction (D-28d/D-29): clamp(Σ TETAP, MIN, MAX) × rate, rounded.
  SELECT e.pec_nik nik, SUM(ROUND(CASE WHEN mx.v IS NOT NULL AND t.tetap > mx.v THEN mx.v WHEN t.tetap < mn.v THEN mn.v ELSE t.tetap END * e.pec_amount, 0)) amt
  FROM payroll.pyr_employee_component e JOIN payroll.pyr_component c ON c.pc_sysid=e.pec_component_sysid
  JOIN LATERAL (SELECT COALESCE(SUM(x.pec_amount),0) tetap FROM payroll.pyr_employee_component x JOIN payroll.pyr_component xc ON xc.pc_sysid=x.pec_component_sysid
     WHERE x.pec_company_id=$1 AND x.pec_nik=e.pec_nik AND x.pec_division=e.pec_division AND x.pec_frz_flag=2
       AND xc.pc_code IN ('T001','T002','T003','T004','T007','T009','T055')) t ON true
  LEFT JOIN LATERAL (SELECT prc_value v FROM payroll.pyr_rule_constant WHERE prc_rule_set_id=$4 AND prc_division=$2
     AND prc_key = CASE c.pc_code WHEN 'P014' THEN 'NAKMIN' ELSE 'PENMIN' END) mn ON true
  LEFT JOIN LATERAL (SELECT prc_value v FROM payroll.pyr_rule_constant WHERE prc_rule_set_id=$4 AND prc_division=$2 AND c.pc_code='P018' AND prc_key='PENMAX') mx ON true
  WHERE e.pec_company_id=$1 AND e.pec_division=$2 AND e.pec_frz_flag<>2 AND c.pc_code IN ('P014','P018') AND COALESCE(e.pec_sts,'F')='F'
  GROUP BY 1
), adj AS (
  SELECT pta_nik nik, COALESCE(SUM(pta_amount) FILTER (WHERE pta_type='Tunjangan'),0) earn, COALESCE(SUM(pta_amount) FILTER (WHERE pta_type='Potongan'),0) ded
  FROM payroll.pyr_tax_adj WHERE pta_company_id=$1 AND pta_division=$2 AND pta_yymm=$3 GROUP BY 1
)
SELECT c.nik, COALESCE(e.pe_tax_id,''),
  e.pe_prev_employee_id IS NOT NULL OR EXISTS (SELECT 1 FROM payroll.pyr_employee o WHERE o.pe_company_id=$1 AND o.pe_npwp_hash=e.pe_npwp_hash AND o.pe_id<>e.pe_id)
   OR EXISTS (SELECT 1 FROM payroll.pyr_employee o WHERE o.pe_prev_employee_id=e.pe_id) chain,
  (SELECT count(*) FROM payroll.pyr_employee o JOIN payroll.pyr_run r ON r.pr_company_id=$1 AND r.pr_division=o.pe_division AND r.pr_status='ACTIVE'
     AND r.pr_parent_run_id IS NULL AND r.pr_yymm BETWEEN $3/100*100+1 AND $3-1
   JOIN payroll.pyr_salary_detail d ON d.psd_run_id=r.pr_id AND d.psd_nik=o.pe_nik AND d.psd_type='Potongan' AND d.psd_id=1
   WHERE o.pe_company_id=$1 AND o.pe_id<>e.pe_id AND (o.pe_id=e.pe_prev_employee_id OR o.pe_prev_employee_id=e.pe_id
     OR (e.pe_npwp_hash IS NOT NULL AND o.pe_npwp_hash=e.pe_npwp_hash))) other_ytd,
  (COALESCE(y.gross,0)+COALESCE(thr.gross,0))::text, COALESCE(y.deduct,0)::text, (COALESCE(y.p1,0)+COALESCE(p7.amt,0)+COALESCE(thr.paid,0))::text,
  c.gross::text, (c.deduct+COALESCE(frz.amt,0))::text, COALESCE(adj.earn,0)::text, COALESCE(adj.ded,0)::text, c.has_p1, c.p1
FROM c JOIN payroll.pyr_employee e ON e.pe_company_id=$1 AND e.pe_division=$2 AND e.pe_nik=c.nik
LEFT JOIN y ON y.nik=c.nik LEFT JOIN p7 ON p7.nik=c.nik LEFT JOIN thr ON thr.nik=c.nik LEFT JOIN adj ON adj.nik=c.nik LEFT JOIN frz ON frz.nik=c.nik
ORDER BY c.nik`

type taxTables struct {
	ptkp       map[string]decimal.Decimal
	jobRate    decimal.Decimal
	jobMax     decimal.Decimal
	layers     [][3]decimal.Decimal // min, max, rate
	cat        map[string]string
	ter        map[string][][3]decimal.Decimal
	kesMin     decimal.Decimal
	kesMax     decimal.Decimal
	hasKesMax  bool
	hasKesMins bool
}

func loadTables(t *testing.T, e env, div string) taxTables {
	t.Helper()
	tt := taxTables{ptkp: map[string]decimal.Decimal{}, cat: map[string]string{}, ter: map[string][][3]decimal.Decimal{}}
	rows, err := e.own.Query(`SELECT psv_vs_code, psv_code, COALESCE(psv_name,''), COALESCE(psv_short_name,''), COALESCE(psv_field_01,''), COALESCE(psv_field_02,'')
FROM payroll.pyr_static_value WHERE psv_rule_set_id=$1 AND psv_vs_code IN ('EMPTAX','PAYROLLSETUP')`, ruleSetID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	ptkpCodes := map[string]bool{"TK": true, "K": true, "K0": true, "K1": true, "K2": true, "K3": true, "TK1": true, "TK2": true, "TK3": true}
	for rows.Next() {
		var vs, code, name, short, f1, f2 string
		if err := rows.Scan(&vs, &code, &name, &short, &f1, &f2); err != nil {
			t.Fatal(err)
		}
		switch {
		case vs == "EMPTAX":
			tt.cat[code] = f1
		case ptkpCodes[code]:
			tt.ptkp[code] = decimal.RequireFromString(name)
		case code == "BIAYAJABATAN":
			tt.jobRate, tt.jobMax = decimal.RequireFromString(name), decimal.RequireFromString(short)
		case strings.HasPrefix(code, "TARIF"):
			tt.layers = append(tt.layers, [3]decimal.Decimal{decimal.RequireFromString(name), decimal.RequireFromString(short), decimal.RequireFromString(f1)})
		case (f1 == "A" || f1 == "B" || f1 == "C") && strings.HasPrefix(code, f1) && f2 != "":
			tt.ter[f1] = append(tt.ter[f1], [3]decimal.Decimal{decimal.RequireFromString(name), decimal.RequireFromString(short), decimal.RequireFromString(f2)})
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	var kmin, kmax sql.NullString
	if err := e.own.QueryRow(`SELECT (SELECT prc_value::text FROM payroll.pyr_rule_constant WHERE prc_rule_set_id=$1 AND prc_key='KESMIN' AND prc_division=$2),
  (SELECT prc_value::text FROM payroll.pyr_rule_constant WHERE prc_rule_set_id=$1 AND prc_key='KESMAX' AND prc_division=$2)`, ruleSetID, div).Scan(&kmin, &kmax); err != nil {
		t.Fatal(err)
	}
	if kmin.Valid {
		tt.kesMin, tt.hasKesMins = decimal.RequireFromString(kmin.String), true
	}
	if kmax.Valid {
		tt.kesMax, tt.hasKesMax = decimal.RequireFromString(kmax.String), true
	}
	return tt
}

// progressiveV1 is the legacy layer sum quoted in D-29 (PKP above Max → tax on the whole Max; PKP = Max → 0; no TRUNC, no floor).
func progressiveV1(layers [][3]decimal.Decimal, pkp decimal.Decimal) decimal.Decimal {
	var tax decimal.Decimal
	for _, l := range layers {
		lower := l[0].Sub(decimal.NewFromInt(1))
		var b decimal.Decimal
		switch {
		case pkp.GreaterThan(l[1]):
			b = l[1]
		case pkp.LessThan(l[1]) && pkp.GreaterThanOrEqual(lower):
			b = pkp.Sub(lower)
		}
		tax = tax.Add(b.Mul(l[2]).Shift(-2))
	}
	return tax
}

type decRow struct {
	nik, taxID                                                   string
	chain                                                        bool
	otherYTD                                                     int
	ytdGross, ytdDeduct, ytdPaid, curGross, curDeduct, adjE, adjD decimal.Decimal
	hasP1                                                        bool
	p1                                                           sql.NullString
}

func (r decRow) expected(tt taxTables) (*decimal.Decimal, string) {
	ptkp, ok := tt.ptkp[r.taxID]
	if !ok {
		return nil, "tanpa PTKP"
	}
	twelve := decimal.NewFromInt(12)
	gross := r.ytdGross.Add(r.curGross).Add(r.adjE)
	deduct := r.ytdDeduct.Add(r.curDeduct).Add(r.adjD)
	jc := decimal.Min(gross.Mul(tt.jobRate).Shift(-2), tt.jobMax.Mul(twelve))
	pkp := gross.Sub(deduct).Sub(jc).Sub(ptkp.Mul(twelve))
	tax := progressiveV1(tt.layers, pkp)
	p1 := tax.Sub(r.ytdPaid).Round(0)
	return &p1, fmt.Sprintf("bruto %s (YTD %s + bulan ini %s + adj %s) | pengurang %s (YTD %s + bulan ini %s + adj %s) | BJ %s | PTKP %s×12 | PKP %s | PPh setahun %s | dibayar Jan–Nov %s | P1 %s",
		gross, r.ytdGross, r.curGross, r.adjE, deduct, r.ytdDeduct, r.curDeduct, r.adjD, jc, ptkp, pkp, tax, r.ytdPaid, p1)
}

func checkDecember(t *testing.T, e env, rep *report, div string, _ lines) {
	t.Helper()
	tt := loadTables(t, e, div)
	rows, err := e.own.Query(decSQL, company, div, 202612, ruleSetID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var match, mismatch, chainMis, chainOK, chainBad, noP1 int
	var hand []string
	for rows.Next() {
		var r decRow
		var v [7]string
		if err := rows.Scan(&r.nik, &r.taxID, &r.chain, &r.otherYTD, &v[0], &v[1], &v[2], &v[3], &v[4], &v[5], &v[6], &r.hasP1, &r.p1); err != nil {
			t.Fatal(err)
		}
		ds := make([]decimal.Decimal, 7)
		for i, s := range v {
			ds[i] = decimal.RequireFromString(s)
		}
		r.ytdGross, r.ytdDeduct, r.ytdPaid, r.curGross, r.curDeduct, r.adjE, r.adjD = ds[0], ds[1], ds[2], ds[3], ds[4], ds[5], ds[6]
		if !r.hasP1 {
			noP1++
			continue
		}
		want, why := r.expected(tt)
		got := nullDec(r.p1)
		ok := eqAmt(want, got)
		switch {
		case ok && r.chain:
			chainOK++
		case ok:
			match++
		case r.chain && r.otherYTD > 0:
			// The per-NIK SQL misses the YTD of the other placement (13§3.3): a mismatch is only explained when that placement has
			// P1 lines Jan–Nov; otherwise it is a real difference.
			chainMis++
			rep.detailf("DEC chain %s %s: run %s, hitung per-NIK %s (penempatan lain punya %d baris P1 Jan–Nov, YTD lintas penempatan)", div, r.nik,
				fmtAmt(got, true), fmtAmt(want, true), r.otherYTD)
		case r.chain:
			chainBad++
			rep.detailf("DEC chain tanpa YTD lain %s %s: run %s, hitung %s | %s", div, r.nik, fmtAmt(got, true), fmtAmt(want, true), why)
		default:
			mismatch++
			rep.detailf("DEC %s %s: run %s, hitung %s | %s", div, r.nik, fmtAmt(got, true), fmtAmt(want, true), why)
		}
		if ok && !r.chain && len(hand) < 3 && want != nil && !want.IsZero() && r.ytdPaid.IsPositive() {
			hand = append(hand, fmt.Sprintf("%s %s: %s", div, r.nik, why))
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for _, h := range hand {
		rep.handf("%s", h)
	}
	rep.addf("invarian P1 202612 %s (06§6.2/D-29, SQL Jan–Nov terpisah): cocok %d, beda %d, rantai cocok %d / beda karena YTD penempatan lain %d / beda tanpa penjelasan %d, tanpa P1 %d",
		div, match, mismatch, chainOK, chainMis, chainBad, noP1)
	if mismatch > 0 || chainBad > 0 {
		t.Errorf("202612 %s: %d NIK (+%d rantai tanpa YTD lain) P1 tidak sama dengan hitung ulang 06§6.2", div, mismatch, chainBad)
	}
}

// checkJanuary: 202701 P1 = TER without YTD; T15/P15 not NULL wherever 202612 had them, based on clamp(JAN_BASE) (D-64 (4)).
func checkJanuary(t *testing.T, e env, rep *report, div string, dec, jan lines) {
	t.Helper()
	tt := loadTables(t, e, div)
	rows, err := e.own.Query(`SELECT d.psd_nik, COALESCE(e.pe_tax_id,''),
  COALESCE(SUM(d.psd_amount) FILTER (WHERE COALESCE(d.psd_tax,0)=2 AND NOT (d.psd_type='Potongan' AND d.psd_id IN (0,1,5))),0)
   - COALESCE(SUM(d.psd_amount) FILTER (WHERE d.psd_type='Potongan' AND d.psd_id IN (4,8)),0),
  (array_agg(d.psd_amount::text) FILTER (WHERE d.psd_type='Potongan' AND d.psd_id=1))[1],
  bool_or(d.psd_type='Potongan' AND d.psd_id=1),
  (array_agg(d.psd_remarks) FILTER (WHERE d.psd_type='Tunjangan' AND d.psd_id=15))[1],
  (SELECT pec.pec_amount::text FROM payroll.pyr_employee_component pec JOIN payroll.pyr_component c ON c.pc_sysid=pec.pec_component_sysid
    WHERE c.pc_code='T901' AND pec.pec_company_id=$1 AND pec.pec_nik=d.psd_nik AND pec.pec_division=$2)
FROM payroll.pyr_salary_detail d JOIN payroll.pyr_run r ON r.pr_id=d.psd_run_id
JOIN payroll.pyr_employee e ON e.pe_company_id=$1 AND e.pe_division=$2 AND e.pe_nik=d.psd_nik
WHERE r.pr_company_id=$1 AND r.pr_division=$2 AND r.pr_yymm=202701 AND r.pr_status='ACTIVE' AND r.pr_parent_run_id IS NULL
GROUP BY d.psd_nik, e.pe_tax_id ORDER BY 1`, company, div)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var terOK, terBad, baseOK, baseBad int
	hasJanBase := map[string]bool{}
	for rows.Next() {
		var nik, taxID, gross string
		var p1, remark, t901 sql.NullString
		var hasP1 bool
		if err := rows.Scan(&nik, &taxID, &gross, &p1, &hasP1, &remark, &t901); err != nil {
			t.Fatal(err)
		}
		if hasP1 {
			want := terP1(tt, taxID, decimal.RequireFromString(gross))
			if eqAmt(want, nullDec(p1)) {
				terOK++
			} else {
				terBad++
				rep.detailf("JAN TER %s %s: run %s, hitung %s (gross %s, status %s)", div, nik, fmtAmt(nullDec(p1), true), fmtAmt(want, true), gross, taxID)
			}
		}
		hasJanBase[nik] = t901.Valid
		if remark.Valid && t901.Valid {
			b := decimal.RequireFromString(t901.String)
			switch {
			case tt.hasKesMax && b.GreaterThan(tt.kesMax):
				b = tt.kesMax
			case b.LessThan(tt.kesMin):
				b = tt.kesMin
			}
			if strings.HasPrefix(remark.String, "Basis ") && remarkBase(remark.String).Equal(b) {
				baseOK++
			} else {
				baseBad++
				rep.detailf("JAN T015 %s %s: remark %q, JAN_BASE clamp %s", div, nik, remark.String, b)
			}
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	var nullJan, bpjsSame, bpjsDiff, sameNonPPh, diffNonPPh int
	noJanBase := map[string]bool{}
	for _, nik := range sortedKeys(dec) {
		j, ok := jan[nik]
		if !ok {
			continue
		}
		for _, code := range []string{"T015", "P015"} {
			d, okD := dec[nik][code]
			if !okD || d == nil {
				continue
			}
			v, okJ := j[code]
			switch {
			case (!okJ || v == nil) && !hasJanBase[nik]:
				noJanBase[nik] = true
				rep.detailf("JAN NULL tanpa T901 %s %s %s: 202612 %s, 202701 NULL (TETAP 0/NULL, legacy juga NULL)", div, nik, code, d)
			case !okJ || v == nil:
				nullJan++
				rep.detailf("JAN NULL %s %s %s: 202612 %s, 202701 NULL/tidak ada", div, nik, code, d)
			case v.Equal(*d):
				bpjsSame++
			default:
				bpjsDiff++
				rep.detailf("JAN BPJS %s %s %s: 202612 %s, 202701 %s", div, nik, code, d, v)
			}
		}
		// January differs from December only by the JAN_BASE-based BPJS Kes (T15/P15/P16 of NIK without T901) and the T901 line itself.
		var other []string
		for _, d := range diffLines(dec[nik], j) {
			if !janOnly[d[:4]] {
				other = append(other, d)
			}
		}
		if len(other) == 0 {
			sameNonPPh++
		} else {
			diffNonPPh++
			rep.detailf("JAN DIFF %s %s: %v", div, nik, other)
		}
	}
	rep.addf("invarian 202701 %s: P1 TER tanpa YTD cocok %d / beda %d; T015 basis = clamp(JAN_BASE) cocok %d / beda %d; T15/P15 NULL %d, NULL karena tanpa T901 (TETAP 0) %d NIK; T15/P15 = 202612 %d / beda %d; NIK non-PPh (di luar T901/T15/P15/P16) = 202612 %d / beda %d",
		div, terOK, terBad, baseOK, baseBad, nullJan, len(noJanBase), bpjsSame, bpjsDiff, sameNonPPh, diffNonPPh)
	if terBad > 0 || baseBad > 0 || nullJan > 0 || baseOK == 0 || diffNonPPh > 0 {
		t.Errorf("202701 %s: TER beda %d, basis JAN_BASE beda %d (cocok %d), T15/P15 NULL %d, non-PPh beda %d", div, terBad, baseBad, baseOK, nullJan, diffNonPPh)
	}
}

// janOnly are the January-only differences allowed by D-64 (4): T901 (JAN_BASE line, LEGACY_NULL) and BPJS Kes T15/P15 (+P16 its basis).
var janOnly = map[string]bool{"T901": true, "T015": true, "P015": true, "P016": true}

func terP1(tt taxTables, taxID string, gross decimal.Decimal) *decimal.Decimal {
	cat, ok := tt.cat[taxID]
	if !ok {
		return nil
	}
	for _, b := range tt.ter[cat] {
		if gross.GreaterThanOrEqual(b[0]) && gross.LessThanOrEqual(b[1]) {
			p := gross.Mul(b[2]).Shift(-2).Round(0)
			return &p
		}
	}
	return nil
}

func remarkBase(s string) decimal.Decimal {
	f := strings.Fields(s)
	if len(f) < 2 {
		return decimal.NewFromInt(-1)
	}
	d, err := decimal.NewFromString(f[1])
	if err != nil {
		return decimal.NewFromInt(-1)
	}
	return d
}

func nullDec(s sql.NullString) *decimal.Decimal {
	if !s.Valid {
		return nil
	}
	d := decimal.RequireFromString(s.String)
	return &d
}
