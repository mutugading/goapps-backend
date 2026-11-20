package audit

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	masterapp "github.com/mutugading/goapps-backend/services/payroll/internal/application/master"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/component"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/master"
)

func TestMaskJSON(t *testing.T) {
	got, err := MaskJSON(map[string]any{
		"nama": "Budi", "npwp": "12.345.678.9-012.000",
		"changes": map[string]any{"No_Rekening": "123", "amount": "1000.00"},
		"tags":    []any{map[string]any{"ktp": "3374"}},
		"unknown": "rahasia",
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	for _, leak := range []string{"12.345", "3374", `"123"`, "rahasia"} {
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

// Review PR #283: personal data of an employee never reaches pyr_audit_log.
func TestMaskJSON_EmployeePersonalData(t *testing.T) {
	e := master.Employee{ID: 7, NIK: "S1", Nama: "Budi", Email: "budi@x.test", BirthDate: "1990-01-02",
		BirthPlace: "Solo", Religion: "ISLAM", Sex: "L", BankAcntName: "BUDI S", NPWP: "12.345", KTP: "3374",
		BankAcnt: "5550001111", Dept: "PRD"}
	got, err := MaskJSON(e)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(got, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"Email", "BirthDate", "BirthPlace", "Religion", "Sex", "BankAcntName", "npwp", "ktp", "bank_acct"} {
		if m[k] != masked {
			t.Errorf("%s = %v, want %s", k, m[k], masked)
		}
	}
	for k, want := range map[string]any{"NIK": "S1", "Nama": "Budi", "Dept": "PRD"} {
		if m[k] != want {
			t.Errorf("%s = %v, want %v", k, m[k], want)
		}
	}
}

// Every field of an audited struct must be classified (allowed or denied), so a new
// field is a conscious decision instead of a silent "***" or a silent leak.
func TestAuditedStructFieldsClassified(t *testing.T) {
	for _, v := range []any{master.Currency{}, master.Company{}, master.Division{}, master.Employee{},
		component.Component{}, component.TemplateItem{}, component.Attachment{}, component.Change{},
		masterapp.LevelChange{}} {
		rt := reflect.TypeOf(v)
		for i := range rt.NumField() {
			f := rt.Field(i)
			key := f.Name
			if tag := strings.Split(f.Tag.Get("json"), ",")[0]; tag != "" {
				key = tag
			}
			k := strings.ToLower(key)
			_, allowed := allowedKeys[k]
			_, denied := deniedKeys[k]
			switch {
			case allowed && denied:
				t.Errorf("%s.%s is both allowed and denied", rt.Name(), f.Name)
			case !allowed && !denied:
				t.Errorf("%s.%s (%q) is not classified: add it to allowedKeys or deniedKeys in mask.go", rt.Name(), f.Name, key)
			}
		}
	}
}

func TestMaskJSON_KeepsLargeInt(t *testing.T) {
	got, err := MaskJSON(map[string]any{"id": int64(9007199254740993)})
	if err != nil || string(got) != `{"id":9007199254740993}` {
		t.Fatalf("got %s %v", got, err)
	}
}
