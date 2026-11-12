// Text-level guard for IAM 000101 (no DB needed): two permissions, valid code format,
// granted only to SUPER_ADMIN, no "::" casts.
package postgres_test

import (
	"os"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMBSpinFG101Permissions(t *testing.T) {
	up, err := os.ReadFile("000101_seed_mb_spin_fg_permissions.up.sql")
	require.NoError(t, err)
	down, err := os.ReadFile("000101_seed_mb_spin_fg_permissions.down.sql")
	require.NoError(t, err)

	codeRe := regexp.MustCompile(`^[a-z][a-z0-9]*\.[a-z][a-z0-9]*\.[a-z][a-z0-9]*\.[a-z]+$`)
	for _, c := range []string{"finance.product.master.sync", "finance.yarnmaster.mbspin.restore"} {
		assert.Regexp(t, codeRe, c)
		assert.Contains(t, string(up), "'"+c+"'")
		assert.Contains(t, string(down), "'"+c+"'")
	}

	grants := regexp.MustCompile(`(?s)INSERT INTO role_permissions.*?ON CONFLICT`).FindAllString(string(up), -1)
	require.Len(t, grants, 1)
	assert.Contains(t, grants[0], "r.role_code = 'SUPER_ADMIN'")
	assert.NotContains(t, grants[0], "r.role_code IN")
	for _, s := range []string{string(up), string(down)} {
		assert.NotContains(t, s, "::")
	}
	assert.NotContains(t, string(down), "mst_role")
}
