package mbspin_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appmbspin "github.com/mutugading/goapps-backend/services/finance/internal/application/mbspin"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

type fakeUnsuperseder struct {
	res *mbspin.UnsupersedeResult
	err error
	got [3]string
}

func (f *fakeUnsuperseder) Unsupersede(_ context.Context, h, s uuid.UUID, actor string) (*mbspin.UnsupersedeResult, error) {
	f.got = [3]string{h.String(), s.String(), actor}
	return f.res, f.err
}

type fakeSync struct{ ch chan uuid.UUID }

func (f fakeSync) TriggerSpinSync(_ context.Context, id uuid.UUID, _ string) error {
	f.ch <- id
	return nil
}

func TestUnsupersedeHandler_TriggersSyncForRestoredSpin(t *testing.T) {
	h, s := uuid.New(), uuid.New()
	repo := &fakeUnsuperseder{res: &mbspin.UnsupersedeResult{BatchID: uuid.New(), AffectedProductIDs: []int64{1}}}
	sync := fakeSync{ch: make(chan uuid.UUID, 1)}
	hd := appmbspin.NewUnsupersedeHandler(repo).WithLookupSyncTrigger(sync)
	res, err := hd.Handle(context.Background(), appmbspin.UnsupersedeCommand{HeadID: h, SpinID: s, ActorUserID: "u"})
	require.NoError(t, err)
	assert.Equal(t, repo.res, res)
	assert.Equal(t, [3]string{h.String(), s.String(), "u"}, repo.got)
	assert.Equal(t, s, <-sync.ch)
}

func TestUnsupersedeHandler_ErrorNoSync(t *testing.T) {
	boom := errors.New("boom")
	sync := fakeSync{ch: make(chan uuid.UUID, 1)}
	hd := appmbspin.NewUnsupersedeHandler(&fakeUnsuperseder{err: boom}).WithLookupSyncTrigger(sync)
	_, err := hd.Handle(context.Background(), appmbspin.UnsupersedeCommand{})
	require.ErrorIs(t, err, boom)
	assert.Empty(t, sync.ch)
}
