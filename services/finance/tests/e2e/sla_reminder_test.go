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

	seedOverdueUserFillTask(ctx, t, rawDB)

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

// seedOverdueUserFillTask inserts a USER-assigned, SLA-overdue fill task (plus its
// parent product request) so the reminder/SLA jobs always have at least one
// notifiable task, regardless of what fixture data the DB happens to carry.
// The legacy notifier fallback (used when no FillEventNotifier is wired, as in
// this test) only notifies USER-type tasks — the DEPT-type rows seeded by
// migration 000367 are silently skipped, which is what made this test flaky
// against an otherwise-empty CI database.
func seedOverdueUserFillTask(ctx context.Context, t *testing.T, db *sql.DB) {
	t.Helper()

	var requestID int64
	err := db.QueryRowContext(ctx, `
		INSERT INTO cost_product_request (
			cpr_request_no, cpr_request_type_id, cpr_title, cpr_customer_name,
			cpr_product_classification, cpr_urgency_level, cpr_status, cpr_requester_user_id
		) VALUES (
			generate_cost_request_no(), (SELECT crt_type_id FROM cost_request_type WHERE crt_code = 'QUOTE'),
			'[E2E] SLA reminder test request', 'E2E Test Customer',
			'existing', 'medium', 'ROUTING_DEFINED', 'e2e-test-requester'
		)
		RETURNING cpr_request_id`).Scan(&requestID)
	require.NoError(t, err, "seed test product request")

	var productSysID int64
	err = db.QueryRowContext(ctx, `
		INSERT INTO cost_product_master (
			cpm_product_code, cpm_product_type_id, cpm_product_name, cpm_created_by, cpm_updated_by
		) VALUES (
			'E2E-SLA-' || floor(random() * 1000000)::text,
			(SELECT cpt_type_id FROM cost_product_type LIMIT 1),
			'[E2E] SLA reminder test product', 'e2e-test', 'e2e-test'
		)
		RETURNING cpm_product_sys_id`).Scan(&productSysID)
	require.NoError(t, err, "seed test product master")

	var routeHeadID int64
	err = db.QueryRowContext(ctx, `
		INSERT INTO cost_route_head (crh_product_sys_id, crh_routing_status, crh_created_by)
		VALUES ($1, 'COMPLETE', 'e2e-test')
		RETURNING crh_head_id`, productSysID).Scan(&routeHeadID)
	require.NoError(t, err, "seed test route head")

	_, err = db.ExecContext(ctx, `
		INSERT INTO cost_fill_task (
			cft_request_id, cft_route_head_id, cft_route_level,
			cft_filler_type, cft_filler_value,
			cft_sla_fill_hours, cft_status, cft_total_params, cft_filled_params,
			cft_activated_at
		) VALUES (
			$1, $2, 1,
			'USER', 'e2e-test-filler',
			1, 'ACTIVE', 1, 0,
			NOW() - INTERVAL '10 hours'
		)`, requestID, routeHeadID)
	require.NoError(t, err, "seed overdue USER fill task")
}
