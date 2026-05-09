// Package product holds application-layer command handlers for the Product aggregate.
//
// Handlers in this package coordinate between domain logic (in internal/domain/product)
// and persistence (via the product.Repository interface). They never reach into infrastructure
// directly — repos are injected.
//
// Phase 1 scope: CRUD + Duplicate (master fields only). Workflow transitions
// (Submit/Confirm/Lock/Unlock) and routing/param/RM duplication land in later phases.
package product
