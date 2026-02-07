package postgres_test

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/config"
	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/postgres"
)

func skipIfNoIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test (set INTEGRATION_TEST=true)")
	}
}

func setupTestDB(t *testing.T) *postgres.DB {
	t.Helper()
	skipIfNoIntegration(t)

	cfg := &config.DatabaseConfig{
		Host:            envOrDefault("TEST_DB_HOST", "localhost"),
		Port:            intEnvOrDefault("TEST_DB_PORT", 5435),
		User:            envOrDefault("TEST_DB_USER", "iam"),
		Password:        envOrDefault("TEST_DB_PASSWORD", "iam123"),
		Name:            envOrDefault("TEST_DB_NAME", "iam_db_test"),
		SSLMode:         envOrDefault("TEST_DB_SSLMODE", "disable"),
		MaxOpenConns:    5,
		MaxIdleConns:    2,
		ConnMaxLifetime: 5 * time.Minute,
	}

	db, err := postgres.NewConnection(cfg)
	if err != nil {
		t.Fatalf("Failed to connect to test database: %v", err)
	}

	t.Cleanup(func() { db.Close() })
	return db
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func intEnvOrDefault(key string, defaultVal int) int {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal
	}
	return n
}

// cleanupUser hard-deletes a user by ID from the database so tests leave no residue.
func cleanupUser(t *testing.T, db *postgres.DB, userID fmt.Stringer) {
	t.Helper()
	ctx := context.Background()
	_, _ = db.ExecContext(ctx, "DELETE FROM mst_user_detail WHERE user_id = $1", userID.String())
	_, _ = db.ExecContext(ctx, "DELETE FROM mst_user WHERE user_id = $1", userID.String())
}

// cleanupRole hard-deletes a role by ID from the database so tests leave no residue.
func cleanupRole(t *testing.T, db *postgres.DB, roleID fmt.Stringer) {
	t.Helper()
	ctx := context.Background()
	_, _ = db.ExecContext(ctx, "DELETE FROM mst_role WHERE role_id = $1", roleID.String())
}
