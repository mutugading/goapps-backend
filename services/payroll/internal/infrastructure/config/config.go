// Package config provides configuration management using Viper.
package config

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds all configuration for the Payroll service.
type Config struct {
	App      AppConfig      `mapstructure:"app"`
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	JWT      JWTConfig      `mapstructure:"jwt"`
	PII      PIIConfig      `mapstructure:"pii"`
	CORS     CORSConfig     `mapstructure:"cors"`
	Tracing  TracingConfig  `mapstructure:"tracing"`
	Logger   LoggerConfig   `mapstructure:"logger"`
}

// AppConfig holds application-level configuration.
type AppConfig struct {
	Name    string `mapstructure:"name"`
	Version string `mapstructure:"version"`
	Env     string `mapstructure:"env"`
}

// ServerConfig holds server configuration.
type ServerConfig struct {
	GRPCPort    int           `mapstructure:"grpc_port"`
	HTTPPort    int           `mapstructure:"http_port"`
	GRPCTimeout time.Duration `mapstructure:"grpc_timeout"`
}

// DatabaseConfig holds database configuration. The service connects as
// payroll_app (never the owner), so RLS always applies.
type DatabaseConfig struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	User            string        `mapstructure:"user"`
	Password        string        `mapstructure:"password"`
	Name            string        `mapstructure:"name"`
	SSLMode         string        `mapstructure:"ssl_mode"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
}

// ConnectionString returns the PostgreSQL connection string.
func (c *DatabaseConfig) ConnectionString() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.Name, c.SSLMode,
	)
}

// JWTConfig holds JWT validation configuration (shared secret with IAM).
type JWTConfig struct {
	AccessTokenSecret string `mapstructure:"access_token_secret"`
}

// PIIConfig holds the keys protecting PII columns (D-17): EncKey feeds pgcrypto
// pgp_sym_encrypt, HMACKey feeds the NPWP hash (13§3.3). Never stored in the DB.
type PIIConfig struct {
	EncKey  string `mapstructure:"enc_key"`
	HMACKey string `mapstructure:"hmac_key"`
}

// CORSConfig holds CORS configuration for the BFF/frontend origin(s).
type CORSConfig struct {
	AllowedOrigins []string `mapstructure:"allowed_origins"`
	MaxAge         int      `mapstructure:"max_age"`
}

// TracingConfig holds Jaeger/OpenTelemetry configuration.
type TracingConfig struct {
	Enabled     bool   `mapstructure:"enabled"`
	ServiceName string `mapstructure:"service_name"`
	Endpoint    string `mapstructure:"endpoint"`
	Insecure    bool   `mapstructure:"insecure"`
}

// LoggerConfig holds logging configuration.
type LoggerConfig struct {
	Level      string `mapstructure:"level"`
	Format     string `mapstructure:"format"`
	PrettyJSON bool   `mapstructure:"pretty_json"`
}

// Load reads configuration from file and environment variables.
func Load() (*Config, error) {
	v := viper.New()

	setDefaults(v)

	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("./config")

	// Config file is optional; env vars can override.
	if err := v.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if !errors.As(err, &configFileNotFoundError) {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
	}

	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	bindEnvVars(v)

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// minJWTSecretLen is the HS256 key size (32 bytes) required outside development.
const minJWTSecretLen = 32

// placeholderJWTSecrets are the sample values shipped in config.yaml/compose.
var placeholderJWTSecrets = []string{"change-this-in-production", "dev-access-secret-change-in-production"}

// Validate rejects configurations that must not start. Outside development the
// JWT secret must be set, not a sample value and at least 32 bytes long.
func (c *Config) Validate() error {
	secret := c.JWT.AccessTokenSecret
	if secret == "" {
		return errors.New("config: jwt.access_token_secret (JWT_ACCESS_SECRET) is required")
	}
	if c.PII.EncKey == "" || c.PII.HMACKey == "" {
		return errors.New("config: pii.enc_key (PAYROLL_PII_KEY) and pii.hmac_key (PAYROLL_PII_HMAC_KEY) are required")
	}
	if c.App.Env == "development" {
		return nil
	}
	if len(c.PII.EncKey) < minJWTSecretLen || len(c.PII.HMACKey) < minJWTSecretLen {
		return fmt.Errorf("config: pii keys must be at least %d bytes in %q", minJWTSecretLen, c.App.Env)
	}
	if slices.Contains(placeholderJWTSecrets, secret) {
		return fmt.Errorf("config: jwt.access_token_secret is a sample value, not allowed in %q", c.App.Env)
	}
	if len(secret) < minJWTSecretLen {
		return fmt.Errorf("config: jwt.access_token_secret must be at least %d bytes in %q", minJWTSecretLen, c.App.Env)
	}
	return nil
}

func setDefaults(v *viper.Viper) {
	// App defaults.
	v.SetDefault("app.name", "payroll-service")
	v.SetDefault("app.version", "0.1.0")
	v.SetDefault("app.env", "development")

	// Server defaults (ports 50054/8084, unused by other services -- D-10).
	v.SetDefault("server.grpc_port", 50054)
	v.SetDefault("server.http_port", 8084)
	v.SetDefault("server.grpc_timeout", 60*time.Second)

	// Database defaults (payroll-postgres, port 5437 -- D-10, wired from T002).
	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", 5437)
	v.SetDefault("database.user", "payroll_app")
	v.SetDefault("database.password", "payroll_app123")
	v.SetDefault("database.name", "payroll")
	v.SetDefault("database.ssl_mode", "disable")
	v.SetDefault("database.max_open_conns", 25)
	v.SetDefault("database.max_idle_conns", 5)
	v.SetDefault("database.conn_max_lifetime", 5*time.Minute)

	// JWT defaults (must match IAM access token secret).
	v.SetDefault("jwt.access_token_secret", "change-this-in-production")
	// Development-only sample keys; Validate requires real keys outside development.
	v.SetDefault("pii.enc_key", "dev-pii-enc-key-change-in-production")
	v.SetDefault("pii.hmac_key", "dev-pii-hmac-key-change-in-production")

	// CORS defaults.
	v.SetDefault("cors.allowed_origins", []string{"http://localhost:3000"})
	v.SetDefault("cors.max_age", 300)

	// Tracing defaults.
	v.SetDefault("tracing.enabled", true)
	v.SetDefault("tracing.service_name", "payroll-service")
	v.SetDefault("tracing.endpoint", "localhost:4317")
	v.SetDefault("tracing.insecure", true)

	// Logger defaults.
	v.SetDefault("logger.level", "info")
	v.SetDefault("logger.format", "json")
	v.SetDefault("logger.pretty_json", false)
}

func bindEnvVars(v *viper.Viper) {
	envBindings := []struct {
		key     string
		envName string
	}{
		// Database.
		{"database.host", "PAYROLL_DATABASE_HOST"},
		{"database.port", "PAYROLL_DATABASE_PORT"},
		{"database.user", "PAYROLL_DATABASE_USER"},
		{"database.password", "PAYROLL_DATABASE_PASSWORD"},
		{"database.name", "PAYROLL_DATABASE_NAME"},
		{"database.ssl_mode", "PAYROLL_DATABASE_SSLMODE"},
		// JWT.
		{"jwt.access_token_secret", "JWT_ACCESS_SECRET"},
		// PII (D-17).
		{"pii.enc_key", "PAYROLL_PII_KEY"},
		{"pii.hmac_key", "PAYROLL_PII_HMAC_KEY"},
		// CORS.
		{"cors.allowed_origins", "CORS_ALLOWED_ORIGINS"},
		// Tracing.
		{"tracing.enabled", "TRACING_ENABLED"},
		{"tracing.endpoint", "JAEGER_ENDPOINT"},
		// App.
		{"app.env", "APP_ENV"},
		{"logger.level", "LOG_LEVEL"},
	}

	for _, binding := range envBindings {
		if err := v.BindEnv(binding.key, binding.envName); err != nil {
			// Log but don't fail -- environment binding errors are non-critical.
			fmt.Printf("Warning: failed to bind env %s: %v\n", binding.envName, err)
		}
	}
}
