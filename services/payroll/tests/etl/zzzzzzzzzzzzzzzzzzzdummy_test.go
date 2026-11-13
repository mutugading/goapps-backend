package etl

import (
	"context"
	"strings"
	"testing"

	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/etl"
)

// T029a (D-64): dummy-period clone (delta, no THR, rejects), purge (owner DSN) and dummy-janbase. Source 204010, targets 204011 (clone), 204012 APPROVED, 204009 CLOSED, 204008 attd_days non-dummy, 204101 janbase.
var dummySeeded bool

func seedDummySource(t *testing.T) {
	t.Helper()
	if dummySeeded {
		return
	}
	dummySeeded = true
	for _, q := range []string{
		`INSERT INTO payroll.pyr_component (pc_company_id, pc_code, pc_type, pc_name, pc_status, pc_valid_from) VALUES
		  (1,'T001','Tunjangan','Gapok','ACTIVE',202001), (1,'T002','Tunjangan','Jabatan','ACTIVE',202001),
		  (1,'T901','Tunjangan','JAN_BASE','ACTIVE',202001) ON CONFLICT (pc_company_id, pc_code) DO NOTHING`,
		`INSERT INTO payroll.pyr_employee (pe_company_id, pe_nik, pe_division, pe_nama, pe_level, pe_tax_id, pe_sts_payroll, pe_orion_sub_acnt) VALUES
		  (1,'DM-A','SOLO','A','Staff','TK',2,'DM-A'), (1,'DM-B','SOLO','B','Staff','TK',2,'DM-B') ON CONFLICT DO NOTHING`,
		`INSERT INTO payroll.pyr_employee_component (pec_company_id, pec_nik, pec_division, pec_component_sysid, pec_sts, pec_tax, pec_amount, pec_frz_flag)
		  SELECT 1, n.nik, 'SOLO', c.pc_sysid, 'F', 2, n.amt, 2 FROM payroll.pyr_component c
		  JOIN (VALUES ('DM-A','T001',3000000),('DM-A','T002',500000),('DM-B','T001',4000000)) n(nik, code, amt) ON n.code = c.pc_code WHERE c.pc_company_id = 1`,
		`INSERT INTO payroll.pyr_attd_days (pad_company_id, pad_yymm, pad_division, pad_days) VALUES (1,204010,'SOLO',26)`,
		`INSERT INTO payroll.pyr_att_snapshot (pas_company_id, pas_yymm, pas_division, pas_source_batch_ids, pas_checksum, pas_locked, created_by)
		  VALUES (1,204010,'SOLO','{7}','real',false,'etl')`,
		`INSERT INTO payroll.pyr_att_snapshot_row (pasr_snapshot_id, pasr_kind, pasr_nik, pasr_data, pasr_premi_fail, pasr_beras_fail)
		  SELECT pas_snapshot_id, 'REKAP', n, '{"MPAR_DAY_WORK":26}'::jsonb, 1, 0 FROM payroll.pyr_att_snapshot, (VALUES ('DM-A'),('DM-B')) v(n)
		  WHERE pas_yymm = 204010 AND pas_division = 'SOLO'`,
		`INSERT INTO payroll.pyr_addition (pa_company_id, pa_yymm, pa_nik, pa_division, pa_type, pa_id, pa_amount, created_by) VALUES
		  (1,204010,'DM-A','SOLO','Tunjangan',5,100,'x'), (1,204010,'DM-A','SOLO','Tunjangan',75,999,'x'),
		  (1,204010,'DM-A','SOLO','Potongan',75,999,'x'), (1,204010,'DM-B','SOLO','Potongan',7,999,'x'), (1,204010,'DM-B','SOLO','Potongan',9,50,'x')`,
	} {
		if _, err := ownDB.Exec(q); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}
}

func TestDummyPeriod(t *testing.T) {
	ctx := context.Background()
	seedDummySource(t)
	q := etl.DummyPeriodQuery{Company: 1, From: 204010, To: 204011, Division: "SOLO", WorkdaysDelta: -1}

	t.Run("clone, delta, no THR, idempotent", func(t *testing.T) {
		rep, err := etl.RunDummyPeriod(ctx, etlDB, q)
		if err != nil {
			t.Fatal(err)
		}
		if rep.Rows != 2 || rep.Additions != 2 || rep.AttdDays != 25 || rep.Unchanged {
			t.Fatalf("rep = %+v", rep)
		}
		if got := scalar(t, `SELECT pad_days::text FROM payroll.pyr_attd_days WHERE pad_yymm=204011`); got != "25" {
			t.Errorf("pad_days = %s", got)
		}
		if got := scalar(t, `SELECT created_by||pas_source_batch_ids::text FROM payroll.pyr_att_snapshot WHERE pas_yymm=204011`); got != "DUMMY-D64{-64}" {
			t.Errorf("marker = %s", got)
		}
		if got := scalar(t, `SELECT string_agg(pa_type::text||pa_id, ',' ORDER BY pa_type, pa_id) FROM payroll.pyr_addition WHERE pa_yymm=204011`); got != "Tunjangan5,Potongan9" {
			t.Errorf("additions = %s (THR T75/P75/P7 harus tidak ikut)", got)
		}
		if got := scalar(t, `SELECT count(*)::text FROM payroll.pyr_addition WHERE pa_yymm=204011 AND created_by <> 'DUMMY-D64'`); got != "0" {
			t.Errorf("addition tanpa marker: %s", got)
		}
		if got := scalar(t, `SELECT count(*)::text FROM payroll.pyr_att_snapshot_row r JOIN payroll.pyr_att_snapshot s ON s.pas_snapshot_id=r.pasr_snapshot_id WHERE s.pas_yymm=204011`); got != "2" {
			t.Errorf("rows = %s", got)
		}
		rep2, err := etl.RunDummyPeriod(ctx, etlDB, q)
		if err != nil || !rep2.Unchanged || rep2.SnapshotID != rep.SnapshotID || rep2.Additions != 0 {
			t.Fatalf("run kedua: %+v %v", rep2, err)
		}
		if got := scalar(t, `SELECT count(*)::text FROM payroll.pyr_att_snapshot WHERE pas_yymm=204011`); got != "1" {
			t.Errorf("snapshot ganda: %s", got)
		}
	})

	t.Run("rejects real, APPROVED, CLOSED, bad input", func(t *testing.T) {
		bad := q
		bad.To = 204010 // target = real period with a non-dummy snapshot
		bad.From = 204011
		if _, err := etl.RunDummyPeriod(ctx, etlDB, bad); err == nil || !strings.Contains(err.Error(), "non-dummy") {
			t.Fatalf("periode riil harus ditolak: %v", err)
		}
		if got := scalar(t, `SELECT count(*)::text FROM payroll.pyr_att_snapshot WHERE pas_yymm=204010`); got != "1" {
			t.Fatalf("periode riil berubah: %s", got)
		}
		for _, st := range []string{"APPROVED", "CLOSED"} {
			ym := map[string]int{"APPROVED": 204012, "CLOSED": 204009}[st]
			if _, err := ownDB.Exec(`INSERT INTO payroll.pyr_period (pp_company_id, pp_yymm, pp_division, pp_date_from, pp_date_to, pp_status)
			  VALUES (1,$1,'SOLO','2040-01-01','2040-01-31',$2::payroll.period_status)`, ym, st); err != nil {
				t.Fatal(err)
			}
			c := q
			c.To = ym
			if _, err := etl.RunDummyPeriod(ctx, etlDB, c); err == nil || !strings.Contains(err.Error(), st) {
				t.Errorf("%s harus ditolak: %v", st, err)
			}
		}
		c := q
		c.To = 204008 // pyr_attd_days non-dummy without a dummy snapshot
		if _, err := ownDB.Exec(`INSERT INTO payroll.pyr_attd_days (pad_company_id, pad_yymm, pad_division, pad_days) VALUES (1,204008,'SOLO',20)`); err != nil {
			t.Fatal(err)
		}
		if _, err := etl.RunDummyPeriod(ctx, etlDB, c); err == nil || !strings.Contains(err.Error(), "attd_days") {
			t.Errorf("attd_days non-dummy harus ditolak: %v", err)
		}
		for _, c := range []etl.DummyPeriodQuery{
			{Company: 1, From: 204010, To: 204010, Division: "SOLO"}, {Company: 1, From: 204010, To: 204005, Division: ""},
			{Company: 1, From: 204099, To: 204005, Division: "SOLO"}, {Company: 1, From: 204011, To: 204004, Division: "SOLO", WorkdaysDelta: -99},
		} {
			if _, err := etl.RunDummyPeriod(ctx, etlDB, c); err == nil {
				t.Errorf("harus ditolak: %+v", c)
			}
		}
	})

	t.Run("no snapshot in source", func(t *testing.T) {
		c := q
		c.From, c.To = 204006, 204007
		if _, err := etl.RunDummyPeriod(ctx, etlDB, c); err == nil || !strings.Contains(err.Error(), "snapshot") {
			t.Fatalf("sumber tanpa snapshot: %v", err)
		}
	})

	t.Run("etl role has no DELETE; purge needs owner and touches dummy only", func(t *testing.T) {
		if _, err := etl.RunDummyPurge(ctx, etlDB, 1, 204011, "SOLO"); err == nil {
			t.Fatal("payroll_etl tidak boleh bisa purge")
		}
		if _, err := etl.RunDummyPurge(ctx, ownDB, 1, 204010, "SOLO"); err == nil || !strings.Contains(err.Error(), "non-dummy") {
			t.Fatalf("purge periode riil harus ditolak: %v", err)
		}
		// real addition in the dummy period stays (marker differs)
		if _, err := ownDB.Exec(`INSERT INTO payroll.pyr_addition (pa_company_id, pa_yymm, pa_nik, pa_division, pa_type, pa_id, pa_amount, created_by) VALUES (1,204011,'DM-B','SOLO','Tunjangan',6,1,'user')`); err != nil {
			t.Fatal(err)
		}
		rep, err := etl.RunDummyPurge(ctx, ownDB, 1, 204011, "SOLO")
		if err != nil {
			t.Fatal(err)
		}
		if rep.Rows != 2 || rep.Additions != 2 || rep.AttdDays != 1 {
			t.Fatalf("purge rep = %+v", rep)
		}
		if got := scalar(t, `SELECT (SELECT count(*) FROM payroll.pyr_att_snapshot WHERE pas_yymm=204011)||'/'||(SELECT count(*) FROM payroll.pyr_attd_days WHERE pad_yymm=204011)||'/'||
		  (SELECT count(*) FROM payroll.pyr_addition WHERE pa_yymm=204011)||'/'||(SELECT count(*) FROM payroll.pyr_att_snapshot WHERE pas_yymm=204010)||'/'||
		  (SELECT count(*) FROM payroll.pyr_addition WHERE pa_yymm=204010)`); got != "0/0/1/1/5" {
			t.Errorf("sisa snapshot/attd/add(204011)/snapshot/add(riil) = %s", got)
		}
		if rep2, err := etl.RunDummyPurge(ctx, ownDB, 1, 204011, "SOLO"); err != nil || rep2.SnapshotID != 0 {
			t.Errorf("purge ulang harus no-op: %+v %v", rep2, err)
		}
		if got := scalar(t, `SELECT pad_days::text FROM payroll.pyr_attd_days WHERE pad_yymm=204008`); got != "20" {
			t.Errorf("attd_days periode lain tersentuh: %s", got)
		}
	})
}

func TestDummyJanBase(t *testing.T) {
	ctx := context.Background()
	seedDummySource(t)
	const jan, dec = 204101, 204012
	t.Cleanup(func() { // the suite shares one DB: other employees (T001) also got a dummy T901
		_, _ = ownDB.Exec(`DELETE FROM payroll.pyr_employee_component WHERE pec_component_sysid IN (SELECT pc_sysid FROM payroll.pyr_component WHERE pc_code='T901')`)
	})
	sum := func() string {
		return scalar(t, `SELECT coalesce(string_agg(e.pec_nik||':'||e.pec_amount::numeric(18,0)||':'||e.pec_valid_from||'-'||e.pec_valid_to, ',' ORDER BY e.pec_nik), '')
		  FROM payroll.pyr_employee_component e JOIN payroll.pyr_component c ON c.pc_sysid=e.pec_component_sysid WHERE c.pc_code='T901' AND e.pec_nik LIKE 'DM-%'`)
	}
	rep, err := etl.RunDummyJanBase(ctx, etlDB, 1, jan)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Source != dec || rep.Written < 2 {
		t.Fatalf("rep = %+v", rep)
	}
	want := "DM-A:3500000:204101-204101,DM-B:4000000:204101-204101"
	if got := sum(); got != want {
		t.Fatalf("T901 = %s, want %s", got, want)
	}
	if _, err := etl.RunDummyJanBase(ctx, etlDB, 1, jan); err != nil || sum() != want {
		t.Fatalf("tidak idempoten: %v %s", err, sum())
	}
	if _, err := ownDB.Exec(`UPDATE payroll.pyr_employee_component SET pec_amount = 5000000 WHERE pec_nik='DM-A' AND pec_component_sysid IN (SELECT pc_sysid FROM payroll.pyr_component WHERE pc_code='T001')`); err != nil {
		t.Fatal(err)
	}
	if _, err = etl.RunDummyJanBase(ctx, etlDB, 1, jan); err != nil || !strings.Contains(sum(), "DM-A:5500000") {
		t.Fatalf("run ulang tidak memperbarui: %v %s", err, sum())
	}
	if _, err := ownDB.Exec(`UPDATE payroll.pyr_employee_component SET updated_by='ETL' WHERE pec_nik='DM-B' AND pec_component_sysid IN (SELECT pc_sysid FROM payroll.pyr_component WHERE pc_code='T901')`); err != nil {
		t.Fatal(err)
	}
	if _, err := etl.RunDummyJanBase(ctx, etlDB, 1, jan); err == nil || !strings.Contains(err.Error(), "non-dummy") {
		t.Fatalf("T901 non-dummy harus menolak: %v", err)
	}
	if _, err := etl.RunDummyJanBase(ctx, etlDB, 1, 99); err == nil {
		t.Fatal("yymm tidak valid harus ditolak")
	}
}
