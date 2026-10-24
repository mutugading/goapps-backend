// Package idempotency defines the Idempotency-Key store contract (D-19).
package idempotency

import "context"

// Key identifies one idempotent request: the caller, the RPC and the client key.
type Key struct {
	UserID string
	Method string
	Key    string
}

// State is what Begin found for a key.
type State int

// Begin outcomes.
const (
	// Started: the key was claimed; the caller must run the handler, then Complete or Abort.
	Started State = iota
	// Replay: an identical request already finished; the stored response is returned.
	Replay
	// Conflict: the key exists for a different request body or a different user scope.
	Conflict
	// InProgress: the first request with this key is still running.
	InProgress
)

// Store persists results for 24 hours.
type Store interface {
	// Begin claims the key (taking over expired or stale rows) or reports its state.
	// On Started it returns the claim token that Complete and Abort must present.
	Begin(ctx context.Context, k Key, requestHash, scopeHash string) (state State, claim string, response []byte, err error)
	// Complete stores the successful response of a claimed key; a stale claim is a no-op.
	Complete(ctx context.Context, k Key, claim string, response []byte) error
	// Abort releases a claimed key after a failed request so the client can retry.
	Abort(ctx context.Context, k Key, claim string) error
}
