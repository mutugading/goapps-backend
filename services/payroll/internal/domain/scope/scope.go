// Package scope models payroll data scopes (company:division) that drive the
// row-level security policies (D-06, D-11).
package scope

import (
	"context"
	"errors"
	"strconv"
	"strings"
)

// Sentinel errors.
var (
	ErrInvalid    = errors.New("invalid payroll scope")
	ErrNoIdentity = errors.New("payroll identity missing from context")
)

// Scope is one company:division pair a user may access.
type Scope struct {
	companyID int16
	division  string
}

// New validates and builds a scope. The division must not contain the
// separators used by the app.scopes setting.
func New(companyID int16, division string) (Scope, error) {
	if companyID <= 0 || division == "" || strings.ContainsAny(division, ",:") {
		return Scope{}, ErrInvalid
	}
	return Scope{companyID: companyID, division: division}, nil
}

// CompanyID returns the company id.
func (s Scope) CompanyID() int16 { return s.companyID }

// Division returns the division code.
func (s Scope) Division() string { return s.division }

// String formats the scope as "company_id:division" (04§17).
func (s Scope) String() string {
	return strconv.Itoa(int(s.companyID)) + ":" + s.division
}

// Join formats scopes as the comma-separated value of app.scopes.
// No scopes yields an empty string, which RLS treats as no access.
func Join(scopes []Scope) string {
	parts := make([]string, len(scopes))
	for i, s := range scopes {
		parts[i] = s.String()
	}
	return strings.Join(parts, ",")
}

// Identity is the authenticated caller with its resolved scopes.
type Identity struct {
	UserID string
	Scopes []Scope
}

type ctxKey struct{}

// WithIdentity stores the identity in the context.
func WithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// FromContext returns the identity stored by WithIdentity.
func FromContext(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(ctxKey{}).(Identity)
	return id, ok
}

// Resolver loads the scopes of a user from pyr_user_scope. Admin users
// (all_scopes) resolve to every active company:division.
type Resolver interface {
	Resolve(ctx context.Context, userID string) ([]Scope, error)
}
