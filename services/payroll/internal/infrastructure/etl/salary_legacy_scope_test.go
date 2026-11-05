package etl

import "testing"

// D-35.1 as revised by D-43: only JAKARTA before 202601 is out of scope; SOLO 202607 is loaded (synthetic MAIN, D-35.2).
func TestLegacySkipped_Scope(t *testing.T) {
	cases := []struct {
		yymm, division string
		want           bool
	}{
		{"202512", "JAKARTA", true},
		{"201712", "JAKARTA", true},
		{"202601", "JAKARTA", false},
		{"202607", "JAKARTA", false},
		{"202607", "SOLO", false}, // D-43: no longer skipped
		{"202512", "SOLO", false},
		{"201712", "SOLO", false},
	}
	for _, c := range cases {
		if got := legacySkipped(snapKey{c.yymm, c.division}); got != c.want {
			t.Errorf("legacySkipped(%s/%s) = %v, want %v", c.yymm, c.division, got, c.want)
		}
	}
}
