package distribution

import (
	"context"
	"database/sql"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/run"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/scope"
)

func identityFrom(ctx context.Context) (string, bool) {
	id, ok := scope.FromContext(ctx)
	return id.UserID, ok
}

func snapshot() *sql.TxOptions { return run.SnapshotTx }
