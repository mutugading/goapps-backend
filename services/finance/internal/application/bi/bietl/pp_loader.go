package bietl

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	oracleinfra "github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/oracle"
)

// TargetPPPack is the job target_type of ETL_PP_PACK_DAILY (PP-03): VIEW_BI_PACK_PROD_DAILY →
// bi_pp_stg_pack_prod_daily (→ PP_PACK_* facts, P004).
const TargetPPPack = "PP_PACK"

// PPWindowDays is the re-pull horizon of the packed-production job (PP-03).
const PPWindowDays = 35

// PackProdSource fetches packed-production view rows from Oracle (HRIS_ORACLE connection).
type PackProdSource interface {
	FetchPackProdDaily(ctx context.Context, d1, d2 time.Time) ([]oracleinfra.PackProdDailyRow, error)
}

// PackProdStaging replaces the staging window in one transaction (ETL§C).
type PackProdStaging interface {
	ReplacePackProdDaily(ctx context.Context, d1, d2 time.Time, rows []oracleinfra.PackProdDailyRow) (int, error)
}

// PPLoader runs the packed-production ETL. It is registered on the router for HRIS_ORACLE +
// PP_PACK only (PP-03), so the HRIS loader stays untouched. Job start/end/status go to bi_job_log
// through the job handler; this loader logs only the window and row count.
type PPLoader struct {
	src     PackProdSource
	staging PackProdStaging
	log     zerolog.Logger
	now     func() time.Time
}

// NewPPLoader constructs a PPLoader.
func NewPPLoader(src PackProdSource, staging PackProdStaging, log zerolog.Logger) *PPLoader {
	return &PPLoader{src: src, staging: staging, log: log, now: time.Now}
}

// Load implements Runner.
func (l *PPLoader) Load(ctx context.Context, targetType, _ string, _ uuid.UUID) (int, error) {
	if targetType != TargetPPPack {
		return 0, fmt.Errorf("unsupported packed-production target_type %q", targetType)
	}
	d1, d2 := PPWindow(l.now())
	rows, err := l.src.FetchPackProdDaily(ctx, d1, d2)
	if err != nil {
		return 0, fmt.Errorf("fetch pack prod daily: %w", err)
	}
	if len(rows) == 0 {
		// The window is still replaced (empty source = empty staging), but a whole empty month is
		// unusual enough to surface in the logs.
		l.log.Warn().Str("d1", d1.Format(time.DateOnly)).Str("d2", d2.Format(time.DateOnly)).
			Msg("packed production view returned 0 rows; staging window will be emptied")
	}
	n, err := l.staging.ReplacePackProdDaily(ctx, d1, d2, rows)
	if err != nil {
		return 0, fmt.Errorf("replace pack prod staging: %w", err)
	}
	l.log.Info().Str("target_type", targetType).
		Str("d1", d1.Format(time.DateOnly)).Str("d2", d2.Format(time.DateOnly)).
		Int("rows", n).Msg("packed production staging loaded")
	return n, nil
}

// PPWindow returns [d1, d2] for the pull (PP-03): d2 = yesterday, d1 = today − 35, both WIB
// calendar dates whatever the server TZ is. Widening to whole weeks/months for the fact
// aggregation happens at aggregation time (P004), not here.
func PPWindow(now time.Time) (d1, d2 time.Time) {
	now = now.In(ppZone)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return today.AddDate(0, 0, -PPWindowDays), today.AddDate(0, 0, -1)
}

// ppZone is the plant business time zone (WIB); the finance container runs in UTC.
var ppZone = loadPPZone()

func loadPPZone() *time.Location {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return time.FixedZone("WIB", 7*60*60)
	}
	return loc
}
