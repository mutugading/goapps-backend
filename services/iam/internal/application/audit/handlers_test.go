// Package audit_test provides unit tests for application layer audit handlers.
package audit_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	appaudit "github.com/mutugading/goapps-backend/services/iam/internal/application/audit"
	domainaudit "github.com/mutugading/goapps-backend/services/iam/internal/domain/audit"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// =============================================================================
// Mock Repository
// =============================================================================

// MockAuditRepository is a mock implementation of audit.Repository.
type MockAuditRepository struct {
	mock.Mock
}

func (m *MockAuditRepository) Create(ctx context.Context, log *domainaudit.Log) error {
	args := m.Called(ctx, log)
	return args.Error(0)
}

func (m *MockAuditRepository) GetByID(ctx context.Context, id uuid.UUID) (*domainaudit.Log, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domainaudit.Log), args.Error(1)
}

func (m *MockAuditRepository) List(ctx context.Context, params domainaudit.ListParams) ([]*domainaudit.Log, int64, error) {
	args := m.Called(ctx, params)
	return args.Get(0).([]*domainaudit.Log), args.Get(1).(int64), args.Error(2)
}

func (m *MockAuditRepository) GetSummary(ctx context.Context, timeRange string, serviceName string) (*domainaudit.Summary, error) {
	args := m.Called(ctx, timeRange, serviceName)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domainaudit.Summary), args.Error(1)
}

// =============================================================================
// Helper
// =============================================================================

func createTestLog(id uuid.UUID) *domainaudit.Log {
	userID := uuid.New()
	recordID := uuid.New()
	return domainaudit.ReconstructLog(
		id,
		domainaudit.EventTypeCreate,
		"mst_users",
		&recordID,
		&userID,
		"admin",
		"Admin User",
		"192.168.1.1",
		"Mozilla/5.0",
		"iam",
		json.RawMessage(`{}`),
		json.RawMessage(`{"name":"test"}`),
		json.RawMessage(`[{"field":"name","old":"","new":"test"}]`),
		time.Now(),
	)
}

// =============================================================================
// GetHandler
// =============================================================================

func TestGetHandler(t *testing.T) {
	t.Run("success - returns audit log by ID", func(t *testing.T) {
		mockRepo := new(MockAuditRepository)
		handler := appaudit.NewGetHandler(mockRepo)
		ctx := context.Background()

		logID := uuid.New()
		expected := createTestLog(logID)

		mockRepo.On("GetByID", ctx, logID).Return(expected, nil)

		query := appaudit.GetQuery{LogID: logID.String()}
		result, err := handler.Handle(ctx, query)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, logID, result.ID())
		assert.Equal(t, domainaudit.EventTypeCreate, result.EventType())
		assert.Equal(t, "mst_users", result.TableName())
		assert.Equal(t, "admin", result.Username())
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - not found", func(t *testing.T) {
		mockRepo := new(MockAuditRepository)
		handler := appaudit.NewGetHandler(mockRepo)
		ctx := context.Background()

		logID := uuid.New()
		mockRepo.On("GetByID", ctx, logID).Return(nil, shared.ErrNotFound)

		query := appaudit.GetQuery{LogID: logID.String()}
		result, err := handler.Handle(ctx, query)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, shared.ErrNotFound)
	})

	t.Run("error - invalid UUID", func(t *testing.T) {
		mockRepo := new(MockAuditRepository)
		handler := appaudit.NewGetHandler(mockRepo)
		ctx := context.Background()

		query := appaudit.GetQuery{LogID: "not-a-valid-uuid"}
		result, err := handler.Handle(ctx, query)

		assert.Nil(t, result)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid log ID")
	})
}

// =============================================================================
// ListHandler
// =============================================================================

func TestListHandler(t *testing.T) {
	t.Run("success - returns paginated results", func(t *testing.T) {
		mockRepo := new(MockAuditRepository)
		handler := appaudit.NewListHandler(mockRepo)
		ctx := context.Background()

		log1 := createTestLog(uuid.New())
		log2 := createTestLog(uuid.New())

		mockRepo.On("List", ctx, mock.AnythingOfType("audit.ListParams")).Return(
			[]*domainaudit.Log{log1, log2},
			int64(2),
			nil,
		)

		query := appaudit.ListQuery{Page: 1, PageSize: 10, EventType: "CREATE"}
		result, err := handler.Handle(ctx, query)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Len(t, result.Logs, 2)
		assert.Equal(t, int64(2), result.TotalItems)
		assert.Equal(t, int32(1), result.CurrentPage)
		assert.Equal(t, int32(10), result.PageSize)
		assert.Equal(t, int32(1), result.TotalPages)
		mockRepo.AssertExpectations(t)
	})

	t.Run("default pagination when values are zero", func(t *testing.T) {
		mockRepo := new(MockAuditRepository)
		handler := appaudit.NewListHandler(mockRepo)
		ctx := context.Background()

		mockRepo.On("List", ctx, mock.MatchedBy(func(p domainaudit.ListParams) bool {
			return p.Page == 1 && p.PageSize == 10
		})).Return(
			[]*domainaudit.Log{},
			int64(0),
			nil,
		)

		query := appaudit.ListQuery{Page: 0, PageSize: 0}
		result, err := handler.Handle(ctx, query)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Empty(t, result.Logs)
		assert.Equal(t, int32(1), result.CurrentPage)
		assert.Equal(t, int32(10), result.PageSize)
		assert.Equal(t, int32(0), result.TotalPages)
		mockRepo.AssertExpectations(t)
	})
}

// =============================================================================
// SummaryHandler
// =============================================================================

func TestSummaryHandler(t *testing.T) {
	t.Run("success - returns summary", func(t *testing.T) {
		mockRepo := new(MockAuditRepository)
		handler := appaudit.NewSummaryHandler(mockRepo)
		ctx := context.Background()

		expected := &domainaudit.Summary{
			TotalEvents:      100,
			LoginCount:       30,
			LoginFailedCount: 5,
			LogoutCount:      20,
			CreateCount:      15,
			UpdateCount:      10,
			DeleteCount:      5,
			ExportCount:      10,
			ImportCount:      5,
			TopUsers: []domainaudit.UserActivity{
				{
					UserID:     uuid.New(),
					Username:   "admin",
					FullName:   "Admin User",
					EventCount: 50,
				},
			},
			EventsByHour: []domainaudit.HourlyCount{
				{Hour: 9, Count: 20},
				{Hour: 10, Count: 15},
			},
		}

		mockRepo.On("GetSummary", ctx, "7d", "iam").Return(expected, nil)

		query := appaudit.SummaryQuery{TimeRange: "7d", ServiceName: "iam"}
		result, err := handler.Handle(ctx, query)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, int64(100), result.TotalEvents)
		assert.Equal(t, int64(30), result.LoginCount)
		assert.Equal(t, int64(5), result.LoginFailedCount)
		assert.Len(t, result.TopUsers, 1)
		assert.Len(t, result.EventsByHour, 2)
		mockRepo.AssertExpectations(t)
	})

	t.Run("default time range when empty", func(t *testing.T) {
		mockRepo := new(MockAuditRepository)
		handler := appaudit.NewSummaryHandler(mockRepo)
		ctx := context.Background()

		expected := &domainaudit.Summary{
			TotalEvents: 50,
		}

		mockRepo.On("GetSummary", ctx, "24h", "").Return(expected, nil)

		query := appaudit.SummaryQuery{TimeRange: "", ServiceName: ""}
		result, err := handler.Handle(ctx, query)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, int64(50), result.TotalEvents)
		mockRepo.AssertExpectations(t)
	})
}
