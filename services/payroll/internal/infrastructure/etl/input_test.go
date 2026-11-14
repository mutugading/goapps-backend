package etl

import "testing"

func TestBadYYMM(t *testing.T) {
	cols := []string{"MPA_YYMM", "MPA_NIK"}
	idx := map[string]int{"MPA_YYMM": 0, "MPA_NIK": 1}
	for v, wantBad := range map[string]bool{
		"202508": false, "200001": false, "209912": false,
		"2027": true, "32026": true, "20203": true, "202513": true, "202500": true, "": true, "abc": true,
	} {
		if got := badYYMM([]string{v, "x"}, idx, cols) != ""; got != wantBad {
			t.Errorf("badYYMM(%q) bad=%v, want %v", v, got, wantBad)
		}
	}
	// kolom tanpa suffix _YYMM tidak diperiksa
	if badYYMM([]string{"zzz"}, map[string]int{"MPA_NIK": 0}, []string{"MPA_NIK"}) != "" {
		t.Error("kolom non-YYMM tidak boleh diperiksa")
	}
}
