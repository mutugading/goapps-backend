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

// T033a review: the env check is a whitelist, so spelling variants of production and unknown values are refused too.
func TestDummyRejectedForEnvVariants(t *testing.T) {
	for _, env := range []string{"Production", "PRODUCTION", "prod", "production ", "prd", "live"} {
		t.Setenv("APP_ENV", env)
		for name, run := range map[string]func([]string) error{"dummy-period": dummyPeriod, "dummy-janbase": dummyJanBase} {
			if err := run([]string{"-dsn", ""}); err == nil || !strings.Contains(err.Error(), "D-68") {
				t.Errorf("APP_ENV=%q %s: want D-68 rejection, got %v", env, name, err)
			}
		}
	}
	for _, env := range []string{"staging", "Staging", "local", "test"} {
		t.Setenv("APP_ENV", env)
		if err := dummyPeriod([]string{"-dsn", ""}); err == nil || strings.Contains(err.Error(), "D-68") {
			t.Errorf("APP_ENV=%q: want argument error (guard passes), got %v", env, err)
		}
	}
}
