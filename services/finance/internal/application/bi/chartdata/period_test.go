package chartdata_test

import (
	"testing"
	"time"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/bi/chartdata"
)

func TestResolvePeriod_WeeklyPresets(t *testing.T) {
	now := time.Date(2026, 10, 15, 10, 0, 0, 0, time.UTC) // Thursday
	got := chartdata.ResolvePeriod("THIS_WEEK", time.Time{}, time.Time{}, "WEEKLY", now)
	if want := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC); !got.From.Equal(want) || !got.To.Equal(now) {
		t.Fatalf("THIS_WEEK = %v..%v, want from %v", got.From, got.To, want)
	}
	got = chartdata.ResolvePeriod("LAST_WEEK", time.Time{}, time.Time{}, "WEEKLY", now)
	if from, to := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC); !got.From.Equal(from) || !got.To.Equal(to) {
		t.Fatalf("LAST_WEEK = %v..%v", got.From, got.To)
	}
	// Sunday belongs to the week that started the previous Monday.
	sun := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)
	got = chartdata.ResolvePeriod("THIS_WEEK", time.Time{}, time.Time{}, "WEEKLY", sun)
	if want := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC); !got.From.Equal(want) {
		t.Fatalf("THIS_WEEK on Sunday from = %v, want %v", got.From, want)
	}
}
