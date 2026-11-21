package etl

import "testing"

// T033h (review #283 #6): NPWP that is not 15/16 digits or one repeated digit is stored NULL (no hash) + warning.
func TestCleanNPWP(t *testing.T) {
	for _, tc := range []struct {
		in, want string
		warn     bool
	}{
		{"", "", false},
		{"12.345.678.9-012.000", "12.345.678.9-012.000", false},
		{"987654321012000", "987654321012000", false},
		{"3372010101900001", "3372010101900001", false}, // 16 digit (NIK as NPWP)
		{"00.000.000.0-000.000", "", true},
		{"999999999999999", "", true},
		{"12.345.678", "", true},
		{"12345678901234567", "", true},
		{"-", "", false}, // no digits at all = no NPWP
	} {
		got, w := cleanNPWP(tc.in)
		if got != tc.want || (w != "") != tc.warn {
			t.Errorf("cleanNPWP(%q) = %q, warn %q; want %q, warn %v", tc.in, got, w, tc.want, tc.warn)
		}
	}
}
