package costproductrequest_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	app "github.com/mutugading/goapps-backend/services/finance/internal/application/costproductrequest"
)

func TestTemplateHandler_Handle(t *testing.T) {
	t.Run("returns the expected D6 header row exactly", func(t *testing.T) {
		h := app.NewTemplateHandler()

		result, err := h.Handle()
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.NotEmpty(t, result.FileContent)
		assert.Equal(t, "cost_product_request_import_template.xlsx", result.FileName)

		f, err := excelize.OpenReader(bytes.NewReader(result.FileContent))
		require.NoError(t, err)
		defer func() { _ = f.Close() }()

		sheets := f.GetSheetList()
		require.Contains(t, sheets, "CPR Import Template")
		rows, err := f.GetRows("CPR Import Template")
		require.NoError(t, err)
		require.NotEmpty(t, rows)

		assert.Equal(t, []string{
			"Request type", "Title", "Description", "Customer name", "Customer code",
			"Urgency", "Needed by (YYYY-MM-DD)", "Product description", "Shade code",
			"Shade name", "Tube (Paper/Plastic)", "Reference product", "Target volume",
			"Target price range",
		}, rows[0])
	})

	t.Run("includes an example row after the header", func(t *testing.T) {
		h := app.NewTemplateHandler()

		result, err := h.Handle()
		require.NoError(t, err)

		f, err := excelize.OpenReader(bytes.NewReader(result.FileContent))
		require.NoError(t, err)
		defer func() { _ = f.Close() }()
		rows, err := f.GetRows("CPR Import Template")
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(rows), 2)
		assert.NotEmpty(t, rows[1])
	})

	t.Run("includes an instructions sheet noting create-only semantics", func(t *testing.T) {
		h := app.NewTemplateHandler()

		result, err := h.Handle()
		require.NoError(t, err)

		f, err := excelize.OpenReader(bytes.NewReader(result.FileContent))
		require.NoError(t, err)
		defer func() { _ = f.Close() }()
		assert.Contains(t, f.GetSheetList(), "Instructions")
	})
}
