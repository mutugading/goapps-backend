package notification_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib" // pgx driver
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	notifinfra "github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/notification"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("IAM_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://iam:iam123@localhost:5435/iam_db?sslmode=disable"
	}
	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestDBUserResolver_GetByUserID_UnknownUUID_ReturnsEmpty(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("set INTEGRATION_TEST=true to run")
	}
	db := testDB(t)
	r := notifinfra.NewDBUserResolver(db)

	ids, err := r.GetByUserID(context.Background(), uuid.New())
	require.NoError(t, err)
	assert.Empty(t, ids)
}

func TestDBUserResolver_GetByPermission_UnknownCode_ReturnsEmpty(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("set INTEGRATION_TEST=true to run")
	}
	db := testDB(t)
	r := notifinfra.NewDBUserResolver(db)

	ids, err := r.GetByPermission(context.Background(), "nonexistent.permission.code")
	require.NoError(t, err)
	assert.Empty(t, ids)
}
