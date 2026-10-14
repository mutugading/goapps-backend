package audit

import (
	"strings"
	"testing"
)

func TestMaskJSON(t *testing.T) {
	got, err := MaskJSON(map[string]any{
		"nama": "Budi", "npwp": "12.345.678.9-012.000",
		"nested": map[string]any{"No_Rekening": "123", "gaji": "1000.00"},
		"list":   []any{map[string]any{"ktp": "3374"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	for _, leak := range []string{"12.345", "3374", `"123"`} {
		if strings.Contains(s, leak) {
			t.Fatalf("PII leaked (%s): %s", leak, s)
		}
	}
	for _, keep := range []string{"Budi", "1000.00"} {
		if !strings.Contains(s, keep) {
			t.Fatalf("non-PII %s lost: %s", keep, s)
		}
	}
}

func TestMaskJSON_Nil(t *testing.T) {
	if b, err := MaskJSON(nil); b != nil || err != nil {
		t.Fatalf("got %v %v, want nil nil", b, err)
	}
}
