// Package postgres provides the PostgreSQL connection and repositories for
// the payroll database. The service always connects as payroll_app, so every
// query is subject to RLS; scope is passed per transaction (ScopedTx).
package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib" // PostgreSQL driver with SCRAM-SHA-256 support
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/apperr"
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
	return db.transactionOpts(ctx, nil, fn)
}

// txBeginner is what *sql.DB and *sql.Conn share.
type txBeginner interface {
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

func (db *DB) transactionOpts(ctx context.Context, opts *sql.TxOptions, fn func(tx *sql.Tx) error) error {
	return runTx(ctx, db.DB, opts, fn)
}

func runTx(ctx context.Context, b txBeginner, opts *sql.TxOptions, fn func(tx *sql.Tx) error) error {
	tx, err := b.BeginTx(ctx, opts)
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
	return db.ScopedTxOpts(ctx, nil, fn)
}

// ScopedTxOpts is ScopedTx with transaction options (nil = default isolation).
func (db *DB) ScopedTxOpts(ctx context.Context, opts *sql.TxOptions, fn func(tx *sql.Tx) error) error {
	return scopedTx(ctx, db.DB, opts, fn)
}

// ScopedTxOptsLocked is ScopedTxOpts that first takes a session advisory lock (namespace, key) on the connection and
// only then begins the transaction, so a REPEATABLE READ snapshot is taken after the lock was granted (T033d): the
// snapshot of a waiter always includes what the previous holder committed. A transaction-level lock cannot do this
// because the snapshot is taken at the start of the first statement, before it waits. The lock is held until the
// transaction ended; if it cannot be released the connection is discarded. A session lock needs a direct connection
// (CLI jobs, docs/database.md): behind PgBouncer in transaction mode lock and unlock may hit different server
// connections. Waiting is unbounded unless ctx has a deadline.
func (db *DB) ScopedTxOptsLocked(ctx context.Context, opts *sql.TxOptions, namespace, key string, fn func(tx *sql.Tx) error) (err error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("failed to get connection: %w", err)
	}
	defer func() {
		if cerr := conn.Close(); cerr != nil {
			log.Warn().Err(cerr).Msg("close locked connection")
		}
	}()
	if _, err = conn.ExecContext(ctx, `SELECT pg_advisory_lock(hashtext($1), hashtext($2))`, namespace, key); err != nil {
		return fmt.Errorf("advisory lock %s: %w", namespace, err)
	}
	defer func() {
		// The caller's ctx may be cancelled already; the lock must still be released.
		uctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if _, uerr := conn.ExecContext(uctx, `SELECT pg_advisory_unlock(hashtext($1), hashtext($2))`, namespace, key); uerr != nil {
			log.Error().Err(uerr).Str("lock", namespace).Msg("advisory unlock failed; discarding connection")
			_ = conn.Raw(func(any) error { return driver.ErrBadConn }) //nolint:errcheck // marks the connection bad so the pool drops it
		}
	}()
	return scopedTx(ctx, conn, opts, fn)
}

func scopedTx(ctx context.Context, b txBeginner, opts *sql.TxOptions, fn func(tx *sql.Tx) error) error {
	id, ok := scope.FromContext(ctx)
	if !ok {
		return scope.ErrNoIdentity
	}
	err := runTx(ctx, b, opts, func(tx *sql.Tx) error {
		if err := setSession(ctx, tx, id.UserID, scope.Join(id.Scopes)); err != nil {
			return err
		}
		return fn(tx)
	})
	// REPEATABLE READ: a concurrent commit of the same period after our snapshot (SQLSTATE 40001) is the
	// same situation as a held run lock; the caller retries (T023f).
	var pe *pgconn.PgError
	if opts != nil && errors.As(err, &pe) && pe.Code == "40001" {
		return apperr.New(apperr.CodeRunInProgress, "periode ini baru saja diubah proses lain; ulangi", nil)
	}
	return err
}

// setSession sets the RLS settings for the current transaction only.
func setSession(ctx context.Context, tx *sql.Tx, userID, scopes string) error {
	const q = `SELECT set_config('app.user_id', $1, true), set_config('app.scopes', $2, true)`
	if _, err := tx.ExecContext(ctx, q, userID, scopes); err != nil {
		return fmt.Errorf("failed to set payroll session scope: %w", err)
	}
	return nil
}
