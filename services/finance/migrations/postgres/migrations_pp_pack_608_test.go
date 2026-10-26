package postgres_test

import (
	"os"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readMig(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	require.NoError(t, err)
	return string(b)
}

// Text-level guard for P002 migrations 000608-000610 (no DB needed).
func TestPackProdStaging608MigrationText(t *testing.T) {
	up := readMig(t, "000608_create_bi_pp_stg_pack_prod_daily.up.sql")
	down := readMig(t, "000608_create_bi_pp_stg_pack_prod_daily.down.sql")

	assert.Contains(t, up, "BEGIN;")
	assert.Contains(t, up, "COMMIT;")
	assert.Contains(t, up, "stg_id               BIGSERIAL      PRIMARY KEY")
	assert.Contains(t, up, "CREATE TABLE IF NOT EXISTS bi_pp_stg_pack_prod_daily")
	for _, c := range []string{"ax_qty", "fq_qty", "dg_qty", "jlt_qty", "total_qty"} {
		assert.Regexp(t, regexp.MustCompile(c+`\s+NUMERIC\(12,6\)\s+NOT NULL DEFAULT 0`), up)
	}
	assert.NotRegexp(t, `(?i)\b(float|double|real)\b`, up)
	for _, c := range []string{"pack_date", "week_start", "month_start"} {
		assert.Contains(t, up, "("+c+")")
	}
	assert.Contains(t, down, "DROP TABLE IF EXISTS bi_pp_stg_pack_prod_daily")
}

func TestPackProdRegistry609MigrationText(t *testing.T) {
	up := readMig(t, "000609_seed_bi_metric_registry_pack.up.sql")
	down := readMig(t, "000609_seed_bi_metric_registry_pack.down.sql")
	for _, m := range []string{"PACK_TOTAL", "PACK_AX", "PACK_FQ", "PACK_DG", "PACK_JLT"} {
		assert.Contains(t, up, "'"+m+"'")
		assert.Contains(t, down, "'"+m+"'")
	}
	assert.Contains(t, up, "ON CONFLICT (metric_name) DO NOTHING")
}

func TestPackProdJob610MigrationText(t *testing.T) {
	up := readMig(t, "000610_seed_bi_job_etl_pp_pack_daily.up.sql")
	down := readMig(t, "000610_seed_bi_job_etl_pp_pack_daily.down.sql")
	assert.Contains(t, up, "source_code = 'HRIS_ORACLE'")
	assert.Contains(t, up, "'ETL_PP_PACK_DAILY'")
	assert.Contains(t, up, "'PP_PACK'")
	assert.Contains(t, up, "'0 7 * * *'") // PP-03: 07:00 WIB
	assert.Contains(t, up, "ON CONFLICT (job_name) DO NOTHING")
	assert.Contains(t, down, "job_name = 'ETL_PP_PACK_DAILY'")
}
