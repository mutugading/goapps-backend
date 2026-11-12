package postgres_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Text-level guard for 000609 (supersede audit old/new values). No DB needed.
func TestMBSupersedeItem609MigrationText(t *testing.T) {
	up := readMig(t, "000609_add_mb_supersede_item_old_values.up.sql")
	down := readMig(t, "000609_add_mb_supersede_item_old_values.down.sql")

	assert.Contains(t, up, "BEGIN;")
	assert.Contains(t, up, "COMMIT;")
	assert.NotContains(t, up, "::")
	assert.NotRegexp(t, `(?im)^\s*(UPDATE|DELETE|INSERT|DROP)\s`, up, "schema only, additive")

	cols := []string{
		"mssi_param_code", "mssi_old_mb_spin_id", "mssi_old_value_numeric",
		"mssi_new_value_numeric", "mssi_repoint_kind",
	}
	addColLine := regexp.MustCompile(`(?im)ADD COLUMN.*$`)
	lines := addColLine.FindAllString(up, -1)
	assert.Len(t, lines, len(cols))
	for _, line := range lines {
		assert.Contains(t, line, "IF NOT EXISTS", "idempotent: %s", line)
		assert.NotContains(t, strings.ToUpper(line), "NOT NULL", "nullable: %s", line)
		assert.NotContains(t, strings.ToUpper(line), "REFERENCES", "no FK: %s", line)
	}
	for _, c := range cols {
		assert.Contains(t, up, "ADD COLUMN IF NOT EXISTS "+c)
		assert.Contains(t, down, "DROP COLUMN IF EXISTS "+c)
	}

	assert.Contains(t, down, "BEGIN;")
	assert.Contains(t, down, "COMMIT;")
	assert.NotContains(t, down, "DROP TABLE")
	assert.NotContains(t, down, "mssi_old_value_text", "down must not drop 000607 columns")
}
