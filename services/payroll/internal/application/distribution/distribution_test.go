package distribution

import (
	"errors"
	"net/textproto"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/config"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/pdf"
)

func TestScrub_UTF8AndMask(t *testing.T) {
	long := strings.Repeat("é", 400) // 800 bytes, 400 runes: byte slicing at 500 would cut a rune
	if got := scrub(long); !utf8.ValidString(got) || len([]rune(got)) != 400 {
		t.Fatalf("short rune string altered: %d runes", len([]rune(got)))
	}
	long = strings.Repeat("é", 700)
	got := scrub(long)
	if !utf8.ValidString(got) || len([]rune(got)) != 500 {
		t.Fatalf("truncation: valid=%v runes=%d", utf8.ValidString(got), len([]rune(got)))
	}
	if got := scrub("bad\xffbyte"); !utf8.ValidString(got) {
		t.Fatal("invalid UTF-8 kept")
	}
	if got := scrub("rcpt satu@example.test refused"); strings.Contains(got, "satu@") || !strings.Contains(got, "s***@example.test") {
		t.Fatalf("address not masked: %s", got)
	}
}

func TestOutcome_BackoffFinalAndPermanent(t *testing.T) {
	now := time.Date(2036, 6, 1, 8, 0, 0, 0, time.UTC)
	s := &Service{Cfg: config.SMTPConfig{MaxAttempts: 5}, Now: func() time.Time { return now }}
	temp := &textproto.Error{Code: 451, Msg: "try later"}
	for attempt, want := range []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour} {
		st, next, _ := s.outcome(temp, attempt+1)
		if st != StFailed || !next.Valid || next.Time.Sub(now) != want {
			t.Fatalf("attempt %d: %s next=%v want +%s", attempt+1, st, next.Time.Sub(now), want)
		}
	}
	if st, next, _ := s.outcome(temp, 5); st != StFinal || next.Valid {
		t.Fatalf("max attempts: %s %v", st, next)
	}
	if st, _, _ := s.outcome(&textproto.Error{Code: 550, Msg: "no such user"}, 1); st != StFinal {
		t.Fatalf("5xx permanent: %s", st)
	}
	if st, _, _ := s.outcome(pdf.ErrNoBirthDate, 1); st != StNoPass {
		t.Fatalf("no dob: %s", st)
	}
	if st, _, _ := s.outcome(nil, 1); st != StSent {
		t.Fatalf("ok: %s", st)
	}
	if st, _, _ := s.outcome(errors.New("dial tcp: refused"), 1); st != StFailed {
		t.Fatalf("network: %s", st)
	}
}
