package mbhead

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

// errSupersedeReaderMissing is returned when the handler was built without a reader.
var errSupersedeReaderMissing = errors.New("mb head supersede preview is not configured")

// PreviewSupersedeQuery asks which live spins a Validate of MbhID would supersede.
type PreviewSupersedeQuery struct {
	MbhID uuid.UUID
}

// PreviewSupersedeHandler backs PreviewMBHeadSupersede (read-only, design §2.3).
type PreviewSupersedeHandler struct {
	repo   mbhead.Repository
	reader mbspin.SupersedeReader
}

// NewPreviewSupersedeHandler creates a PreviewSupersedeHandler.
func NewPreviewSupersedeHandler(repo mbhead.Repository, reader mbspin.SupersedeReader) *PreviewSupersedeHandler {
	return &PreviewSupersedeHandler{repo: repo, reader: reader}
}

// Handle loads the head (mbhead.ErrNotFound when missing) and lists the supersede
// candidates of its root spin identity (head shade + head FG). A head without FG or
// without shade never conflicts: RequiresConfirmation=false, no candidates.
func (h *PreviewSupersedeHandler) Handle(ctx context.Context, q PreviewSupersedeQuery) (*mbspin.SupersedePreview, error) {
	if h.reader == nil {
		return nil, errSupersedeReaderMissing
	}
	head, err := h.repo.GetByID(ctx, q.MbhID)
	if err != nil {
		return nil, err
	}
	out := &mbspin.SupersedePreview{}
	fg := head.FgProductID()
	if fg == nil || *fg <= 0 {
		return out, nil
	}
	cands, err := h.reader.PreviewSupersede(ctx, head.ID(), head.ShadeCode(), *fg)
	if err != nil {
		return nil, err
	}
	out.Candidates = cands
	out.RequiresConfirmation = len(cands) > 0
	return out, nil
}
