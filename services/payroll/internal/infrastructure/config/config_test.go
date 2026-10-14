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
			cfg := Config{App: AppConfig{Env: tt.env}, JWT: JWTConfig{AccessTokenSecret: tt.secret}, PII: PIIConfig{EncKey: long, HMACKey: long}}
			if err := cfg.Validate(); (err != nil) != tt.wantErr {
				t.Fatalf("Validate() err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidate_PIIKeys(t *testing.T) {
	long := strings.Repeat("p", minJWTSecretLen)
	tests := []struct {
		name, env, enc, hmac string
		wantErr              bool
	}{
		{"missing enc key rejected", "development", "", long, true},
		{"missing hmac key rejected", "development", long, "", true},
		{"short ok in development", "development", "dev", "dev", false},
		{"short rejected in production", "production", "short", long, true},
		{"strong ok in production", "production", long, long, false},
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
