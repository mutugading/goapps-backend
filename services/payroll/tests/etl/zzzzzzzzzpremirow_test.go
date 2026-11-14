package etl

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/etl"
)

// T009e9 (D-46): in an OPEN period, T40 attached (frz=2) without a DETAIL row ⇒ PF=1; with a row ⇒ D-33;
// without a component ⇒ NULL; periods not listed keep D-33 only.
func TestETL_AttLegacyFailMissingPremiRow(t *testing.T) {
	ctx := context.Background()
	if _, err := etl.RunAttLegacy(ctx, etlDB, etl.PSVDir{Dir: "testdata"}, etl.AttLegacyOptions{CompanyID: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := ownDB.Exec(`UPDATE payroll.pyr_att_snapshot_row r SET pasr_premi_fail=NULL, pasr_beras_fail=NULL FROM payroll.pyr_att_snapshot s
		WHERE s.pas_snapshot_id=r.pasr_snapshot_id AND s.pas_company_id=1 AND s.pas_yymm IN (202606,202607) AND s.pas_source_batch_ids='{}'`); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	files := map[string]string{
		etl.TableEmployeeComponent: "MPCE_NIK|MPCE_TYPE|MPCE_ID|MPCE_AMOUNT|MPCE_FRZ_FLAG|MPCE_DIVISION\n" +
			"2026-0001|Tunjangan|40|50000|2|SOLO\n" + // no DETAIL row
			"2026-0002|Tunjangan|40|50000||SOLO\n" + // DETAIL row with value
			"2026-0009|Tunjangan|40|50000|2|SOLO\n", // MAIN without REKAP
		etl.TableSalaryDetail: "PD_YYMM|PD_NIK|PD_NO|PD_TYPE|PD_AMOUNT|PD_STATUS|PD_DIVISION\n" +
			"202606|2026-0002|40|Tunjangan|50000|F|SOLO\n",
		etl.TableSalaryMain: "PM_YYMM|PM_NIK|PM_DIVISION\n" +
			"202606|2026-0001|SOLO\n202606|2026-0002|SOLO\n202606|2026-0003|JAKARTA\n202606|2026-0009|SOLO\n" +
			"202607|2026-0001|SOLO\n", // T40 without a row, but 202607 is not OPEN
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name+".psv"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	src := etl.PSVDir{Dir: dir}
	pf := func(yymm, div, nik string) string {
		return scalar(t, `SELECT coalesce(r.pasr_premi_fail::text,'NULL') FROM payroll.pyr_att_snapshot_row r JOIN payroll.pyr_att_snapshot s ON s.pas_snapshot_id=r.pasr_snapshot_id
			WHERE s.pas_company_id=1 AND s.pas_source_batch_ids='{}' AND r.pasr_kind='REKAP' AND s.pas_yymm=`+yymm+` AND s.pas_division='`+div+`' AND r.pasr_nik='`+nik+`'`)
	}
	// Without OpenPeriods: D-33 only (backward compatible).
	if _, err := etl.RunAttLegacyFail(ctx, etlDB, src, etl.AttLegacyFailOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := pf("202606", "SOLO", "2026-0001"); got != "NULL" {
		t.Fatalf("tanpa periode OPEN PF %s, want NULL", got)
	}
	o := etl.AttLegacyFailOptions{OpenPeriods: []string{"202606"}}
	rep, err := etl.RunAttLegacyFail(ctx, etlDB, src, o)
	if err != nil {
		t.Fatal(err)
	}
	if rep.PremiNoRow != 1 || rep.PremiFailed != 1 || rep.PremiSet != 2 || len(rep.NoSnapshotRow) != 1 ||
		rep.PremiNoRowBy["202606/SOLO"] != 1 || len(rep.PremiNoRowBy) != 1 {
		t.Errorf("laporan tak sesuai: %+v", rep)
	}
	for _, tc := range []struct{ yymm, div, nik, want string }{
		{"202606", "SOLO", "2026-0001", "1"},
		{"202606", "SOLO", "2026-0002", "0"},
		{"202606", "JAKARTA", "2026-0003", "NULL"},
		{"202607", "SOLO", "2026-0001", "NULL"},
	} {
		if got := pf(tc.yymm, tc.div, tc.nik); got != tc.want {
			t.Errorf("%s/%s %s: PF %s, want %s", tc.yymm, tc.div, tc.nik, got, tc.want)
		}
	}
	if _, err := etl.RunAttLegacyFail(ctx, etlDB, src, o); err != nil {
		t.Fatalf("run ulang: %v", err)
	}
	if got := pf("202606", "SOLO", "2026-0001"); got != "1" {
		t.Errorf("run ulang PF %s", got)
	}
	if _, err := etl.RunAttLegacyFail(ctx, etlDB, src, etl.AttLegacyFailOptions{OpenPeriods: []string{"2026"}}); err == nil {
		t.Error("periode OPEN tidak valid diterima")
	}
}
