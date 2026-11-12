// Package postgres_test — manual FG edit on spins (mb-spin-fg-poy-link P8).
//
// Gated by INTEGRATION_TEST=true. Fixtures are prefixed ITEST-MBSPINFGMAN- and
// hard-deleted before/after each test.
package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

const fgmPrefix = "ITEST-MBSPINFGMAN-"

type MBSpinFgManualSuite struct {
	suite.Suite
	db     *postgres.DB
	repo   *postgres.MBSpinRepository
	ctx    context.Context
	typeID int64
	other  int64
}

func TestMBSpinFgManualSuite(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	suite.Run(t, new(MBSpinFgManualSuite))
}

func (s *MBSpinFgManualSuite) SetupSuite() {
	s.ctx = context.Background()
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		getEnvOrDefault("TEST_DB_HOST", "localhost"), getEnvOrDefault("TEST_DB_PORT", "5434"),
		getEnvOrDefault("TEST_DB_USER", "finance"), getEnvOrDefault("TEST_DB_PASSWORD", "finance123"),
		getEnvOrDefault("TEST_DB_NAME", "finance_db"))
	raw, err := sql.Open("pgx", dsn)
	require.NoError(s.T(), err)
	require.NoError(s.T(), waitForDB(raw, 10*time.Second))
	s.db = postgres.NewDBFromSQL(raw)
	s.repo = postgres.NewMBSpinRepository(s.db)
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT cpt_type_id FROM cost_product_type WHERE cpt_type_code = 'POY' LIMIT 1`).Scan(&s.typeID))
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT cpt_type_id FROM cost_product_type WHERE cpt_type_code <> 'POY' LIMIT 1`).Scan(&s.other))
}

func (s *MBSpinFgManualSuite) TearDownSuite() {
	if s.db == nil {
		return
	}
	s.cleanup()
	require.NoError(s.T(), s.db.Close())
}

func (s *MBSpinFgManualSuite) SetupTest()    { s.cleanup() }
func (s *MBSpinFgManualSuite) TearDownTest() { s.cleanup() }

func (s *MBSpinFgManualSuite) exec(q string, args ...any) {
	_, err := s.db.ExecContext(s.ctx, q, args...)
	require.NoError(s.T(), err)
}

func (s *MBSpinFgManualSuite) cleanup() {
	like := fgmPrefix + "%"
	s.exec(`UPDATE mst_mb_spin SET mbs_superseded_by_spin_id = NULL, mbs_parent_spin_id = NULL WHERE mbs_mgt_name LIKE $1`, like)
	s.exec(`DELETE FROM mst_mb_spin WHERE mbs_mgt_name LIKE $1`, like)
	s.exec(`DELETE FROM mst_mb_head WHERE mbh_mb_costing LIKE $1`, like)
	s.exec(`DELETE FROM cost_product_master WHERE cpm_product_code LIKE $1`, like)
}

func (s *MBSpinFgManualSuite) newHead(tag string) uuid.UUID {
	id := uuid.New()
	s.exec(`INSERT INTO mst_mb_head (mbh_id, mbh_mb_costing, created_by) VALUES ($1,$2,'itest')`, id, fgmPrefix+tag)
	return id
}

func (s *MBSpinFgManualSuite) newProduct(code string, typeID int64, shade, erp *string, active bool) int64 {
	var id int64
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx, `
		INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name, cpm_shade_code, cpm_erp_item_code, cpm_is_active, cpm_created_by, cpm_updated_by)
		VALUES ($1,$2,$3,$4,$5,$6,'itest','itest') RETURNING cpm_product_sys_id`,
		fgmPrefix+code, typeID, fgmPrefix+code, shade, erp, active).Scan(&id))
	return id
}

type fgmSpin struct {
	shade      *string
	source     *string
	fg         *int64
	inactive   bool
	superseded bool
}

func (s *MBSpinFgManualSuite) newSpin(head uuid.UUID, tag string, o fgmSpin) uuid.UUID {
	id := uuid.New()
	s.exec(`
		INSERT INTO mst_mb_spin (mbs_id, mbs_mbh_id, mbs_mgt_name, mbs_mb_costing, mbs_shade_code,
			mbs_fg_product_id, mbs_fg_link_source, mbs_is_active, created_by)
		VALUES ($1,$2,$3,$3,$4,$5,$6,$7,'itest')`,
		id, head, fgmPrefix+tag, o.shade, o.fg, o.source, !o.inactive)
	if o.superseded {
		by := uuid.New()
		s.exec(`INSERT INTO mst_mb_spin (mbs_id, mbs_mbh_id, mbs_mgt_name, mbs_mb_costing, mbs_is_active, created_by)
			VALUES ($1,$2,$3,$3,TRUE,'itest')`, by, head, fgmPrefix+"Z"+tag)
		s.exec(`UPDATE mst_mb_spin SET mbs_superseded_by_spin_id = $2 WHERE mbs_id = $1`, id, by)
	}
	return id
}

func (s *MBSpinFgManualSuite) fgOf(id uuid.UUID) (fg sql.NullInt64, item, src sql.NullString) {
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT mbs_fg_product_id, mbs_fg_item_code, mbs_fg_link_source FROM mst_mb_spin WHERE mbs_id=$1`, id).Scan(&fg, &item, &src))
	return fg, item, src
}

func (s *MBSpinFgManualSuite) snapshot(id uuid.UUID) string {
	var v string
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx, `
		SELECT COALESCE(CAST(sp.mbs_dozing AS text),'') || '|' || COALESCE(sp.mbs_shade_code,'') || '|' ||
		       COALESCE((SELECT CAST(MAX(cpp_updated_at) AS text) FROM cost_product_parameter),'') || '|' ||
		       COALESCE((SELECT CAST(h.updated_at AS text) FROM mst_mb_head h WHERE h.mbh_id = sp.mbs_mbh_id),'')
		  FROM mst_mb_spin sp WHERE sp.mbs_id = $1`, id).Scan(&v))
	return v
}

func fgmStr(v string) *string { return &v }

func (s *MBSpinFgManualSuite) TestSetManualSnapshotsItemCodeAndLeavesOthersAlone() {
	sh := "FGM-A"
	h := s.newHead("H1")
	poy := s.newProduct("P1", s.typeID, fgmStr(" "+sh+" "), fgmStr(" ERP-1 "), true)
	sp := s.newSpin(h, "A1", fgmSpin{shade: fgmStr(sh)})
	before := s.snapshot(sp)

	require.NoError(s.T(), s.repo.ApplyManualFg(s.ctx, sp, poy, "itest"))
	fg, item, src := s.fgOf(sp)
	require.Equal(s.T(), poy, fg.Int64)
	require.Equal(s.T(), "ERP-1", item.String)
	require.Equal(s.T(), mbspin.FgLinkSourceManual, src.String)
	require.Equal(s.T(), before, s.snapshot(sp), "head, cpp and spin values must be unchanged")
}

func (s *MBSpinFgManualSuite) TestClear() {
	sh := "FGM-B"
	h := s.newHead("H2")
	poy := s.newProduct("P2", s.typeID, &sh, nil, true)
	sp := s.newSpin(h, "A1", fgmSpin{shade: &sh, fg: &poy, source: fgmStr("BACKFILL")})

	require.NoError(s.T(), s.repo.ApplyManualFg(s.ctx, sp, 0, "itest"))
	fg, item, src := s.fgOf(sp)
	require.False(s.T(), fg.Valid)
	require.False(s.T(), item.Valid)
	require.False(s.T(), src.Valid)
}

func (s *MBSpinFgManualSuite) TestRejections() {
	sh := "FGM-C"
	h := s.newHead("H3")
	good := s.newProduct("PG", s.typeID, &sh, nil, true)
	wrongShade := s.newProduct("PW", s.typeID, fgmStr("FGM-OTHER"), nil, true)
	blankShade := s.newProduct("PB", s.typeID, nil, nil, true)
	notPoy := s.newProduct("PN", s.other, &sh, nil, true)
	inactivePoy := s.newProduct("PI", s.typeID, &sh, nil, false)
	sp := s.newSpin(h, "A1", fgmSpin{shade: &sh})

	cases := []struct {
		name string
		spin uuid.UUID
		fg   int64
		want error
	}{
		{"shade mismatch", sp, wrongShade, mbspin.ErrFgShadeMismatch},
		{"blank-shade POY", sp, blankShade, mbspin.ErrFgShadeMismatch},
		{"not POY", sp, notPoy, mbspin.ErrFgNotPoy},
		{"inactive POY", sp, inactivePoy, mbspin.ErrFgNotPoy},
		{"missing POY", sp, 999999999, mbspin.ErrFgNotPoy},
		{"missing spin", uuid.New(), good, mbspin.ErrNotFound},
		{"spin without shade", s.newSpin(h, "N1", fgmSpin{}), good, mbspin.ErrFgShadeMismatch},
		{"recipe-linked", s.newSpin(h, "R2", fgmSpin{shade: &sh, fg: &good, source: fgmStr("RECIPE")}), good + 0, mbspin.ErrFgFollowsRecipe},
		{"superseded", s.newSpin(h, "S1", fgmSpin{shade: &sh, superseded: true}), good, mbspin.ErrFgSpinNotLive},
		{"inactive spin", s.newSpin(h, "I1", fgmSpin{shade: &sh, inactive: true}), good, mbspin.ErrFgSpinNotLive},
	}
	for _, c := range cases {
		err := s.repo.ApplyManualFg(s.ctx, c.spin, c.fg, "itest")
		require.ErrorIs(s.T(), err, c.want, c.name)
	}
	// Recipe-linked clear is rejected too, and nothing was written to the plain spin.
	sh2 := "FGM-C2"
	rec := s.newSpin(h, "R3", fgmSpin{shade: &sh2, fg: &good, source: fgmStr("RECIPE")})
	require.ErrorIs(s.T(), s.repo.ApplyManualFg(s.ctx, rec, 0, "itest"), mbspin.ErrFgFollowsRecipe)
	fg, _, _ := s.fgOf(sp)
	require.False(s.T(), fg.Valid)
}

func (s *MBSpinFgManualSuite) TestDuplicateLiveShadeFgRejected() {
	sh := "FGM-D"
	h := s.newHead("H4")
	poy := s.newProduct("PD", s.typeID, &sh, nil, true)
	_ = s.newSpin(h, "L1", fgmSpin{shade: &sh, fg: &poy, source: fgmStr("MANUAL")})
	sp := s.newSpin(h, "N2", fgmSpin{shade: &sh})

	require.ErrorIs(s.T(), s.repo.ApplyManualFg(s.ctx, sp, poy, "itest"), mbspin.ErrSpinShadeFgExists)
	fg, _, _ := s.fgOf(sp)
	require.False(s.T(), fg.Valid)
}

// P8-R BE-3: a root spin whose head carries an FG follows the recipe, even when its link
// source is BACKFILL; otherwise the next recipe save would silently revert the manual edit.
func (s *MBSpinFgManualSuite) TestRootSpinOfHeadWithFgRejectsManualEdit() {
	sh := "FGM-R"
	h := s.newHead("H5")
	poy := s.newProduct("P1", s.typeID, &sh, nil, true)
	other := s.newProduct("P2", s.typeID, &sh, nil, true)
	s.exec(`UPDATE mst_mb_head SET mbh_fg_product_id = $2 WHERE mbh_id = $1`, h, poy)
	root := s.newSpin(h, "R", fgmSpin{shade: &sh, fg: &poy, source: fgmStr("BACKFILL")})

	require.ErrorIs(s.T(), s.repo.ApplyManualFg(s.ctx, root, other, "itest"), mbspin.ErrFgFollowsRecipe)
	require.ErrorIs(s.T(), s.repo.ApplyManualFg(s.ctx, root, 0, "itest"), mbspin.ErrFgFollowsRecipe)
	fg, _, src := s.fgOf(root)
	require.Equal(s.T(), poy, fg.Int64)
	require.Equal(s.T(), "BACKFILL", src.String)
}

// P8-R BE-1: un-superseding the spin that a previous un-supersede deactivated has no supersede
// batch / log row behind it and must be rejected, leaving both spins untouched.
func (s *MBSpinFgManualSuite) TestUnsupersedeWithoutBatchRejected() {
	h := s.newHead("H6")
	sOld := s.newSpin(h, "O", fgmSpin{})
	sNew := s.newSpin(h, "N", fgmSpin{})
	// State after un-supersede(sOld): sNew inactive and superseded by sOld; sOld live.
	s.exec(`UPDATE mst_mb_spin SET mbs_is_active = FALSE, mbs_superseded_by_spin_id = $2 WHERE mbs_id = $1`, sNew, sOld)

	repo := postgres.NewMBUnsupersedeRepository(s.db)
	_, err := repo.Unsupersede(s.ctx, h, sNew, "itest")
	require.ErrorIs(s.T(), err, mbspin.ErrUnsupersedeNoBatch)

	var activeOld, activeNew bool
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx, `SELECT mbs_is_active FROM mst_mb_spin WHERE mbs_id=$1`, sOld).Scan(&activeOld))
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx, `SELECT mbs_is_active FROM mst_mb_spin WHERE mbs_id=$1`, sNew).Scan(&activeNew))
	require.True(s.T(), activeOld)
	require.False(s.T(), activeNew)
}
