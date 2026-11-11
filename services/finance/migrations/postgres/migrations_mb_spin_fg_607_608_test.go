package postgres_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Text-level guard for 000607 (MB spin <-> FG link + supersede) and 000608 (cost_product_etl_run). No DB needed.
func TestMBSpinFG607608MigrationText(t *testing.T) {
	up := readMig(t, "000607_add_mb_fg_and_supersede.up.sql")
	down := readMig(t, "000607_add_mb_fg_and_supersede.down.sql")

	// 000607 up: additive only, transactional, no cast syntax.
	assert.Contains(t, up, "BEGIN;")
	assert.Contains(t, up, "COMMIT;")
	assert.NotContains(t, up, "::")
	assert.NotRegexp(t, `(?im)^\s*(UPDATE|DELETE|INSERT)\s`, up, "schema only, no data writes")

	for _, s := range []string{
		"uix_mbs_shade_fg_live",
		"WHERE mbs_fg_product_id IS NOT NULL",
		"mbs_superseded_by_spin_id IS NULL",
		"chk_mbs_fg_link_source",
		"CREATE TABLE IF NOT EXISTS mst_mb_spin_supersede_item",
		"ON DELETE CASCADE",
	} {
		assert.Contains(t, up, s)
	}

	// Every new column must be NULL-able so existing rows stay valid without backfill.
	addColLine := regexp.MustCompile(`(?im)ADD COLUMN.*$`)
	for _, line := range addColLine.FindAllString(up, -1) {
		assert.NotContains(t, strings.ToUpper(line), "NOT NULL", "ADD COLUMN must be nullable: %s", line)
	}
	assert.Equal(t, 11, len(addColLine.FindAllString(up, -1)), "expected 11 ADD COLUMN clauses")
	assert.NotRegexp(t, `(?im)^\s*ALTER TABLE.*NOT NULL DEFAULT`, up)

	// 000607 down: drops only objects this migration created.
	assert.Contains(t, down, "BEGIN;")
	assert.Contains(t, down, "COMMIT;")
	assert.Contains(t, down, "DROP TABLE IF EXISTS mst_mb_spin_supersede_item")
	assert.Contains(t, down, "DROP INDEX IF EXISTS uix_mbs_shade_fg_live")
	assert.Contains(t, down, "DROP CONSTRAINT IF EXISTS chk_mbs_fg_link_source")
	for _, c := range []string{
		"mbh_fg_product_id", "mbh_fg_item_code", "mbh_superseded_by_head_id", "mbh_superseded_at", "mbh_superseded_by",
		"mbs_fg_product_id", "mbs_fg_item_code", "mbs_fg_link_source", "mbs_superseded_by_spin_id", "mbs_superseded_at", "mbs_superseded_by",
	} {
		assert.Contains(t, down, "DROP COLUMN IF EXISTS "+c, "down must drop %s", c)
	}
	assert.NotContains(t, down, "DROP COLUMN IF EXISTS mbs_final_product")
	assert.NotContains(t, down, "DROP COLUMN IF EXISTS mbs_shade_code")

	// 000608 up/down: audit table for Product Master ETL runs.
	up8 := readMig(t, "000608_create_cost_product_etl_run.up.sql")
	down8 := readMig(t, "000608_create_cost_product_etl_run.down.sql")

	assert.Contains(t, up8, "BEGIN;")
	assert.Contains(t, up8, "COMMIT;")
	assert.Contains(t, up8, "CREATE TABLE IF NOT EXISTS cost_product_etl_run")
	assert.Contains(t, up8, "CHECK (cper_run_type IN ('PREVIEW', 'APPLY'))")
	assert.NotContains(t, up8, "::")
	assert.Contains(t, down8, "DROP TABLE IF EXISTS cost_product_etl_run")
}
