// Package product_test contains domain-layer tests for the Product aggregate.
package product_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/product"
)

// helpers

func validProduct(t *testing.T) *product.Product {
	t.Helper()
	p, err := product.NewProduct(
		"PROD-001", "Test Product", "ITEM-001",
		"SC01", "Shade One",
		uuid.New(), "DEPT-A",
		"COMMERCIAL",
		uuid.Nil,
		"admin",
	)
	require.NoError(t, err)
	return p
}

// =============================================================================
// NewProduct
// =============================================================================

func TestNewProduct_Valid(t *testing.T) {
	deptID := uuid.New()
	reqID := uuid.New()

	p, err := product.NewProduct(
		"PROD-001", "Test Product", "ITEM-001",
		"SC01", "Shade One",
		deptID, "DEPT-A",
		"COMMERCIAL",
		reqID,
		"admin",
	)

	require.NoError(t, err)
	require.NotNil(t, p)

	assert.NotEqual(t, uuid.Nil, p.ID())
	assert.Equal(t, "PROD-001", p.Code().String())
	assert.Equal(t, "Test Product", p.Name().String())
	assert.Equal(t, "ITEM-001", p.ItemCode().String())
	assert.Equal(t, "SC01", p.ShadeCode().String())
	assert.Equal(t, "Shade One", p.ShadeName().String())
	assert.Equal(t, product.StatusDraft, p.ProductStatus())
	assert.Equal(t, product.WorkflowDraft, p.WorkflowStatus())
	assert.Equal(t, deptID, p.CreatedByDeptID())
	assert.Equal(t, "DEPT-A", p.CreatedByDeptCode())
	assert.Equal(t, product.PurposeCommercial, p.Purpose())
	assert.Equal(t, reqID, p.CurrentRequestID())
	assert.Equal(t, "admin", p.CreatedBy())
	assert.False(t, p.CreatedAt().IsZero())
	assert.Nil(t, p.UpdatedAt())
	assert.Equal(t, "", p.UpdatedBy())
	assert.Nil(t, p.DeletedAt())
	assert.False(t, p.IsDeleted())
	assert.Equal(t, 0, p.UnlockCount())
	assert.Equal(t, uuid.Nil, p.DuplicatedFromID())
	assert.Nil(t, p.CopiedWithOptions())
}

func TestNewProduct_Invalid(t *testing.T) {
	deptID := uuid.New()

	long30 := func(n int) string {
		b := make([]byte, n)
		for i := range b {
			b[i] = 'A'
		}
		return string(b)
	}

	tests := []struct {
		name      string
		code      string
		pname     string
		itemCode  string
		shadeCode string
		shadeName string
		purpose   string
		wantErr   error
	}{
		{
			name: "empty code",
			code: "", pname: "Name", itemCode: "IC-001",
			shadeCode: "", shadeName: "", purpose: "COMMERCIAL",
			wantErr: product.ErrInvalidCode,
		},
		{
			name: "code too long (31 chars)",
			code: long30(31), pname: "Name", itemCode: "IC-001",
			shadeCode: "", shadeName: "", purpose: "COMMERCIAL",
			wantErr: product.ErrInvalidCode,
		},
		{
			name: "whitespace-only code",
			code: "   ", pname: "Name", itemCode: "IC-001",
			shadeCode: "", shadeName: "", purpose: "COMMERCIAL",
			wantErr: product.ErrInvalidCode,
		},
		{
			name: "empty name",
			code: "PROD-001", pname: "", itemCode: "IC-001",
			shadeCode: "", shadeName: "", purpose: "COMMERCIAL",
			wantErr: product.ErrInvalidName,
		},
		{
			name: "name too long (201 chars)",
			code: "PROD-001", pname: long30(201), itemCode: "IC-001",
			shadeCode: "", shadeName: "", purpose: "COMMERCIAL",
			wantErr: product.ErrInvalidName,
		},
		{
			name: "empty item code",
			code: "PROD-001", pname: "Name", itemCode: "",
			shadeCode: "", shadeName: "", purpose: "COMMERCIAL",
			wantErr: product.ErrInvalidItemCode,
		},
		{
			name: "item code too long (31 chars)",
			code: "PROD-001", pname: "Name", itemCode: long30(31),
			shadeCode: "", shadeName: "", purpose: "COMMERCIAL",
			wantErr: product.ErrInvalidItemCode,
		},
		{
			name: "shade code too long (31 chars)",
			code: "PROD-001", pname: "Name", itemCode: "IC-001",
			shadeCode: long30(31), shadeName: "", purpose: "COMMERCIAL",
			wantErr: product.ErrInvalidShadeCode,
		},
		{
			name: "shade name too long (101 chars)",
			code: "PROD-001", pname: "Name", itemCode: "IC-001",
			shadeCode: "", shadeName: long30(101), purpose: "COMMERCIAL",
			wantErr: product.ErrInvalidShadeName,
		},
		{
			name: "invalid purpose",
			code: "PROD-001", pname: "Name", itemCode: "IC-001",
			shadeCode: "", shadeName: "", purpose: "UNKNOWN",
			wantErr: product.ErrInvalidPurpose,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := product.NewProduct(
				tc.code, tc.pname, tc.itemCode,
				tc.shadeCode, tc.shadeName,
				deptID, "DEPT-A",
				tc.purpose,
				uuid.Nil,
				"admin",
			)
			require.Error(t, err)
			assert.True(t, errors.Is(err, tc.wantErr), "expected %v but got %v", tc.wantErr, err)
		})
	}
}

// =============================================================================
// Update
// =============================================================================

func TestProduct_Update_OnDraft(t *testing.T) {
	p := validProduct(t)

	err := p.Update("Updated Name", "SC02", "Shade Two", "TESTING", "editor")

	require.NoError(t, err)
	assert.Equal(t, "Updated Name", p.Name().String())
	assert.Equal(t, "SC02", p.ShadeCode().String())
	assert.Equal(t, "Shade Two", p.ShadeName().String())
	assert.Equal(t, product.PurposeTesting, p.Purpose())
	assert.NotNil(t, p.UpdatedAt())
	assert.Equal(t, "editor", p.UpdatedBy())
}

func TestProduct_Update_WhenLocked(t *testing.T) {
	now := time.Now()
	// Build a product directly in LOCKED workflow state via ReconstructProduct.
	p := product.ReconstructProduct(
		uuid.New(),
		"PROD-001", "Name", "ITEM-001", "", "",
		"ACTIVE", "LOCKED",
		uuid.New(), "DEPT-A",
		"COMMERCIAL",
		uuid.Nil, "",
		nil,
		uuid.Nil, 0,
		uuid.Nil,
		&now, "admin", "202504",
		1,
		time.Now(), "admin",
		nil, "",
		nil, "",
	)

	err := p.Update("New Name", "", "", "COMMERCIAL", "editor")

	require.Error(t, err)
	assert.True(t, errors.Is(err, product.ErrLocked))
}

func TestProduct_Update_InvalidFields(t *testing.T) {
	p := validProduct(t)

	t.Run("empty name", func(t *testing.T) {
		err := p.Update("", "", "", "COMMERCIAL", "editor")
		require.Error(t, err)
		assert.True(t, errors.Is(err, product.ErrInvalidName))
	})

	t.Run("invalid purpose", func(t *testing.T) {
		err := p.Update("Name", "", "", "BADPURPOSE", "editor")
		require.Error(t, err)
		assert.True(t, errors.Is(err, product.ErrInvalidPurpose))
	})
}

// =============================================================================
// Duplicate
// =============================================================================

func TestProduct_Duplicate_Success(t *testing.T) {
	src := validProduct(t)
	opts := product.CopyOptions{IncludeValues: true, IncludeRM: true}

	dup, err := src.Duplicate("PROD-002", "Duplicate Product", "test note", opts, uuid.Nil, "duplicator")

	require.NoError(t, err)
	require.NotNil(t, dup)

	assert.NotEqual(t, src.ID(), dup.ID(), "new product must have a different ID")
	assert.Equal(t, "PROD-002", dup.Code().String())
	assert.Equal(t, "Duplicate Product", dup.Name().String())
	assert.Equal(t, src.ItemCode().String(), dup.ItemCode().String(), "item code must be inherited")
	assert.Equal(t, src.ShadeCode().String(), dup.ShadeCode().String())
	assert.Equal(t, src.ShadeName().String(), dup.ShadeName().String())
	assert.Equal(t, src.CreatedByDeptID(), dup.CreatedByDeptID())
	assert.Equal(t, src.Purpose(), dup.Purpose())
	assert.Equal(t, src.ID(), dup.DuplicatedFromID())
	assert.Equal(t, "test note", dup.DuplicationNote())
	require.NotNil(t, dup.CopiedWithOptions())
	assert.Equal(t, opts, *dup.CopiedWithOptions())
	assert.Equal(t, product.StatusDraft, dup.ProductStatus())
	assert.Equal(t, product.WorkflowDraft, dup.WorkflowStatus())
	assert.Equal(t, "duplicator", dup.CreatedBy())
	assert.False(t, dup.IsDeleted())
}

func TestProduct_Duplicate_FromDeleted(t *testing.T) {
	p := validProduct(t)
	err := p.SoftDelete("admin")
	require.NoError(t, err)

	_, err = p.Duplicate("PROD-002", "Name", "", product.CopyOptions{}, uuid.Nil, "admin")

	require.Error(t, err)
	assert.True(t, errors.Is(err, product.ErrSourceDeleted))
}

func TestProduct_Duplicate_SameCode(t *testing.T) {
	p := validProduct(t)

	_, err := p.Duplicate("PROD-001", "Another Name", "", product.CopyOptions{}, uuid.Nil, "admin")

	require.Error(t, err)
	assert.True(t, errors.Is(err, product.ErrSelfDuplication))
}

// =============================================================================
// SoftDelete
// =============================================================================

func TestProduct_SoftDelete_Twice(t *testing.T) {
	p := validProduct(t)

	err := p.SoftDelete("admin")
	require.NoError(t, err)
	assert.True(t, p.IsDeleted())
	assert.NotNil(t, p.DeletedAt())
	assert.Equal(t, "admin", p.DeletedBy())

	// Second call must return ErrNotFound.
	err = p.SoftDelete("admin")
	require.Error(t, err)
	assert.True(t, errors.Is(err, product.ErrNotFound))
}

// =============================================================================
// WorkflowStatus helpers
// =============================================================================

func TestWorkflowStatus_IsEditable_IsTerminal(t *testing.T) {
	tests := []struct {
		status     product.WorkflowStatus
		isEditable bool
		isTerminal bool
	}{
		{product.WorkflowDraft, true, false},
		{product.WorkflowSubmitted, false, false},
		{product.WorkflowConfirmed, false, false},
		{product.WorkflowLocked, false, true},
		{product.WorkflowUnlockRequested, false, false},
	}

	for _, tc := range tests {
		t.Run(tc.status.String(), func(t *testing.T) {
			assert.Equal(t, tc.isEditable, tc.status.IsEditable())
			assert.Equal(t, tc.isTerminal, tc.status.IsTerminal())
		})
	}
}

func TestWorkflowStatus_NewWorkflowStatus_Valid(t *testing.T) {
	ws, err := product.NewWorkflowStatus("DRAFT")
	require.NoError(t, err)
	assert.Equal(t, product.WorkflowDraft, ws)
}

func TestWorkflowStatus_NewWorkflowStatus_Invalid(t *testing.T) {
	_, err := product.NewWorkflowStatus("UNKNOWN")
	require.Error(t, err)
	assert.True(t, errors.Is(err, product.ErrInvalidWorkflowStatus))
}

// =============================================================================
// CopyOptions
// =============================================================================

func TestCopyOptions_IsAny(t *testing.T) {
	assert.False(t, product.CopyOptions{}.IsAny(), "all false should return false")

	assert.True(t, product.CopyOptions{IncludeValues: true}.IsAny())
	assert.True(t, product.CopyOptions{IncludeRouting: true}.IsAny())
	assert.True(t, product.CopyOptions{IncludeRM: true}.IsAny())
	assert.True(t, product.CopyOptions{IncludeAttachments: true}.IsAny())
	assert.True(t, product.CopyOptions{
		IncludeValues: true, IncludeRouting: true, IncludeRM: true, IncludeAttachments: true,
	}.IsAny())
}

// =============================================================================
// ReconstructProduct
// =============================================================================

func TestReconstructProduct(t *testing.T) {
	id := uuid.New()
	deptID := uuid.New()
	now := time.Now()
	updatedAt := now.Add(time.Hour)
	updatedBy := "editor"
	opts := &product.CopyOptions{IncludeValues: true}

	p := product.ReconstructProduct(
		id,
		"PROD-001", "Product Name", "ITEM-001", "SC01", "Shade",
		"ACTIVE", "CONFIRMED",
		deptID, "DEPT-A",
		"TESTING",
		uuid.Nil, "",
		opts,
		uuid.Nil, 0,
		uuid.Nil,
		nil, "", "",
		2,
		now, "creator",
		&updatedAt, updatedBy,
		nil, "",
	)

	require.NotNil(t, p)
	assert.Equal(t, id, p.ID())
	assert.Equal(t, "PROD-001", p.Code().String())
	assert.Equal(t, "Product Name", p.Name().String())
	assert.Equal(t, product.StatusActive, p.ProductStatus())
	assert.Equal(t, product.WorkflowConfirmed, p.WorkflowStatus())
	assert.Equal(t, product.PurposeTesting, p.Purpose())
	assert.Equal(t, 2, p.UnlockCount())
	assert.Equal(t, "creator", p.CreatedBy())
	assert.Equal(t, updatedBy, p.UpdatedBy())
	assert.Equal(t, opts, p.CopiedWithOptions())
}
