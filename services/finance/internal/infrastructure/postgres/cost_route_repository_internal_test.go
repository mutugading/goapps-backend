package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestRouteSortColumn guards the L1 sort bug: every key the frontend list can
// send (and every key in the proto ListRoutesRequest.sort_by in-list) must map
// to a real cost_route_head/product column. An unmapped key that fell through
// to a literal already carrying "DESC" produced "<col> DESC ASC" -> invalid SQL
// -> empty list.
func TestRouteSortColumn(t *testing.T) {
	tests := []struct {
		name   string
		sortBy string
		want   string
	}{
		{"empty defaults to created_at", "", "h.crh_created_at"},
		{"unknown key defaults to created_at", "bogus", "h.crh_created_at"},
		{"created_at", "created_at", "h.crh_created_at"},
		{"product_code", "product_code", "p.cpm_product_code"},
		{"status", "status", "h.crh_routing_status"},
		{"head_id", "head_id", "h.crh_head_id"},
		{"version", "version", "h.crh_version"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, routeSortColumn(tt.sortBy))
		})
	}
}

// TestRouteOrderBy proves each UI-exposed sort key produces valid, single-
// direction ORDER BY SQL (no "DESC ASC" concatenation), with a stable head_id
// secondary tiebreaker on non-head_id columns.
func TestRouteOrderBy(t *testing.T) {
	tests := []struct {
		name      string
		sortBy    string
		sortOrder string
		want      string
	}{
		{"empty defaults to created_at desc", "", "", "h.crh_created_at DESC, h.crh_head_id ASC"},
		{"head_id asc has no secondary", "head_id", "asc", "h.crh_head_id ASC"},
		{"head_id desc has no secondary", "head_id", "desc", "h.crh_head_id DESC"},
		{"version asc gets secondary", "version", "asc", "h.crh_version ASC, h.crh_head_id ASC"},
		{"version desc gets secondary", "version", "desc", "h.crh_version DESC, h.crh_head_id ASC"},
		{"product_code asc", "product_code", "asc", "p.cpm_product_code ASC, h.crh_head_id ASC"},
		{"status desc", "status", "desc", "h.crh_routing_status DESC, h.crh_head_id ASC"},
		{"desc is case-insensitive", "version", "DESC", "h.crh_version DESC, h.crh_head_id ASC"},
		{"unknown direction falls back to asc", "version", "sideways", "h.crh_version ASC, h.crh_head_id ASC"},
		{"explicit key with empty order defaults asc", "version", "", "h.crh_version ASC, h.crh_head_id ASC"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, routeOrderBy(tt.sortBy, tt.sortOrder))
		})
	}
}
