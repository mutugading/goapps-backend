// Package postgres — end-to-end integration tests for the MB spin supersede flow
// (mb-spin-fg-poy-link P3-T7): first Validate through TransitionWithAutoGenOpts (the real
// autoGenCostProduct path) with and without confirm_supersede, and UnsupersedeMBSpin.
//
// Gated by INTEGRATION_TEST=true. Every fixture (heads, spins, products, the auto-generated
// MB products whose name is the prefixed MB costing) carries the ITEST-MBSUPF- prefix and is
// hard-deleted before/after each test. The MB_SP_CODE mst_parameter row is only read.
package postgres

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

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbsource"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

const mbSupFlowPrefix = "ITEST-MBSUPF-"

type MBSupersedeFlowSuite struct {
	suite.Suite
	db       *DB
	ctx      context.Context
	headRepo *MBHeadRepository
	unsup    *MBUnsupersedeRepository
	poyType  int64
	spParam  uuid.UUID
	machine  uuid.UUID
}

func TestMBSupersedeFlowSuite(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	suite.Run(t, new(MBSupersedeFlowSuite))
}

func (s *MBSupersedeFlowSuite) SetupSuite() {
	s.ctx = context.Background()
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		mbSpinAutoGenEnvOrDefault("TEST_DB_HOST", "localhost"), mbSpinAutoGenEnvOrDefault("TEST_DB_PORT", "5434"),
		mbSpinAutoGenEnvOrDefault("TEST_DB_USER", "finance"), mbSpinAutoGenEnvOrDefault("TEST_DB_PASSWORD", "finance123"),
		mbSpinAutoGenEnvOrDefault("TEST_DB_NAME", "finance_db"))
	raw, err := sql.Open("pgx", dsn)
	require.NoError(s.T(), err)
	require.NoError(s.T(), mbSpinAutoGenWaitForDB(raw, 10*time.Second))
	s.db = NewDBFromSQL(raw)
	s.headRepo = NewMBHeadRepository(s.db, NewMBCompositionRepository(s.db))
	s.unsup = NewMBUnsupersedeRepository(s.db)
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT cpt_type_id FROM cost_product_type WHERE cpt_type_code = 'POY' LIMIT 1`).Scan(&s.poyType))
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT id FROM mst_parameter WHERE param_code = 'MB_SP_CODE' AND deleted_at IS NULL LIMIT 1`).Scan(&s.spParam))
	// Borrowed (read only): the frozen MB_MACHINE_FIXED_TOTAL param needs a machine with a fixed cost.
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT mc_id FROM mst_machine WHERE deleted_at IS NULL AND mc_tot_fxd_cst IS NOT NULL LIMIT 1`).Scan(&s.machine))
}

func (s *MBSupersedeFlowSuite) TearDownSuite() {
	if s.db == nil {
		return
	}
	s.cleanup()
	require.NoError(s.T(), s.db.Close())
}

func (s *MBSupersedeFlowSuite) SetupTest()    { s.cleanup() }
func (s *MBSupersedeFlowSuite) TearDownTest() { s.cleanup() }

func (s *MBSupersedeFlowSuite) exec(q string, args ...any) {
	_, err := s.db.ExecContext(s.ctx, q, args...)
	require.NoError(s.T(), err)
}

func (s *MBSupersedeFlowSuite) cleanup() {
	like := mbSupFlowPrefix + "%"
	const heads = `SELECT mbh_id FROM mst_mb_head WHERE mbh_mb_costing LIKE $1`
	const spins = `SELECT mbs_id FROM mst_mb_spin WHERE mbs_mbh_id IN (` + heads + `)`
	const products = `SELECT cpm_product_sys_id FROM cost_product_master WHERE cpm_product_code LIKE $1 OR cpm_product_name LIKE $1`
	s.exec(`DELETE FROM mst_mb_spin_supersede_item WHERE mssi_old_spin_id IN (`+spins+`) OR mssi_new_spin_id IN (`+spins+`)`, like)
	s.exec(`UPDATE mst_mb_spin SET mbs_superseded_by_spin_id = NULL WHERE mbs_mbh_id IN (`+heads+`)`, like)
	s.exec(`DELETE FROM mst_mb_spin WHERE mbs_mbh_id IN (`+heads+`)`, like)
	s.exec(`UPDATE mst_mb_head SET mbh_superseded_by_head_id = NULL, mbh_cost_product_id = NULL WHERE mbh_mb_costing LIKE $1`, like)
	s.exec(`DELETE FROM mst_mb_head WHERE mbh_mb_costing LIKE $1`, like)
	s.exec(`DELETE FROM cost_route_rm WHERE crm_parent_product_sys_id IN (`+products+`)`, like)
	s.exec(`DELETE FROM cost_route_seq WHERE crs_product_sys_id IN (`+products+`)`, like)
	s.exec(`DELETE FROM cost_route_head WHERE crh_product_sys_id IN (`+products+`)`, like)
	s.exec(`DELETE FROM cost_product_master WHERE cpm_product_code LIKE $1 OR cpm_product_name LIKE $1`, like)
}

// newPoy inserts a POY product (FG or a consumer of MB_SP_CODE).
func (s *MBSupersedeFlowSuite) newPoy(code string) int64 {
	var id int64
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx, `
		INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name, cpm_is_active, cpm_created_by, cpm_updated_by)
		VALUES ($1, $2, $3, TRUE, 'itest', 'itest') RETURNING cpm_product_sys_id`,
		mbSupFlowPrefix+code, s.poyType, mbSupFlowPrefix+"poy "+code).Scan(&id))
	return id
}

// flowHead is a DRAFT head row plus the in-memory entity Validate runs with.
type flowHead struct {
	id     uuid.UUID
	entity *mbhead.Entity
	orion  string
}

func (s *MBSupersedeFlowSuite) newHead(tag, shade string, fg *int64) flowHead {
	h := flowHead{id: uuid.New(), orion: mbSupFlowPrefix + "OR-" + uuid.NewString()[:6]}
	costing := mbSupFlowPrefix + tag + "-" + uuid.NewString()[:6]
	s.exec(`INSERT INTO mst_mb_head (mbh_id, mbh_mb_costing, mbh_shade_code, mbh_fg_product_id, mbh_current_version, mbh_entry_status, created_by)
		VALUES ($1, $2, $3, $4, 1, 'DRAFT', 'itest')`, h.id, costing, shade, fg)
	orion := h.orion
	machine := s.machine
	e, err := mbhead.New(mbhead.NewParams{MBCosting: costing, CreatedBy: "itest", ShadeCode: shade, OrionItemCode: &orion, MachineID: &machine})
	require.NoError(s.T(), err)
	one := "1"
	e.FreezeParams(&one, &one, &one, &one, &one, &one, "A", "S")
	if fg != nil {
		require.NoError(s.T(), e.SetFg(fg, nil))
	}
	h.entity = e
	return h
}

func (s *MBSupersedeFlowSuite) validate(h flowHead, confirm bool) (*mbhead.SupersedeOutcome, error) {
	return s.headRepo.TransitionWithAutoGenOpts(s.ctx, h.id, mbhead.StatusDraft, mbhead.StatusValidated, 1, "",
		"itest-validator", nil, h.entity, mbhead.AutoGenOptions{ConfirmSupersede: confirm})
}

// rootSpin returns the single spin auto-generated for the head.
func (s *MBSupersedeFlowSuite) rootSpin(head uuid.UUID) uuid.UUID {
	var id uuid.UUID
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT mbs_id FROM mst_mb_spin WHERE mbs_mbh_id = $1 AND deleted_at IS NULL`, head).Scan(&id))
	return id
}

func (s *MBSupersedeFlowSuite) count(q string, args ...any) int {
	var n int
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx, q, args...).Scan(&n))
	return n
}

func (s *MBSupersedeFlowSuite) spParamRow(product int64, text string, spin *uuid.UUID, filledBy string) {
	s.exec(`INSERT INTO cost_product_parameter (cpp_product_sys_id, cpp_param_id, cpp_value_text, cpp_value_mb_spin_id, cpp_filled_by, cpp_created_by)
		VALUES ($1, $2, $3, $4, $5, 'itest')`, product, s.spParam, text, spin, filledBy)
}

type flowParam struct {
	text     string
	spin     sql.NullString
	filledBy string
}

func (s *MBSupersedeFlowSuite) spRow(product int64) flowParam {
	var p flowParam
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx, `
		SELECT COALESCE(cpp_value_text, ''), CAST(cpp_value_mb_spin_id AS TEXT), COALESCE(cpp_filled_by, '')
		FROM cost_product_parameter WHERE cpp_product_sys_id = $1 AND cpp_param_id = $2`, product, s.spParam).
		Scan(&p.text, &p.spin, &p.filledBy))
	return p
}

type flowSpin struct {
	active       bool
	supersededBy sql.NullString
	supersededAt sql.NullTime
	fg           sql.NullInt64
	costProduct  sql.NullInt64
}

func (s *MBSupersedeFlowSuite) spin(id uuid.UUID) flowSpin {
	var f flowSpin
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx, `
		SELECT mbs_is_active, CAST(mbs_superseded_by_spin_id AS TEXT), mbs_superseded_at, mbs_fg_product_id, mbs_cost_product_id
		FROM mst_mb_spin WHERE mbs_id = $1`, id).Scan(&f.active, &f.supersededBy, &f.supersededAt, &f.fg, &f.costProduct))
	return f
}

type flowHeadRow struct {
	active       bool
	status       string
	supersededBy sql.NullString
	supersededAt sql.NullTime
	costProduct  sql.NullInt64
}

func (s *MBSupersedeFlowSuite) head(id uuid.UUID) flowHeadRow {
	var h flowHeadRow
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx, `
		SELECT mbh_is_active, mbh_entry_status, CAST(mbh_superseded_by_head_id AS TEXT), mbh_superseded_at, mbh_cost_product_id
		FROM mst_mb_head WHERE mbh_id = $1`, id).Scan(&h.active, &h.status, &h.supersededBy, &h.supersededAt, &h.costProduct))
	return h
}

func (s *MBSupersedeFlowSuite) productActive(id int64) bool {
	var active bool
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT cpm_is_active FROM cost_product_master WHERE cpm_product_sys_id = $1`, id).Scan(&active))
	return active
}

// supersedeFixture is scenario A's state right before the new head is validated.
type supersedeFixture struct {
	fg                  int64
	oldHead, newHead    flowHead
	oldSpin             uuid.UUID
	oldMB               int64
	pPointer, pText, p3 int64
}

func (s *MBSupersedeFlowSuite) seedScenarioA() supersedeFixture {
	f := supersedeFixture{fg: s.newPoy("FG")}
	shade := mbSupFlowPrefix + "SH-A"
	f.oldHead = s.newHead("OLD", shade, &f.fg)
	out, err := s.validate(f.oldHead, false)
	require.NoError(s.T(), err)
	require.Nil(s.T(), out, "first recipe has nothing to supersede")
	f.oldSpin = s.rootSpin(f.oldHead.id)
	f.oldMB = s.head(f.oldHead.id).costProduct.Int64
	require.NotZero(s.T(), f.oldMB)

	f.pPointer, f.pText, f.p3 = s.newPoy("P1"), s.newPoy("P2"), s.newPoy("P3")
	s.spParamRow(f.pPointer, f.oldHead.orion, &f.oldSpin, "manual")
	s.spParamRow(f.pText, f.oldHead.orion, nil, "import")
	s.spParamRow(f.p3, f.oldHead.orion, &f.oldSpin, "auto_mb_source")

	// Same shade (different case / padding) + same FG on another recipe.
	f.newHead = s.newHead("NEW", " "+mbSupFlowPrefix+"sh-a ", &f.fg)
	return f
}

func (s *MBSupersedeFlowSuite) TestScenarioA_WithoutConfirm_409AndNothingChanged() {
	f := s.seedScenarioA()

	out, err := s.validate(f.newHead, false)
	require.ErrorIs(s.T(), err, mbspin.ErrSupersedeConfirmationRequired)
	require.Nil(s.T(), out)
	require.Contains(s.T(), err.Error(), f.oldHead.orion, "error names the old spin code (bulk failure message)")

	h := s.head(f.newHead.id)
	require.Equal(s.T(), mbhead.StatusDraft, h.status, "transition rolled back")
	require.False(s.T(), h.costProduct.Valid, "no MB product generated")
	require.Zero(s.T(), s.count(`SELECT COUNT(*) FROM mst_mb_spin WHERE mbs_mbh_id = $1`, f.newHead.id))
	require.Zero(s.T(), s.count(`SELECT COUNT(*) FROM cost_product_master WHERE cpm_product_name = $1`, f.newHead.entity.MBCosting()))
	require.Zero(s.T(), s.count(`SELECT COUNT(*) FROM mst_mb_workflow_log WHERE mbwl_mbh_id = $1`, f.newHead.id))
	require.Zero(s.T(), s.count(`SELECT COUNT(*) FROM mst_mb_spin_supersede_item WHERE mssi_old_spin_id = $1`, f.oldSpin))

	old := s.spin(f.oldSpin)
	require.True(s.T(), old.active)
	require.False(s.T(), old.supersededBy.Valid)
	require.True(s.T(), s.head(f.oldHead.id).active)
	require.True(s.T(), s.productActive(f.oldMB))
	require.Equal(s.T(), flowParam{f.oldHead.orion, sql.NullString{String: f.oldSpin.String(), Valid: true}, "manual"}, s.spRow(f.pPointer))
	require.Equal(s.T(), flowParam{f.oldHead.orion, sql.NullString{}, "import"}, s.spRow(f.pText))
}

// confirmScenarioA validates the new head with confirm and checks the supersede end state.
func (s *MBSupersedeFlowSuite) confirmScenarioA(f supersedeFixture) (uuid.UUID, *mbhead.SupersedeOutcome) {
	out, err := s.validate(f.newHead, true)
	require.NoError(s.T(), err)
	require.NotNil(s.T(), out)
	newSpin := s.rootSpin(f.newHead.id)
	require.Equal(s.T(), newSpin, out.NewSpinID)
	require.Equal(s.T(), []uuid.UUID{f.oldSpin}, out.OldSpinIDs)
	require.Equal(s.T(), []uuid.UUID{f.oldHead.id}, out.DeactivatedHeadIDs)
	require.Equal(s.T(), []int64{f.oldMB}, out.DeactivatedProductIDs)
	require.Subset(s.T(), out.AffectedProductIDs, []int64{f.pPointer, f.pText, f.p3})

	old := s.spin(f.oldSpin)
	require.False(s.T(), old.active)
	require.Equal(s.T(), newSpin.String(), old.supersededBy.String)
	require.True(s.T(), old.supersededAt.Valid)
	ns := s.spin(newSpin)
	require.True(s.T(), ns.active)
	require.False(s.T(), ns.supersededBy.Valid)
	require.Equal(s.T(), f.fg, ns.fg.Int64)

	want := flowParam{f.newHead.orion, sql.NullString{String: newSpin.String(), Valid: true}, mbsource.FilledBySupersede}
	require.Equal(s.T(), want, s.spRow(f.pPointer), "pointer row (manual) repointed")
	require.Equal(s.T(), want, s.spRow(f.pText), "text-only row repointed")
	require.Equal(s.T(), want, s.spRow(f.p3))

	kinds := map[string]int{}
	rows, err := s.db.QueryContext(s.ctx, `SELECT mssi_repoint_kind, mssi_batch_id, mssi_new_spin_id FROM mst_mb_spin_supersede_item
		WHERE mssi_old_spin_id = $1 AND mssi_param_code = 'MB_SP_CODE'`, f.oldSpin)
	require.NoError(s.T(), err)
	for rows.Next() {
		var kind string
		var batch, ns2 uuid.UUID
		require.NoError(s.T(), rows.Scan(&kind, &batch, &ns2))
		require.Equal(s.T(), out.BatchID, batch)
		require.Equal(s.T(), newSpin, ns2)
		kinds[kind]++
	}
	require.NoError(s.T(), rows.Err())
	require.NoError(s.T(), rows.Close())
	require.Equal(s.T(), map[string]int{mbspin.SupersedeRepointPointer: 2, mbspin.SupersedeRepointTextOnly: 1}, kinds)

	oh := s.head(f.oldHead.id)
	require.False(s.T(), oh.active)
	require.Equal(s.T(), mbhead.StatusValidated, oh.status, "workflow status not changed")
	require.Equal(s.T(), f.newHead.id.String(), oh.supersededBy.String)
	require.True(s.T(), oh.supersededAt.Valid)
	require.False(s.T(), s.productActive(f.oldMB))
	require.Equal(s.T(), mbhead.StatusValidated, s.head(f.newHead.id).status)

	for _, id := range []uuid.UUID{f.newHead.id, f.oldHead.id} {
		require.Equal(s.T(), 1, s.count(`SELECT COUNT(*) FROM mst_mb_workflow_log WHERE mbwl_mbh_id = $1
			AND mbwl_to_state = 'SUPERSEDE' AND mbwl_meta->>'batch_id' = $2`, id, out.BatchID.String()))
	}
	return newSpin, out
}

func (s *MBSupersedeFlowSuite) TestScenarioA_WithConfirm_SupersedesInOneTx() {
	s.confirmScenarioA(s.seedScenarioA())
}

func (s *MBSupersedeFlowSuite) TestUnsupersede_RestoresParamsAndSkipsChanged() {
	f := s.seedScenarioA()
	newSpin, _ := s.confirmScenarioA(f)

	// A param changed after the supersede must be skipped and left untouched.
	s.exec(`UPDATE cost_product_parameter SET cpp_value_text = 'ITEST-MANUAL-X', cpp_value_mb_spin_id = NULL, cpp_filled_by = 'manual'
		WHERE cpp_product_sys_id = $1 AND cpp_param_id = $2`, f.p3, s.spParam)

	res, err := s.unsup.Unsupersede(s.ctx, f.oldHead.id, f.oldSpin, "itest-restorer")
	require.NoError(s.T(), err)
	require.Equal(s.T(), f.oldSpin, res.Spin.ID())
	require.Equal(s.T(), newSpin, res.NewSpinID)

	require.Equal(s.T(), flowParam{f.oldHead.orion, sql.NullString{String: f.oldSpin.String(), Valid: true}, "manual"}, s.spRow(f.pPointer))
	require.Equal(s.T(), flowParam{f.oldHead.orion, sql.NullString{}, "import"}, s.spRow(f.pText))
	require.Equal(s.T(), flowParam{"ITEST-MANUAL-X", sql.NullString{}, "manual"}, s.spRow(f.p3), "changed row untouched")

	var changedSkipped bool
	for _, sk := range res.Skipped {
		if sk.ProductSysID == f.p3 && sk.ParamCode == "MB_SP_CODE" {
			require.Equal(s.T(), mbspin.UnsupersedeSkipChanged, sk.Reason)
			changedSkipped = true
		}
	}
	require.True(s.T(), changedSkipped, "changed row reported as skipped: %+v", res.Skipped)
	require.Subset(s.T(), res.AffectedProductIDs, []int64{f.pPointer, f.pText})
	require.NotContains(s.T(), res.AffectedProductIDs, f.p3)
	require.Equal(s.T(), 2, s.count(`SELECT COUNT(*) FROM mst_mb_spin_supersede_item WHERE mssi_old_spin_id = $1
		AND mssi_param_code = 'MB_SP_CODE' AND mssi_reverted_at IS NOT NULL`, f.oldSpin))

	old := s.spin(f.oldSpin)
	require.True(s.T(), old.active)
	require.False(s.T(), old.supersededBy.Valid)
	require.False(s.T(), old.supersededAt.Valid)
	ns := s.spin(newSpin)
	require.False(s.T(), ns.active)
	require.Equal(s.T(), f.oldSpin.String(), ns.supersededBy.String)

	oh := s.head(f.oldHead.id)
	require.True(s.T(), oh.active)
	require.False(s.T(), oh.supersededBy.Valid)
	require.False(s.T(), oh.supersededAt.Valid)
	require.True(s.T(), s.productActive(f.oldMB))
	require.True(s.T(), s.head(f.newHead.id).active, "new head is not deactivated automatically")
	require.Equal(s.T(), 1, s.count(`SELECT COUNT(*) FROM mst_mb_workflow_log WHERE mbwl_mbh_id = $1 AND mbwl_to_state = 'UNSUPERSEDE'`, f.oldHead.id))

	_, err = s.unsup.Unsupersede(s.ctx, f.oldHead.id, f.oldSpin, "itest-restorer")
	require.ErrorIs(s.T(), err, mbspin.ErrNotSuperseded, "a restored spin cannot be restored twice")
}

func (s *MBSupersedeFlowSuite) TestScenarioB_SameShadeOtherFg_NoConflict() {
	fgA, fgB := s.newPoy("FGA"), s.newPoy("FGB")
	shade := mbSupFlowPrefix + "SH-B"
	a := s.newHead("BA", shade, &fgA)
	b := s.newHead("BB", shade, &fgB)
	_, err := s.validate(a, false)
	require.NoError(s.T(), err)
	out, err := s.validate(b, false)
	require.NoError(s.T(), err)
	require.Nil(s.T(), out)

	sa, sb := s.spin(s.rootSpin(a.id)), s.spin(s.rootSpin(b.id))
	require.True(s.T(), sa.active && !sa.supersededBy.Valid)
	require.True(s.T(), sb.active && !sb.supersededBy.Valid)
	require.True(s.T(), s.head(a.id).active)
}

func (s *MBSupersedeFlowSuite) TestScenarioC_LegacySpinWithoutFg_Unaffected() {
	shade := mbSupFlowPrefix + "SH-C"
	legacy := s.newHead("CL", shade, nil)
	_, err := s.validate(legacy, false)
	require.NoError(s.T(), err)
	legacySpin := s.rootSpin(legacy.id)
	require.False(s.T(), s.spin(legacySpin).fg.Valid)

	fg := s.newPoy("FGC")
	withFg := s.newHead("CF", shade, &fg)
	out, err := s.validate(withFg, false)
	require.NoError(s.T(), err)
	require.Nil(s.T(), out)

	// A head without FG never checks for a conflict, even against a live FG spin.
	noFg := s.newHead("CN", shade, nil)
	out, err = s.validate(noFg, false)
	require.NoError(s.T(), err)
	require.Nil(s.T(), out)

	ls := s.spin(legacySpin)
	require.True(s.T(), ls.active)
	require.False(s.T(), ls.supersededBy.Valid)
	require.True(s.T(), s.head(legacy.id).active)
	require.Zero(s.T(), s.count(`SELECT COUNT(*) FROM mst_mb_spin_supersede_item WHERE mssi_old_spin_id = $1`, legacySpin))
}

// P3-R1: a supersede that wrote no param rows must still be restorable.
func (s *MBSupersedeFlowSuite) TestUnsupersede_NoParamRows_Succeeds() {
	fg := s.newPoy("FGNP")
	shade := mbSupFlowPrefix + "SH-NP"
	oldHead := s.newHead("NPO", shade, &fg)
	_, err := s.validate(oldHead, false)
	require.NoError(s.T(), err)
	oldSpin := s.rootSpin(oldHead.id)
	newHead := s.newHead("NPN", shade, &fg)
	out, err := s.validate(newHead, true)
	require.NoError(s.T(), err)
	require.NotNil(s.T(), out)
	require.Zero(s.T(), s.count(`SELECT COUNT(*) FROM mst_mb_spin_supersede_item WHERE mssi_old_spin_id = $1`, oldSpin))

	res, err := s.unsup.Unsupersede(s.ctx, oldHead.id, oldSpin, "itest-restorer")
	require.NoError(s.T(), err)
	require.Equal(s.T(), uuid.Nil, res.BatchID)
	require.Zero(s.T(), res.ParamsReverted)
	require.True(s.T(), s.spin(oldSpin).active)
	require.False(s.T(), s.spin(oldSpin).supersededBy.Valid)
	require.False(s.T(), s.spin(out.NewSpinID).active)
	require.True(s.T(), s.head(oldHead.id).active)
}

// P3-R3: only what the supersede deactivated is re-activated; a product that was already
// inactive before the supersede stays inactive.
func (s *MBSupersedeFlowSuite) TestUnsupersede_ReactivatesOnlyWhatSupersedeDeactivated() {
	f := s.seedScenarioA()
	s.exec(`UPDATE cost_product_master SET cpm_is_active = FALSE WHERE cpm_product_sys_id = $1`, f.oldMB)
	out, err := s.validate(f.newHead, true)
	require.NoError(s.T(), err)
	require.NotNil(s.T(), out)
	require.Empty(s.T(), out.DeactivatedProductIDs, "already inactive product is not deactivated by supersede")

	_, err = s.unsup.Unsupersede(s.ctx, f.oldHead.id, f.oldSpin, "itest-restorer")
	require.NoError(s.T(), err)
	require.True(s.T(), s.head(f.oldHead.id).active)
	require.False(s.T(), s.productActive(f.oldMB), "pre-existing inactive product stays inactive")
}

// P3-R4/R6: a third live spin with the same (shade, FG) blocks the restore, nothing changes.
func (s *MBSupersedeFlowSuite) TestUnsupersede_ThirdLiveSpin_Conflict() {
	f := s.seedScenarioA()
	newSpin, _ := s.confirmScenarioA(f)
	s.exec(`UPDATE mst_mb_spin SET mbs_is_active = FALSE WHERE mbs_id = $1`, newSpin)
	third := s.newHead("THIRD", mbSupFlowPrefix+"SH-A", &f.fg)
	s.exec(`INSERT INTO mst_mb_spin (mbs_id, mbs_mbh_id, mbs_mgt_name, mbs_mb_costing, mbs_shade_code, mbs_fg_product_id, mbs_is_active, created_at, created_by)
		VALUES ($1, $2, 'ITEST-MBSUPF-T', 'ITEST-MBSUPF-T', $3, $4, TRUE, NOW(), 'itest')`,
		uuid.New(), third.id, mbSupFlowPrefix+"SH-A", f.fg)

	_, err := s.unsup.Unsupersede(s.ctx, f.oldHead.id, f.oldSpin, "itest-restorer")
	require.ErrorIs(s.T(), err, mbspin.ErrUnsupersedeConflict)
	require.False(s.T(), s.spin(f.oldSpin).active)
	require.Equal(s.T(), newSpin.String(), s.spin(f.oldSpin).supersededBy.String)
	require.False(s.T(), s.head(f.oldHead.id).active)
}
