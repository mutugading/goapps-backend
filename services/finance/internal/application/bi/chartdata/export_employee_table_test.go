package chartdata

import (
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/chart"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/factmetric"
)

// ShapeEmployeeTableForTest exposes shapeEmployeeTable to the external test package.
func ShapeEmployeeTableForTest(rows []factmetric.AggRow) ([]Series, []string) {
	return shapeEmployeeTable(rows, chart.NumberFormat("raw"), 2)
}
