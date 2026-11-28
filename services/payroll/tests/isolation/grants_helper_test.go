package isolation

import (
	"context"
	"database/sql"
	"fmt"
	"os"
)

// applyGrants runs deploy/grants.sql as the owner after the migrations (D-66 C(2): GRANT/REVOKE live outside
// the migrations, ADR 0001). The file is plain SQL (no psql meta-commands), so it runs as one simple query.
func applyGrants(ctx context.Context, db *sql.DB) error {
	body, err := os.ReadFile("../../deploy/grants.sql") //nolint:gosec // fixed in-repo path
	if err != nil {
		return fmt.Errorf("read grants.sql: %w", err)
	}
	if _, err := db.ExecContext(ctx, string(body)); err != nil {
		return fmt.Errorf("grants.sql: %w", err)
	}
	return nil
}
