// Package postgres_test — FG backfill repository + apply flow integration tests (mb-spin-fg-poy-link P6-T3a).
//
// Gated by INTEGRATION_TEST=true. All fixtures are prefixed ITEST-MBSPINBF- and
// hard-deleted before/after each test; candidates are filtered to that prefix so
// existing data never influences assertions.
package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

const bfPrefix = "ITEST-MBSPINBF-"

type MBSpinFgBackfillSuite struct {
	suite.Suite
	db     *postgres.DB
	repo   *postgres.MBSpinFgBackfillRepository
	ctx    context.Context
	typeID int64
}

func TestMBSpinFgBackfillSuite(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	suite.Run(t, new(MBSpinFgBackfillSuite))
}

func (s *MBSpinFgBackfillSuite) SetupSuite() {
	s.ctx = context.Background()
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		getEnvOrDefault("TEST_DB_HOST", "localhost"), getEnvOrDefault("TEST_DB_PORT", "5434"),
		getEnvOrDefault("TEST_DB_USER", "finance"), getEnvOrDefault("TEST_DB_PASSWORD", "finance123"),
		getEnvOrDefault("TEST_DB_NAME", "finance_db"))
	raw, err := sql.Open("pgx", dsn)
	require.NoError(s.T(), err)
	require.NoError(s.T(), waitForDB(raw, 10*time.Second))
	s.db = postgres.NewDBFromSQL(raw)
	s.repo = postgres.NewMBSpinFgBackfillRepository(s.db)
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT cpt_type_id FROM cost_product_type WHERE cpt_type_code = 'POY' LIMIT 1`).Scan(&s.typeID))
}

func (s *MBSpinFgBackfillSuite) TearDownSuite() {
	if s.db == nil {
		return
	}
	s.cleanup()
	require.NoError(s.T(), s.db.Close())
}

func (s *MBSpinFgBackfillSuite) SetupTest()    { s.cleanup() }
func (s *MBSpinFgBackfillSuite) TearDownTest() { s.cleanup() }

func (s *MBSpinFgBackfillSuite) exec(q string, args ...any) {
	_, err := s.db.ExecContext(s.ctx, q, args...)
	require.NoError(s.T(), err)
}

func (s *MBSpinFgBackfillSuite) cleanup() {
	like := bfPrefix + "%"
	s.exec(`UPDATE mst_mb_spin SET mbs_superseded_by_spin_id = NULL, mbs_parent_spin_id = NULL WHERE mbs_mgt_name LIKE $1`, like)
	s.exec(`DELETE FROM mst_mb_spin WHERE mbs_mgt_name LIKE $1`, like)
	s.exec(`DELETE FROM mst_mb_head WHERE mbh_mb_costing LIKE $1`, like)
	s.exec(`DELETE FROM cost_product_master WHERE cpm_product_code LIKE $1`, like)
}

func (s *MBSpinFgBackfillSuite) newHead(tag string, fg *int64) uuid.UUID {
	id := uuid.New()
	s.exec(`INSERT INTO mst_mb_head (mbh_id, mbh_mb_costing, mbh_fg_product_id, created_by) VALUES ($1,$2,$3,'itest')`,
		id, bfPrefix+"H-"+tag, fg)
	return id
}

// newPoy inserts a POY; name is stored as given (matcher normalizes "POY " away).
func (s *MBSpinFgBackfillSuite) newPoy(code, name string, shade, erp *string, active bool) int64 {
	var id int64
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx, `
		INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name, cpm_shade_code, cpm_erp_item_code, cpm_is_active, cpm_created_by, cpm_updated_by)
		VALUES ($1,$2,$3,$4,$5,$6,'itest','itest') RETURNING cpm_product_sys_id`,
		bfPrefix+code, s.typeID, name, shade, erp, active).Scan(&id))
	return id
}

type bfSpin struct {
	head   uuid.UUID
	parent *uuid.UUID
	shade  *string
	final  *string
	fg     *int64
}

func (s *MBSpinFgBackfillSuite) newSpin(tag string, o bfSpin) uuid.UUID {
	id := uuid.New()
	s.exec(`
		INSERT INTO mst_mb_spin (mbs_id, mbs_mbh_id, mbs_parent_spin_id, mbs_mgt_name, mbs_mb_costing, mbs_shade_code,
			mbs_final_product, mbs_fg_product_id, mbs_fg_link_source, mbs_is_active, created_by)
		VALUES ($1,$2,$3,$4,$4,$5,$6,$7,CASE WHEN CAST($7 AS BIGINT) IS NULL THEN NULL ELSE 'MANUAL' END,TRUE,'itest')`,
		id, o.head, o.parent, bfPrefix+"S-"+tag, o.shade, o.final, o.fg)
	return id
}

func bfStr(v string) *string { return &v }

func (s *MBSpinFgBackfillSuite) candidates() []mbspin.BackfillCandidate {
	all, err := s.repo.ListBackfillCandidates(s.ctx)
	require.NoError(s.T(), err)
	var out []mbspin.BackfillCandidate
	for _, c := range all {
		if strings.HasPrefix(c.MgtName, bfPrefix) {
			out = append(out, c)
		}
	}
	return out
}

func (s *MBSpinFgBackfillSuite) rowsBySpin() map[string]mbspin.BackfillRow {
	m := map[string]mbspin.BackfillRow{}
	for _, r := range mbspin.ClassifyBackfillAll(s.candidates()) {
		m[r.Candidate.SpinID] = r
	}
	return m
}

func (s *MBSpinFgBackfillSuite) apply(spin uuid.UUID, poy int64) mbspin.BackfillApplyOutcome {
	out, err := s.repo.ApplyBackfillRow(s.ctx, mbspin.BackfillApplyRequest{SpinID: spin.String(), PoyProductSysID: poy, Actor: "itest-backfill"})
	require.NoError(s.T(), err)
	return out
}

type spinFg struct {
	fg     sql.NullInt64
	item   sql.NullString
	source sql.NullString
}

func (s *MBSpinFgBackfillSuite) spinFg(id uuid.UUID) spinFg {
	var r spinFg
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT mbs_fg_product_id, mbs_fg_item_code, mbs_fg_link_source FROM mst_mb_spin WHERE mbs_id=$1`, id).Scan(&r.fg, &r.item, &r.source))
	return r
}

func (s *MBSpinFgBackfillSuite) headFg(id uuid.UUID) (fg sql.NullInt64, item sql.NullString) {
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT mbh_fg_product_id, mbh_fg_item_code FROM mst_mb_head WHERE mbh_id=$1`, id).Scan(&fg, &item))
	return fg, item
}

func (s *MBSpinFgBackfillSuite) snapshot() string {
	var out []string
	for _, q := range []string{
		`SELECT COUNT(*) || '|' || COALESCE(MAX(updated_at)::text,'') || '|' || COUNT(mbs_fg_product_id) FROM mst_mb_spin`,
		`SELECT COUNT(*) || '|' || COALESCE(MAX(updated_at)::text,'') || '|' || COUNT(mbh_fg_product_id) FROM mst_mb_head`,
		`SELECT COUNT(*) || '|' || COALESCE(MAX(cpp_updated_at)::text,'') FROM cost_product_parameter`,
	} {
		var v string
		// Test-only snapshot; "::" is fine here (Go driver, not the user's SQL client).
		require.NoError(s.T(), s.db.QueryRowContext(s.ctx, q).Scan(&v))
		out = append(out, v)
	}
	return strings.Join(out, " ## ")
}

func (s *MBSpinFgBackfillSuite) TestDryRunIsReadOnlyAndBuckets() {
	shA, shB, shC, shE, shF, shG := bfPrefix+"SA", bfPrefix+"SB", bfPrefix+"SC", bfPrefix+"SE", bfPrefix+"SF", bfPrefix+"SG"
	h := s.newHead("bk", nil)
	pA := s.newPoy("PA", "POY ALPHA 1/36", &shA, nil, true)
	pB := s.newPoy("PB", "POY BETA 2/48", &shB, nil, true)
	_ = s.newPoy("PE", "POY QQQ", &shE, nil, true)
	_ = s.newPoy("PF", "POY FOX", &shF, nil, true)
	pG := s.newPoy("PG", "POY GOLF", &shG, nil, true)

	spA := s.newSpin("A", bfSpin{head: h, shade: &shA, final: bfStr("POY ALPHA 1")})
	spB := s.newSpin("B", bfSpin{head: h, shade: &shB})
	spC := s.newSpin("C", bfSpin{head: h, shade: &shC})
	spD := s.newSpin("D", bfSpin{head: h})
	spE := s.newSpin("E", bfSpin{head: h, shade: &shE, final: bfStr("ZZZ")})
	spF1 := s.newSpin("F1", bfSpin{head: h, shade: &shF})
	spF2 := s.newSpin("F2", bfSpin{head: h, shade: &shF})
	spG1 := s.newSpin("G1", bfSpin{head: h, shade: &shG, fg: &pG})
	spG2 := s.newSpin("G2", bfSpin{head: h, shade: &shG})

	before := s.snapshot()
	rows := s.rowsBySpin()
	require.Equal(s.T(), before, s.snapshot(), "dry-run (candidate listing) must not write anything")

	check := func(id uuid.UUID, action mbspin.BackfillAction, reason string, poy int64) {
		r, ok := rows[id.String()]
		require.True(s.T(), ok)
		require.Equal(s.T(), action, r.Action, "spin %s", id)
		require.Equal(s.T(), reason, r.Reason, "spin %s", id)
		if poy != 0 {
			require.NotNil(s.T(), r.Poy)
			require.Equal(s.T(), poy, r.Poy.ProductSysID)
		}
	}
	check(spA, mbspin.BackfillActionApply, mbspin.BackfillReasonSpecShadeUnique, pA)
	check(spB, mbspin.BackfillActionReview, mbspin.BackfillReasonShadeOnlyUnique, pB)
	check(spC, mbspin.BackfillActionSkip, mbspin.BackfillReasonNoPoyWithShade, 0)
	check(spD, mbspin.BackfillActionSkip, mbspin.BackfillReasonNoShade, 0)
	check(spE, mbspin.BackfillActionSkip, mbspin.BackfillReasonSpecNoMatch, 0)
	check(spF1, mbspin.BackfillActionSkip, mbspin.BackfillReasonConflictSameTarget, 0)
	check(spF2, mbspin.BackfillActionSkip, mbspin.BackfillReasonConflictSameTarget, 0)
	check(spG1, mbspin.BackfillActionSkip, mbspin.BackfillReasonFgAlreadySet, 0)
	check(spG2, mbspin.BackfillActionSkip, mbspin.BackfillReasonConflictExistingLive, 0)
	require.Equal(s.T(), spG1.String(), rows[spG2.String()].ConflictSpinID)
}

func (s *MBSpinFgBackfillSuite) TestApplyRootWritesSpinAndHead() {
	sh := bfPrefix + "R1"
	erp := "ORION-R1"
	h := s.newHead("r1", nil)
	poy := s.newPoy("R1", "POY R1", &sh, &erp, true)
	spin := s.newSpin("R1", bfSpin{head: h, shade: &sh})

	cppBefore := s.cppSnapshot()
	out := s.apply(spin, poy)
	require.Equal(s.T(), mbspin.BackfillResultApplied, out.Result)
	require.True(s.T(), out.HeadUpdated)

	got := s.spinFg(spin)
	require.Equal(s.T(), poy, got.fg.Int64)
	require.Equal(s.T(), erp, got.item.String)
	require.Equal(s.T(), mbspin.FgLinkSourceBackfill, got.source.String)
	hfg, hitem := s.headFg(h)
	require.Equal(s.T(), poy, hfg.Int64)
	require.Equal(s.T(), erp, hitem.String)
	require.Equal(s.T(), cppBefore, s.cppSnapshot(), "cost_product_parameter must be untouched")

	// Idempotent rerun.
	again := s.apply(spin, poy)
	require.Equal(s.T(), mbspin.BackfillResultSkipped, again.Result)
	require.Equal(s.T(), mbspin.BackfillReasonAlreadyLinked, again.Reason)
	require.Equal(s.T(), got, s.spinFg(spin))
}

func (s *MBSpinFgBackfillSuite) cppSnapshot() string {
	var v string
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT COUNT(*) || '|' || COALESCE(MAX(cpp_updated_at)::text,'') FROM cost_product_parameter`).Scan(&v))
	return v
}

func (s *MBSpinFgBackfillSuite) TestApplyNonRootDoesNotTouchHead() {
	sh := bfPrefix + "NR"
	h := s.newHead("nr", nil)
	poy := s.newPoy("NR", "POY NR", &sh, nil, true)
	root := s.newSpin("NRroot", bfSpin{head: h})
	child := s.newSpin("NRchild", bfSpin{head: h, parent: &root, shade: &sh})

	out := s.apply(child, poy)
	require.Equal(s.T(), mbspin.BackfillResultApplied, out.Result)
	require.False(s.T(), out.HeadUpdated)
	hfg, _ := s.headFg(h)
	require.False(s.T(), hfg.Valid, "non-root spin must not set the head FG")
	got := s.spinFg(child)
	require.Equal(s.T(), poy, got.fg.Int64)
	require.False(s.T(), got.item.Valid, "blank ERP item code is written as NULL")
}

func (s *MBSpinFgBackfillSuite) TestExistingFgNeverOverwritten() {
	sh := bfPrefix + "OW"
	other := s.newPoy("OWo", "POY OWO", bfStr(bfPrefix+"OWX"), nil, true)
	h := s.newHead("ow", &other)
	poy := s.newPoy("OW", "POY OW", &sh, nil, true)

	// Spin already has a (different) FG.
	withFg := s.newSpin("OWfg", bfSpin{head: h, shade: &sh, fg: &other})
	out := s.apply(withFg, poy)
	require.Equal(s.T(), mbspin.BackfillResultSkipped, out.Result)
	require.Equal(s.T(), mbspin.BackfillReasonFgAlreadySet, out.Reason)
	require.Equal(s.T(), other, s.spinFg(withFg).fg.Int64)
	require.Equal(s.T(), "MANUAL", s.spinFg(withFg).source.String)

	// Root spin applies, but a head that already has an FG keeps it.
	sh2 := bfPrefix + "OW2"
	poy2 := s.newPoy("OW2", "POY OW2", &sh2, nil, true)
	root := s.newSpin("OWroot", bfSpin{head: h, shade: &sh2})
	out = s.apply(root, poy2)
	require.Equal(s.T(), mbspin.BackfillResultApplied, out.Result)
	require.False(s.T(), out.HeadUpdated)
	hfg, _ := s.headFg(h)
	require.Equal(s.T(), other, hfg.Int64)
}

func (s *MBSpinFgBackfillSuite) TestApplySkipsInvalidStates() {
	h := s.newHead("sk", nil)
	shM, shI, shL := bfPrefix+"MM", bfPrefix+"II", bfPrefix+"LL"
	assertSkipped := func(spin uuid.UUID, poy int64, reason string) {
		out := s.apply(spin, poy)
		require.Equal(s.T(), mbspin.BackfillResultSkipped, out.Result)
		require.Equal(s.T(), reason, out.Reason)
		require.False(s.T(), s.spinFg(spin).fg.Valid, "skip must not write")
	}

	// shade mismatch
	pMis := s.newPoy("MM", "POY MM", bfStr(bfPrefix+"MMo"), nil, true)
	assertSkipped(s.newSpin("MM", bfSpin{head: h, shade: &shM}), pMis, mbspin.BackfillReasonShadeMismatch)

	// inactive POY
	pIn := s.newPoy("II", "POY II", &shI, nil, false)
	assertSkipped(s.newSpin("II", bfSpin{head: h, shade: &shI}), pIn, mbspin.BackfillReasonPoyInvalid)

	// non-live spins: inactive, deleted, superseded
	pL := s.newPoy("LL", "POY LL", &shL, nil, true)
	inactive := s.newSpin("LLina", bfSpin{head: h, shade: &shL})
	s.exec(`UPDATE mst_mb_spin SET mbs_is_active = FALSE WHERE mbs_id=$1`, inactive)
	assertSkipped(inactive, pL, mbspin.BackfillReasonSpinNotLive)
	deleted := s.newSpin("LLdel", bfSpin{head: h, shade: &shL})
	s.exec(`UPDATE mst_mb_spin SET deleted_at = NOW(), deleted_by='itest' WHERE mbs_id=$1`, deleted)
	assertSkipped(deleted, pL, mbspin.BackfillReasonSpinNotLive)
	succ := s.newSpin("LLsucc", bfSpin{head: h, shade: bfStr(bfPrefix + "LLs")})
	old := s.newSpin("LLold", bfSpin{head: h, shade: &shL})
	s.exec(`UPDATE mst_mb_spin SET mbs_superseded_by_spin_id=$1 WHERE mbs_id=$2`, succ, old)
	assertSkipped(old, pL, mbspin.BackfillReasonSpinNotLive)

	// unknown POY id
	assertSkipped(s.newSpin("MISS", bfSpin{head: h, shade: bfStr(bfPrefix + "MISS")}), -42, mbspin.BackfillReasonPoyInvalid)
}

func (s *MBSpinFgBackfillSuite) TestApplyConflictExistingLive() {
	sh := bfPrefix + "CX"
	h := s.newHead("cx", nil)
	poy := s.newPoy("CX", "POY CX", &sh, nil, true)
	s.newSpin("CXlive", bfSpin{head: h, shade: &sh, fg: &poy})
	target := s.newSpin("CXtarget", bfSpin{head: h, shade: &sh})

	out := s.apply(target, poy)
	require.Equal(s.T(), mbspin.BackfillResultSkipped, out.Result)
	require.Equal(s.T(), mbspin.BackfillReasonConflictExistingLive, out.Reason)
	require.False(s.T(), s.spinFg(target).fg.Valid)
}
