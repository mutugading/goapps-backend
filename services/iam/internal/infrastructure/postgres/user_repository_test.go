package postgres_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/user"
	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/postgres"
)

func uniqueSuffix() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func TestUserRepository_CreateAndGetByID(t *testing.T) {
	db := setupTestDB(t)
	repo := postgres.NewUserRepository(db)
	ctx := context.Background()

	suffix := uniqueSuffix()
	username := "testuser_" + suffix
	email := "test_" + suffix + "@example.com"

	u, err := user.NewUser(username, email, "$2a$10$hashedpassword", "integration-test")
	require.NoError(t, err)

	t.Cleanup(func() { cleanupUser(t, db, u.ID()) })

	err = repo.Create(ctx, u, nil)
	require.NoError(t, err)

	got, err := repo.GetByID(ctx, u.ID())
	require.NoError(t, err)

	assert.Equal(t, u.ID(), got.ID())
	assert.Equal(t, username, got.Username())
	assert.Equal(t, email, got.Email())
	assert.Equal(t, "$2a$10$hashedpassword", got.PasswordHash())
	assert.True(t, got.IsActive())
	assert.False(t, got.IsLocked())
	assert.Equal(t, 0, got.FailedLoginAttempts())
	assert.False(t, got.TwoFactorEnabled())
}

func TestUserRepository_GetByUsername(t *testing.T) {
	db := setupTestDB(t)
	repo := postgres.NewUserRepository(db)
	ctx := context.Background()

	suffix := uniqueSuffix()
	username := "testuser_" + suffix
	email := "test_" + suffix + "@example.com"

	u, err := user.NewUser(username, email, "$2a$10$hashedpassword", "integration-test")
	require.NoError(t, err)

	t.Cleanup(func() { cleanupUser(t, db, u.ID()) })

	err = repo.Create(ctx, u, nil)
	require.NoError(t, err)

	got, err := repo.GetByUsername(ctx, username)
	require.NoError(t, err)

	assert.Equal(t, u.ID(), got.ID())
	assert.Equal(t, username, got.Username())
	assert.Equal(t, email, got.Email())
}

func TestUserRepository_ExistsByUsername(t *testing.T) {
	db := setupTestDB(t)
	repo := postgres.NewUserRepository(db)
	ctx := context.Background()

	suffix := uniqueSuffix()
	username := "testuser_" + suffix
	email := "test_" + suffix + "@example.com"

	// Should not exist before creation.
	exists, err := repo.ExistsByUsername(ctx, username)
	require.NoError(t, err)
	assert.False(t, exists)

	u, err := user.NewUser(username, email, "$2a$10$hashedpassword", "integration-test")
	require.NoError(t, err)

	t.Cleanup(func() { cleanupUser(t, db, u.ID()) })

	err = repo.Create(ctx, u, nil)
	require.NoError(t, err)

	// Should exist after creation.
	exists, err = repo.ExistsByUsername(ctx, username)
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestUserRepository_List(t *testing.T) {
	db := setupTestDB(t)
	repo := postgres.NewUserRepository(db)
	ctx := context.Background()

	suffix := uniqueSuffix()
	const count = 3
	var createdUsers []*user.User

	for i := 0; i < count; i++ {
		username := fmt.Sprintf("testlistuser_%s_%d", suffix, i)
		email := fmt.Sprintf("testlist_%s_%d@example.com", suffix, i)

		u, err := user.NewUser(username, email, "$2a$10$hashedpassword", "integration-test")
		require.NoError(t, err)
		createdUsers = append(createdUsers, u)

		err = repo.Create(ctx, u, nil)
		require.NoError(t, err)
	}

	t.Cleanup(func() {
		for _, u := range createdUsers {
			cleanupUser(t, db, u.ID())
		}
	})

	// List with search filter to only pick up our test users.
	params := user.ListParams{
		Page:     1,
		PageSize: 10,
		Search:   "testlistuser_" + suffix,
	}

	users, total, err := repo.List(ctx, params)
	require.NoError(t, err)
	assert.Equal(t, int64(count), total)
	assert.Len(t, users, count)
}

func TestUserRepository_Update(t *testing.T) {
	db := setupTestDB(t)
	repo := postgres.NewUserRepository(db)
	ctx := context.Background()

	suffix := uniqueSuffix()
	username := "testuser_" + suffix
	email := "test_" + suffix + "@example.com"

	u, err := user.NewUser(username, email, "$2a$10$hashedpassword", "integration-test")
	require.NoError(t, err)

	t.Cleanup(func() { cleanupUser(t, db, u.ID()) })

	err = repo.Create(ctx, u, nil)
	require.NoError(t, err)

	// Update the email via the domain method.
	newEmail := "updated_" + suffix + "@example.com"
	err = u.Update(&newEmail, nil, "integration-test")
	require.NoError(t, err)

	err = repo.Update(ctx, u)
	require.NoError(t, err)

	got, err := repo.GetByID(ctx, u.ID())
	require.NoError(t, err)

	assert.Equal(t, newEmail, got.Email())
	assert.NotNil(t, got.Audit().UpdatedAt)
}

func TestUserRepository_SoftDelete(t *testing.T) {
	db := setupTestDB(t)
	repo := postgres.NewUserRepository(db)
	ctx := context.Background()

	suffix := uniqueSuffix()
	username := "testuser_" + suffix
	email := "test_" + suffix + "@example.com"

	u, err := user.NewUser(username, email, "$2a$10$hashedpassword", "integration-test")
	require.NoError(t, err)

	// Always hard-delete in cleanup regardless of soft-delete state.
	t.Cleanup(func() { cleanupUser(t, db, u.ID()) })

	err = repo.Create(ctx, u, nil)
	require.NoError(t, err)

	err = repo.Delete(ctx, u.ID(), "integration-test")
	require.NoError(t, err)

	// GetByID should return ErrNotFound after soft delete.
	_, err = repo.GetByID(ctx, u.ID())
	assert.ErrorIs(t, err, shared.ErrNotFound)

	// ExistsByUsername should return false after soft delete.
	exists, err := repo.ExistsByUsername(ctx, username)
	require.NoError(t, err)
	assert.False(t, exists)
}
