package audit

import (
	"context"
	"database/sql"
	"fmt"

	domain "github.com/mutugading/goapps-backend/services/payroll/internal/domain/audit"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/scope"
)

// Writer inserts into payroll.pyr_audit_log as payroll_app (INSERT only; the
// table is append-only by grant, 04§11).
type Writer struct{}

var _ domain.Writer = Writer{}

// NewWriter creates a Writer.
func NewWriter() Writer { return Writer{} }

const insertSQL = `
INSERT INTO payroll.pyr_audit_log
  (pal_actor, pal_action, pal_entity, pal_entity_key, pal_company_id, pal_division, pal_before, pal_after, pal_ip)
VALUES ($1, $2, NULLIF($3,''), NULLIF($4,''), $5, NULLIF($6,''), $7::jsonb, $8::jsonb, NULLIF($9,'')::inet)`

// Write appends e. The actor is the identity in ctx (the same value as
// app.user_id of the transaction); company/division must be inside the scope,
// otherwise RLS WITH CHECK rejects the row (D-14).
func (Writer) Write(ctx context.Context, tx *sql.Tx, e domain.Entry) error {
	id, ok := scope.FromContext(ctx)
	if !ok {
		return scope.ErrNoIdentity
	}
	if e.Action == "" {
		return fmt.Errorf("audit: action required")
	}
	before, err := MaskJSON(e.Before)
	if err != nil {
		return err
	}
	after, err := MaskJSON(e.After)
	if err != nil {
		return err
	}
	companyID := e.CompanyID
	if companyID == 0 {
		companyID = 1
	}
	if _, err := tx.ExecContext(ctx, insertSQL, id.UserID, e.Action, e.Entity, e.EntityKey,
		companyID, e.Division, nullBytes(before), nullBytes(after), e.IP); err != nil {
		return fmt.Errorf("failed to write audit log: %w", err)
	}
	return nil
}

func nullBytes(b []byte) any {
	if b == nil {
		return nil
	}
	return string(b)
}
