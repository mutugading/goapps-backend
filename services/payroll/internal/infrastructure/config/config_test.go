package config

import (
	"strings"
	"testing"
)

func TestValidate_JWTSecret(t *testing.T) {
	long := strings.Repeat("k", minJWTSecretLen)
	tests := []struct {
		name, env, secret string
		wantErr           bool
	}{
		{"empty always rejected", "development", "", true},
		{"sample ok in development", "development", "change-this-in-production", false},
		{"sample rejected in production", "production", "change-this-in-production", true},
		{"compose sample rejected in staging", "staging", "dev-access-secret-change-in-production", true},
		{"short rejected in production", "production", "short", true},
		{"strong ok in production", "production", long, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{App: AppConfig{Env: tt.env}, JWT: JWTConfig{AccessTokenSecret: tt.secret}, PII: PIIConfig{EncKey: long, HMACKey: long + "h"}}
			if err := cfg.Validate(); (err != nil) != tt.wantErr {
				t.Fatalf("Validate() err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidate_PIIKeys(t *testing.T) {
	long := strings.Repeat("p", minJWTSecretLen)
	long2 := strings.Repeat("q", minJWTSecretLen)
	devEnc, devHMAC := "dev-pii-enc-key-"+placeholderPIIMarker, "dev-pii-hmac-key-"+placeholderPIIMarker
	tests := []struct {
		name, env, enc, hmac string
		wantErr              bool
	}{
		{"missing enc key rejected", "development", "", long, true},
		{"missing hmac key rejected", "development", long, "", true},
		{"short ok in development", "development", "dev", "dev", false},
		{"short rejected in production", "production", "short", long2, true},
		{"strong ok in production", "production", long, long2, false},
		{"dev sample enc key rejected in production", "production", devEnc, long, true},
		{"dev sample hmac key rejected in staging", "staging", long, devHMAC, true},
		{"sample marker inside a long key rejected", "production", strings.Repeat("x", 20) + placeholderPIIMarker, long, true},
		{"equal enc and hmac key rejected in production", "production", long, long, true},
		{"equal keys ok in development", "development", "dev", "dev", false},
		{"dev sample keys ok in development", "development", devEnc, devHMAC, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{App: AppConfig{Env: tt.env}, JWT: JWTConfig{AccessTokenSecret: long}, PII: PIIConfig{EncKey: tt.enc, HMACKey: tt.hmac}}
			if err := cfg.Validate(); (err != nil) != tt.wantErr {
				t.Fatalf("Validate() err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestOracleSyncConfigured(t *testing.T) {
	if (OracleSyncConfig{}).Configured() {
		t.Fatal("empty config must not be Configured")
	}
	full := OracleSyncConfig{Host: "h", Port: 1521, Service: "s", User: "u", Password: "p"}
	if !full.Configured() {
		t.Fatal("full config must be Configured")
	}
	full.Password = ""
	if full.Configured() {
		t.Fatal("missing password must not be Configured")
	}
}

func TestLoad_OracleSyncEnv(t *testing.T) {
	t.Setenv("PAYROLL_SYNC_ORACLE_HOST", "10.0.0.1")
	t.Setenv("PAYROLL_SYNC_ORACLE_SERVICE", "HRIS")
	t.Setenv("PAYROLL_SYNC_ORACLE_USER", "PAYROLL_SYNC_RO")
	t.Setenv("PAYROLL_SYNC_ORACLE_PASSWORD", "x")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.OracleSync.Configured() || cfg.OracleSync.Port != 1521 || cfg.OracleSync.User != "PAYROLL_SYNC_RO" {
		t.Fatalf("unexpected %+v", cfg.OracleSync)
	}
}

func TestEnvFromOS(t *testing.T) {
	t.Setenv("APP_ENV", "")
	if got := EnvFromOS(); got != DefaultEnv {
		t.Fatalf("unset APP_ENV = %q, want %q", got, DefaultEnv)
	}
	t.Setenv("APP_ENV", "production")
	if got := EnvFromOS(); got != "production" {
		t.Fatalf("EnvFromOS() = %q, want production", got)
	}
}
