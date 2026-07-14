package mbpush

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

// formatCostValue renders a cost-per-unit float as a fixed-decimal string matching
// cst_mb_cost.mbc_cost_value NUMERIC(20,6) — %v/%g can emit scientific notation for extreme
// magnitudes, which is not valid input for a NUMERIC column parameter.
func formatCostValue(v float64) string {
	return strconv.FormatFloat(v, 'f', 6, 64)
}

// MBHeadReaderAdapter adapts *postgres.MBHeadRepository to MBHeadReader. Kept in the application
// layer (importing infrastructure directly) per the rmcost/costimportetl precedent — no domain
// package wraps cst_mb_cost/mb_head push-eligibility concerns, so a full domain abstraction would
// be premature for this one narrow read.
type MBHeadReaderAdapter struct {
	repo *postgres.MBHeadRepository
}

// NewMBHeadReaderAdapter constructs an MBHeadReaderAdapter.
func NewMBHeadReaderAdapter(repo *postgres.MBHeadRepository) *MBHeadReaderAdapter {
	return &MBHeadReaderAdapter{repo: repo}
}

var _ MBHeadReader = (*MBHeadReaderAdapter)(nil)

// ListValidated maps postgres.MBHeadCandidate rows into mbpush's own port type.
func (a *MBHeadReaderAdapter) ListValidated(ctx context.Context) ([]MBHeadCandidate, error) {
	rows, err := a.repo.ListValidated(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]MBHeadCandidate, len(rows))
	for i, r := range rows {
		out[i] = MBHeadCandidate{
			MBHID:         r.MBHID,
			Code:          r.Code,
			Name:          r.Name,
			CostProductID: r.CostProductID,
			IsBoughtout:   r.IsBoughtout,
		}
	}
	return out, nil
}

// CostReaderAdapter adapts *postgres.CostResultRepository to CostReader.
type CostReaderAdapter struct {
	repo *postgres.CostResultRepository
}

// NewCostReaderAdapter constructs a CostReaderAdapter.
func NewCostReaderAdapter(repo *postgres.CostResultRepository) *CostReaderAdapter {
	return &CostReaderAdapter{repo: repo}
}

var _ CostReader = (*CostReaderAdapter)(nil)

// GetActiveCalculated returns the active CALCULATED cost row for the tuple. Not-found or a
// status other than CALCULATED both surface as found=false, err=nil — a normal skip path.
func (a *CostReaderAdapter) GetActiveCalculated(ctx context.Context, productSysID int64, period, calcType string) (int64, string, bool, error) {
	res, err := a.repo.GetActive(ctx, productSysID, period, costcalc.CalculationType(calcType))
	if errors.Is(err, costcalc.ErrCostNotFound) {
		return 0, "", false, nil
	}
	if err != nil {
		return 0, "", false, fmt.Errorf("get active calculated cost: %w", err)
	}
	if res.Status() != costcalc.ResultStatusCalculated {
		return 0, "", false, nil
	}
	return res.ID(), formatCostValue(res.CostPerUnit()), true, nil
}

// GetActiveCalculatedTx is the transaction-scoped variant, used inside Execute's savepoint scope.
func (a *CostReaderAdapter) GetActiveCalculatedTx(ctx context.Context, tx *sql.Tx, productSysID int64, period, calcType string) (int64, string, bool, error) {
	res, err := a.repo.GetActiveTx(ctx, tx, productSysID, period, costcalc.CalculationType(calcType))
	if errors.Is(err, costcalc.ErrCostNotFound) {
		return 0, "", false, nil
	}
	if err != nil {
		return 0, "", false, fmt.Errorf("get active calculated cost tx: %w", err)
	}
	if res.Status() != costcalc.ResultStatusCalculated {
		return 0, "", false, nil
	}
	return res.ID(), formatCostValue(res.CostPerUnit()), true, nil
}

// MarkApprovedFromCalculatedTx transitions a CALCULATED row directly to APPROVED inside tx.
func (a *CostReaderAdapter) MarkApprovedFromCalculatedTx(ctx context.Context, tx *sql.Tx, costID int64, by string) error {
	return a.repo.MarkApprovedFromCalculatedTx(ctx, tx, costID, by)
}
