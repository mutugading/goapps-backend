// Package oracle reads the HRIS attendance sources (05§2) with go-ora (pure Go, D-42).
// Everything is read-only SELECT with bind variables; NUMBER is fetched as text
// (TO_CHAR) and parsed to decimal, never through float (rule 4).
package oracle

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	goora "github.com/sijms/go-ora/v2"

	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/config"
)

const (
	// QueryTimeout bounds each query (a period can be tens of thousands of rows).
	QueryTimeout = 5 * time.Minute
	// FetchSize is the row prefetch per round trip (05§4 step 3).
	FetchSize = 1000
)

// Client wraps the Oracle connection pool.
type Client struct {
	db *sql.DB
}

// NewClient opens (lazily; no ping) a pool for cfg. Fails when cfg is not Configured.
func NewClient(cfg config.OracleSyncConfig) (*Client, error) {
	if !cfg.Configured() {
		return nil, fmt.Errorf("oracle sync not configured: set PAYROLL_SYNC_ORACLE_HOST/PORT/SERVICE/USER/PASSWORD")
	}
	url := goora.BuildUrl(cfg.Host, cfg.Port, cfg.Service, cfg.User, cfg.Password,
		map[string]string{"PREFETCH_ROWS": fmt.Sprint(FetchSize)})
	db, err := sql.Open("oracle", url)
	if err != nil {
		return nil, fmt.Errorf("oracle open: %w", err)
	}
	db.SetMaxOpenConns(3)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(10 * time.Minute)
	return &Client{db: db}, nil
}

// Ping verifies connectivity.
func (c *Client) Ping(ctx context.Context) error { return c.db.PingContext(ctx) }

// Close closes the pool.
func (c *Client) Close() error { return c.db.Close() }
