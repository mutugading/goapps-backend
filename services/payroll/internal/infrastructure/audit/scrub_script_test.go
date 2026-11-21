package audit

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestScrubScriptAllowlist keeps the allowlist copied into scripts/fix/t033g2_audit_scrub.sql (D-77 (2b)) equal
// to allowedKeys: the one-off scrub of old audit rows must mask exactly what MaskJSON masks today.
func TestScrubScriptAllowlist(t *testing.T) {
	body, err := os.ReadFile("../../../scripts/fix/t033g2_audit_scrub.sql")
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	i, j := strings.Index(s, "-- ALLOWLIST BEGIN"), strings.Index(s, "-- ALLOWLIST END")
	if i < 0 || j < i {
		t.Fatal("allowlist markers not found")
	}
	block := s[i:j]
	block = block[strings.Index(block, "\n"):] // skip the BEGIN comment line
	var lines []string
	for _, l := range strings.Split(block, "\n") {
		if k := strings.Index(l, "--"); k >= 0 {
			l = l[:k]
		}
		lines = append(lines, l)
	}
	got := map[string]struct{}{}
	for _, m := range regexp.MustCompile(`'([^']*)'`).FindAllStringSubmatch(strings.Join(lines, "\n"), -1) {
		got[m[1]] = struct{}{}
	}
	var missing, extra []string
	for k := range allowedKeys {
		if _, ok := got[k]; !ok {
			missing = append(missing, k)
		}
	}
	for k := range got {
		if _, ok := allowedKeys[k]; !ok {
			extra = append(extra, k)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) != 0 || len(extra) != 0 {
		t.Fatalf("scrub script allowlist out of sync with mask.go: missing %v, extra %v", missing, extra)
	}
}
