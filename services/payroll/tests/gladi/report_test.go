package gladi

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/run"
)

// report collects the rehearsal outcome: summary.txt (aggregates, no NIK), detail.txt (per NIK, stays in /tmp) and hand.txt
// (3 NIK worked out by hand from the independent SQL).
type report struct {
	summary, detail, hand []string
}

func newReport() *report { return &report{} }

func (r *report) addf(f string, a ...any) { r.summary = append(r.summary, fmt.Sprintf(f, a...)) }

func (r *report) detailf(f string, a ...any) { r.detail = append(r.detail, fmt.Sprintf(f, a...)) }

func (r *report) handf(f string, a ...any) { r.hand = append(r.hand, fmt.Sprintf(f, a...)) }

// warnings keeps the count per code in the summary and the NIK in the detail.
func (r *report) warnings(yymm int, div string, ws []run.Issue) {
	n := map[string]int{}
	for _, w := range ws {
		n[w.Code]++
		r.detailf("WARN %d %s %s %s: %s", yymm, div, w.Code, w.NIK, w.Message)
	}
	for _, c := range sortedKeys(n) {
		r.addf("  warning %d %s %s: %d", yymm, div, c, n[c])
	}
}

func (r *report) write(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	for name, ls := range map[string][]string{"summary.txt": r.summary, "detail.txt": r.detail, "hand.txt": r.hand} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(strings.Join(ls, "\n")+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, l := range r.summary {
		t.Log(l)
	}
	t.Logf("laporan: %s (summary/detail/hand)", dir)
}
