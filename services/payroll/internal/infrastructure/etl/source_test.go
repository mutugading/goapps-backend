package etl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPSVDirRepairsNonUTF8(t *testing.T) {
	dir := t.TempDir()
	body := "A|B\nx|a\xa0b@mail.com\ny|Caf\xc3\xa9\n"
	if err := os.WriteFile(filepath.Join(dir, "T.psv"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	tb, err := PSVDir{Dir: dir}.Read("T")
	if err != nil {
		t.Fatal(err)
	}
	if got := tb.Rows[0][1]; got != "a b@mail.com" {
		t.Errorf("row0 = %q", got)
	}
	if got := tb.Rows[1][1]; got != "Café" {
		t.Errorf("valid UTF-8 must be kept, got %q", got)
	}
	if len(tb.Notes) != 1 || !strings.Contains(tb.Notes[0], "baris 2") {
		t.Errorf("notes = %v", tb.Notes)
	}
}

func TestHrisMapDuplicateNIK(t *testing.T) {
	tb := &Table{Name: "HM_EMP_DATA", Cols: []string{"HMEMD_NIK", "HMEMD_SYS_ID"}, Rows: [][]string{
		{"2461", "A"}, {"1", "S1"}, {"2461", "B"}, {"1", "S1"},
	}}
	m, warns, err := hrisMap(tb)
	if err != nil {
		t.Fatal(err)
	}
	if m["2461"] != "B" || m["1"] != "S1" {
		t.Errorf("map = %v", m)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "2461") || !strings.Contains(warns[0], "A, B") {
		t.Errorf("warns = %v", warns)
	}
}
