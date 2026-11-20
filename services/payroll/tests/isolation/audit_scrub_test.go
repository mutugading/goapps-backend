package isolation

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/audit"
)

// T033g5 (D-77 (2b)): the owner scrub script (scripts/fix/t033g2_audit_scrub.sql) re-implements audit.MaskJSON in SQL.
// TestScrubScriptAllowlist (package audit) keeps the key lists equal; this runs the script's own pg_temp.audit_mask and
// its allowlist on nested fixtures and requires the same result as maskTree, so the traversal (objects, arrays,
// key case, scalars, null) cannot drift either.
func TestScrubScriptAuditMaskMatchesMaskJSON(t *testing.T) {
	raw, err := os.ReadFile("../../scripts/fix/t033g2_audit_scrub.sql")
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	fi := strings.Index(s, "CREATE FUNCTION pg_temp.audit_mask")
	fj := strings.Index(s[fi:], "END $$;")
	if fi < 0 || fj < 0 {
		t.Fatal("audit_mask function not found in scrub script")
	}
	fn := s[fi : fi+fj+len("END $$;")]
	ai, aj := strings.Index(s, "-- ALLOWLIST BEGIN"), strings.Index(s, "-- ALLOWLIST END")
	if ai < 0 || aj < ai {
		t.Fatal("allowlist markers not found")
	}
	block := s[ai:aj]
	block = block[strings.Index(block, "\n"):]
	var allow []string
	for _, l := range strings.Split(block, "\n") {
		if k := strings.Index(l, "--"); k >= 0 {
			l = l[:k]
		}
		for _, m := range regexp.MustCompile(`'([^']*)'`).FindAllStringSubmatch(l, -1) {
			allow = append(allow, m[1])
		}
	}
	if len(allow) < 50 {
		t.Fatalf("allowlist parse looks wrong: %d keys", len(allow))
	}

	ctx := context.Background()
	conn, err := ownDB.Conn(ctx) // pg_temp objects live per session
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.ExecContext(ctx, fn); err != nil {
		t.Fatalf("create pg_temp.audit_mask: %v", err)
	}

	fixtures := map[string]any{
		"flat": map[string]any{"nik": "S1", "Email": "a@b.c", "NPWP": "12.345", "nama": "A", "BirthDate": "2000-01-01"},
		"denied key holds object": map[string]any{
			"before": map[string]any{"nik": "S1"},
			"after":  map[string]any{"Email": map[string]any{"nik": "S1", "deep": []any{1, 2}}},
		},
		"allowed key nests denied": map[string]any{
			"changes": []any{
				map[string]any{"nik": "S1", "Religion": "x", "amount": "100.50", "sex": "L"},
				map[string]any{"sts": "F", "BankAcntName": "Budi", "tags": []any{"a", map[string]any{"npwp": "1", "code": "C"}}},
			},
			"run": map[string]any{"id": 7, "frz": true, "alasan": nil, "rekening": "123"},
		},
		"scalars and empties": map[string]any{
			"yymm": 202609, "active": false, "name": nil, "tags": []any{}, "after": map[string]any{}, "KTP": []any{"1", "2"},
		},
		"mixed case allowed": map[string]any{"NIK": "S1", "Amount": "1", "ScopeDivisions": []any{"SOLO"}, "NoSuchKey": "x"},
		"top level array":    []any{map[string]any{"nik": "S1", "email": "e"}, "str", 3, nil},
		"top level scalar":   "plain",
	}
	for name, fx := range fixtures {
		want, err := audit.MaskJSON(fx)
		if err != nil {
			t.Fatalf("%s: MaskJSON: %v", name, err)
		}
		in, err := json.Marshal(fx)
		if err != nil {
			t.Fatal(err)
		}
		var got []byte
		if err := conn.QueryRowContext(ctx, `SELECT pg_temp.audit_mask($1::jsonb, $2::text[])::text`, string(in), allow).Scan(&got); err != nil {
			t.Fatalf("%s: audit_mask: %v", name, err)
		}
		var g, w any
		if err := json.Unmarshal(got, &g); err != nil {
			t.Fatalf("%s: decode sql result %s: %v", name, got, err)
		}
		if err := json.Unmarshal(want, &w); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(g, w) {
			t.Errorf("%s: scrub SQL differs from MaskJSON\n  sql: %s\n  go:  %s", name, got, want)
		}
	}
}
