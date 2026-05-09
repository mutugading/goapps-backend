// Package product contains the Product aggregate and its supporting types.
package product

import "strings"

// =============================================================================
// Code Value Object
// =============================================================================

// Code represents a validated product code.
type Code struct {
	value string
}

// NewCode creates a validated Code value object.
// The code must be 1–30 non-whitespace-only characters.
func NewCode(s string) (Code, error) {
	if strings.TrimSpace(s) == "" {
		return Code{}, ErrInvalidCode
	}
	if len(s) > 30 {
		return Code{}, ErrInvalidCode
	}
	return Code{value: s}, nil
}

// String returns the string representation of the code.
func (c Code) String() string { return c.value }

// =============================================================================
// Name Value Object
// =============================================================================

// Name represents a validated product name.
type Name struct {
	value string
}

// NewName creates a validated Name value object.
// The name must be 1–200 non-whitespace-only characters.
func NewName(s string) (Name, error) {
	if strings.TrimSpace(s) == "" {
		return Name{}, ErrInvalidName
	}
	if len(s) > 200 {
		return Name{}, ErrInvalidName
	}
	return Name{value: s}, nil
}

// String returns the string representation of the name.
func (n Name) String() string { return n.value }

// =============================================================================
// ItemCode Value Object
// =============================================================================

// ItemCode represents a validated product item code.
type ItemCode struct {
	value string
}

// NewItemCode creates a validated ItemCode value object.
// The item code must be 1–30 non-whitespace-only characters.
func NewItemCode(s string) (ItemCode, error) {
	if strings.TrimSpace(s) == "" {
		return ItemCode{}, ErrInvalidItemCode
	}
	if len(s) > 30 {
		return ItemCode{}, ErrInvalidItemCode
	}
	return ItemCode{value: s}, nil
}

// String returns the string representation of the item code.
func (ic ItemCode) String() string { return ic.value }

// =============================================================================
// ShadeCode Value Object (optional)
// =============================================================================

// ShadeCode represents an optional product shade code.
// An empty value is valid and indicates no shade code is set.
type ShadeCode struct {
	value string
}

// NewShadeCode creates a ShadeCode value object.
// An empty string is valid (indicates no shade code). Max 30 chars.
func NewShadeCode(s string) (ShadeCode, error) {
	if len(s) > 30 {
		return ShadeCode{}, ErrInvalidShadeCode
	}
	return ShadeCode{value: s}, nil
}

// String returns the string representation of the shade code.
func (sc ShadeCode) String() string { return sc.value }

// IsEmpty reports whether the shade code is unset.
func (sc ShadeCode) IsEmpty() bool { return sc.value == "" }

// =============================================================================
// ShadeName Value Object (optional)
// =============================================================================

// ShadeName represents an optional product shade name.
// An empty value is valid and indicates no shade name is set.
type ShadeName struct {
	value string
}

// NewShadeName creates a ShadeName value object.
// An empty string is valid (indicates no shade name). Max 100 chars.
func NewShadeName(s string) (ShadeName, error) {
	if len(s) > 100 {
		return ShadeName{}, ErrInvalidShadeName
	}
	return ShadeName{value: s}, nil
}

// String returns the string representation of the shade name.
func (sn ShadeName) String() string { return sn.value }

// IsEmpty reports whether the shade name is unset.
func (sn ShadeName) IsEmpty() bool { return sn.value == "" }

// =============================================================================
// Status Value Object
// =============================================================================

// Status is the lifecycle status of a product master record.
type Status string

const (
	// StatusDraft indicates the product is in draft state.
	StatusDraft Status = "DRAFT"
	// StatusParamPending indicates the product is waiting for parameters to be filled.
	StatusParamPending Status = "PARAM_PENDING"
	// StatusActive indicates the product is active.
	StatusActive Status = "ACTIVE"
	// StatusInactive indicates the product is inactive.
	StatusInactive Status = "INACTIVE"
)

// NewStatus parses and validates a Status from its string representation.
func NewStatus(s string) (Status, error) {
	ps := Status(s)
	if !ps.IsValid() {
		return "", ErrInvalidProductStatus
	}
	return ps, nil
}

// IsValid reports whether the Status is one of the recognized values.
func (ps Status) IsValid() bool {
	switch ps {
	case StatusDraft, StatusParamPending, StatusActive, StatusInactive:
		return true
	default:
		return false
	}
}

// String returns the canonical string form of the status.
func (ps Status) String() string { return string(ps) }

// =============================================================================
// WorkflowStatus Value Object
// =============================================================================

// WorkflowStatus represents the approval-workflow state of a product.
type WorkflowStatus string

const (
	// WorkflowDraft indicates the product costing is in draft.
	WorkflowDraft WorkflowStatus = "DRAFT"
	// WorkflowSubmitted indicates the product costing has been submitted for review.
	WorkflowSubmitted WorkflowStatus = "SUBMITTED"
	// WorkflowConfirmed indicates the product costing has been confirmed.
	WorkflowConfirmed WorkflowStatus = "CONFIRMED"
	// WorkflowLocked indicates the product costing is locked (terminal state in Phase 1).
	WorkflowLocked WorkflowStatus = "LOCKED"
	// WorkflowUnlockRequested indicates an unlock request is pending.
	WorkflowUnlockRequested WorkflowStatus = "UNLOCK_REQUESTED"
)

// NewWorkflowStatus parses and validates a WorkflowStatus from its string representation.
func NewWorkflowStatus(s string) (WorkflowStatus, error) {
	ws := WorkflowStatus(s)
	if !ws.IsValid() {
		return "", ErrInvalidWorkflowStatus
	}
	return ws, nil
}

// IsValid reports whether the WorkflowStatus is one of the recognized values.
func (ws WorkflowStatus) IsValid() bool {
	switch ws {
	case WorkflowDraft, WorkflowSubmitted, WorkflowConfirmed, WorkflowLocked, WorkflowUnlockRequested:
		return true
	default:
		return false
	}
}

// String returns the canonical string form of the workflow status.
func (ws WorkflowStatus) String() string { return string(ws) }

// IsTerminal reports whether this is a terminal workflow state.
// In Phase 1, only LOCKED is terminal.
func (ws WorkflowStatus) IsTerminal() bool {
	return ws == WorkflowLocked
}

// IsEditable reports whether the product may be modified in this state.
// In Phase 1, only DRAFT is editable.
func (ws WorkflowStatus) IsEditable() bool {
	return ws == WorkflowDraft
}

// =============================================================================
// Purpose Value Object
// =============================================================================

// Purpose represents the intended use of the product costing.
type Purpose string

const (
	// PurposeCommercial indicates the product is for commercial use.
	PurposeCommercial Purpose = "COMMERCIAL"
	// PurposeTesting indicates the product is for internal testing.
	PurposeTesting Purpose = "TESTING"
	// PurposeTrial indicates the product is for trial/pilot use.
	PurposeTrial Purpose = "TRIAL"
)

// NewPurpose parses and validates a Purpose from its string representation.
func NewPurpose(s string) (Purpose, error) {
	p := Purpose(s)
	if !p.IsValid() {
		return "", ErrInvalidPurpose
	}
	return p, nil
}

// IsValid reports whether the Purpose is one of the recognized values.
func (p Purpose) IsValid() bool {
	switch p {
	case PurposeCommercial, PurposeTesting, PurposeTrial:
		return true
	default:
		return false
	}
}

// String returns the canonical string form of the purpose.
func (p Purpose) String() string { return string(p) }

// =============================================================================
// CopyOptions Value Object
// =============================================================================

// CopyOptions controls what data is carried over during a product duplication.
// Callers should not mutate the fields of a CopyOptions obtained via a getter —
// there is no deep protection since Go does not support truly immutable structs.
type CopyOptions struct {
	// IncludeValues copies parameter values from the source product.
	IncludeValues bool
	// IncludeRouting copies routing/BOM data from the source product.
	IncludeRouting bool
	// IncludeRM copies raw-material associations from the source product.
	IncludeRM bool
	// IncludeAttachments copies file attachments from the source product.
	IncludeAttachments bool
}

// IsAny reports whether at least one copy option is enabled.
func (co CopyOptions) IsAny() bool {
	return co.IncludeValues || co.IncludeRouting || co.IncludeRM || co.IncludeAttachments
}
