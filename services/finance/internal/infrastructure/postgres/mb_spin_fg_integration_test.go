// Package postgres_test — FG (POY) link read/filter/reader integration tests (mb-spin-fg-poy-link P2-T2).
//
// Gated by INTEGRATION_TEST=true. Fixtures are prefixed ITEST-MBSPINFG- and hard-deleted
// before/after each test; the borrowed POY product is only read, except for the shade-fill
// test which uses its own throw-away product.
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

const mbSpinFgPrefix = "ITEST-MBSPINFG-"

type MBSpinFgSuite struct {
	suite.Suite
	db     *postgres.DB
	repo   *postgres.MBSpinRepository
	reader *postgres.FgProductReader
	ctx    context.Context
	headID uuid.UUID
	poyID  int64
	typeID int64
}

func TestMBSpinFgSuite(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	suite.Run(t, new(MBSpinFgSuite))
}

func (s *MBSpinFgSuite) SetupSuite() {
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
	s.reader = postgres.NewFgProductReader(s.db)

	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT mbh_id FROM mst_mb_head WHERE deleted_at IS NULL LIMIT 1`).Scan(&s.headID))
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT cpt_type_id FROM cost_product_type WHERE cpt_type_code = 'POY' LIMIT 1`).Scan(&s.typeID))
}

func (s *MBSpinFgSuite) TearDownSuite() {
	if s.db == nil {
		return
	}
	s.cleanup()
	require.NoError(s.T(), s.db.Close())
}

func (s *MBSpinFgSuite) SetupTest()    { s.cleanup() }
func (s *MBSpinFgSuite) TearDownTest() { s.cleanup() }

func (s *MBSpinFgSuite) cleanup() {
	_, err := s.db.ExecContext(s.ctx, `UPDATE mst_mb_spin SET mbs_superseded_by_spin_id = NULL WHERE mbs_mgt_name LIKE $1`, mbSpinFgPrefix+"%")
	require.NoError(s.T(), err)
	_, err = s.db.ExecContext(s.ctx, `DELETE FROM mst_mb_spin WHERE mbs_mgt_name LIKE $1`, mbSpinFgPrefix+"%")
	require.NoError(s.T(), err)
	_, err = s.db.ExecContext(s.ctx, `DELETE FROM cost_product_master WHERE cpm_product_code LIKE $1`, mbSpinFgPrefix+"%")
	require.NoError(s.T(), err)
}

func (s *MBSpinFgSuite) newPoy(code string, shade *string, erp *string) int64 {
	var id int64
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx, `
		INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name, cpm_shade_code, cpm_erp_item_code, cpm_is_active, cpm_created_by, cpm_updated_by)
		VALUES ($1, $2, $3, $4, $5, TRUE, 'itest', 'itest') RETURNING cpm_product_sys_id`,
		mbSpinFgPrefix+code, s.typeID, "POY "+code, shade, erp).Scan(&id))
	return id
}

func (s *MBSpinFgSuite) insertSpin(id uuid.UUID, shade string, fg *int64, fgItem *string, supersededBy *uuid.UUID, createdAt time.Time) {
	_, err := s.db.ExecContext(s.ctx, `
		INSERT INTO mst_mb_spin (mbs_id, mbs_mbh_id, mbs_mgt_name, mbs_mb_costing, mbs_shade_code,
			mbs_fg_product_id, mbs_fg_item_code, mbs_fg_link_source, mbs_superseded_by_spin_id,
			mbs_is_active, created_at, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,CASE WHEN CAST($6 AS BIGINT) IS NULL THEN NULL ELSE 'MANUAL' END,$8,TRUE,$9,'itest')`,
		id, s.headID, mbSpinFgPrefix+id.String()[:8], mbSpinFgPrefix+id.String()[:8], shade, fg, fgItem, supersededBy, createdAt)
	require.NoError(s.T(), err)
}

func (s *MBSpinFgSuite) TestFgRoundTripAndReadModel() {
	erp := "ORION-FG-1"
	pid := s.newPoy("RT", nil, &erp)
	id := uuid.New()
	s.insertSpin(id, mbSpinFgPrefix+"SH1", &pid, &erp, nil, baseTime())

	got, err := s.repo.GetByID(s.ctx, id)
	require.NoError(s.T(), err)
	require.NotNil(s.T(), got.FgProductID())
	require.Equal(s.T(), pid, *got.FgProductID())
	require.Equal(s.T(), erp, *got.FgItemCode())
	require.Equal(s.T(), mbspin.FgLinkSourceManual, *got.FgLinkSource())
	require.Equal(s.T(), mbSpinFgPrefix+"RT", *got.FgProductCode())
	require.Equal(s.T(), "POY RT", *got.FgProductName())
	require.False(s.T(), got.IsSuperseded())
}

func (s *MBSpinFgSuite) TestListFilters_FgAndSuperseded() {
	erp := "ITEST-ORION-FGX"
	pid := s.newPoy("LF", nil, &erp)
	live, old := uuid.New(), uuid.New()
	s.insertSpin(live, mbSpinFgPrefix+"A", &pid, &erp, nil, baseTime())
	s.insertSpin(old, mbSpinFgPrefix+"B", &pid, &erp, &live, baseTime())

	list := func(f mbspin.ListFilter) []*mbspin.Entity {
		f.Search = mbSpinFgPrefix
		items, _, err := s.repo.List(s.ctx, f)
		require.NoError(s.T(), err)
		return items
	}
	require.Len(s.T(), list(mbspin.ListFilter{}), 1, "superseded hidden by default")
	require.Len(s.T(), list(mbspin.ListFilter{IncludeSuperseded: true}), 2)
	require.Len(s.T(), list(mbspin.ListFilter{FgProductID: pid}), 1)
	require.Len(s.T(), list(mbspin.ListFilter{FgProductID: pid + 999999}), 0)
	require.Len(s.T(), list(mbspin.ListFilter{FgSearch: "itest-orion-fgx"}), 1)
	require.Len(s.T(), list(mbspin.ListFilter{FgSearch: "POY LF"}), 1)
	require.Len(s.T(), list(mbspin.ListFilter{FgSearch: "no-such-fg"}), 0)

	items := list(mbspin.ListFilter{IncludeSuperseded: true})
	for _, it := range items {
		if it.ID() == old {
			require.True(s.T(), it.IsSuperseded())
			require.NotNil(s.T(), it.SupersededBySpinCode())
		}
	}
}

func (s *MBSpinFgSuite) TestUniqueShadeFg_MapsToErrSpinShadeFgExists() {
	pid := s.newPoy("UQ", nil, nil)
	first, second := uuid.New(), uuid.New()
	s.insertSpin(first, mbSpinFgPrefix+"DUP", &pid, nil, nil, baseTime())
	s.insertSpin(second, mbSpinFgPrefix+"OTHER", &pid, nil, nil, baseTime())

	// Make the second spin collide with the first via the repository Update path.
	_, err := s.db.ExecContext(s.ctx, `UPDATE mst_mb_spin SET mbs_shade_code = $1 WHERE mbs_id = $2`, mbSpinFgPrefix+"DUP", second)
	require.Error(s.T(), err, "raw SQL must trip the unique index")

	e, err := s.repo.GetByID(s.ctx, second)
	require.NoError(s.T(), err)
	sc := mbSpinFgPrefix + "DUP"
	e.HydrateShadeAndLDR(mbspin.ShadeAndLDR{ShadeCode: &sc, LDRType: e.LDRType()})
	require.ErrorIs(s.T(), s.repo.Update(s.ctx, e), mbspin.ErrSpinShadeFgExists)
}

func (s *MBSpinFgSuite) TestFgProductReader() {
	erp := "ITEST-ERP"
	pid := s.newPoy("RD", nil, &erp)
	p, err := s.reader.GetFgProduct(s.ctx, pid)
	require.NoError(s.T(), err)
	require.Equal(s.T(), "POY", p.TypeCode)
	require.True(s.T(), p.IsActive)
	require.Equal(s.T(), erp, *p.ErpItemCode)
	require.Nil(s.T(), p.ShadeCode)

	_, err = s.reader.GetFgProduct(s.ctx, -1)
	require.ErrorIs(s.T(), err, mbspin.ErrFgNotPoy)

	ok, err := s.reader.FillEmptyFgShade(s.ctx, pid, "SHADE-X", "itest")
	require.NoError(s.T(), err)
	require.True(s.T(), ok)
	ok, err = s.reader.FillEmptyFgShade(s.ctx, pid, "SHADE-Y", "itest")
	require.NoError(s.T(), err)
	require.False(s.T(), ok, "already filled must not be overwritten")
}

// TestCreate_UniqueShadeFg409 covers createSpinOn: a second live spin for the same
// shade + FG must map to ErrSpinShadeFgExists, not ErrAlreadyExists.
func (s *MBSpinFgSuite) TestCreate_UniqueShadeFg409() {
	pid := s.newPoy("CR", nil, nil)
	shade := mbSpinFgPrefix + "CRSH"
	mk := func(name string) *mbspin.Entity {
		e, err := mbspin.New(s.headID, mbSpinFgPrefix+name, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "itest")
		require.NoError(s.T(), err)
		e.HydrateShadeAndLDR(mbspin.ShadeAndLDR{ShadeCode: &shade, LDRType: e.LDRType()})
		item := "ERP-CR"
		require.NoError(s.T(), e.WithFg(&pid, &item, mbspin.FgLinkSourceRecipe))
		return e
	}
	require.NoError(s.T(), s.repo.Create(s.ctx, mk("CR1")))
	require.ErrorIs(s.T(), s.repo.Create(s.ctx, mk("CR2")), mbspin.ErrSpinShadeFgExists)
}

// TestDuplicate_WithFg covers insertClone: FG is written as DUPLICATE, an empty POY
// shade is filled from the source shade, a second clone onto the same FG is 409, and
// a non-POY / mismatching shade is rejected.
func (s *MBSpinFgSuite) TestDuplicate_WithFg() {
	shade := mbSpinFgPrefix + "DPSH"
	src := uuid.New()
	s.insertSpin(src, shade, nil, nil, nil, baseTime())

	erp := "ERP-DP"
	pid := s.newPoy("DP", nil, &erp)
	out, err := s.repo.DuplicateSpin(s.ctx, mbspin.DuplicateInput{SourceSpinID: src, FgProductID: &pid, ActorUserID: "itest"})
	require.NoError(s.T(), err)
	clone, err := s.repo.GetByID(s.ctx, out.NewSpinID)
	require.NoError(s.T(), err)
	require.NotNil(s.T(), clone.FgProductID())
	require.Equal(s.T(), pid, *clone.FgProductID())
	require.Equal(s.T(), erp, *clone.FgItemCode())
	require.Equal(s.T(), mbspin.FgLinkSourceDuplicate, *clone.FgLinkSource())

	p, err := s.reader.GetFgProduct(s.ctx, pid)
	require.NoError(s.T(), err)
	require.NotNil(s.T(), p.ShadeCode, "empty POY shade must be filled from the source shade")

	_, err = s.repo.DuplicateSpin(s.ctx, mbspin.DuplicateInput{SourceSpinID: src, FgProductID: &pid, ActorUserID: "itest"})
	require.ErrorIs(s.T(), err, mbspin.ErrSpinShadeFgExists)

	other := "OTHER-SHADE"
	bad := s.newPoy("DPBAD", &other, nil)
	_, err = s.repo.DuplicateSpin(s.ctx, mbspin.DuplicateInput{SourceSpinID: src, FgProductID: &bad, ActorUserID: "itest"})
	require.ErrorIs(s.T(), err, mbspin.ErrFgShadeMismatch)
}
