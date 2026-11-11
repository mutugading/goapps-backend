// Package postgres_test — FG-aware MB lookup integration tests (mb-spin-fg-poy-link P4-T1):
// MBSpinRepository.FindByShadeAndFg and FgCandidateReader.FgCandidates.
//
// Gated by INTEGRATION_TEST=true. Fixtures are prefixed ITEST-FGLK- and hard-deleted
// before/after each test.
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

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbsource"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

const fgLookupPrefix = "ITEST-FGLK-"

type FgLookupSuite struct {
	suite.Suite
	db     *postgres.DB
	repo   *postgres.MBSpinRepository
	reader *postgres.FgCandidateReader
	ctx    context.Context
	headID uuid.UUID
	types  map[string]int64
}

func TestFgLookupSuite(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	suite.Run(t, new(FgLookupSuite))
}

func (s *FgLookupSuite) SetupSuite() {
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
	s.reader = postgres.NewFgCandidateReader(s.db)

	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT mbh_id FROM mst_mb_head WHERE deleted_at IS NULL LIMIT 1`).Scan(&s.headID))
	s.types = map[string]int64{}
	for _, code := range []string{"POY", "DTY", "MEL"} {
		var id int64
		require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
			`SELECT cpt_type_id FROM cost_product_type WHERE cpt_type_code = $1 LIMIT 1`, code).Scan(&id))
		s.types[code] = id
	}
}

func (s *FgLookupSuite) TearDownSuite() {
	if s.db == nil {
		return
	}
	s.cleanup()
	require.NoError(s.T(), s.db.Close())
}

func (s *FgLookupSuite) SetupTest()    { s.cleanup() }
func (s *FgLookupSuite) TearDownTest() { s.cleanup() }

func (s *FgLookupSuite) cleanup() {
	stmts := []string{
		`DELETE FROM mst_mb_spin WHERE mbs_mgt_name LIKE $1 AND mbs_superseded_by_spin_id IS NOT NULL`,
		`DELETE FROM mst_mb_spin WHERE mbs_mgt_name LIKE $1`,
		`DELETE FROM cost_route_head WHERE crh_product_sys_id IN (
			SELECT cpm_product_sys_id FROM cost_product_master WHERE cpm_product_code LIKE $1)`,
		`DELETE FROM cost_product_master WHERE cpm_product_code LIKE $1`,
	}
	for _, q := range stmts {
		_, err := s.db.ExecContext(s.ctx, q, fgLookupPrefix+"%")
		require.NoError(s.T(), err)
	}
}

func (s *FgLookupSuite) product(code, typ string) int64 {
	var id int64
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx, `
		INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name, cpm_is_active, cpm_created_by, cpm_updated_by)
		VALUES ($1, $2, $3, TRUE, 'itest', 'itest') RETURNING cpm_product_sys_id`,
		fgLookupPrefix+code, s.types[typ], typ+" "+code).Scan(&id))
	return id
}

func (s *FgLookupSuite) head(pid int64, status string, version int) int64 {
	var id int64
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx, `
		INSERT INTO cost_route_head (crh_product_sys_id, crh_routing_status, crh_version, crh_created_by, crh_updated_by)
		VALUES ($1, $2, $3, 'itest', 'itest') RETURNING crh_head_id`, pid, status, version).Scan(&id))
	return id
}

func (s *FgLookupSuite) seq(headID, pid int64, level, seq int) int64 {
	var id int64
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx, `
		INSERT INTO cost_route_seq (crs_head_id, crs_product_sys_id, crs_route_level, crs_route_seq, crs_created_by, crs_updated_by)
		VALUES ($1, $2, $3, $4, 'itest', 'itest') RETURNING crs_seq_id`, headID, pid, level, seq).Scan(&id))
	return id
}

func (s *FgLookupSuite) rmProduct(seqID, parent, rm int64) {
	_, err := s.db.ExecContext(s.ctx, `
		INSERT INTO cost_route_rm (crm_seq_id, crm_parent_product_sys_id, crm_rm_product_sys_id, crm_rm_type,
			crm_route_rm_ratio, crm_created_by, crm_updated_by)
		VALUES ($1, $2, $3, 'PRODUCT', 1.0, 'itest', 'itest')`, seqID, parent, rm)
	require.NoError(s.T(), err)
}

func (s *FgLookupSuite) rmRef(seqID, parent int64, typ, ref string) {
	col := "crm_rm_item_code"
	if typ == "GROUP" {
		col = "crm_rm_group_code"
	}
	_, err := s.db.ExecContext(s.ctx, `
		INSERT INTO cost_route_rm (crm_seq_id, crm_parent_product_sys_id, `+col+`, crm_rm_type,
			crm_route_rm_ratio, crm_created_by, crm_updated_by)
		VALUES ($1, $2, $3, $4, 1.0, 'itest', 'itest')`, seqID, parent, ref, typ)
	require.NoError(s.T(), err)
}

func (s *FgLookupSuite) spin(shade string, fg *int64, active bool, supersededBy *uuid.UUID, deleted bool) uuid.UUID {
	id := uuid.New()
	var deletedAt *time.Time
	if deleted {
		now := time.Now()
		deletedAt = &now
	}
	_, err := s.db.ExecContext(s.ctx, `
		INSERT INTO mst_mb_spin (mbs_id, mbs_mbh_id, mbs_mgt_name, mbs_mb_costing, mbs_shade_code,
			mbs_fg_product_id, mbs_fg_link_source, mbs_superseded_by_spin_id, mbs_is_active, deleted_at, created_by)
		VALUES ($1,$2,$3,$3,$4,$5,CASE WHEN CAST($5 AS BIGINT) IS NULL THEN NULL ELSE 'MANUAL' END,$6,$7,$8,'itest')`,
		id, s.headID, fgLookupPrefix+id.String()[:8], shade, fg, supersededBy, active, deletedAt)
	require.NoError(s.T(), err)
	return id
}

func (s *FgLookupSuite) TestFindByShadeAndFg_OnlyLiveActiveNonSupersededFgSpin() {
	poy := s.product("POY1", "POY")
	poy2 := s.product("POY2", "POY")
	shade := fgLookupPrefix + "sh1"

	live := s.spin("  "+shade+" ", &poy, true, nil, false)
	_ = s.spin(shade, &poy, true, &live, false) // superseded -> ignored
	_ = s.spin(shade, nil, true, nil, false)    // legacy (no FG) -> ignored
	_ = s.spin(shade, &poy2, false, nil, false) // inactive -> ignored
	_ = s.spin(shade, &poy2, true, nil, true)   // deleted -> ignored

	keyLower := mbsource.ShadeFgKey{Shade: " " + fgLookupPrefix + "SH1", FgProductID: poy}
	got, err := s.repo.FindByShadeAndFg(s.ctx, []mbsource.ShadeFgKey{
		keyLower,
		{Shade: shade, FgProductID: poy2},
		{Shade: fgLookupPrefix + "NOPE", FgProductID: poy},
	})
	s.Require().NoError(err)
	s.Require().Len(got, 1)
	norm := mbsource.ShadeFgKey{Shade: mbsource.NormalizeShade(shade), FgProductID: poy}
	s.Require().Contains(got, norm)
	s.Require().Equal(live, got[norm].ID())

	empty, err := s.repo.FindByShadeAndFg(s.ctx, nil)
	s.Require().NoError(err)
	s.Require().Empty(empty)
}

func (s *FgLookupSuite) TestFgCandidates_Rules() {
	poySelf := s.product("PSELF", "POY")
	poyA := s.product("PA", "POY")
	poyB := s.product("PB", "POY")
	dtyRM := s.product("DRM", "DTY")

	// DTY with a live DRAFT route: POY RM + ITEM/GROUP/non-POY product RMs.
	dty := s.product("DTY", "DTY")
	h := s.head(dty, "DRAFT", 1)
	sq := s.seq(h, dty, 1, 1)
	s.rmProduct(sq, dty, poyA)
	s.rmProduct(sq, dty, dtyRM)
	s.rmRef(sq, dty, "ITEM", "ITEST-FGLK-I")
	s.rmRef(sq, dty, "GROUP", "ITEST-FGLK-G")

	// MEL: poyB at level 1 seq 2, poyA at level 1 seq 1 (then again at level 2) -> [poyA, poyB].
	mel := s.product("MEL", "MEL")
	hm := s.head(mel, "COMPLETE", 1)
	s1 := s.seq(hm, mel, 1, 2)
	s.rmProduct(s1, mel, poyB)
	s2 := s.seq(hm, mel, 1, 1)
	s.rmProduct(s2, mel, poyA)
	s3 := s.seq(hm, mel, 2, 1)
	s.rmProduct(s3, mel, poyA)
	// Deleted seq with another POY is ignored.
	poyDel := s.product("PDEL", "POY")
	s4 := s.seq(hm, mel, 3, 1)
	s.rmProduct(s4, mel, poyDel)
	_, err := s.db.ExecContext(s.ctx, `UPDATE cost_route_seq SET crs_deleted_at = NOW() WHERE crs_seq_id = $1`, s4)
	s.Require().NoError(err)

	// LOCKED-only product: the highest version LOCKED head wins.
	locked := s.product("LCK", "DTY")
	hOld := s.head(locked, "LOCKED", 1)
	s.rmProduct(s.seq(hOld, locked, 1, 1), locked, poyB)
	hNew := s.head(locked, "LOCKED", 2)
	s.rmProduct(s.seq(hNew, locked, 1, 1), locked, poyA)

	// Active head preferred over a LOCKED one.
	both := s.product("BOTH", "DTY")
	hl := s.head(both, "LOCKED", 3)
	s.rmProduct(s.seq(hl, both, 1, 1), both, poyA)
	ha := s.head(both, "DRAFT", 4)
	s.rmProduct(s.seq(ha, both, 1, 1), both, poyB)

	// Soft-deleted live head is skipped: the older LOCKED head is used.
	softDel := s.product("SDEL", "DTY")
	hLive := s.head(softDel, "DRAFT", 2)
	s.rmProduct(s.seq(hLive, softDel, 1, 1), softDel, poyA)
	hLk := s.head(softDel, "LOCKED", 1)
	s.rmProduct(s.seq(hLk, softDel, 1, 1), softDel, poyB)
	_, err = s.db.ExecContext(s.ctx, `UPDATE cost_route_head SET crh_deleted_at = NOW() WHERE crh_head_id = $1`, hLive)
	s.Require().NoError(err)

	noRoute := s.product("NORT", "DTY")

	got, err := s.reader.FgCandidates(s.ctx, []int64{poySelf, dty, mel, locked, both, softDel, noRoute})
	s.Require().NoError(err)
	s.Require().Equal([]int64{poySelf}, got[poySelf])
	s.Require().Equal([]int64{poyA}, got[dty])
	s.Require().Equal([]int64{poyA, poyB}, got[mel])
	s.Require().Equal([]int64{poyA}, got[locked])
	s.Require().Equal([]int64{poyB}, got[both])
	s.Require().Equal([]int64{poyB}, got[softDel])
	s.Require().NotContains(got, noRoute)
	s.Require().Len(got, 6)

	empty, err := s.reader.FgCandidates(s.ctx, nil)
	s.Require().NoError(err)
	s.Require().Empty(empty)
}
