// Package e2e provides end-to-end tests for the finance service gRPC API.
package e2e

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"

	costfillapp "github.com/mutugading/goapps-backend/services/finance/internal/application/costfillassignment"
	costnotif "github.com/mutugading/goapps-backend/services/finance/internal/application/costnotification"
	fillnotifierinfra "github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/fillnotifier"
	finpg "github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

// TestSLAReminderJobsFire verifies that the SLA notifier job and reminder job can
// run against a real database without errors and emit at least one notification
// for any overdue or pending fill tasks that exist.
func TestSLAReminderJobsFire(t *testing.T) {
	if os.Getenv("E2E_TEST") != "true" {
		t.Skip("skipping: E2E_TEST not set")
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://finance:finance123@localhost:5434/finance_db?sslmode=disable"
	}
	rawDB, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	defer func() { _ = rawDB.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db := finpg.NewDBFromSQL(rawDB)
	costNotifRepo := finpg.NewCostNotificationRepository(db)
	emitter := costnotif.NewEmitter(costNotifRepo)
	fillTaskRepo := finpg.NewCostFillTaskRepository(db)

	var before int
	_ = db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM cost_notification WHERE cn_trigger_type IN ('SLA_OVERDUE','PENDING_FILL','PENDING_APPROVAL')").
		Scan(&before)

	const reminderGapHours = 4
	slaNotifier := fillnotifierinfra.New(fillTaskRepo, emitter)
	slaJob := costfillapp.NewSLANotifierJob(fillTaskRepo, slaNotifier, reminderGapHours)
	require.NoError(t, slaJob.RunWithError(ctx), "SLA job should complete without error")

	reminderJob := fillnotifierinfra.NewReminderJob(fillTaskRepo, emitter, reminderGapHours)
	reminderJob.Run()

	var after int
	_ = db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM cost_notification WHERE cn_trigger_type IN ('SLA_OVERDUE','PENDING_FILL','PENDING_APPROVAL')").
		Scan(&after)

	t.Logf("notifications before=%d after=%d (delta=%d)", before, after, after-before)
	require.Greater(t, after, before, "expected at least one new SLA/reminder notification to be created")
}
