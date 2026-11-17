package main

import (
	"bytes"
	"testing"
)

func TestValidateCmd_UsageErrors(t *testing.T) {
	t.Setenv("PAYROLL_RO_DSN", "")
	for _, args := range [][]string{
		{},
		{"-yymm-from", "202601", "-yymm-to", "202612"},
		{"-yymm-from", "202601", "-dsn", "x"},
		{"-yymm-from", "202612", "-yymm-to", "202601", "-dsn", "postgres://x@127.0.0.1:1/x"},
		{"-bogus"},
	} {
		var out, errb bytes.Buffer
		if code := validateCmd(args, &out, &errb); code != exitError || out.Len() != 0 {
			t.Errorf("%v: exit %d out %q", args, code, out.String())
		}
	}
}
