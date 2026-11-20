package master

import "testing"

func TestNewCurrency(t *testing.T) {
	for _, tc := range []struct {
		name         string
		code         string
		minor, round int
		wantErrField string
	}{
		{"ok IDR", "IDR", 0, -2, ""},
		{"ok USD", "USD", 2, 2, ""},
		{"lowercase", "idr", 0, 0, "code"},
		{"too long", "IDRR", 0, 0, "code"},
		{"minor high", "USD", 5, 0, "minor_unit"},
		{"round low", "USD", 2, -7, "takehome_round"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := NewCurrency(tc.code, tc.minor, tc.round)
			checkField(t, err, tc.wantErrField)
			if err == nil && (c.Code != tc.code || c.MinorUnit != tc.minor || c.TakehomeRound != tc.round) {
				t.Fatalf("got %+v", c)
			}
		})
	}
}

func TestNewCompany(t *testing.T) {
	c, err := NewCompany(" MGT ", " Mutu Gading ", "")
	if err != nil || c.Code != "MGT" || c.Name != "Mutu Gading" || c.BaseCurrency != DefaultCurrency || !c.Active {
		t.Fatalf("default currency/trim: %+v %v", c, err)
	}
	for _, tc := range []struct{ name, code, nm, ccy, field string }{
		{"empty code", " ", "X", "", "code"},
		{"long code", "ABCDEFGHIJK", "X", "", "code"},
		{"empty name", "MGT", "", "", "name"},
		{"bad currency", "MGT", "X", "rp", "base_currency"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewCompany(tc.code, tc.nm, tc.ccy)
			checkField(t, err, tc.field)
		})
	}
}

func TestValidateCompanyNPWP(t *testing.T) {
	checkField(t, ValidateCompanyNPWP(""), "")
	checkField(t, ValidateCompanyNPWP("01.234.567.8-901.000"), "")
	checkField(t, ValidateCompanyNPWP(MaskedValue), "npwp")
	checkField(t, ValidateCompanyNPWP("0123456789012345678901234567890"), "npwp")
}

func TestNewDivision(t *testing.T) {
	d, err := NewDivision(1, " SOLO ", "")
	if err != nil || d.Code != "SOLO" || d.CompanyID != 1 {
		t.Fatalf("%+v %v", d, err)
	}
	checkField(t, errOf(NewDivision(0, "SOLO", "")), "company_id")
	checkField(t, errOf(NewDivision(1, "", "")), "code")
	checkField(t, errOf(NewDivision(1, "SOLO", string(make([]byte, 101)))), "name")
}

func errOf(_ Division, err error) error { return err }

func checkField(t *testing.T, err error, field string) {
	t.Helper()
	if field == "" {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}
	ve, ok := IsValidation(err)
	if !ok || ve.Field != field {
		t.Fatalf("want validation error on %q, got %v", field, err)
	}
}
