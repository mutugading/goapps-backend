package mbspin

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

// LookupSyncTrigger queues a master-lookup sync for the yarn products referencing one MB
// Spin. Best-effort: the caller only logs a returned error.
type LookupSyncTrigger interface {
	TriggerSpinSync(ctx context.Context, spinID uuid.UUID, actor string) error
}

const unsupersedeSyncTimeout = 2 * time.Minute

// UnsupersedeCommand restores a superseded spin.
type UnsupersedeCommand struct {
	HeadID      uuid.UUID
	SpinID      uuid.UUID
	ActorUserID string
}

// UnsupersedeHandler handles the UnsupersedeMBSpin command.
type UnsupersedeHandler struct {
	repo       mbspin.Unsuperseder
	lookupSync LookupSyncTrigger
}

// NewUnsupersedeHandler creates an UnsupersedeHandler.
func NewUnsupersedeHandler(repo mbspin.Unsuperseder) *UnsupersedeHandler {
	return &UnsupersedeHandler{repo: repo}
}

// WithLookupSyncTrigger attaches the after-commit lookup-sync port. nil disables it.
func (h *UnsupersedeHandler) WithLookupSyncTrigger(t LookupSyncTrigger) *UnsupersedeHandler {
	h.lookupSync = t
	return h
}

// Handle runs the un-supersede, then (async, best-effort) queues a lookup sync for the
// restored spin: the reverted params point at it again.
func (h *UnsupersedeHandler) Handle(ctx context.Context, cmd UnsupersedeCommand) (*mbspin.UnsupersedeResult, error) {
	res, err := h.repo.Unsupersede(ctx, cmd.HeadID, cmd.SpinID, cmd.ActorUserID)
	if err != nil {
		return nil, err
	}
	h.triggerSync(ctx, res, cmd)
	return res, nil
}

func (h *UnsupersedeHandler) triggerSync(ctx context.Context, res *mbspin.UnsupersedeResult, cmd UnsupersedeCommand) {
	if h.lookupSync == nil || res == nil || len(res.AffectedProductIDs) == 0 {
		return
	}
	trigger := h.lookupSync
	logger := log.With().Str("mbs_id", cmd.SpinID.String()).Str("batch_id", res.BatchID.String()).Logger()
	bg := context.WithoutCancel(ctx)
	go func() {
		bgCtx, cancel := context.WithTimeout(bg, unsupersedeSyncTimeout)
		defer cancel()
		if err := trigger.TriggerSpinSync(bgCtx, cmd.SpinID, cmd.ActorUserID); err != nil {
			logger.Warn().Err(err).Msg("MB un-supersede: lookup sync for restored spin not queued")
		}
	}()
}
