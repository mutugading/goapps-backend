package run

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func decodeRaw(t *testing.T, s string) map[string]any {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var raw map[string]any
	if err := dec.Decode(&raw); err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestRecapColumns_NameColumn_Skipped(t *testing.T) {
	tests := []struct {
		name, data, prefix string
		want               map[string]string
		wantErr            bool
	}{
		{"rekap name skipped", `{"MPAR_NAMA":"Budi","MPAR_DAY_WORK":"25","MPAR_LEMBUR_1":3.5,"MPAR_X":null}`,
			"MPAR_", map[string]string{"day_work": "25", "lembur_1": "3.5"}, false},
		{"add name skipped", `{"MPARA_NAMA":"Budi","MPARA_SHIFT_ADD":2}`, "MPARA_", map[string]string{"shift_add": "2"}, false},
		{"numeric name column skipped too", `{"MPAR_NAMA":123,"MPAR_DAY_WORK":20}`, "MPAR_", map[string]string{"day_work": "20"}, false},
		{"NAMA without prefix ignored", `{"NAMA":"Budi","MPAR_DAY_WORK":20}`, "MPAR_", map[string]string{"day_work": "20"}, false},
		{"other text still rejected", `{"MPAR_NAMA":"Budi","MPAR_DAY_WORK":"dua"}`, "MPAR_", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := recapColumns(decodeRaw(t, tt.data), tt.prefix)
			if tt.wantErr {
				if !errors.Is(err, ErrBadSnapshotValue) {
					t.Fatalf("want ErrBadSnapshotValue, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for k, v := range tt.want {
				if d, ok := got[k]; !ok || d.String() != v {
					t.Fatalf("%s: got %v, want %s", k, got[k], v)
				}
			}
		})
	}
}
