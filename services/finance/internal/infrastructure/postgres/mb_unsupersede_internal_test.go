package postgres

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

// P3-R2: CHILD rows follow their product's trigger row.
func TestItemReasons_ChildFollowsTrigger(t *testing.T) {
	items := []unsupersedeItem{
		{ProductSysID: 1, Kind: mbspin.SupersedeRepointPointer, CppExists: true, Matches: false, OldExisted: true},
		{ProductSysID: 1, Kind: mbspin.SupersedeRepointChild, CppExists: true, Matches: true, OldExisted: true},
		{ProductSysID: 2, Kind: mbspin.SupersedeRepointPointer, CppExists: true, Matches: true, OldExisted: true},
		{ProductSysID: 2, Kind: mbspin.SupersedeRepointChild, CppExists: true, Matches: true, OldExisted: true},
	}
	got := itemReasons(items)
	require.Equal(t, []string{mbspin.UnsupersedeSkipChanged, mbspin.UnsupersedeSkipTriggerKept, "", ""}, got)
}
