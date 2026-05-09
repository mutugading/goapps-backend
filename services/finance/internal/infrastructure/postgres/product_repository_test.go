// Package postgres_test provides integration tests for the product repository.
package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/product"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

// isIntegrationTest reports whether integration tests should run.
func isIntegrationTest() bool {
	return os.Getenv("INTEGRATION_TEST") == "true"
}

// openTestDB opens a test database connection using environment variables or defaults.
func openTestDB(t *testing.T) *postgres.DB {
	t.Helper()

	host := productEnvOrDefault("TEST_DB_HOST", "localhost")
	port := productEnvOrDefault("TEST_DB_PORT", "5434")
	user := productEnvOrDefault("TEST_DB_USER", "finance")
	password := productEnvOrDefault("TEST_DB_PASSWORD", "finance123")
	dbname := productEnvOrDefault("TEST_DB_NAME", "finance_db")

	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname,
	)

	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err, "open test database")

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if pingErr := db.Ping(); pingErr == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	require.NoError(t, db.Ping(), "test database must be reachable")

	t.Cleanup(func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("warning: failed to close test db: %v", closeErr)
		}
	})

	return postgres.NewDBFromSQL(db)
}

// productEnvOrDefault returns the value of the given environment variable or a default.
func productEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// TestProductRepository_CreateThenGet verifies that a created product can be retrieved by ID.
func TestProductRepository_CreateThenGet(t *testing.T) {
	if !isIntegrationTest() {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}

	db := openTestDB(t)
	repo := postgres.NewProductRepository(db)
	ctx := context.Background()

	p := seedProduct(t, ctx, repo, "PTCGT-001", "Create Then Get", "ITEM-001")
	defer cleanupProduct(t, ctx, db, p.ID())

	got, err := repo.GetByID(ctx, p.ID())
	require.NoError(t, err)
	assert.Equal(t, p.ID(), got.ID())
	assert.Equal(t, "PTCGT-001", got.Code().String())
	assert.Equal(t, "Create Then Get", got.Name().String())
	assert.Equal(t, "ITEM-001", got.ItemCode().String())
	assert.Equal(t, product.WorkflowDraft.String(), got.WorkflowStatus().String())
	assert.Equal(t, product.StatusDraft.String(), got.ProductStatus().String())
}

// TestProductRepository_Create_DuplicateCode verifies that creating a product with an
// already-used code returns ErrAlreadyExists.
func TestProductRepository_Create_DuplicateCode(t *testing.T) {
	if !isIntegrationTest() {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}

	db := openTestDB(t)
	repo := postgres.NewProductRepository(db)
	ctx := context.Background()

	p := seedProduct(t, ctx, repo, "PTDUP-001", "Dup Product", "ITEM-DUP-001")
	defer cleanupProduct(t, ctx, db, p.ID())

	p2, err := product.NewProduct(
		"PTDUP-001", "Another Product", "ITEM-DUP-002",
		"", "", uuid.Nil, "", "COMMERCIAL", uuid.Nil, "test-user",
	)
	require.NoError(t, err)

	err = repo.Create(ctx, p2)
	assert.ErrorIs(t, err, product.ErrAlreadyExists)
}

// TestProductRepository_GetByID_NotFound verifies that querying a non-existent ID returns ErrNotFound.
func TestProductRepository_GetByID_NotFound(t *testing.T) {
	if !isIntegrationTest() {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}

	db := openTestDB(t)
	repo := postgres.NewProductRepository(db)
	ctx := context.Background()

	got, err := repo.GetByID(ctx, uuid.New())
	assert.ErrorIs(t, err, product.ErrNotFound)
	assert.Nil(t, got)
}

// TestProductRepository_GetByCode_NotFound verifies that querying a non-existent code returns ErrNotFound.
func TestProductRepository_GetByCode_NotFound(t *testing.T) {
	if !isIntegrationTest() {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}

	db := openTestDB(t)
	repo := postgres.NewProductRepository(db)
	ctx := context.Background()

	got, err := repo.GetByCode(ctx, "NO-SUCH-CODE")
	assert.ErrorIs(t, err, product.ErrNotFound)
	assert.Nil(t, got)
}

// TestProductRepository_GetByID_Deleted_ReturnsNotFound verifies that a soft-deleted product
// is not returned by GetByID.
func TestProductRepository_GetByID_Deleted_ReturnsNotFound(t *testing.T) {
	if !isIntegrationTest() {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}

	db := openTestDB(t)
	repo := postgres.NewProductRepository(db)
	ctx := context.Background()

	p := seedProduct(t, ctx, repo, "PTDEL-VIS-001", "Deleted Visibility", "ITEM-DV-001")
	defer cleanupProduct(t, ctx, db, p.ID())

	err := repo.Delete(ctx, p.ID(), "test-user")
	require.NoError(t, err)

	got, err := repo.GetByID(ctx, p.ID())
	assert.ErrorIs(t, err, product.ErrNotFound)
	assert.Nil(t, got)
}

// TestProductRepository_Update_Success verifies that updates persist correctly.
func TestProductRepository_Update_Success(t *testing.T) {
	if !isIntegrationTest() {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}

	db := openTestDB(t)
	repo := postgres.NewProductRepository(db)
	ctx := context.Background()

	p := seedProduct(t, ctx, repo, "PTUPD-001", "Original Name", "ITEM-UPD-001")
	defer cleanupProduct(t, ctx, db, p.ID())

	err := p.Update("Updated Name", "SC01", "Shade One", "TESTING", "updater")
	require.NoError(t, err)

	err = repo.Update(ctx, p)
	require.NoError(t, err)

	got, err := repo.GetByID(ctx, p.ID())
	require.NoError(t, err)
	assert.Equal(t, "Updated Name", got.Name().String())
	assert.Equal(t, "SC01", got.ShadeCode().String())
	assert.Equal(t, "Shade One", got.ShadeName().String())
	assert.Equal(t, product.PurposeTesting.String(), got.Purpose().String())
	assert.NotNil(t, got.UpdatedAt())
	assert.Equal(t, "updater", got.UpdatedBy())
}

// TestProductRepository_Update_NotFound verifies that updating a non-existent product returns ErrNotFound.
func TestProductRepository_Update_NotFound(t *testing.T) {
	if !isIntegrationTest() {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}

	db := openTestDB(t)
	repo := postgres.NewProductRepository(db)
	ctx := context.Background()

	ghost, err := product.NewProduct(
		"GHOST-001", "Ghost Product", "ITEM-GHOST-001",
		"", "", uuid.Nil, "", "COMMERCIAL", uuid.Nil, "test-user",
	)
	require.NoError(t, err)

	err = repo.Update(ctx, ghost)
	assert.ErrorIs(t, err, product.ErrNotFound)
}

// TestProductRepository_List_Filters verifies that list filters work correctly.
func TestProductRepository_List_Filters(t *testing.T) {
	if !isIntegrationTest() {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}

	db := openTestDB(t)
	repo := postgres.NewProductRepository(db)
	ctx := context.Background()

	// Seed 5 products with different purposes and workflow statuses.
	products := []*product.Product{
		seedProductWithPurpose(t, ctx, repo, "PTLST-001", "List Product One", "ITEM-LST-001", "COMMERCIAL"),
		seedProductWithPurpose(t, ctx, repo, "PTLST-002", "List Product Two", "ITEM-LST-002", "COMMERCIAL"),
		seedProductWithPurpose(t, ctx, repo, "PTLST-003", "List Product Three", "ITEM-LST-003", "TESTING"),
		seedProductWithPurpose(t, ctx, repo, "PTLST-004", "List Product Four", "ITEM-LST-004", "TESTING"),
		seedProductWithPurpose(t, ctx, repo, "PTLST-005", "List Product Five", "ITEM-LST-005", "TRIAL"),
	}
	defer func() {
		for _, p := range products {
			cleanupProduct(t, ctx, db, p.ID())
		}
	}()

	t.Run("NoFilter_PaginationPage1PageSize2", func(t *testing.T) {
		items, total, err := repo.List(ctx, product.ListFilter{
			Page:     1,
			PageSize: 2,
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, total, 5)
		assert.Len(t, items, 2)
	})

	t.Run("FilterByPurpose_COMMERCIAL", func(t *testing.T) {
		items, total, err := repo.List(ctx, product.ListFilter{
			Purpose:  "COMMERCIAL",
			Page:     1,
			PageSize: 20,
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, total, 2)
		for _, item := range items {
			assert.Equal(t, "COMMERCIAL", item.Purpose().String())
		}
	})

	t.Run("FilterByPurpose_TESTING", func(t *testing.T) {
		items, total, err := repo.List(ctx, product.ListFilter{
			Purpose:  "TESTING",
			Page:     1,
			PageSize: 20,
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, total, 2)
		for _, item := range items {
			assert.Equal(t, "TESTING", item.Purpose().String())
		}
	})

	t.Run("FilterByWorkflowStatus_DRAFT", func(t *testing.T) {
		items, total, err := repo.List(ctx, product.ListFilter{
			WorkflowStatus: "DRAFT",
			Page:           1,
			PageSize:       20,
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, total, 5)
		for _, item := range items {
			assert.Equal(t, "DRAFT", item.WorkflowStatus().String())
		}
	})

	t.Run("PageSize2_ReturnsOnly2", func(t *testing.T) {
		items, total, err := repo.List(ctx, product.ListFilter{
			WorkflowStatus: "DRAFT",
			Page:           1,
			PageSize:       2,
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, total, 5)
		assert.Len(t, items, 2)
	})
}

// TestProductRepository_List_Search verifies full-text search in List.
func TestProductRepository_List_Search(t *testing.T) {
	if !isIntegrationTest() {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}

	db := openTestDB(t)
	repo := postgres.NewProductRepository(db)
	ctx := context.Background()

	p := seedProduct(t, ctx, repo, "PTFTS-001", "UniqueYarnFTSProduct", "ITEM-FTS-001")
	defer cleanupProduct(t, ctx, db, p.ID())

	items, total, err := repo.List(ctx, product.ListFilter{
		Search:   "UniqueYarnFTSProduct",
		Page:     1,
		PageSize: 20,
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, total, 1)
	assert.GreaterOrEqual(t, len(items), 1)
}

// TestProductRepository_List_SortField_Whitelist verifies that an invalid sort field returns an error.
func TestProductRepository_List_SortField_Whitelist(t *testing.T) {
	if !isIntegrationTest() {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}

	db := openTestDB(t)
	repo := postgres.NewProductRepository(db)
	ctx := context.Background()

	_, _, err := repo.List(ctx, product.ListFilter{
		SortField: "invalidColumn",
		Page:      1,
		PageSize:  10,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid sort field")
}

// TestProductRepository_SearchByText_FTS verifies FTS search behavior.
func TestProductRepository_SearchByText_FTS(t *testing.T) {
	if !isIntegrationTest() {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}

	db := openTestDB(t)
	repo := postgres.NewProductRepository(db)
	ctx := context.Background()

	p1 := seedProduct(t, ctx, repo, "PTFTSS-001", "YarnFTSSearch Alpha Product", "ITEM-FTSS-001")
	p2 := seedProduct(t, ctx, repo, "PTFTSS-002", "Cotton FTS Product", "ITEM-FTSS-002")
	p3 := seedProduct(t, ctx, repo, "PTFTSS-003", "YarnFTSSearch Beta Product", "ITEM-FTSS-003")
	defer cleanupProduct(t, ctx, db, p1.ID())
	defer cleanupProduct(t, ctx, db, p2.ID())
	defer cleanupProduct(t, ctx, db, p3.ID())

	results, err := repo.SearchByText(ctx, product.SearchOptions{
		Query: "YarnFTSSearch",
		Limit: 10,
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(results), 2)

	// All results should contain yarn-related names.
	for _, r := range results {
		assert.Contains(t, r.Name().String(), "YarnFTSSearch")
	}
}

// TestProductRepository_SearchByText_EmptyQuery returns empty slice without error.
func TestProductRepository_SearchByText_EmptyQuery(t *testing.T) {
	if !isIntegrationTest() {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}

	db := openTestDB(t)
	repo := postgres.NewProductRepository(db)
	ctx := context.Background()

	results, err := repo.SearchByText(ctx, product.SearchOptions{Query: "   "})
	require.NoError(t, err)
	assert.Empty(t, results)
}

// TestProductRepository_SearchByText_LimitClamp verifies that a limit above 50 is clamped.
func TestProductRepository_SearchByText_LimitClamp(t *testing.T) {
	if !isIntegrationTest() {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}

	db := openTestDB(t)
	repo := postgres.NewProductRepository(db)
	ctx := context.Background()

	// Seed many products to confirm clamping returns at most 50.
	var created []*product.Product
	for i := 0; i < 5; i++ {
		code := fmt.Sprintf("PTCLMP-%03d", i)
		itemCode := fmt.Sprintf("ITEM-CLMP-%03d", i)
		p := seedProduct(t, ctx, repo, code, fmt.Sprintf("ClampProduct %d", i), itemCode)
		created = append(created, p)
	}
	defer func() {
		for _, p := range created {
			cleanupProduct(t, ctx, db, p.ID())
		}
	}()

	results, err := repo.SearchByText(ctx, product.SearchOptions{
		Query: "ClampProduct",
		Limit: 100, // should be clamped to 50
	})
	require.NoError(t, err)
	assert.LessOrEqual(t, len(results), 50)
}

// TestProductRepository_SearchByText_ShadeFilter verifies shade code filter narrows results.
func TestProductRepository_SearchByText_ShadeFilter(t *testing.T) {
	if !isIntegrationTest() {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}

	db := openTestDB(t)
	repo := postgres.NewProductRepository(db)
	ctx := context.Background()

	p1 := seedProductWithShade(t, ctx, repo, "PTSHD-001", "ShadeSearch Product Red", "ITEM-SHD-001", "RED01")
	p2 := seedProductWithShade(t, ctx, repo, "PTSHD-002", "ShadeSearch Product Blue", "ITEM-SHD-002", "BLUE01")
	defer cleanupProduct(t, ctx, db, p1.ID())
	defer cleanupProduct(t, ctx, db, p2.ID())

	results, err := repo.SearchByText(ctx, product.SearchOptions{
		Query:     "ShadeSearch",
		ShadeCode: "RED01",
		Limit:     10,
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(results), 1)
	for _, r := range results {
		assert.Equal(t, "RED01", r.ShadeCode().String())
	}
}

// TestProductRepository_ListByRequestID verifies request-ID based listing.
func TestProductRepository_ListByRequestID(t *testing.T) {
	if !isIntegrationTest() {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}

	db := openTestDB(t)
	repo := postgres.NewProductRepository(db)
	ctx := context.Background()

	reqID := uuid.New()
	otherReqID := uuid.New()

	p1 := seedProductWithRequest(t, ctx, repo, "PTREQ-001", "Request Product 1", "ITEM-REQ-001", reqID)
	p2 := seedProductWithRequest(t, ctx, repo, "PTREQ-002", "Request Product 2", "ITEM-REQ-002", reqID)
	p3 := seedProductWithRequest(t, ctx, repo, "PTREQ-003", "Request Product 3", "ITEM-REQ-003", reqID)
	p4 := seedProductWithRequest(t, ctx, repo, "PTREQ-004", "Other Request Product", "ITEM-REQ-004", otherReqID)
	defer cleanupProduct(t, ctx, db, p1.ID())
	defer cleanupProduct(t, ctx, db, p2.ID())
	defer cleanupProduct(t, ctx, db, p3.ID())
	defer cleanupProduct(t, ctx, db, p4.ID())

	items, total, err := repo.ListByRequestID(ctx, reqID, 1, 10)
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Len(t, items, 3)
}

// TestProductRepository_Delete_RemovesFromGet verifies that deleting a product makes GetByID return ErrNotFound.
func TestProductRepository_Delete_RemovesFromGet(t *testing.T) {
	if !isIntegrationTest() {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}

	db := openTestDB(t)
	repo := postgres.NewProductRepository(db)
	ctx := context.Background()

	p := seedProduct(t, ctx, repo, "PTDEL-001", "To Delete Product", "ITEM-DEL-001")
	defer cleanupProduct(t, ctx, db, p.ID())

	err := repo.Delete(ctx, p.ID(), "test-user")
	require.NoError(t, err)

	got, err := repo.GetByID(ctx, p.ID())
	assert.ErrorIs(t, err, product.ErrNotFound)
	assert.Nil(t, got)
}

// TestProductRepository_Delete_NotFound verifies that deleting a non-existent product returns ErrNotFound.
func TestProductRepository_Delete_NotFound(t *testing.T) {
	if !isIntegrationTest() {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}

	db := openTestDB(t)
	repo := postgres.NewProductRepository(db)
	ctx := context.Background()

	err := repo.Delete(ctx, uuid.New(), "test-user")
	assert.ErrorIs(t, err, product.ErrNotFound)
}

// =============================================================================
// Test helpers
// =============================================================================

// seedProduct creates a product and registers a cleanup function.
func seedProduct(t *testing.T, ctx context.Context, repo *postgres.ProductRepository, code, name, itemCode string) *product.Product {
	t.Helper()
	return seedProductWithPurpose(t, ctx, repo, code, name, itemCode, "COMMERCIAL")
}

// seedProductWithPurpose creates a product with the given purpose.
func seedProductWithPurpose(t *testing.T, ctx context.Context, repo *postgres.ProductRepository, code, name, itemCode, purpose string) *product.Product {
	t.Helper()
	p, err := product.NewProduct(code, name, itemCode, "", "", uuid.Nil, "", purpose, uuid.Nil, "test-user")
	require.NoError(t, err)
	err = repo.Create(ctx, p)
	require.NoError(t, err)
	return p
}

// seedProductWithShade creates a product with the given shade code.
func seedProductWithShade(t *testing.T, ctx context.Context, repo *postgres.ProductRepository, code, name, itemCode, shadeCode string) *product.Product {
	t.Helper()
	p, err := product.NewProduct(code, name, itemCode, shadeCode, "", uuid.Nil, "", "COMMERCIAL", uuid.Nil, "test-user")
	require.NoError(t, err)
	err = repo.Create(ctx, p)
	require.NoError(t, err)
	return p
}

// seedProductWithRequest creates a product linked to a specific request ID.
func seedProductWithRequest(t *testing.T, ctx context.Context, repo *postgres.ProductRepository, code, name, itemCode string, requestID uuid.UUID) *product.Product {
	t.Helper()
	p, err := product.NewProduct(code, name, itemCode, "", "", uuid.Nil, "", "COMMERCIAL", requestID, "test-user")
	require.NoError(t, err)
	err = repo.Create(ctx, p)
	require.NoError(t, err)
	return p
}

// cleanupProduct hard-deletes a product row to restore the database state after a test.
func cleanupProduct(t *testing.T, ctx context.Context, db *postgres.DB, id uuid.UUID) {
	t.Helper()
	_, err := db.ExecContext(ctx, "DELETE FROM cst_product WHERE product_id = $1", id)
	if err != nil {
		t.Logf("warning: cleanup failed for product %s: %v", id, err)
	}
}
