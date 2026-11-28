package cliutil

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestConfirm(t *testing.T) {
	for _, tc := range []struct {
		name, in string
		yes      bool
		wantErr  bool
	}{
		{"yes flag skips prompt", "", true, false},
		{"typed token", "42\n", false, false},
		{"typed token with spaces", "  42 \n", false, false},
		{"wrong token", "43\n", false, true},
		{"empty line", "\n", false, true},
		{"eof (non-interactive)", "", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			err := Confirm(strings.NewReader(tc.in), &out, tc.yes, "kirim batch 42", "42")
			if tc.wantErr != (err != nil) || (err != nil && !errors.Is(err, ErrNotConfirmed)) {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
