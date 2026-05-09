// Package product_test contains application-layer handler tests for the Product aggregate.
package product_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/product"
	domainproduct "github.com/mutugading/goapps-backend/services/finance/internal/domain/product"
)

// =============================================================================
// Mock Repository
// =============================================================================

// MockRepository is a mock implementation of domainproduct.Repository.
type MockRepository struct {
	mock.Mock
}

func (m *MockRepository) Create(ctx context.Context, p *domainproduct.Product) error {
	args := m.Called(ctx, p)
	return args.Error(0)
}

func (m *MockRepository) GetByID(ctx context.Context, id uuid.UUID) (*domainproduct.Product, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domainproduct.Product), args.Error(1)
}

func (m *MockRepository) GetByCode(ctx context.Context, code string) (*domainproduct.Product, error) {
	args := m.Called(ctx, code)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domainproduct.Product), args.Error(1)
}

func (m *MockRepository) List(ctx context.Context, f domainproduct.ListFilter) ([]*domainproduct.Product, int, error) {
	args := m.Called(ctx, f)
	return args.Get(0).([]*domainproduct.Product), args.Int(1), args.Error(2)
}

func (m *MockRepository) Update(ctx context.Context, p *domainproduct.Product) error {
	args := m.Called(ctx, p)
	return args.Error(0)
}

func (m *MockRepository) Delete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	args := m.Called(ctx, id, deletedBy)
	return args.Error(0)
}

func (m *MockRepository) SearchByText(ctx context.Context, opts domainproduct.SearchOptions) ([]*domainproduct.Product, error) {
	args := m.Called(ctx, opts)
	return args.Get(0).([]*domainproduct.Product), args.Error(1)
}

func (m *MockRepository) ListByRequestID(ctx context.Context, requestID uuid.UUID, page, pageSize int) ([]*domainproduct.Product, int, error) {
	args := m.Called(ctx, requestID, page, pageSize)
	return args.Get(0).([]*domainproduct.Product), args.Int(1), args.Error(2)
}

// =============================================================================
// Helpers
// =============================================================================

// newValidProduct builds a valid Product for use in tests.
func newValidProduct(t *testing.T) *domainproduct.Product {
	t.Helper()
	p, err := domainproduct.NewProduct(
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

// newLockedProduct builds a Product that is in LOCKED workflow state via ReconstructProduct.
func newLockedProduct(t *testing.T) *domainproduct.Product {
	t.Helper()
	now := time.Now().UTC()
	return domainproduct.ReconstructProduct(
		uuid.New(),
		"PROD-LOCK", "Locked Product", "ITEM-LOCK",
		"", "",
		"DRAFT", "LOCKED",
		uuid.New(), "DEPT-A",
		"COMMERCIAL",
		uuid.Nil, "",
		nil,
		uuid.Nil, 0,
		uuid.Nil,
		&now, "admin", "202504",
		0,
		time.Now().UTC(), "admin",
		nil, "",
		nil, "",
	)
}

// newDeletedProduct builds a Product that has been soft-deleted via ReconstructProduct.
func newDeletedProduct(t *testing.T) *domainproduct.Product {
	t.Helper()
	now := time.Now().UTC()
	return domainproduct.ReconstructProduct(
		uuid.New(),
		"PROD-DEL", "Deleted Product", "ITEM-DEL",
		"", "",
		"DRAFT", "DRAFT",
		uuid.New(), "DEPT-A",
		"COMMERCIAL",
		uuid.Nil, "",
		nil,
		uuid.Nil, 0,
		uuid.Nil,
		nil, "", "",
		0,
		time.Now().UTC(), "admin",
		nil, "",
		&now, "admin",
	)
}

// =============================================================================
// CreateHandler tests
// =============================================================================

func TestCreateHandler_Handle_Success(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := product.NewCreateHandler(mockRepo)
	ctx := context.Background()

	cmd := product.CreateCommand{
		Code:      "PROD-001",
		Name:      "Test Product",
		ItemCode:  "ITEM-001",
		ShadeCode: "SC01",
		ShadeName: "Shade One",
		DeptID:    uuid.New(),
		DeptCode:  "DEPT-A",
		Purpose:   "COMMERCIAL",
		CreatedBy: "admin",
	}

	mockRepo.On("Create", ctx, mock.AnythingOfType("*product.Product")).Return(nil)

	result, err := handler.Handle(ctx, cmd)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "PROD-001", result.Code().String())
	assert.Equal(t, "Test Product", result.Name().String())
	assert.NotEqual(t, uuid.Nil, result.ID())
	mockRepo.AssertExpectations(t)
}

func TestCreateHandler_Handle_InvalidPurpose(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := product.NewCreateHandler(mockRepo)
	ctx := context.Background()

	cmd := product.CreateCommand{
		Code:      "PROD-001",
		Name:      "Test Product",
		ItemCode:  "ITEM-001",
		DeptID:    uuid.New(),
		DeptCode:  "DEPT-A",
		Purpose:   "INVALID_PURPOSE",
		CreatedBy: "admin",
	}

	result, err := handler.Handle(ctx, cmd)

	assert.Nil(t, result)
	require.Error(t, err)
	assert.ErrorIs(t, err, domainproduct.ErrInvalidPurpose)
	mockRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestCreateHandler_Handle_RepoError(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := product.NewCreateHandler(mockRepo)
	ctx := context.Background()

	cmd := product.CreateCommand{
		Code:      "PROD-001",
		Name:      "Test Product",
		ItemCode:  "ITEM-001",
		DeptID:    uuid.New(),
		DeptCode:  "DEPT-A",
		Purpose:   "COMMERCIAL",
		CreatedBy: "admin",
	}

	mockRepo.On("Create", ctx, mock.AnythingOfType("*product.Product")).Return(domainproduct.ErrAlreadyExists)

	result, err := handler.Handle(ctx, cmd)

	assert.Nil(t, result)
	require.Error(t, err)
	assert.ErrorIs(t, err, domainproduct.ErrAlreadyExists)
	mockRepo.AssertExpectations(t)
}

// =============================================================================
// GetHandler tests
// =============================================================================

func TestGetHandler_Handle_Success(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := product.NewGetHandler(mockRepo)
	ctx := context.Background()

	p := newValidProduct(t)
	mockRepo.On("GetByID", ctx, p.ID()).Return(p, nil)

	result, err := handler.Handle(ctx, product.GetCommand{ID: p.ID()})

	require.NoError(t, err)
	assert.Equal(t, p.ID(), result.ID())
	mockRepo.AssertExpectations(t)
}

func TestGetHandler_Handle_NotFound(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := product.NewGetHandler(mockRepo)
	ctx := context.Background()

	id := uuid.New()
	mockRepo.On("GetByID", ctx, id).Return(nil, domainproduct.ErrNotFound)

	result, err := handler.Handle(ctx, product.GetCommand{ID: id})

	assert.Nil(t, result)
	require.Error(t, err)
	assert.ErrorIs(t, err, domainproduct.ErrNotFound)
	mockRepo.AssertExpectations(t)
}

// =============================================================================
// ListHandler tests
// =============================================================================

func TestListHandler_Handle_Success(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := product.NewListHandler(mockRepo)
	ctx := context.Background()

	p1 := newValidProduct(t)
	p2, err := domainproduct.NewProduct(
		"PROD-002", "Another Product", "ITEM-002",
		"", "",
		uuid.New(), "DEPT-B",
		"TESTING",
		uuid.Nil, "admin",
	)
	require.NoError(t, err)

	mockRepo.On("List", ctx, mock.AnythingOfType("product.ListFilter")).
		Return([]*domainproduct.Product{p1, p2}, 2, nil)

	q := product.ListQuery{Page: 1, PageSize: 10}
	result, err := handler.Handle(ctx, q)

	require.NoError(t, err)
	assert.Len(t, result.Products, 2)
	assert.Equal(t, 2, result.TotalItems)
	assert.Equal(t, int32(1), result.TotalPages)
	assert.Equal(t, int32(1), result.CurrentPage)
	assert.Equal(t, int32(10), result.PageSize)
	mockRepo.AssertExpectations(t)
}

func TestListHandler_Handle_RepoError(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := product.NewListHandler(mockRepo)
	ctx := context.Background()

	repoErr := errors.New("database error")
	mockRepo.On("List", ctx, mock.AnythingOfType("product.ListFilter")).
		Return([]*domainproduct.Product{}, 0, repoErr)

	result, err := handler.Handle(ctx, product.ListQuery{Page: 1, PageSize: 10})

	assert.Nil(t, result)
	require.Error(t, err)
	assert.ErrorIs(t, err, repoErr)
	mockRepo.AssertExpectations(t)
}

// =============================================================================
// UpdateHandler tests
// =============================================================================

func TestUpdateHandler_Handle_Success(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := product.NewUpdateHandler(mockRepo)
	ctx := context.Background()

	p := newValidProduct(t)
	mockRepo.On("GetByID", ctx, p.ID()).Return(p, nil)
	mockRepo.On("Update", ctx, mock.AnythingOfType("*product.Product")).Return(nil)

	cmd := product.UpdateCommand{
		ID:        p.ID(),
		Name:      "Updated Name",
		ShadeCode: "SC02",
		ShadeName: "Shade Two",
		Purpose:   "TESTING",
		UpdatedBy: "admin",
	}

	result, err := handler.Handle(ctx, cmd)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "Updated Name", result.Name().String())
	mockRepo.AssertExpectations(t)
}

func TestUpdateHandler_Handle_NotFound(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := product.NewUpdateHandler(mockRepo)
	ctx := context.Background()

	id := uuid.New()
	mockRepo.On("GetByID", ctx, id).Return(nil, domainproduct.ErrNotFound)

	result, err := handler.Handle(ctx, product.UpdateCommand{ID: id, Name: "X", Purpose: "COMMERCIAL", UpdatedBy: "admin"})

	assert.Nil(t, result)
	require.Error(t, err)
	assert.ErrorIs(t, err, domainproduct.ErrNotFound)
	mockRepo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
}

func TestUpdateHandler_Handle_LockedReturnsErrLocked(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := product.NewUpdateHandler(mockRepo)
	ctx := context.Background()

	locked := newLockedProduct(t)
	mockRepo.On("GetByID", ctx, locked.ID()).Return(locked, nil)

	cmd := product.UpdateCommand{
		ID:        locked.ID(),
		Name:      "New Name",
		Purpose:   "COMMERCIAL",
		UpdatedBy: "admin",
	}

	result, err := handler.Handle(ctx, cmd)

	assert.Nil(t, result)
	require.Error(t, err)
	assert.ErrorIs(t, err, domainproduct.ErrLocked)
	mockRepo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
}

func TestUpdateHandler_Handle_InvalidPurpose(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := product.NewUpdateHandler(mockRepo)
	ctx := context.Background()

	p := newValidProduct(t)
	mockRepo.On("GetByID", ctx, p.ID()).Return(p, nil)

	cmd := product.UpdateCommand{
		ID:        p.ID(),
		Name:      "Valid Name",
		Purpose:   "INVALID",
		UpdatedBy: "admin",
	}

	result, err := handler.Handle(ctx, cmd)

	assert.Nil(t, result)
	require.Error(t, err)
	assert.ErrorIs(t, err, domainproduct.ErrInvalidPurpose)
	mockRepo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
}

// =============================================================================
// DeleteHandler tests
// =============================================================================

func TestDeleteHandler_Handle_Success(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := product.NewDeleteHandler(mockRepo)
	ctx := context.Background()

	id := uuid.New()
	mockRepo.On("Delete", ctx, id, "admin").Return(nil)

	err := handler.Handle(ctx, product.DeleteCommand{ID: id, DeletedBy: "admin"})

	assert.NoError(t, err)
	mockRepo.AssertExpectations(t)
}

func TestDeleteHandler_Handle_NotFound(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := product.NewDeleteHandler(mockRepo)
	ctx := context.Background()

	id := uuid.New()
	mockRepo.On("Delete", ctx, id, "admin").Return(domainproduct.ErrNotFound)

	err := handler.Handle(ctx, product.DeleteCommand{ID: id, DeletedBy: "admin"})

	require.Error(t, err)
	assert.ErrorIs(t, err, domainproduct.ErrNotFound)
	mockRepo.AssertExpectations(t)
}

// =============================================================================
// DuplicateHandler tests
// =============================================================================

func TestDuplicateHandler_Handle_Success(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := product.NewDuplicateHandler(mockRepo)
	ctx := context.Background()

	source := newValidProduct(t)
	mockRepo.On("GetByID", ctx, source.ID()).Return(source, nil)
	mockRepo.On("Create", ctx, mock.AnythingOfType("*product.Product")).Return(nil)

	cmd := product.DuplicateCommand{
		SourceID:        source.ID(),
		NewCode:         "PROD-COPY",
		NewName:         "Copy of Test Product",
		DuplicationNote: "Duplicate for testing.",
		Options:         domainproduct.CopyOptions{IncludeValues: true},
		CreatedBy:       "admin",
	}

	result, err := handler.Handle(ctx, cmd)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotEqual(t, source.ID(), result.ID())
	assert.Equal(t, "PROD-COPY", result.Code().String())
	assert.Equal(t, source.ID(), result.DuplicatedFromID())
	assert.Equal(t, "ITEM-001", result.ItemCode().String())
	mockRepo.AssertExpectations(t)
}

func TestDuplicateHandler_Handle_SourceNotFound(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := product.NewDuplicateHandler(mockRepo)
	ctx := context.Background()

	id := uuid.New()
	mockRepo.On("GetByID", ctx, id).Return(nil, domainproduct.ErrNotFound)

	result, err := handler.Handle(ctx, product.DuplicateCommand{
		SourceID:  id,
		NewCode:   "PROD-COPY",
		NewName:   "Copy",
		CreatedBy: "admin",
	})

	assert.Nil(t, result)
	require.Error(t, err)
	assert.ErrorIs(t, err, domainproduct.ErrNotFound)
	mockRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestDuplicateHandler_Handle_SourceDeleted(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := product.NewDuplicateHandler(mockRepo)
	ctx := context.Background()

	deleted := newDeletedProduct(t)
	mockRepo.On("GetByID", ctx, deleted.ID()).Return(deleted, nil)

	result, err := handler.Handle(ctx, product.DuplicateCommand{
		SourceID:  deleted.ID(),
		NewCode:   "PROD-COPY",
		NewName:   "Copy",
		CreatedBy: "admin",
	})

	assert.Nil(t, result)
	require.Error(t, err)
	assert.ErrorIs(t, err, domainproduct.ErrSourceDeleted)
	mockRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestDuplicateHandler_Handle_SelfDuplication(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := product.NewDuplicateHandler(mockRepo)
	ctx := context.Background()

	source := newValidProduct(t)
	mockRepo.On("GetByID", ctx, source.ID()).Return(source, nil)

	// Use the same code as the source.
	result, err := handler.Handle(ctx, product.DuplicateCommand{
		SourceID:  source.ID(),
		NewCode:   source.Code().String(),
		NewName:   "Same Code Copy",
		CreatedBy: "admin",
	})

	assert.Nil(t, result)
	require.Error(t, err)
	assert.ErrorIs(t, err, domainproduct.ErrSelfDuplication)
	mockRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestDuplicateHandler_Handle_OptionsPropagated(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := product.NewDuplicateHandler(mockRepo)
	ctx := context.Background()

	source := newValidProduct(t)
	opts := domainproduct.CopyOptions{
		IncludeValues:      true,
		IncludeRouting:     true,
		IncludeRM:          false,
		IncludeAttachments: false,
	}

	mockRepo.On("GetByID", ctx, source.ID()).Return(source, nil)
	mockRepo.On("Create", ctx, mock.AnythingOfType("*product.Product")).Return(nil)

	result, err := handler.Handle(ctx, product.DuplicateCommand{
		SourceID:  source.ID(),
		NewCode:   "PROD-OPTS",
		NewName:   "Options Test",
		Options:   opts,
		CreatedBy: "admin",
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.CopiedWithOptions())
	assert.Equal(t, opts, *result.CopiedWithOptions())
	mockRepo.AssertExpectations(t)
}
