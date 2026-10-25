// Command hris-att-backfill re-pulls VIEW_BI_ATT_DAILY for an explicit date range into
// bi_hr_stg_att_daily and recomputes HR_HEADCOUNT for the touched payroll periods (H033).
// The scheduled job only pulls the last 35 days (HD-04); use this once to fill earlier
// periods so ratio KPIs (OT per headcount, % overtime) have a denominator for every period
// that has OT facts. Usage: hris-att-backfill -d1 2026-07-26 -d2 2026-08-25
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/config"
	oracleinfra "github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/oracle"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
	"github.com/rs/zerolog"
)

const dateLayout = "2006-01-02"

func main() {
	if err := run(); err != nil {
		log.Fatalf("hris-att-backfill: %v", err)
	}
}

func run() error {
	d1s := flag.String("d1", "", "first work date (YYYY-MM-DD)")
	d2s := flag.String("d2", "", "last work date (YYYY-MM-DD)")
	withOT := flag.Bool("ot", false, "also re-pull VIEW_BI_OT_DAILY for the same range")
	withATT := flag.Bool("att", true, "re-pull VIEW_BI_ATT_DAILY (false = OT only, e.g. legacy OT history before 2026-06-26)")
	flag.Parse()
	if !*withATT && !*withOT {
		return fmt.Errorf("nothing to do: -att=false needs -ot")
	}
	d1, err := time.Parse(dateLayout, *d1s)
	if err != nil {
		return fmt.Errorf("-d1: %w", err)
	}
	d2, err := time.Parse(dateLayout, *d2s)
	if err != nil {
		return fmt.Errorf("-d2: %w", err)
	}
	if d2.Before(d1) {
		return fmt.Errorf("-d2 before -d1")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if !cfg.OracleHRIS.Configured() {
		return fmt.Errorf("ORACLE_HRIS_* not configured")
	}
	db, err := postgres.NewConnection(&cfg.Database)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer func() { _ = db.Close() }()
	hris, err := oracleinfra.NewClient(cfg.OracleHRIS, zerolog.Nop())
	if err != nil {
		return fmt.Errorf("connect oracle hris: %w", err)
	}
	defer func() { _ = hris.Close() }()

	var sourceID uuid.UUID
	if err = db.QueryRow(`SELECT source_id FROM bi_job WHERE target_type = 'HR_ATT_STAGING'`).Scan(&sourceID); err != nil {
		return fmt.Errorf("find HRIS source: %w", err)
	}

	ctx := context.Background()
	if *withATT {
		rows, err := oracleinfra.NewHRISATTRepository(hris).FetchATTDaily(ctx, d1, d2)
		if err != nil {
			return fmt.Errorf("fetch ATT daily: %w", err)
		}
		n, err := postgres.NewBiHRStagingRepository(db).ReplaceATTDaily(ctx, d1, d2, rows, sourceID)
		if err != nil {
			return fmt.Errorf("replace ATT daily: %w", err)
		}
		log.Printf("backfilled %d ATT staging rows for %s..%s", n, *d1s, *d2s)
	}
	if !*withOT {
		return nil
	}
	otRows, err := oracleinfra.NewHRISOTRepository(hris).FetchOTDaily(ctx, d1, d2)
	if err != nil {
		return fmt.Errorf("fetch OT daily: %w", err)
	}
	m, err := postgres.NewBiHRStagingRepository(db).ReplaceOTDaily(ctx, d1, d2, otRows, sourceID)
	if err != nil {
		return fmt.Errorf("replace OT daily: %w", err)
	}
	log.Printf("backfilled %d OT staging rows for %s..%s", m, *d1s, *d2s)
	return nil
}
