// Package audit defines the append-only audit log entry written for every
// payroll mutation (07 Konvensi umum, Audit; 04§8 pyr_audit_log).
package audit

import (
	"context"
	"database/sql"
)

// Entry is one audit record. Actor and IP come from the request identity.
type Entry struct {
	CompanyID int16
	Division  string // empty = global (stored NULL)
	Action    string
	Entity    string
	EntityKey string
	Before    any // marshalled to jsonb, PII masked
	After     any
	IP        string // empty = unknown
}

// Writer appends entries inside the caller's transaction so the audit row
// commits or rolls back with the mutation.
type Writer interface {
	Write(ctx context.Context, tx *sql.Tx, e Entry) error
}
