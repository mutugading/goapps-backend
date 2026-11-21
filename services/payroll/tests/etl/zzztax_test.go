package etl

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/run"
	"github.com/mutugading/goapps-backend/services/payroll/internal/engine"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/postgres"
)

// seedTax builds tax year 2031 of SOLO from LEGACY runs (T016c): Y-A has an earlier placement Y-A-OLD
// with the same NPWP, Y-B has no NPWP and a pe_prev_employee_id chain, Y-C's chain has another NPWP,
// Y-S carries the P0 basis (its P0 row is frozen, so rule v1 skips P0).
func seedTax(t *testing.T) {
	t.Helper()
	for _, q := range []string{
		`INSERT INTO payroll.pyr_component (pc_company_id, pc_code, pc_type, pc_name, pc_status, pc_valid_from, pc_subsidy_eligible)
		 VALUES (1,'T001','Tunjangan','Gaji Pokok','ACTIVE',202001,true), (1,'T074','Tunjangan','Beras','ACTIVE',202001,true),
		        (1,'T006','Tunjangan','Lembur','ACTIVE',202001,false), (1,'T073','Tunjangan','Subsidi Beras','ACTIVE',202001,false),
		        (1,'T075','Tunjangan','THR','ACTIVE',202001,false), (1,'P000','Potongan','Subsidi PPh','ACTIVE',202001,false)
		 ON CONFLICT (pc_company_id, pc_code) DO UPDATE SET pc_subsidy_eligible = EXCLUDED.pc_subsidy_eligible`,
		`INSERT INTO payroll.pyr_component (pc_company_id, pc_code, pc_type, pc_name, pc_status, pc_valid_from)
		 VALUES (1,'P001','Potongan','PPh 21','ACTIVE',202001), (1,'P005','Potongan','PPh Perusahaan','ACTIVE',202001),
		        (1,'P004','Potongan','Absensi','ACTIVE',202001), (1,'P008','Potongan','Deduction Impact','ACTIVE',202001),
		        (1,'P014','Potongan','JHT','ACTIVE',202001), (1,'P018','Potongan','Pensiun','ACTIVE',202001),
		        (1,'P022','Potongan','Hutang JHT','ACTIVE',202001)
		 ON CONFLICT (pc_company_id, pc_code) DO NOTHING`,
		`INSERT INTO payroll.pyr_employee (pe_company_id, pe_nik, pe_division, pe_nama, pe_level, pe_tax_id, pe_sts_payroll, pe_orion_sub_acnt, pe_npwp_hash) VALUES
		  (1,'Y-A','SOLO','A','Staff','K1',2,'Y-A','\xaa'), (1,'Y-A-OLD','SOLO','A lama','Staff','K1',1,'Y-A-OLD','\xaa'),
		  (1,'Y-B','SOLO','B','Staff','TK',2,'Y-B',NULL), (1,'Y-B-OLD','SOLO','B lama','Staff','TK',1,'Y-B-OLD',NULL),
		  (1,'Y-C','SOLO','C','Staff','TK',2,'Y-C','\xcc'), (1,'Y-C-OLD','SOLO','C lama','Staff','TK',1,'Y-C-OLD','\xdd'),
		  (1,'Y-S','SOLO','S','Staff','TK',2,'Y-S','\x55'),
		  (1,'Y-D','SOLO','D','Staff','TK',2,'Y-D','\xd1'), (1,'Y-D-OLD','SOLO','D lama','Staff','TK',1,'Y-D-OLD',NULL),
		  (1,'Y-E','SOLO','E','Staff','TK',2,'Y-E',NULL), (1,'Y-E-OLD','SOLO','E lama','Staff','TK',1,'Y-E-OLD','\xe1'),
		  (1,'Y-E-OLD-OLD','SOLO','E awal','Staff','TK',1,'Y-E-OLD-OLD','\xe2')`,
		`UPDATE payroll.pyr_employee e SET pe_prev_employee_id = o.pe_id FROM payroll.pyr_employee o
		 WHERE e.pe_company_id = 1 AND o.pe_company_id = 1 AND o.pe_nik = e.pe_nik || '-OLD' AND e.pe_nik IN ('Y-B','Y-C','Y-D','Y-E','Y-E-OLD')`,
		`INSERT INTO payroll.pyr_employee_component (pec_company_id, pec_nik, pec_division, pec_component_sysid, pec_sts, pec_tax, pec_amount, pec_frz_flag)
		 SELECT 1, v.nik, 'SOLO', c.pc_sysid, 'F', v.tax, v.amt, v.frz
		 FROM (VALUES ('Y-A','T001',2,2000000,2), ('Y-A','P000',0,0,2), ('Y-B','T001',2,800000,2), ('Y-C','T001',2,1,2),
		              ('Y-S','T001',2,3200000,2), ('Y-S','P000',0,0,1)) v(nik,code,tax,amt,frz)
		 JOIN payroll.pyr_component c ON c.pc_company_id = 1 AND c.pc_code = v.code`,
		`INSERT INTO payroll.pyr_period (pp_company_id, pp_yymm, pp_division, pp_date_from, pp_date_to)
		 SELECT 1, y, 'SOLO', make_date(2031, y % 100, 25) - interval '1 month' + interval '1 day', make_date(2031, y % 100, 25)
		 FROM unnest(ARRAY[203101,203102,203103,203104,203106,203107,203112]) y`,
		`INSERT INTO payroll.pyr_attd_days (pad_company_id, pad_yymm, pad_division, pad_days)
		 SELECT 1, y, 'SOLO', 26 FROM unnest(ARRAY[203101,203102,203104,203107,203112]) y`,
		`INSERT INTO payroll.pyr_att_snapshot (pas_company_id, pas_yymm, pas_division, pas_source_batch_ids, pas_checksum, pas_locked)
		 SELECT 1, y, 'SOLO', '{1}', 'tax', true FROM unnest(ARRAY[203101,203102,203104,203107,203112]) y`,
		`INSERT INTO payroll.pyr_att_snapshot_row (pasr_snapshot_id, pasr_kind, pasr_nik, pasr_data)
		 SELECT s.pas_snapshot_id, 'REKAP', n, '{"MPAR_DAY_WORK":26,"MPAR_PAY_TOT":26}'::jsonb
		 FROM payroll.pyr_att_snapshot s, unnest(ARRAY['Y-A','Y-B','Y-C','Y-D','Y-E','Y-S']) n
		 WHERE s.pas_company_id = 1 AND s.pas_division = 'SOLO' AND s.pas_yymm BETWEEN 203101 AND 203112`,
		`INSERT INTO payroll.pyr_run (pr_company_id, pr_yymm, pr_division, pr_pay_date, pr_rule_set_id, pr_snapshot_id, pr_status, pr_kind)
		 VALUES (1,203101,'SOLO','2031-01-25',0,NULL,'ACTIVE','LEGACY'), (1,203102,'SOLO','2031-02-25',0,NULL,'SUPERSEDED','LEGACY'),
		        (1,203103,'SOLO','2031-03-25',0,NULL,'ACTIVE','LEGACY'), (1,203106,'SOLO','2031-06-25',0,NULL,'ACTIVE','LEGACY')`,
		`INSERT INTO payroll.pyr_run (pr_company_id, pr_yymm, pr_division, pr_pay_date, pr_rule_set_id, pr_snapshot_id, pr_status, pr_kind)
		 SELECT 1, 203102, 'SOLO', '2031-02-25', 0, pas_snapshot_id, 'ACTIVE', 'ENGINE' FROM payroll.pyr_att_snapshot
		 WHERE pas_company_id = 1 AND pas_yymm = 203102 AND pas_division = 'SOLO'`,
		// sysid NULL on the T074 line: the subsidy flag then comes from the component code.
		`INSERT INTO payroll.pyr_salary_detail (psd_run_id, psd_nik, psd_type, psd_id, psd_amount, psd_tax, psd_tax_paid_company, psd_component_sysid, psd_meta)
		 SELECT r.pr_id, v.nik, v.typ::payroll.comp_type, v.id, v.amt, v.tax, v.tpc, CASE WHEN v.sysid THEN c.pc_sysid END, v.meta::jsonb
		 FROM (VALUES
		   (203101,'ACTIVE','Y-A-OLD','Tunjangan',1,1000000,2,'N',true,NULL),
		   (203101,'ACTIVE','Y-A-OLD','Potongan',4,50000,0,'N',true,NULL),
		   (203101,'ACTIVE','Y-A-OLD','Potongan',1,20000,0,'N',true,NULL),
		   (203102,'SUPERSEDED','Y-A','Tunjangan',1,999999,2,'N',true,NULL),
		   (203102,'SUPERSEDED','Y-S','Potongan',0,-1,0,'N',true,'{"subsidy_basis_source":"LOCK_NOW","subsidy_basis":"1"}'),
		   (203102,'ACTIVE','Y-S','Potongan',0,-10000,0,'N',true,'{"subsidy_basis_source":"LOCK_NOW","subsidy_basis":"3100000"}'),
		   (203103,'ACTIVE','Y-A','Tunjangan',1,2000000,2,'N',true,NULL),
		   (203103,'ACTIVE','Y-A','Tunjangan',73,100000,2,'Y',true,NULL),
		   (203103,'ACTIVE','Y-A','Tunjangan',75,300000,2,'N',true,NULL),
		   (203103,'ACTIVE','Y-A','Potongan',8,10000,0,'N',true,NULL),
		   (203103,'ACTIVE','Y-A','Potongan',14,40000,0,'N',true,NULL),
		   (203103,'ACTIVE','Y-A','Potongan',22,5000,0,'N',true,NULL),
		   (203103,'ACTIVE','Y-A','Potongan',1,30000,0,'N',true,NULL),
		   (203103,'ACTIVE','Y-A','Potongan',5,7000,0,'N',true,NULL),
		   (203103,'ACTIVE','Y-B-OLD','Tunjangan',1,800000,2,'N',true,NULL),
		   (203103,'ACTIVE','Y-D-OLD','Tunjangan',1,600000,2,'N',true,NULL),
		   (203103,'ACTIVE','Y-S','Tunjangan',1,3100000,2,'N',true,NULL),
		   (203106,'ACTIVE','Y-A','Tunjangan',1,2000000,2,'N',true,NULL),
		   (203106,'ACTIVE','Y-A','Potongan',18,20000,0,'N',true,NULL),
		   (203106,'ACTIVE','Y-A','Potongan',1,30000,0,'N',true,NULL),
		   (203101,'ACTIVE','Y-S','Tunjangan',1,3000000,2,'N',true,NULL),
		   (203101,'ACTIVE','Y-S','Tunjangan',6,500000,2,'N',true,NULL),
		   (203101,'ACTIVE','Y-S','Potongan',1,40000,0,'N',true,NULL),
		   (203106,'ACTIVE','Y-S','Tunjangan',1,3200000,2,'N',true,NULL),
		   (203106,'ACTIVE','Y-S','Tunjangan',74,50000,2,'N',false,NULL)
		 ) v(yymm,sts,nik,typ,id,amt,tax,tpc,sysid,meta)
		 JOIN payroll.pyr_run r ON r.pr_company_id = 1 AND r.pr_division = 'SOLO' AND r.pr_yymm = v.yymm AND r.pr_status::text = v.sts
		 JOIN payroll.pyr_component c ON c.pc_company_id = 1 AND c.pc_code = CASE v.typ WHEN 'Tunjangan' THEN 'T' ELSE 'P' END || lpad(v.id::text, 3, '0')`,
		`INSERT INTO payroll.pyr_addition (pa_company_id, pa_yymm, pa_nik, pa_division, pa_type, pa_id, pa_amount, pa_remarks, pa_source) VALUES
		  (1,203104,'Y-A','SOLO','Potongan',7,-15000,'PPh THR','MANUAL'), (1,203112,'Y-A','SOLO','Potongan',7,-99,'bulan berjalan','MANUAL')`,
		`INSERT INTO payroll.pyr_tax_adj (pta_company_id, pta_yymm, pta_division, pta_nik, pta_type, pta_amount) VALUES
		  (1,203112,'SOLO','Y-A','Tunjangan',500000), (1,203112,'SOLO','Y-A','Potongan',25000), (1,203111,'SOLO','Y-B','Tunjangan',7)`,
	} {
		if _, err := ownDB.Exec(q); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}
	seedTaxThr(t)
}

// seedTaxThr adds THR/Bonus events of 2031 SOLO (T024c, D-56): THR March and December, Bonus May (of
// Y-A-OLD, same NPWP as Y-A) CLOSED, Bonus September still DRAFT. Most lines have no component sysid,
// like the legacy history. Rows go in while the event is DRAFT; the guard then forbids changes.
func seedTaxThr(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { dropTaxThr(t) }) // before the inserts: a failed seed is cleaned up too
	for _, q := range []string{
		`INSERT INTO payroll.pyr_thr_event (pte_company_id, pte_division, pte_kind, pte_yymm, pte_event_date, pte_description, created_by)
		 VALUES (1,'SOLO','THR',203103,'2031-03-10','THR','t'), (1,'SOLO','BONUS',203105,'2031-05-10','Bonus','t'),
		        (1,'SOLO','THR',203112,'2031-12-10','THR Des','t'), (1,'SOLO','BONUS',203109,'2031-09-10','Bonus draft','t')`,
		`INSERT INTO payroll.pyr_thr_run (ptr_event_id, ptr_kind, ptr_status)
		 SELECT pte_id, CASE WHEN pte_yymm = 203109 THEN 'NORMAL' ELSE 'LEGACY' END, 'ACTIVE' FROM payroll.pyr_thr_event
		 WHERE pte_company_id = 1 AND pte_division = 'SOLO' AND pte_yymm BETWEEN 203101 AND 203112`,
		`INSERT INTO payroll.pyr_thr_detail (ptd_run_id, ptd_employee_id, ptd_nik, ptd_component_sysid, ptd_type, ptd_id, ptd_amount, ptd_meta)
		 SELECT r.ptr_id, e.pe_id, v.nik, CASE WHEN v.mapped THEN c.pc_sysid END, v.typ::payroll.comp_type, v.id, v.amt,
		        CASE WHEN NOT v.mapped THEN '{"legacy_unmapped":true}'::jsonb END
		 FROM (VALUES
		   (203103,'Y-A','Tunjangan',1,1000000,true), (203103,'Y-A','Tunjangan',800,50000,false),
		   (203103,'Y-A','Tunjangan',36,0,false),
		   (203103,'Y-A','Potongan',1,40000,false), (203103,'Y-A','Potongan',32,10000,false),
		   (203105,'Y-A-OLD','Tunjangan',1,2000000,false), (203105,'Y-A-OLD','Potongan',1,100000,false),
		   (203105,'Y-A-OLD','Potongan',3,20000,false), (203105,'Y-A-OLD','Potongan',2,5000,false),
		   (203112,'Y-A','Tunjangan',1,500000,false), (203112,'Y-A','Potongan',1,25000,false),
		   (203109,'Y-A','Tunjangan',1,777000,false), (203109,'Y-A','Potongan',1,7000,false)
		 ) v(yymm,nik,typ,id,amt,mapped)
		 JOIN payroll.pyr_thr_event ev ON ev.pte_company_id = 1 AND ev.pte_division = 'SOLO' AND ev.pte_yymm = v.yymm
		 JOIN payroll.pyr_thr_run r ON r.ptr_event_id = ev.pte_id
		 JOIN payroll.pyr_employee e ON e.pe_company_id = 1 AND e.pe_division = 'SOLO' AND e.pe_nik = v.nik
		 JOIN payroll.pyr_component c ON c.pc_company_id = 1 AND c.pc_code = 'T075'`,
		`UPDATE payroll.pyr_thr_event ev SET pte_active_run_id = r.ptr_id,
		   pte_status = CASE WHEN ev.pte_yymm = 203109 THEN 'DRAFT' ELSE 'CLOSED' END
		 FROM payroll.pyr_thr_run r WHERE r.ptr_event_id = ev.pte_id AND ev.pte_company_id = 1 AND ev.pte_division = 'SOLO'
		   AND ev.pte_yymm BETWEEN 203101 AND 203112`,
	} {
		if _, err := ownDB.Exec(q); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}
}

// dropTaxThr removes the 2031 SOLO events of company 1 again (later tests count every THR row of the shared DB). The CLOSED
// guard is switched off inside one owner transaction only, so other sessions never see it disabled.
func dropTaxThr(t *testing.T) {
	t.Helper()
	tx, err := ownDB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, q := range []string{
		`ALTER TABLE payroll.pyr_thr_event DISABLE TRIGGER trg_guard_thr_event`,
		`ALTER TABLE payroll.pyr_thr_run DISABLE TRIGGER trg_guard_thr_run`,
		`ALTER TABLE payroll.pyr_thr_detail DISABLE TRIGGER trg_guard_thr_detail`,
		`UPDATE payroll.pyr_thr_event SET pte_active_run_id = NULL WHERE pte_company_id = 1 AND pte_division = 'SOLO' AND pte_yymm BETWEEN 203101 AND 203112`,
		`DELETE FROM payroll.pyr_thr_run r USING payroll.pyr_thr_event e
		 WHERE e.pte_id = r.ptr_event_id AND e.pte_company_id = 1 AND e.pte_division = 'SOLO' AND e.pte_yymm BETWEEN 203101 AND 203112`,
		`DELETE FROM payroll.pyr_thr_event WHERE pte_company_id = 1 AND pte_division = 'SOLO' AND pte_yymm BETWEEN 203101 AND 203112`,
		`ALTER TABLE payroll.pyr_thr_event ENABLE TRIGGER trg_guard_thr_event`,
		`ALTER TABLE payroll.pyr_thr_run ENABLE TRIGGER trg_guard_thr_run`,
		`ALTER TABLE payroll.pyr_thr_detail ENABLE TRIGGER trg_guard_thr_detail`,
	} {
		if _, err := tx.Exec(q); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func TestRunLoader_TaxInputs(t *testing.T) {
	ctx := context.Background()
	seedTax(t)
	raw, err := sql.Open("pgx", appDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	app := postgres.NewDBFromSQL(raw)
	subsidy := func(periods ...int) func(string) engine.Subsidy {
		var rs []engine.SubsidyRule
		for i, p := range periods {
			rs = append(rs, engine.SubsidyRule{ID: int64(i + 1), BasisPeriod: p, Limit: dec("10000000"), Kind: engine.SubsidyFullPPh})
		}
		return func(string) engine.Subsidy { return engine.Subsidy{Rules: rs} }
	}
	load := func(yymm int, legacy bool, niks ...string) (map[string]engine.Input, []run.Issue, error) {
		var res run.Result
		err := app.ScopedTx(asScopes(t, "SOLO"), func(tx *sql.Tx) error {
			var lerr error
			res, lerr = run.NewLoader().Load(ctx, tx, run.Query{CompanyID: 1, Division: "SOLO", YYMM: yymm, NIKs: niks},
				run.Rules{Set: stubRules{legacy}, Subsidy: subsidy(203101)})
			return lerr
		})
		out := map[string]engine.Input{}
		for _, in := range res.Inputs {
			out[in.Emp.NIK] = in
		}
		return out, res.Warnings, err
	}
	eq := func(t *testing.T, name string, got decimal.Decimal, want string) {
		t.Helper()
		if !got.Equal(dec(want)) {
			t.Errorf("%s = %s, want %s", name, got, want)
		}
	}
	ptr := func(t *testing.T, name string, got *decimal.Decimal, want string) {
		t.Helper()
		switch {
		case want == "" && got != nil:
			t.Errorf("%s = %s, want nil", name, got)
		case want != "" && (got == nil || !got.Equal(dec(want))):
			t.Errorf("%s = %v, want %s", name, got, want)
		}
	}

	t.Run("December YTD (D-29, D-31)", func(t *testing.T) {
		ins, warns, err := load(203112, true, "Y-A", "Y-B", "Y-D", "Y-S")
		if err != nil {
			t.Fatal(err)
		}
		a := ins["Y-A"]
		// Y-A + Y-A-OLD (same NPWP): T1 1.000.000 + 2.000.000 + 2.000.000 − P4 50.000 − P8 10.000;
		// T73 (company-paid), T75 and the SUPERSEDED run are left out. THR/Bonus of CLOSED events before
		// December (D-56): THR March T1 1.000.000 + T800 50.000, Bonus May (Y-A-OLD) 2.000.000.
		eq(t, "YTD.Gross", a.YTD.Gross, "7990000")
		eq(t, "YTD.Deduct", a.YTD.Deduct, "65000") // P14 + P22 + P18
		// P1 20.000 + 30.000 + 30.000 + addition P7 −15.000 (Dec P7 not) + PPh THR no 1 40.000 + Bonus no 1, 3
		// 100.000 + 20.000 (THR no 32, Bonus no 2, December and the DRAFT event not).
		eq(t, "YTD.TaxPaid", a.YTD.TaxPaid, "225000")
		eq(t, "YTDCompany.Gross", a.YTDCompany.Gross, "5400000")
		eq(t, "YTDCompany.CompanyPaid", a.YTDCompany.CompanyPaid, "100000")
		eq(t, "YTDCompany.Deduct", a.YTDCompany.Deduct, "110000") // P4 + P14 + P18
		eq(t, "YTDCompany.ThrBonus", a.YTDCompany.ThrBonus, "3550000") // 1.050.000 + 2.000.000 + THR Dec 500.000
		eq(t, "YTDCompany.P5Paid", a.YTDCompany.P5Paid, "7000")
		eq(t, "TaxAdj.Earn", a.TaxAdj.Earn, "500000")
		eq(t, "TaxAdj.Deduct", a.TaxAdj.Deduct, "25000")
		if !a.Thr.Gross.IsZero() || !a.Thr.PPhPaid.IsZero() {
			t.Errorf("Thr must stay empty until B-THR: %+v", a.Thr)
		}
		b := ins["Y-B"] // no NPWP: chain to Y-B-OLD + warning; tax adj of November is not December's
		eq(t, "Y-B YTD.Gross", b.YTD.Gross, "800000") // no THR/Bonus
		eq(t, "Y-B ThrBonus", b.YTDCompany.ThrBonus, "0")
		eq(t, "Y-B TaxAdj.Earn", b.TaxAdj.Earn, "0")
		eq(t, "Y-D YTD.Gross", ins["Y-D"].YTD.Gross, "600000") // NPWP target, predecessor without NPWP joins
		// every NIK has a recap: only the chain warning and the notice of the December THR event
		if len(warns) != 2 || warns[0].NIK != "Y-B" || warns[0].Code != run.CodeYTDNoNPWP || warns[1].Code != run.CodeThrNotLoaded {
			t.Errorf("warnings: %+v", warns)
		}
		for _, nik := range []string{"Y-C", "Y-E"} { // Y-E: no NPWP itself, two NPWPs further up the chain
			if _, _, err := load(203112, true, nik); !errors.Is(err, run.ErrYTDNPWPConflict) {
				t.Errorf("%s conflict: %v", nik, err)
			}
		}
	})
	t.Run("non-December leaves YTD empty", func(t *testing.T) {
		ins, _, err := load(203107, true, "Y-A", "Y-C")
		if err != nil {
			t.Fatal(err)
		}
		if a := ins["Y-A"]; !a.YTD.Gross.IsZero() || !a.YTD.TaxPaid.IsZero() || !a.YTDCompany.Gross.IsZero() || !a.YTDCompany.ThrBonus.IsZero() || !a.Thr.Gross.IsZero() {
			t.Errorf("YTD outside December: %+v %+v", a.YTD, a.YTDCompany)
		}
	})
	t.Run("rule v1 subsidy basis (D-30)", func(t *testing.T) {
		jul, _, err := load(203107, true, "Y-A", "Y-S")
		if err != nil {
			t.Fatal(err)
		}
		s := jul["Y-S"].Subsidy
		ptr(t, "Jul Basis", s.Basis, "3000000")         // 203101 T1, T6 not subsidy
		ptr(t, "Jul PrevBasis", s.PrevBasis, "3250000") // 203106 T1 + T74 (flag by code)
		if len(s.Rules) != 1 {
			t.Errorf("rules lost: %+v", s.Rules)
		}
		a := jul["Y-A"].Subsidy // no DETAIL of Y-A in 203101 (Y-A-OLD's is another NIK)
		ptr(t, "Y-A Basis", a.Basis, "")
		ptr(t, "Y-A PrevBasis", a.PrevBasis, "2000000")

		apr, _, err := load(203104, true, "Y-S") // latest DETAIL before April: 203103 (the ENGINE 203102 is older)
		if err != nil {
			t.Fatal(err)
		}
		ptr(t, "Apr PrevBasis", apr["Y-S"].Subsidy.PrevBasis, "3100000")

		jan, _, err := load(203101, true, "Y-S") // the basis period itself: no earlier DETAIL, current lines
		if err != nil {
			t.Fatal(err)
		}
		ptr(t, "Jan Basis", jan["Y-S"].Subsidy.Basis, "")
		ptr(t, "Jan PrevBasis", jan["Y-S"].Subsidy.PrevBasis, "")
	})
	t.Run("rule v1 P0 gate: frozen P0 row is not loaded", func(t *testing.T) {
		ins, _, err := load(203107, true, "Y-A", "Y-S")
		if err != nil {
			t.Fatal(err)
		}
		p0 := engine.CompKey{Type: engine.CompP, ID: 0}
		has := func(in engine.Input) bool {
			for _, c := range in.Components {
				if c.Key == p0 {
					return true
				}
			}
			return false
		}
		if !has(ins["Y-A"]) || has(ins["Y-S"]) {
			t.Errorf("P0 rows: Y-A %v, Y-S %v", has(ins["Y-A"]), has(ins["Y-S"]))
		}
		// Rule v1 never adds P0 itself (engine withSubsidyComponent), so no row means no P0 line.
	})
	t.Run("rule v2 lock", func(t *testing.T) {
		ins, _, err := load(203107, false, "Y-S")
		if err != nil {
			t.Fatal(err)
		}
		s := ins["Y-S"].Subsidy
		ptr(t, "Locked", s.Locked, "3100000") // ACTIVE ENGINE run; the SUPERSEDED lock is ignored
		if s.LockedYYMM != 203102 || s.Basis != nil || s.PrevBasis != nil {
			t.Errorf("v2 subsidy: %+v", s)
		}
	})
	t.Run("subsidy rules with two basis periods", func(t *testing.T) {
		err := app.ScopedTx(asScopes(t, "SOLO"), func(tx *sql.Tx) error {
			_, lerr := run.NewLoader().Load(ctx, tx, run.Query{CompanyID: 1, Division: "SOLO", YYMM: 203107, NIKs: []string{"Y-S"}},
				run.Rules{Set: stubRules{true}, Subsidy: subsidy(203101, 203102)})
			return lerr
		})
		if !errors.Is(err, run.ErrSubsidyBasisPeriods) {
			t.Fatalf("got %v", err)
		}
	})
}
