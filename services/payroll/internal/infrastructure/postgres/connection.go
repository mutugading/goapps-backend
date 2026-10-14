// Package postgres provides the PostgreSQL connection and repositories for
// the payroll database. The service always connects as payroll_app, so every
// query is subject to RLS; scope is passed per transaction (ScopedTx).
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // PostgreSQL driver with SCRAM-SHA-256 support
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/scope"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/config"
)

// DB wraps the SQL database connection.
type DB struct {
	*sql.DB
}

// NewConnection creates a new PostgreSQL connection pool.
func NewConnection(cfg *config.DatabaseConfig) (*DB, error) {
	db, err := sql.Open("pgx", cfg.ConnectionString())
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.Info().
		Str("host", cfg.Host).
		Int("port", cfg.Port).
		Str("database", cfg.Name).
		Str("user", cfg.User).
		Msg("Database connection established")

	return &DB{db}, nil
}

// NewDBFromSQL wraps an existing *sql.DB. Used by integration tests.
func NewDBFromSQL(db *sql.DB) *DB {
	return &DB{db}
}

// Close closes the database connection.
func (db *DB) Close() error {
	if err := db.DB.Close(); err != nil {
		return fmt.Errorf("failed to close database connection: %w", err)
	}
	return nil
}

// Transaction executes fn within a database transaction.
func (db *DB) Transaction(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return errors.Join(
				fmt.Errorf("rollback failed: %w", rbErr),
				fmt.Errorf("transaction error: %w", err),
			)
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

// ScopedTx runs fn in a transaction with app.user_id and app.scopes set
// (transaction-local, like SET LOCAL) from the identity in ctx. Without an
// identity it refuses to run; with no scopes RLS returns no rows.
func (db *DB) ScopedTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	id, ok := scope.FromContext(ctx)
	if !ok {
		return scope.ErrNoIdentity
	}
	return db.Transaction(ctx, func(tx *sql.Tx) error {
		if err := setSession(ctx, tx, id.UserID, scope.Join(id.Scopes)); err != nil {
			return err
		}
		return fn(tx)
	})
}

// setSession sets the RLS settings for the current transaction only.
func setSession(ctx context.Context, tx *sql.Tx, userID, scopes string) error {
	const q = `SELECT set_config('app.user_id', $1, true), set_config('app.scopes', $2, true)`
	if _, err := tx.ExecContext(ctx, q, userID, scopes); err != nil {
		return fmt.Errorf("failed to set payroll session scope: %w", err)
	}
	return nil
}
