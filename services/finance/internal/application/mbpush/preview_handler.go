package mbpush

import (
	"context"
	"fmt"
	"strings"
)

const (
	skipReasonMissingActual   = "MISSING_ACTUAL_COST"
	skipReasonMissingSelling  = "MISSING_SELLING_COST"
	skipReasonMissingForecast = "MISSING_FORECAST_COST"
	skipReasonNoCostProduct   = "NO_COST_PRODUCT_LINKED"
)

// PushableMBHead is an MB Head eligible for the push, with per-cost-type availability flags.
type PushableMBHead struct {
	MBHID       string
	Code        string
	Name        string
	HasActual   bool
	HasSelling  bool
	HasForecast bool
}

// SkippedMBHead is an MB Head excluded from the push, with the reason(s) why.
type SkippedMBHead struct {
	MBHID  string
	Code   string
	Name   string
	Reason string
}

// PreviewHandler computes which VALIDATED MB Heads are ready for a push-to-head execution.
type PreviewHandler struct {
	mbHeadReader MBHeadReader
	costReader   CostReader
}

// NewPreviewHandler constructs a PreviewHandler.
func NewPreviewHandler(mbHeadReader MBHeadReader, costReader CostReader) *PreviewHandler {
	return &PreviewHandler{mbHeadReader: mbHeadReader, costReader: costReader}
}

// Preview lists VALIDATED MB Heads split into pushable (all 3 cost types CALCULATED) and skipped
// (with the reason(s) why), per PR-02.
func (h *PreviewHandler) Preview(ctx context.Context, period string) ([]PushableMBHead, []SkippedMBHead, error) {
	candidates, err := h.mbHeadReader.ListValidated(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("list validated mb heads: %w", err)
	}

	pushable := make([]PushableMBHead, 0, len(candidates))
	skipped := make([]SkippedMBHead, 0, len(candidates))
	for _, c := range candidates {
		if c.CostProductID == 0 {
			skipped = append(skipped, SkippedMBHead{MBHID: c.MBHID, Code: c.Code, Name: c.Name, Reason: skipReasonNoCostProduct})
			continue
		}
		p, reasons := h.checkCostTypes(ctx, c, period)
		if len(reasons) > 0 {
			skipped = append(skipped, SkippedMBHead{MBHID: c.MBHID, Code: c.Code, Name: c.Name, Reason: strings.Join(reasons, ", ")})
			continue
		}
		pushable = append(pushable, p)
	}
	return pushable, skipped, nil
}

func (h *PreviewHandler) checkCostTypes(ctx context.Context, c MBHeadCandidate, period string) (PushableMBHead, []string) {
	p := PushableMBHead{MBHID: c.MBHID, Code: c.Code, Name: c.Name}
	var reasons []string

	_, _, foundActual, err := h.costReader.GetActiveCalculated(ctx, c.CostProductID, period, "ACTUAL")
	p.HasActual = err == nil && foundActual
	if !p.HasActual {
		reasons = append(reasons, skipReasonMissingActual)
	}
	_, _, foundSelling, err := h.costReader.GetActiveCalculated(ctx, c.CostProductID, period, "SELLING")
	p.HasSelling = err == nil && foundSelling
	if !p.HasSelling {
		reasons = append(reasons, skipReasonMissingSelling)
	}
	_, _, foundForecast, err := h.costReader.GetActiveCalculated(ctx, c.CostProductID, period, "FORECAST")
	p.HasForecast = err == nil && foundForecast
	if !p.HasForecast {
		reasons = append(reasons, skipReasonMissingForecast)
	}
	return p, reasons
}
