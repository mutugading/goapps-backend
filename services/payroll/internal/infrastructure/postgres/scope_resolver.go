package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/scope"
)

// ScopeResolver reads pyr_user_scope (D-06, D-11). The table's RLS policy only
// exposes the caller's own rows, so app.user_id is set before reading.
type ScopeResolver struct {
	db *DB
}

var _ scope.Resolver = (*ScopeResolver)(nil)

// NewScopeResolver creates a ScopeResolver.
func NewScopeResolver(db *DB) *ScopeResolver {
	return &ScopeResolver{db: db}
}

// resolveScopesSQL expands all_scopes rows to every division of an active
// company; explicit rows are kept only while their company is active.
const resolveScopesSQL = `
SELECT DISTINCT d.pdr_company_id, d.pdr_code
FROM payroll.pyr_user_scope s
JOIN payroll.pyr_division_ref d
  ON s.pus_all_scopes OR (d.pdr_company_id = s.pus_company_id AND d.pdr_code = s.pus_division)
JOIN payroll.pyr_company c ON c.pco_company_id = d.pdr_company_id AND c.pco_active
WHERE s.pus_user_id = $1
ORDER BY 1, 2`

// Resolve returns the scopes of userID. No rows means no access (fail closed).
func (r *ScopeResolver) Resolve(ctx context.Context, userID string) ([]scope.Scope, error) {
	var scopes []scope.Scope
	err := r.db.Transaction(ctx, func(tx *sql.Tx) error {
		if err := setSession(ctx, tx, userID, ""); err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, resolveScopesSQL, userID)
		if err != nil {
			return fmt.Errorf("failed to query user scope: %w", err)
		}
		defer closeRows(rows)
		for rows.Next() {
			var (
				companyID int16
				division  string
			)
			if err := rows.Scan(&companyID, &division); err != nil {
				return fmt.Errorf("failed to scan user scope: %w", err)
			}
			s, err := scope.New(companyID, division)
			if err != nil {
				return fmt.Errorf("user scope %d:%s: %w", companyID, division, err)
			}
			scopes = append(scopes, s)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return scopes, nil
}
