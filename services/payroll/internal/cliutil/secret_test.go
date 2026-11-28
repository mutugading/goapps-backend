package cliutil

import (
	"bytes"
	"flag"
	"strings"
	"testing"
)

func TestSecretString_EnvDefaultNotPrintedAndArgvWarns(t *testing.T) {
	t.Setenv("T_SECRET_DSN", "postgres://u:hunter2@h/db")
	var warn, usage bytes.Buffer
	Warn = &warn
	t.Cleanup(func() { Warn = nil })
	fs := flag.NewFlagSet("x", flag.ContinueOnError)
	fs.SetOutput(&usage)
	dsn := fs.String("plain", "", "x")
	sec := SecretString(fs, "dsn", "DSN", "T_SECRET_DSN", "T_SECRET_FALLBACK")
	_ = dsn

	if err := fs.Parse(nil); err != nil || *sec != "postgres://u:hunter2@h/db" {
		t.Fatalf("env default: %q %v", *sec, err)
	}
	fs.PrintDefaults()
	if strings.Contains(usage.String(), "hunter2") {
		t.Fatalf("default leaked in usage: %s", usage.String())
	}
	if warn.Len() != 0 {
		t.Fatalf("no warning expected without argv: %s", warn.String())
	}

	if err := fs.Parse([]string{"-dsn", "postgres://u:other@h/db"}); err != nil || *sec != "postgres://u:other@h/db" {
		t.Fatalf("argv: %q %v", *sec, err)
	}
	if !strings.Contains(warn.String(), "T_SECRET_DSN") || strings.Contains(warn.String(), "other") {
		t.Fatalf("warning must name the env var and not echo the value: %q", warn.String())
	}
}

func TestSecretString_FallbackEnv(t *testing.T) {
	t.Setenv("T_PRIMARY", "")
	t.Setenv("T_LEGACY", "legacy-dsn")
	fs := flag.NewFlagSet("x", flag.ContinueOnError)
	got := SecretString(fs, "dsn", "DSN", "T_PRIMARY", "T_LEGACY")
	if err := fs.Parse(nil); err != nil || *got != "legacy-dsn" {
		t.Fatalf("fallback: %q %v", *got, err)
	}
}
