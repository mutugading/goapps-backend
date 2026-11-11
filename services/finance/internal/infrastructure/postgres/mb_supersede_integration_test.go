// Package postgres_test — supersede candidate / preview integration tests (mb-spin-fg-poy-link P3-T1).
//
// Gated by INTEGRATION_TEST=true. Fixtures are prefixed ITEST-MBSUP- and hard-deleted
// before/after each test. The borrowed MB_SP_CODE mst_parameter row is only read.
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

const mbSupPrefix = "ITEST-MBSUP-"

type MBSupersedeSuite struct {
	suite.Suite
	db      *postgres.DB
	repo    *postgres.MBSupersedeRepository
	ctx     context.Context
	poyType int64
	spParam uuid.UUID
}

func TestMBSupersedeSuite(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	suite.Run(t, new(MBSupersedeSuite))
}

func (s *MBSupersedeSuite) SetupSuite() {
	s.ctx = context.Background()
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		getEnvOrDefault("TEST_DB_HOST", "localhost"), getEnvOrDefault("TEST_DB_PORT", "5434"),
		getEnvOrDefault("TEST_DB_USER", "finance"), getEnvOrDefault("TEST_DB_PASSWORD", "finance123"),
		getEnvOrDefault("TEST_DB_NAME", "finance_db"))
	raw, err := sql.Open("pgx", dsn)
	require.NoError(s.T(), err)
	require.NoError(s.T(), waitForDB(raw, 10*time.Second))
	s.db = postgres.NewDBFromSQL(raw)
	s.repo = postgres.NewMBSupersedeRepository(s.db)
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT cpt_type_id FROM cost_product_type WHERE cpt_type_code = 'POY' LIMIT 1`).Scan(&s.poyType))
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT id FROM mst_parameter WHERE param_code = 'MB_SP_CODE' AND deleted_at IS NULL LIMIT 1`).Scan(&s.spParam))
}

func (s *MBSupersedeSuite) TearDownSuite() {
	if s.db == nil {
		return
	}
	s.cleanup()
	require.NoError(s.T(), s.db.Close())
}

func (s *MBSupersedeSuite) SetupTest()    { s.cleanup() }
func (s *MBSupersedeSuite) TearDownTest() { s.cleanup() }

func (s *MBSupersedeSuite) exec(q string, args ...any) {
	_, err := s.db.ExecContext(s.ctx, q, args...)
	require.NoError(s.T(), err)
}

func (s *MBSupersedeSuite) cleanup() {
	like := mbSupPrefix + "%"
	s.exec(`UPDATE mst_mb_spin SET mbs_superseded_by_spin_id = NULL WHERE mbs_mgt_name LIKE $1`, like)
	s.exec(`DELETE FROM mst_mb_spin WHERE mbs_mgt_name LIKE $1`, like)
	s.exec(`DELETE FROM mst_mb_head WHERE mbh_mb_costing LIKE $1`, like)
	s.exec(`DELETE FROM cost_product_master WHERE cpm_product_code LIKE $1`, like)
}

func (s *MBSupersedeSuite) newProduct(code string) int64 {
	var id int64
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx, `
		INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name, cpm_is_active, cpm_created_by, cpm_updated_by)
		VALUES ($1, $2, $3, TRUE, 'itest', 'itest') RETURNING cpm_product_sys_id`,
		mbSupPrefix+code, s.poyType, "Prod "+code).Scan(&id))
	return id
}

func (s *MBSupersedeSuite) newHead(code string, costProduct *int64) uuid.UUID {
	id := uuid.New()
	s.exec(`INSERT INTO mst_mb_head (mbh_id, mbh_mb_costing, mbh_cost_product_id, mbh_current_version, mbh_entry_status, created_by)
		VALUES ($1, $2, $3, 1, 'VALIDATED', 'itest')`, id, mbSupPrefix+code, costProduct)
	return id
}

func (s *MBSupersedeSuite) newSpin(head uuid.UUID, shade string, fg *int64, orion *string, costProduct *int64) uuid.UUID {
	id := uuid.New()
	s.exec(`INSERT INTO mst_mb_spin (mbs_id, mbs_mbh_id, mbs_mgt_name, mbs_mb_costing, mbs_orion_item_code, mbs_shade_code,
			mbs_fg_product_id, mbs_fg_link_source, mbs_cost_product_id, mbs_is_active, created_at, created_by)
		VALUES ($1,$2,$3,$3,$4,$5,$6,CASE WHEN CAST($6 AS BIGINT) IS NULL THEN NULL ELSE 'RECIPE' END,$7,TRUE,NOW(),'itest')`,
		id, head, mbSupPrefix+id.String()[:8], orion, shade, fg, costProduct)
	return id
}

func (s *MBSupersedeSuite) newParamRow(product int64, text string, spin *uuid.UUID, filledBy string) {
	s.exec(`INSERT INTO cost_product_parameter (cpp_product_sys_id, cpp_param_id, cpp_value_text, cpp_value_mb_spin_id, cpp_filled_by, cpp_created_by)
		VALUES ($1, $2, $3, $4, $5, 'itest')`, product, s.spParam, text, spin, filledBy)
}


func (s *MBSupersedeSuite) TestCandidateFoundWithCounts() {
	fg := s.newProduct("FG1")
	oldMB := s.newProduct("MBOLD")
	oldHead := s.newHead("OLD", &oldMB)
	newHead := s.newHead("NEW", nil)
	shade := mbSupPrefix + "SH1"
	orion := mbSupPrefix + "ORION1"
	oldSpin := s.newSpin(oldHead, shade, &fg, &orion, &oldMB)

	p1, p2, p3, locked := s.newProduct("P1"), s.newProduct("P2"), s.newProduct("P3"), s.newProduct("PL")
	s.newParamRow(p1, orion, &oldSpin, "auto_mb_source")
	s.newParamRow(p2, "whatever", &oldSpin, "manual")
	s.newParamRow(p3, " "+orion+" ", nil, "import") // text-only, case/space-insensitive
	s.newParamRow(locked, orion, &oldSpin, "manual")
	s.exec(`UPDATE cost_product_master SET cpm_is_locked = TRUE WHERE cpm_product_sys_id = $1`, locked)

	got, err := s.repo.PreviewSupersede(s.ctx, newHead, " "+shade+" ", fg)
	require.NoError(s.T(), err)
	require.Len(s.T(), got, 1)
	c := got[0]
	require.Equal(s.T(), oldSpin, c.OldSpinID)
	require.Equal(s.T(), orion, c.OldSpinCode)
	require.Equal(s.T(), oldHead, c.OldHeadID)
	require.Equal(s.T(), mbSupPrefix+"OLD", c.OldHeadCode)
	require.Equal(s.T(), mbSupPrefix+"FG1", c.FgProductCode)
	require.True(s.T(), c.OldHeadWillDeactivate)
	require.Equal(s.T(), oldMB, c.OldMBProductSysID)
	require.True(s.T(), c.OldMBProductWillDeactivate)
	require.Equal(s.T(), 2, c.ParamRowsPointer)
	require.Equal(s.T(), 1, c.ParamRowsTextOnly)
	require.Equal(s.T(), []mbspin.SupersedeFilledByCount{
		{FilledBy: "auto_mb_source", Count: 1}, {FilledBy: "import", Count: 1}, {FilledBy: "manual", Count: 1},
	}, c.FilledByCounts)
	require.Len(s.T(), c.AffectedProducts, 3)
	require.Len(s.T(), c.Warnings, 1, "locked product reported")
}

func (s *MBSupersedeSuite) TestOldHeadKeepsOtherLiveSpin_NotDeactivated() {
	fg := s.newProduct("FG2")
	oldMB := s.newProduct("MBOLD2")
	oldHead := s.newHead("OLD2", &oldMB)
	newHead := s.newHead("NEW2", nil)
	shade := mbSupPrefix + "SH2"
	s.newSpin(oldHead, shade, &fg, nil, &oldMB)
	s.newSpin(oldHead, mbSupPrefix+"OTHER", nil, nil, &oldMB) // sibling still live

	got, err := s.repo.PreviewSupersede(s.ctx, newHead, shade, fg)
	require.NoError(s.T(), err)
	require.Len(s.T(), got, 1)
	require.False(s.T(), got[0].OldHeadWillDeactivate)
	require.False(s.T(), got[0].OldMBProductWillDeactivate)
}

func (s *MBSupersedeSuite) TestOtherFgSameHeadAndLegacyNotCandidates() {
	fgA, fgB := s.newProduct("FGA"), s.newProduct("FGB")
	oldHead := s.newHead("OLD3", nil)
	newHead := s.newHead("NEW3", nil)
	shade := mbSupPrefix + "SH3"
	s.newSpin(oldHead, shade, &fgB, nil, nil) // other FG
	s.newSpin(newHead, shade, &fgA, nil, nil) // same head
	s.newSpin(oldHead, shade, nil, nil, nil)  // legacy FG NULL

	got, err := s.repo.PreviewSupersede(s.ctx, newHead, shade, fgA)
	require.NoError(s.T(), err)
	require.Empty(s.T(), got)

	got, err = s.repo.PreviewSupersede(s.ctx, newHead, shade, 0)
	require.NoError(s.T(), err)
	require.Empty(s.T(), got, "no FG never conflicts")
}

func (s *MBSupersedeSuite) TestSupersededOrInactiveNotCandidates() {
	fg := s.newProduct("FG4")
	oldHead := s.newHead("OLD4", nil)
	newHead := s.newHead("NEW4", nil)
	shade := mbSupPrefix + "SH4"
	inactive := s.newSpin(oldHead, shade, &fg, nil, nil)
	s.exec(`UPDATE mst_mb_spin SET mbs_is_active = FALSE WHERE mbs_id = $1`, inactive)

	got, err := s.repo.PreviewSupersede(s.ctx, newHead, shade, fg)
	require.NoError(s.T(), err)
	require.Empty(s.T(), got)
}

func (s *MBSupersedeSuite) TestDuplicateOrionCodeTextRowsWarned() {
	fg := s.newProduct("FG5")
	oldHead := s.newHead("OLD5", nil)
	otherHead := s.newHead("OTH5", nil)
	newHead := s.newHead("NEW5", nil)
	shade := mbSupPrefix + "SH5"
	orion := mbSupPrefix + "DUPORION"
	oldSpin := s.newSpin(oldHead, shade, &fg, &orion, nil)
	s.newSpin(otherHead, mbSupPrefix+"SHX", nil, &orion, nil) // same ORION code, other spin

	p1, p2 := s.newProduct("D1"), s.newProduct("D2")
	s.newParamRow(p1, orion, nil, "import")
	s.newParamRow(p2, orion, &oldSpin, "auto_mb_source")

	got, err := s.repo.PreviewSupersede(s.ctx, newHead, shade, fg)
	require.NoError(s.T(), err)
	require.Len(s.T(), got, 1)
	require.Equal(s.T(), 1, got[0].ParamRowsPointer)
	require.Zero(s.T(), got[0].ParamRowsTextOnly)
	require.Len(s.T(), got[0].Warnings, 1)
	require.Contains(s.T(), got[0].Warnings[0], "shared")
}
