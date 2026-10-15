package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	oracleinfra "github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/oracle"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

// Gated by INTEGRATION_TEST=true. Applies migration 000550 (idempotent) and verifies the
// replace-in-window semantics (HD-04): rows inside the window are replaced, rows outside kept.
func TestBiHRStagingRepository_ReplaceOTDaily(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		getEnvOrDefault("TEST_DB_HOST", "localhost"), getEnvOrDefault("TEST_DB_PORT", "5434"),
		getEnvOrDefault("TEST_DB_USER", "finance"), getEnvOrDefault("TEST_DB_PASSWORD", "finance123"),
		getEnvOrDefault("TEST_DB_NAME", "finance_db"))
	raw, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	require.NoError(t, waitForDB(raw, 30*time.Second))
	defer func() { _ = raw.Close() }()

	ddl, err := os.ReadFile("../../../migrations/postgres/000550_create_bi_hr_staging_ot.up.sql")
	require.NoError(t, err)
	_, err = raw.ExecContext(ctx, string(ddl))
	require.NoError(t, err)

	// Fixture dates far in the past so real data is never touched.
	const marker = "TST-H010"
	cleanup := func() {
		_, _ = raw.ExecContext(ctx, `DELETE FROM bi_hr_stg_ot_daily WHERE nik = $1`, marker)
		_, _ = raw.ExecContext(ctx, `DELETE FROM bi_hr_employee WHERE nik = $1`, marker)
	}
	cleanup()
	defer cleanup()

	d := func(s string) time.Time { v, _ := time.Parse("2006-01-02", s); return v }
	row := func(date, typ, calc string) oracleinfra.OTDailyRow {
		return oracleinfra.OTDailyRow{
			WorkDate: d(date), WeekStart: d("1999-01-04"), AttPeriod: "199901", WeekAttPeriod: "199901",
			EmpSysID: "S1", NIK: marker, EmpName: "Test", DeptID: "D1", DeptName: "Dept",
			OTType: typ, ActualHours: decimal.RequireFromString("1.5"),
			CalcHours: decimal.RequireFromString(calc), LimitHours: decimal.RequireFromString(calc),
		}
	}
	repo := postgres.NewBiHRStagingRepository(&postgres.DB{DB: raw})

	n, err := repo.ReplaceOTDaily(ctx, d("1999-01-04"), d("1999-01-10"),
		[]oracleinfra.OTDailyRow{row("1999-01-04", "Normal", "2.25"), row("1999-01-05", "Change Off", "7.5")})
	require.NoError(t, err)
	require.Equal(t, 2, n)

	// Second run over a sub-window: the 01-05 row is replaced, 01-04 (outside) kept.
	n, err = repo.ReplaceOTDaily(ctx, d("1999-01-05"), d("1999-01-10"),
		[]oracleinfra.OTDailyRow{row("1999-01-06", "Normal", "3.125")})
	require.NoError(t, err)
	require.Equal(t, 1, n)

	var cnt int
	require.NoError(t, raw.QueryRowContext(ctx, `SELECT COUNT(*) FROM bi_hr_stg_ot_daily WHERE nik = $1`, marker).Scan(&cnt))
	require.Equal(t, 2, cnt)
	var calc string
	require.NoError(t, raw.QueryRowContext(ctx,
		`SELECT ot_calc_hours::text FROM bi_hr_stg_ot_daily WHERE nik = $1 AND work_date = '1999-01-06'`, marker).Scan(&calc))
	require.True(t, decimal.RequireFromString(calc).Equal(decimal.RequireFromString("3.125")))
	require.NoError(t, raw.QueryRowContext(ctx, `SELECT COUNT(*) FROM bi_hr_employee WHERE nik = $1`, marker).Scan(&cnt))
	require.Equal(t, 1, cnt)
}
