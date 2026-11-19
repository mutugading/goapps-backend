package etl

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/recon"
)

// T032a: `recon validate` aggregates (D-72, 08§6 Postgres side) as payroll_ro, against hand-computed sums.
// Range 204401..204402 holds only this test's LEGACY runs; employees use the unique level VALID_LVL.
func TestValidate_Aggregates(t *testing.T) {
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := ownDB.Exec(q, args...); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}
	exec(`INSERT INTO payroll.pyr_component (pc_company_id, pc_code, pc_type, pc_name, pc_status, pc_valid_from)
	 VALUES (1,'T001','Tunjangan','Gaji Pokok','ACTIVE',202001),(1,'P001','Potongan','BPJS','ACTIVE',202001)
	 ON CONFLICT (pc_company_id, pc_code) DO NOTHING`)
	// (Tunjangan,1,SOLO) and (Potongan,1,SOLO) are mapped (ids 9001/9002 never are).
	exec(`INSERT INTO payroll.pyr_component_legacy_map (pclm_company_id, pclm_legacy_type, pclm_legacy_id, pclm_legacy_division, pclm_sysid)
	 SELECT 1, c.pc_type, 1, 'SOLO', c.pc_sysid FROM payroll.pyr_component c WHERE c.pc_company_id = 1 AND c.pc_code IN ('T001','P001')
	 ON CONFLICT DO NOTHING`)
	exec(`INSERT INTO payroll.pyr_employee (pe_company_id, pe_nik, pe_division, pe_nama, pe_level, pe_sts_payroll, pe_orion_sub_acnt, pe_birth_date)
	 VALUES (1,'V00001','SOLO','V1','VALID_LVL',2,'V1','1990-01-02'),(1,'V00002','SOLO','V2','VALID_LVL',2,'V2',NULL),
	        (1,'V00003','JAKARTA','V3','VALID_LVL',2,'V3',NULL),(1,'V00004','SOLO','V4','VALID_LVL',1,'V4',NULL)`)
	for _, div := range []string{"SOLO", "JAKARTA"} {
		for _, y := range []string{"204401", "204402"} {
			exec(`INSERT INTO payroll.pyr_period (pp_company_id, pp_yymm, pp_division, pp_date_from, pp_date_to, pp_status)
			 VALUES (1,` + y + `,'` + div + `','2044-01-26','2044-02-25','OPEN') ON CONFLICT DO NOTHING`)
		}
	}
	mkRun := func(yymm, div string) int64 {
		var id int64
		if err := ownDB.QueryRow(`INSERT INTO payroll.pyr_run (pr_company_id, pr_yymm, pr_division, pr_pay_date, pr_rule_set_id, pr_status, pr_kind)
		 VALUES (1,` + yymm + `,'` + div + `','2044-02-25',0,'ACTIVE','LEGACY') RETURNING pr_id`).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	solo1, jkt1, solo2 := mkRun("204401", "SOLO"), mkRun("204401", "JAKARTA"), mkRun("204402", "SOLO")
	line := func(run int64, nik, typ string, id int, amt string) {
		exec(`INSERT INTO payroll.pyr_salary_detail (psd_run_id, psd_nik, psd_type, psd_id, psd_amount) VALUES ($1,$2,$3,$4,$5::numeric)`, run, nik, typ, id, amt)
	}
	head := func(run int64, nik, yymm, div, tunj, pot string) {
		exec(`INSERT INTO payroll.pyr_salary_head (psh_run_id, psh_nik, psh_yymm, psh_division, psh_tot_tunjangan, psh_tot_potongan, psh_rounding, psh_company_id)
		 VALUES ($1,$2,`+yymm+`,'`+div+`',$3::numeric,$4::numeric,0,1)`, run, nik, tunj, pot)
	}
	line(solo1, "V00001", "Tunjangan", 1, "1000.10")
	line(solo1, "V00001", "Tunjangan", 9002, "0.20") // unmapped (T,9002,SOLO)
	line(solo1, "V00001", "Potongan", 1, "200.05")
	line(solo1, "V00002", "Tunjangan", 1, "300.00")
	head(solo1, "V00001", "204401", "SOLO", "1000.30", "200.05")
	head(solo1, "V00002", "204401", "SOLO", "300.00", "0")
	line(jkt1, "V00003", "Tunjangan", 9001, "500.00") // unmapped (T,9001,JAKARTA)
	head(jkt1, "V00003", "204401", "JAKARTA", "500.00", "0")
	line(solo2, "V00001", "Tunjangan", 1, "0.01")
	head(solo2, "V00001", "204402", "SOLO", "0.01", "0")

	raw, err := sql.Open("pgx", strings.Replace(appDSN, "user=payroll_app password=payroll_app123", "user=payroll_recon password=payroll_recon123", 1))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	q := recon.Query{CompanyID: 1, YYMMFrom: 204401, YYMMTo: 204402}

	rep, err := recon.Validate(context.Background(), raw, q)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Divisions) != 2 || len(rep.Periods) != 3 {
		t.Fatalf("divisions %v periods %+v", rep.Divisions, rep.Periods)
	}
	want := map[string][7]string{ // DetailT, DetailP, lines, head tunj, head pot, head net, head rows
		"204401/JAKARTA": {"500.00", "0", "1", "500.00", "0", "500.00", "1"},
		"204401/SOLO":    {"1300.30", "200.05", "4", "1300.30", "200.05", "1100.25", "2"},
		"204402/SOLO":    {"0.01", "0", "1", "0.01", "0", "0.01", "1"},
	}
	for _, p := range rep.Periods {
		w, ok := want[fmt.Sprintf("%d/%s", p.YYMM, p.Division)]
		if !ok {
			t.Fatalf("unexpected period %+v", p)
		}
		got := [7]string{p.DetailT.String(), p.DetailP.String(), fmt.Sprint(p.DetailLines), p.HeadTunjangan.String(), p.HeadPotongan.String(), p.HeadNet.String(), fmt.Sprint(p.HeadRows)}
		for i := range w {
			if i == 2 || i == 6 {
				if got[i] != w[i] {
					t.Errorf("%d/%s col %d = %s, want %s", p.YYMM, p.Division, i, got[i], w[i])
				}
			} else if !dec(got[i]).Equal(dec(w[i])) {
				t.Errorf("%d/%s col %d = %s, want %s", p.YYMM, p.Division, i, got[i], w[i])
			}
		}
	}
	if !rep.Totals.DetailT.Equal(dec("1800.31")) || !rep.Totals.HeadNet.Equal(dec("1600.26")) {
		t.Errorf("totals %+v", rep.Totals)
	}
	// Unmapped: (T,9002,SOLO) 1 line + (T,9001,JAKARTA) 1 line; mapped lines are not reported.
	if rep.UnmappedLegacyLines != 2 || len(rep.UnmappedComponents) != 2 || rep.OK() {
		t.Fatalf("unmapped %d %+v failures %v", rep.UnmappedLegacyLines, rep.UnmappedComponents, rep.Failures)
	}
	if len(rep.PlaintextPII) != 0 {
		t.Fatalf("plaintext PII on clean schema: %+v", rep.PlaintextPII)
	}
	// Active per level: V00004 is inactive (sts 1).
	var solo, jkt int64 = -1, -1
	for _, l := range rep.ActiveByLevel {
		if l.Level == "VALID_LVL" && l.Division == "SOLO" {
			solo = l.Active
		}
		if l.Level == "VALID_LVL" && l.Division == "JAKARTA" {
			jkt = l.Active
		}
	}
	if solo != 2 || jkt != 1 {
		t.Errorf("active VALID_LVL solo=%d jakarta=%d, want 2/1 (%+v)", solo, jkt, rep.ActiveByLevel)
	}
	for _, b := range rep.BirthDate {
		if b.Division == "SOLO" && (b.Missing < 2 || b.MissingActive < 1) {
			t.Errorf("birth_date SOLO %+v", b)
		}
	}
	// D-36: alias ids 100 (T, to T70) and 201 (P, to P71) count as mapped only when T70/P71 have a map row.
	line(solo2, "V00001", "Tunjangan", 100, "7.00")
	line(solo2, "V00001", "Potongan", 201, "3.00")
	exec(`UPDATE payroll.pyr_salary_head SET psh_tot_tunjangan = 7.01, psh_tot_potongan = 3.00 WHERE psh_run_id = $1`, solo2)
	rep, err = recon.Validate(context.Background(), raw, q)
	if err != nil || rep.UnmappedLegacyLines != 4 || len(rep.UnmappedComponents) != 4 {
		t.Fatalf("alias without target map: err %v %+v", err, rep.UnmappedComponents)
	}
	exec(`INSERT INTO payroll.pyr_component_legacy_map (pclm_company_id, pclm_legacy_type, pclm_legacy_id, pclm_legacy_division, pclm_sysid)
	 SELECT 1, 'Tunjangan'::payroll.comp_type, 70, 'SOLO', pc_sysid FROM payroll.pyr_component WHERE pc_company_id = 1 AND pc_code = 'T001'
	 UNION ALL SELECT 1, 'Potongan'::payroll.comp_type, 71, 'SOLO', pc_sysid FROM payroll.pyr_component WHERE pc_company_id = 1 AND pc_code = 'P001'`)
	rep, err = recon.Validate(context.Background(), raw, q)
	if err != nil || rep.UnmappedLegacyLines != 2 || len(rep.AliasedD36) != 2 || rep.AliasedD36[0].Lines != 1 {
		t.Fatalf("alias with target map: err %v unmapped %+v aliased %+v", err, rep.UnmappedComponents, rep.AliasedD36)
	}
	// Mapping the two missing components clears the failure.
	exec(`INSERT INTO payroll.pyr_component_legacy_map (pclm_company_id, pclm_legacy_type, pclm_legacy_id, pclm_legacy_division, pclm_sysid)
	 SELECT 1, 'Tunjangan'::payroll.comp_type, 9002, 'SOLO', pc_sysid FROM payroll.pyr_component WHERE pc_company_id = 1 AND pc_code = 'T001'
	 UNION ALL SELECT 1, 'Tunjangan'::payroll.comp_type, 9001, 'JAKARTA', pc_sysid FROM payroll.pyr_component WHERE pc_company_id = 1 AND pc_code = 'T001'`)
	if rep, err = recon.Validate(context.Background(), raw, q); err != nil || !rep.OK() || rep.UnmappedLegacyLines != 0 {
		t.Fatalf("after mapping: err %v ok %v unmapped %d", err, rep.OK(), rep.UnmappedLegacyLines)
	}
	// A plaintext PII column (varchar KTP number) is detected; the ciphertext/hash/name columns are not.
	exec(`ALTER TABLE payroll.pyr_employee ADD COLUMN pe_ktp_no varchar(20)`)
	defer func() { _, _ = ownDB.Exec(`ALTER TABLE payroll.pyr_employee DROP COLUMN pe_ktp_no`) }() //nolint:errcheck // cleanup
	rep, err = recon.Validate(context.Background(), raw, q)
	if err != nil || rep.OK() || len(rep.PlaintextPII) != 1 || rep.PlaintextPII[0].Column != "pe_ktp_no" {
		t.Fatalf("plaintext: err %v ok %v %+v", err, rep.OK(), rep.PlaintextPII)
	}
}
