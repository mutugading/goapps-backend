package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// SMTP TLS modes. TLS = implicit TLS (port 465, D-62 butir 8); STARTTLS = port 587; NONE only for a loopback host
// (test relay), never for the real server.
const (
	SMTPTLS      = "TLS"
	SMTPStartTLS = "STARTTLS"
	SMTPNone     = "NONE"
)

// SMTPConfig is the mail relay of cmd/mailer (D-62 (5)(8)). Everything comes from PAYROLL_SMTP_*; the password only
// from PAYROLL_SMTP_PASSWORD_FILE (secret mount) or PAYROLL_SMTP_PASSWORD, never from the DB or the repository.
type SMTPConfig struct {
	Host        string
	Port        int
	TLSMode     string
	User        string
	Password    string
	From        string
	FromName    string
	ReplyTo     string
	RatePerMin  int // 0 = unlimited (tests)
	MaxAttempts int
}

// LoadSMTP reads and validates the SMTP settings from the environment.
func LoadSMTP() (SMTPConfig, error) {
	c := SMTPConfig{
		Host:     os.Getenv("PAYROLL_SMTP_HOST"),
		User:     os.Getenv("PAYROLL_SMTP_USER"),
		From:     envOr("PAYROLL_SMTP_FROM", "mgtpayslip@mutugading.com"),
		FromName: envOr("PAYROLL_SMTP_FROM_NAME", "HR Mutu Gading"),
		ReplyTo:  os.Getenv("PAYROLL_SMTP_REPLY_TO"),
		TLSMode:  strings.ToUpper(envOr("PAYROLL_SMTP_TLS", SMTPTLS)),
	}
	var err error
	if c.Port, err = envInt("PAYROLL_SMTP_PORT", 465); err != nil {
		return c, err
	}
	if c.RatePerMin, err = envInt("PAYROLL_SMTP_RATE_PER_MINUTE", 30); err != nil {
		return c, err
	}
	if c.MaxAttempts, err = envInt("PAYROLL_SMTP_MAX_ATTEMPTS", 5); err != nil {
		return c, err
	}
	if f := os.Getenv("PAYROLL_SMTP_PASSWORD_FILE"); f != "" {
		b, rerr := os.ReadFile(f) //nolint:gosec // operator-provided secret path
		if rerr != nil {
			return c, fmt.Errorf("config: PAYROLL_SMTP_PASSWORD_FILE: %w", rerr)
		}
		c.Password = strings.TrimRight(string(b), "\r\n")
	} else {
		c.Password = os.Getenv("PAYROLL_SMTP_PASSWORD")
	}
	return c, c.Validate()
}

// Validate checks required fields and refuses cleartext to a non-loopback host.
func (c SMTPConfig) Validate() error {
	var missing []string
	if strings.TrimSpace(c.Host) == "" {
		missing = append(missing, "PAYROLL_SMTP_HOST")
	}
	if strings.TrimSpace(c.From) == "" {
		missing = append(missing, "PAYROLL_SMTP_FROM")
	}
	if c.User != "" && c.Password == "" {
		missing = append(missing, "PAYROLL_SMTP_PASSWORD(_FILE)")
	}
	if len(missing) > 0 {
		return errors.New("config: wajib diisi: " + strings.Join(missing, ", "))
	}
	switch c.TLSMode {
	case SMTPTLS, SMTPStartTLS:
	case SMTPNone:
		if c.Host != "127.0.0.1" && c.Host != "localhost" && c.Host != "::1" {
			return errors.New("config: PAYROLL_SMTP_TLS=NONE hanya untuk host loopback")
		}
	default:
		return fmt.Errorf("config: PAYROLL_SMTP_TLS %q harus TLS, STARTTLS, atau NONE", c.TLSMode)
	}
	if c.Port <= 0 || c.Port > 65535 || c.RatePerMin < 0 || c.MaxAttempts <= 0 {
		return errors.New("config: PAYROLL_SMTP_PORT/RATE_PER_MINUTE/MAX_ATTEMPTS di luar rentang")
	}
	return nil
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) (int, error) {
	v := os.Getenv(k)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("config: %s %q bukan angka", k, v)
	}
	return n, nil
}
