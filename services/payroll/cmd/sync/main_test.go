package main

import (
	"strings"
	"testing"
)

func TestDummyRejectedInProduction(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	for name, run := range map[string]func([]string) error{"dummy-period": dummyPeriod, "dummy-janbase": dummyJanBase} {
		err := run([]string{"-dsn", "postgres://invalid.invalid/none", "-from", "202610", "-to", "202611", "-division", "SOLO", "-yymm", "202701"})
		if err == nil || !strings.Contains(err.Error(), "D-68") {
			t.Fatalf("%s: want D-68 rejection in production, got %v", name, err)
		}
	}
}

func TestDummyAllowedInDevelopment(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	// No -dsn/-to/-yymm: the guard passes and the handler stops on its own argument check (no DB needed).
	for name, run := range map[string]func([]string) error{"dummy-period": dummyPeriod, "dummy-janbase": dummyJanBase} {
		err := run([]string{"-dsn", ""})
		if err == nil || strings.Contains(err.Error(), "D-68") {
			t.Fatalf("%s: want argument error (not D-68) in development, got %v", name, err)
		}
	}
}
