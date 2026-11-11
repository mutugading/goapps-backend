// Integration test for the Oracle ETL Postgres repository (P5-T3). LOCAL PostgreSQL only via
// internal/testutil/pgcontainer. Skipped unless INTEGRATION_TEST=true.
package postgres_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cpm "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
	"github.com/mutugading/goapps-backend/services/finance/internal/testutil/pgcontainer"
)

type etlIT struct {
	ctx context.Context
	t   *testing.T
	raw *sql.DB
}

func (e etlIT) typeID(code string) int32 {
	var id int32
	require.NoError(e.t, e.raw.QueryRowContext(e.ctx, `
		INSERT INTO cost_product_type (cpt_type_code, cpt_type_name) VALUES ($1, $1)
		ON CONFLICT (cpt_type_code) DO UPDATE SET cpt_type_name = cost_product_type.cpt_type_name
		RETURNING cpt_type_id`, code).Scan(&id))
	return id
}

func (e etlIT) product(typeID int32, name, item, shade, flex string, active bool) int64 {
	var id int64
	var it, sh, fx any
	if item != "" {
		it = item
	}
	if shade != "" {
		sh = shade
	}
	if flex != "" {
		fx = flex
	}
	require.NoError(e.t, e.raw.QueryRowContext(e.ctx, `
		INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name,
			cpm_shade_code, cpm_erp_item_code, cpm_flex_01, cpm_is_active, cpm_created_by, cpm_updated_by)
		VALUES (generate_cost_product_code($1, NOW()), $1, $2, $3, $4, $5, $6, 'it', 'it')
		RETURNING cpm_product_sys_id`, typeID, name, sh, it, fx, active).Scan(&id))
	return id
}

func (e etlIT) param(code string) {
	var n int
	require.NoError(e.t, e.raw.QueryRowContext(e.ctx,
		`SELECT COUNT(*) FROM mst_parameter WHERE param_code = $1 AND deleted_at IS NULL`, code).Scan(&n))
	if n > 0 {
		return
	}
	_, err := e.raw.ExecContext(e.ctx, `
		INSERT INTO mst_parameter (param_code, param_name, data_type, param_category, is_active, created_by)
		VALUES ($1, $1, 'NUMBER', 'INPUT', TRUE, 'it')`, code)
	require.NoError(e.t, err)
}

// snapshot captures everything an ETL run must never change on an existing product.
func (e etlIT) snapshot(id int64) string {
	var s string
	require.NoError(e.t, e.raw.QueryRowContext(e.ctx, `
		SELECT CAST(m.cpm_updated_at AS text) || '|' || COALESCE(m.cpm_shade_code, '~') || '|' ||
		       COALESCE(m.cpm_shade_name, '~') || '|' || m.cpm_product_name || '|' || m.cpm_is_active::text
		  FROM cost_product_master m WHERE m.cpm_product_sys_id = $1`, id).Scan(&s))
	return s
}

func (e etlIT) count(q string, args ...any) int {
	var n int
	require.NoError(e.t, e.raw.QueryRowContext(e.ctx, q, args...).Scan(&n))
	return n
}

func TestCostProductEtlRepositoryIntegration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	srv := pgcontainer.Start(ctx, t)
	raw := srv.CreateDatabase(t, "cpm_etl_it", "")
	mig, err := pgcontainer.NewMigrator(ctx, raw, "../../../migrations/postgres")
	require.NoError(t, err)
	require.NoError(t, mig.Up(ctx))

	e := etlIT{ctx: ctx, t: t, raw: raw}
	repo := postgres.NewCostProductEtlRepository(postgres.NewDBFromSQL(raw))
	poy := e.typeID("ZPOY")
	for _, c := range []string{"DENIER", "NO_OF_FILAMENTS", "ACT_DENIER"} {
		e.param(c)
	}

	// a) item+shade (active), inactive, flex_01 only, with LIKE wildcard chars in the item code.
	pA := e.product(poy, "ZZ POY A", "ZZETLA", "S1", "", true)
	pInactive := e.product(poy, "ZZ POY INACT", "ZZETLI", "S1", "", false)
	pFlex := e.product(poy, "ZZ POY FLEX", "", "", "ZZETLF-S1-ZPOY", true)
	pWild := e.product(poy, "ZZ POY WILD", "ZZETL_W", "S1", "", true)
	// b) sibling of ZZETLB (different shade); c) same name+shade, other item.
	pSib := e.product(poy, "ZZ POY SIB", "ZZETLB", "S9", "", true)
	pName := e.product(poy, "ZZ POY SAMENAME", "OTHERITEM", "S1", "", true)

	keys := []cpm.EtlKey{
		{Item: "ZZETLA", Shade: "S1", NameUpper: "ZZ POY A"},
		{Item: "ZZETLI", Shade: "S1", NameUpper: "ZZ POY INACT"},
		{Item: "ZZETLF", Shade: "S1", NameUpper: "ZZ POY FLEX"},
		{Item: "ZZETLXW", Shade: "S1", NameUpper: "NOPE"}, // '_' must not wildcard-match ZZETL_W
		{Item: "ZZETLB", Shade: "S1", NameUpper: "ZZ POY SAMENAME"},
	}
	ms, err := repo.MatchProducts(ctx, keys)
	require.NoError(t, err)
	assert.Equal(t, []int64{pA}, ms.ByItemShade[cpm.EtlRowKey("ZZETLA", "S1")])
	assert.Equal(t, []int64{pInactive}, ms.ByItemShade[cpm.EtlRowKey("ZZETLI", "S1")], "inactive still matches")
	assert.Equal(t, []int64{pFlex}, ms.ByItemShade[cpm.EtlRowKey("ZZETLF", "S1")], "flex_01 match")
	assert.Empty(t, ms.ByItemShade[cpm.EtlRowKey("ZZETLXW", "S1")])
	assert.Equal(t, []int64{pSib}, ms.ByItem["ZZETLB"])
	assert.Empty(t, ms.ByItemShade[cpm.EtlRowKey("ZZETLB", "S1")])
	assert.Equal(t, []int64{pName}, ms.ByNameShade[cpm.EtlNameShadeKey("ZZ POY SAMENAME", "S1")])
	_ = pWild

	// Insert: one new, one that already exists (SKIPPED by re-check), one that appears via flex_01.
	existing := []int64{pA, pInactive, pFlex, pWild, pSib, pName}
	before := map[int64]string{}
	for _, id := range existing {
		before[id] = e.snapshot(id)
	}
	previewID, err := repo.SaveRun(ctx, cpm.EtlRun{Type: cpm.EtlRunPreview, FromDate: time.Now().UTC(),
		Status: cpm.EtlRunCompleted, CreatedBy: "it", Rows: []cpm.EtlRunRow{{RowKey: "ZZETLN|S1"}}})
	require.NoError(t, err)

	out, err := repo.InsertEtlProducts(ctx, cpm.EtlInsertInput{
		PreviewRunID: previewID, FromDate: time.Now().UTC(), Actor: "it-actor",
		Items: []cpm.EtlInsert{
			{RowKey: "ZZETLN|S1", ItemCode: "ZZETLN", ItemName: "ZZ POY NEW", ShadeCode: "S1", ShadeName: "Shade One",
				TypeID: poy, TypeCode: "ZPOY", Denier: "75", Filament: "36", ActDenier: ""},
			{RowKey: "ZZETLA|S1", ItemCode: "ZZETLA", ItemName: "ZZ POY A", ShadeCode: "S1", TypeID: poy, TypeCode: "ZPOY", Denier: "99"},
			{RowKey: "ZZETLF|S1", ItemCode: "ZZETLF", ItemName: "OTHER NAME", ShadeCode: "S1", TypeID: poy, TypeCode: "ZPOY", Denier: "99"},
		},
	})
	require.NoError(t, err)
	require.Len(t, out.Results, 3)
	assert.Equal(t, cpm.EtlResultInserted, out.Results[0].Result)
	assert.Equal(t, cpm.EtlResultSkipped, out.Results[1].Result)
	assert.Equal(t, cpm.EtlResultSkipped, out.Results[2].Result)
	newID := out.Results[0].ProductSysID
	require.NotZero(t, newID)

	var flex, src, item, createdBy string
	require.NoError(t, raw.QueryRowContext(ctx, `SELECT cpm_flex_01, cpm_source, cpm_erp_item_code, cpm_created_by
		FROM cost_product_master WHERE cpm_product_sys_id = $1`, newID).Scan(&flex, &src, &item, &createdBy))
	assert.Equal(t, "ZZETLN-S1-ZPOY", flex)
	assert.Equal(t, "ORACLE_ETL", src)
	assert.Equal(t, "ZZETLN", item)
	assert.Equal(t, "it-actor", createdBy)

	// Params only for the new product, only non-empty values (ACT_DENIER empty -> not written).
	assert.Equal(t, 2, e.count(`SELECT COUNT(*) FROM cost_product_parameter WHERE cpp_product_sys_id = $1`, newID))
	assert.Equal(t, 0, e.count(`SELECT COUNT(*) FROM cost_product_parameter WHERE cpp_product_sys_id = ANY($1)`, pq.Array(existing)))
	assert.Equal(t, 0, e.count(`SELECT COUNT(*) FROM cost_product_master WHERE cpm_source = 'ORACLE_ETL' AND cpm_product_sys_id <> $1`, newID))

	// Existing products are byte-identical.
	for _, id := range existing {
		assert.Equal(t, before[id], e.snapshot(id), "product %d changed", id)
	}

	// APPLY run saved with the preview id, and carries all rows.
	run, err := repo.GetRun(ctx, out.RunID)
	require.NoError(t, err)
	assert.Equal(t, cpm.EtlRunApply, run.Type)
	require.NotNil(t, run.PreviewRunID)
	assert.Equal(t, previewID, *run.PreviewRunID)
	assert.Len(t, run.ApplyRows, 3)

	// Re-applying the same row now SKIPPED (new product exists).
	out2, err := repo.InsertEtlProducts(ctx, cpm.EtlInsertInput{PreviewRunID: previewID, FromDate: time.Now().UTC(), Actor: "it",
		Items: []cpm.EtlInsert{{RowKey: "ZZETLN|S1", ItemCode: "ZZETLN", ItemName: "ZZ POY NEW", ShadeCode: "S1", TypeID: poy, TypeCode: "ZPOY"}}})
	require.NoError(t, err)
	assert.Equal(t, cpm.EtlResultSkipped, out2.Results[0].Result)

	_, err = repo.GetRun(ctx, uuid.New())
	assert.ErrorIs(t, err, cpm.ErrEtlRunNotFound)
}
