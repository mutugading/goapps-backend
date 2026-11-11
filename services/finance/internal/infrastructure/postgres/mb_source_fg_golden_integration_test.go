// Package postgres_test — FG-aware MB source autofill end to end (mb-spin-fg-poy-link P4-T5):
// golden "no live FG spin = old rule", FG wins for POY / DTY / MEL, Re-link (ApplyForce)
// and GetCostProductMbLink explanations against a real database.
//
// Gated by INTEGRATION_TEST=true. Fixtures are prefixed ITEST-FGGD- and hard-deleted
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
	"github.com/stretchr/testify/suite"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbsourceautofill"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductparameter"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbsource"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

const fgGoldenPrefix = "ITEST-FGGD-"

type FgGoldenSuite struct {
	suite.Suite
	db     *postgres.DB
	ctx    context.Context
	headID uuid.UUID
	types  map[string]int64
}

func TestFgGoldenSuite(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	suite.Run(t, new(FgGoldenSuite))
}

func (s *FgGoldenSuite) SetupSuite() {
	s.ctx = context.Background()
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		getEnvOrDefault("TEST_DB_HOST", "localhost"), getEnvOrDefault("TEST_DB_PORT", "5434"),
		getEnvOrDefault("TEST_DB_USER", "finance"), getEnvOrDefault("TEST_DB_PASSWORD", "finance123"),
		getEnvOrDefault("TEST_DB_NAME", "finance_db"))
	raw, err := sql.Open("pgx", dsn)
	s.Require().NoError(err)
	s.Require().NoError(waitForDB(raw, 10*time.Second))
	s.db = postgres.NewDBFromSQL(raw)
	s.Require().NoError(s.db.QueryRowContext(s.ctx,
		`SELECT mbh_id FROM mst_mb_head WHERE deleted_at IS NULL LIMIT 1`).Scan(&s.headID))
	s.types = map[string]int64{}
	for _, code := range []string{"POY", "DTY", "MEL", "FG"} {
		var id int64
		s.Require().NoError(s.db.QueryRowContext(s.ctx,
			`SELECT cpt_type_id FROM cost_product_type WHERE cpt_type_code = $1 LIMIT 1`, code).Scan(&id))
		s.types[code] = id
	}
}

func (s *FgGoldenSuite) TearDownSuite() {
	if s.db == nil {
		return
	}
	s.cleanup()
	s.Require().NoError(s.db.Close())
}

func (s *FgGoldenSuite) SetupTest()    { s.cleanup() }
func (s *FgGoldenSuite) TearDownTest() { s.cleanup() }

const fgGoldenProducts = `SELECT cpm_product_sys_id FROM cost_product_master WHERE cpm_product_code LIKE $1`

func (s *FgGoldenSuite) cleanup() {
	for _, q := range []string{
		`DELETE FROM cost_product_applicable_param WHERE capp_product_sys_id IN (` + fgGoldenProducts + `)`,
		`DELETE FROM cost_product_parameter WHERE cpp_product_sys_id IN (` + fgGoldenProducts + `)`,
		`DELETE FROM mst_mb_spin WHERE mbs_mgt_name LIKE $1 AND mbs_superseded_by_spin_id IS NOT NULL`,
		`DELETE FROM mst_mb_spin WHERE mbs_mgt_name LIKE $1`,
		`DELETE FROM cost_route_head WHERE crh_product_sys_id IN (` + fgGoldenProducts + `)`,
		`DELETE FROM cost_product_master WHERE cpm_product_code LIKE $1`,
		`DELETE FROM cost_superba_cost_sp WHERE shade_code LIKE $1`,
	} {
		_, err := s.db.ExecContext(s.ctx, q, fgGoldenPrefix+"%")
		s.Require().NoError(err)
	}
}

func (s *FgGoldenSuite) product(code, typ, shade string, locked bool) int64 {
	var id int64
	s.Require().NoError(s.db.QueryRowContext(s.ctx, `
		INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name, cpm_shade_code,
			cpm_is_locked, cpm_is_active, cpm_created_by, cpm_updated_by)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5, TRUE, 'itest', 'itest') RETURNING cpm_product_sys_id`,
		fgGoldenPrefix+code, s.types[typ], typ+" "+code, shade, locked).Scan(&id))
	return id
}

// route gives product pid a live DRAFT route whose level-1 seqs (in order) consume rms.
func (s *FgGoldenSuite) route(pid int64, rms ...int64) {
	var head int64
	s.Require().NoError(s.db.QueryRowContext(s.ctx, `
		INSERT INTO cost_route_head (crh_product_sys_id, crh_routing_status, crh_version, crh_created_by, crh_updated_by)
		VALUES ($1, 'DRAFT', 1, 'itest', 'itest') RETURNING crh_head_id`, pid).Scan(&head))
	for i, rm := range rms {
		var seq int64
		s.Require().NoError(s.db.QueryRowContext(s.ctx, `
			INSERT INTO cost_route_seq (crs_head_id, crs_product_sys_id, crs_route_level, crs_route_seq, crs_created_by, crs_updated_by)
			VALUES ($1, $2, 1, $3, 'itest', 'itest') RETURNING crs_seq_id`, head, pid, i+1).Scan(&seq))
		_, err := s.db.ExecContext(s.ctx, `
			INSERT INTO cost_route_rm (crm_seq_id, crm_parent_product_sys_id, crm_rm_product_sys_id, crm_rm_type,
				crm_route_rm_ratio, crm_created_by, crm_updated_by)
			VALUES ($1, $2, $3, 'PRODUCT', 1.0, 'itest', 'itest')`, seq, pid, rm)
		s.Require().NoError(err)
	}
}

type fgSpinOpt struct {
	shade, status, orion string
	ageDays              int
	fg                   int64 // 0 = legacy
	active               bool
	supersededBy         *uuid.UUID
}

func (s *FgGoldenSuite) spin(o fgSpinOpt) uuid.UUID {
	id := uuid.New()
	var fg *int64
	if o.fg != 0 {
		fg = &o.fg
	}
	_, err := s.db.ExecContext(s.ctx, `
		INSERT INTO mst_mb_spin (mbs_id, mbs_mbh_id, mbs_mgt_name, mbs_shade_code, mbs_status, mbs_orion_item_code,
			mbs_cost_rate_mkt, mbs_fg_product_id, mbs_fg_link_source, mbs_superseded_by_spin_id, mbs_is_active,
			created_at, created_by)
		VALUES ($1,$2,$3,$4,$5,$6, 12.5, $7, CASE WHEN CAST($7 AS BIGINT) IS NULL THEN NULL ELSE 'MANUAL' END, $8, $9,
			NOW() - CAST($10 || ' days' AS INTERVAL), 'itest')`,
		id, s.headID, fgGoldenPrefix+o.orion, o.shade, o.status, fgGoldenPrefix+o.orion, fg, o.supersededBy,
		o.active, fmt.Sprint(o.ageDays))
	s.Require().NoError(err)
	return id
}

func (s *FgGoldenSuite) superba(shade string) {
	_, err := s.db.ExecContext(s.ctx, `INSERT INTO cost_superba_cost_sp (legacy_sys_id, shade_code, colour_name, old_value, source)
		VALUES ($1, $2, 'SKY BLUE', 1, 'MANUAL')`, time.Now().UnixNano()%1_000_000_000+int64(len(shade)), shade)
	s.Require().NoError(err)
}

func (s *FgGoldenSuite) paramID(code string) uuid.UUID {
	var id uuid.UUID
	s.Require().NoError(s.db.QueryRowContext(s.ctx,
		`SELECT id FROM mst_parameter WHERE param_code = $1 AND deleted_at IS NULL`, code).Scan(&id))
	return id
}

func (s *FgGoldenSuite) chains() (oldR, newR *mbsource.Resolver, fgP *mbsource.MBSpinFgProvider) {
	paramRepo := postgres.NewParameterRepository(s.db)
	spinRepo := postgres.NewMBSpinRepository(s.db)
	superba := postgres.NewSuperbaCostSpRepository(s.db)
	oldR = mbsource.NewResolver(mbsource.NewMBSpinProvider(spinRepo, paramRepo), mbsource.NewSuperbaProvider(superba))
	fgP = mbsource.NewMBSpinFgProvider(postgres.NewFgCandidateReader(s.db), spinRepo, paramRepo)
	newR = mbsource.NewResolver(fgP, mbsource.NewMBSpinProvider(spinRepo, paramRepo), mbsource.NewSuperbaProvider(superba))
	return oldR, newR, fgP
}

// shadeOnlyResolver hides ResolveTargets so the service runs the pre-P4 Resolve(shades) path.
type shadeOnlyResolver struct{ r *mbsource.Resolver }

func (x shadeOnlyResolver) Resolve(ctx context.Context, shades []string) (map[string]mbsource.Resolution, error) {
	return x.r.Resolve(ctx, shades)
}

func (s *FgGoldenSuite) autofill(r mbsourceautofill.ShadeResolver, ids []int64) mbsourceautofill.Stats {
	svc := mbsourceautofill.New(postgres.NewMBSourceAutoFillStore(s.db), r, postgres.NewParameterRepository(s.db))
	stats, err := svc.AutoFillMBSource(s.ctx, ids, "itest")
	s.Require().NoError(err)
	return stats
}

// snapshot returns every cost_product_parameter / applicable row of the fixture products as text.
func (s *FgGoldenSuite) snapshot() []string {
	rows, err := s.db.QueryContext(s.ctx, `
		SELECT m.cpm_product_code || '|' || p.param_code || '|' || COALESCE(c.cpp_value_text, '<nil>') || '|' ||
		       COALESCE(CAST(c.cpp_value_numeric AS TEXT), '<nil>') || '|' ||
		       COALESCE(CAST(c.cpp_value_mb_spin_id AS TEXT), '<nil>') || '|' || COALESCE(c.cpp_filled_by, '<nil>')
		FROM cost_product_parameter c
		JOIN mst_parameter p ON p.id = c.cpp_param_id
		JOIN cost_product_master m ON m.cpm_product_sys_id = c.cpp_product_sys_id
		WHERE m.cpm_product_code LIKE $1
		UNION ALL
		SELECT 'CAPP|' || m.cpm_product_code || '|' || p.param_code
		FROM cost_product_applicable_param a
		JOIN mst_parameter p ON p.id = a.capp_param_id
		JOIN cost_product_master m ON m.cpm_product_sys_id = a.capp_product_sys_id
		WHERE m.cpm_product_code LIKE $1
		ORDER BY 1`, fgGoldenPrefix+"%")
	s.Require().NoError(err)
	defer func() { s.Require().NoError(rows.Close()) }()
	var out []string
	for rows.Next() {
		var line string
		s.Require().NoError(rows.Scan(&line))
		out = append(out, line)
	}
	s.Require().NoError(rows.Err())
	return out
}

func (s *FgGoldenSuite) resetValues() {
	for _, q := range []string{
		`DELETE FROM cost_product_applicable_param WHERE capp_product_sys_id IN (` + fgGoldenProducts + `)`,
		`DELETE FROM cost_product_parameter WHERE cpp_product_sys_id IN (` + fgGoldenProducts + `)`,
	} {
		_, err := s.db.ExecContext(s.ctx, q, fgGoldenPrefix+"%")
		s.Require().NoError(err)
	}
}

func (s *FgGoldenSuite) mbCode(pid int64) (string, uuid.NullUUID, string) {
	var (
		txt, by sql.NullString
		sid     uuid.NullUUID
	)
	err := s.db.QueryRowContext(s.ctx, `SELECT cpp_value_text, cpp_value_mb_spin_id, cpp_filled_by FROM cost_product_parameter
		WHERE cpp_product_sys_id = $1 AND cpp_param_id = $2`, pid, s.paramID(mbsource.ParamSPCode)).Scan(&txt, &sid, &by)
	if err != nil {
		s.Require().ErrorIs(err, sql.ErrNoRows)
	}
	return txt.String, sid, by.String
}

// legacyFixture builds products and legacy (FG-less) spins covering every old-rule branch,
// plus FG spins that are NOT live (superseded / inactive / other shade). Returns product ids.
func (s *FgGoldenSuite) legacyFixture() []int64 {
	p := fgGoldenPrefix
	poy := s.product("POY", "POY", " "+p+"s1", false)
	poyUp := s.product("POYUP", "POY", p+"UP", false)
	dty := s.product("DTY", "DTY", p+"s1", false)
	s.route(dty, poyUp)
	mel := s.product("MEL", "MEL", p+"s1", false)
	s.route(mel, poyUp, poy)
	fgp := s.product("FG", "FG", p+"s1", false)
	sup := s.product("SUP", "FG", p+"sup", false)
	both := s.product("BOTH", "FG", p+"both", false)
	none := s.product("NONE", "FG", p+"nomatch", false)
	lock := s.product("LOCK", "FG", p+"s1", true)
	manual := s.product("MANUAL", "POY", p+"s1", false)
	_, err := s.db.ExecContext(s.ctx, `INSERT INTO cost_product_parameter (cpp_product_sys_id, cpp_param_id, cpp_value_text,
		cpp_filled_by, cpp_created_by) VALUES ($1, $2, 'MANUAL-PICK', 'user', 'user')`, manual, s.paramID(mbsource.ParamSPCode))
	s.Require().NoError(err)

	// Same shade: R and D newest, Boughtout, Spinning new, Spinning old -> Spinning new (SpinPickOrderSQL).
	s.spin(fgSpinOpt{shade: p + "S1", status: "R and D", orion: "RD", active: true})
	s.spin(fgSpinOpt{shade: p + "s1", status: "Boughtout", orion: "BO", ageDays: 1, active: true})
	pick := s.spin(fgSpinOpt{shade: p + "S1 ", status: "Spinning", orion: "SP-NEW", ageDays: 5, active: true})
	s.spin(fgSpinOpt{shade: p + "S1", status: "Spinning", orion: "SP-OLD", ageDays: 9, active: true})
	s.spin(fgSpinOpt{shade: p + "BOTH", status: "Spinning", orion: "BOTH", ageDays: 1, active: true})
	s.superba(p + "BOTH")
	s.superba(p + "SUP")
	// Non-live FG spins for every candidate: superseded, inactive, other shade.
	s.spin(fgSpinOpt{shade: p + "S1", status: "R and D", orion: "FG-SUPERSEDED", ageDays: 20, fg: poy, active: true, supersededBy: &pick})
	s.spin(fgSpinOpt{shade: p + "S1", status: "R and D", orion: "FG-INACTIVE", ageDays: 20, fg: poyUp, active: false})
	s.spin(fgSpinOpt{shade: p + "OTHER", status: "Spinning", orion: "FG-OTHER", fg: poyUp, active: true})
	return []int64{poy, poyUp, dty, mel, fgp, sup, both, none, lock, manual}
}

// TestGolden_NoLiveFgSpin_NewChainIdenticalToOld is the P4 golden test (integration level):
// same fixture, old chain then new chain, identical stats and identical persisted rows.
func (s *FgGoldenSuite) TestGolden_NoLiveFgSpin_NewChainIdenticalToOld() {
	ids := s.legacyFixture()
	oldR, newR, _ := s.chains()

	oldStats := s.autofill(shadeOnlyResolver{oldR}, ids)
	oldSnap := s.snapshot()
	s.resetValues()
	_, err := s.db.ExecContext(s.ctx, `INSERT INTO cost_product_parameter (cpp_product_sys_id, cpp_param_id, cpp_value_text,
		cpp_filled_by, cpp_created_by) VALUES ($1, $2, 'MANUAL-PICK', 'user', 'user')`, ids[9], s.paramID(mbsource.ParamSPCode))
	s.Require().NoError(err)
	newStats := s.autofill(newR, ids)
	newSnap := s.snapshot()

	s.Require().Equal(oldStats, newStats)
	s.Require().Equal(oldSnap, newSnap)
	// Sanity: the fixture exercises the spin pick order, superba and no match.
	s.Require().Equal(5, oldStats.BySource[mbsource.SourceMBSpin], "POY, DTY, MEL, FG, BOTH")
	s.Require().Equal(1, oldStats.BySource[mbsource.SourceSuperbaCostSP])
	s.Require().Equal(2, oldStats.NoMatch, "POYUP and NONE")
	s.Require().Zero(newStats.BySource[mbsource.SourceMBSpinFg])
	txt, _, _ := s.mbCode(ids[0])
	s.Require().Equal(fgGoldenPrefix+"SP-NEW", txt)
}

// fgFixture: a newer Spinning legacy spin (old-rule pick) plus older R and D FG spins.
type fgFixture struct {
	poy, poy2, dty, mel, melBoth, manual, lock int64
	fgPoy, fgPoy2, legacy                      uuid.UUID
}

func (s *FgGoldenSuite) fgFixture() fgFixture {
	p := fgGoldenPrefix
	var f fgFixture
	f.poy = s.product("POY", "POY", p+"S1", false)
	f.poy2 = s.product("POY2", "POY", p+"S2", false)
	f.dty = s.product("DTY", "DTY", p+"s1", false)
	s.route(f.dty, f.poy)
	f.mel = s.product("MEL", "MEL", p+"s2", false) // POY (no S2 FG spin) then POY2 (has one)
	s.route(f.mel, f.poy, f.poy2)
	f.melBoth = s.product("MELB", "MEL", p+"s1", false) // both POYs have an S1 FG spin
	s.route(f.melBoth, f.poy2, f.poy)
	f.manual = s.product("MANUAL", "DTY", p+"s1", false)
	s.route(f.manual, f.poy)
	f.lock = s.product("LOCK", "POY", p+"s1", true)
	s.route(f.lock, f.poy)

	f.legacy = s.spin(fgSpinOpt{shade: p + "S1", status: "Spinning", orion: "LEGACY", active: true})
	s.spin(fgSpinOpt{shade: p + "S2", status: "Spinning", orion: "LEGACY2", active: true})
	f.fgPoy = s.spin(fgSpinOpt{shade: p + "S1", status: "R and D", orion: "FG-POY", ageDays: 30, fg: f.poy, active: true})
	f.fgPoy2 = s.spin(fgSpinOpt{shade: p + "S2", status: "R and D", orion: "FG-POY2", ageDays: 30, fg: f.poy2, active: true})
	s.spin(fgSpinOpt{shade: p + "S1", status: "R and D", orion: "FG-POY2-S1", ageDays: 30, fg: f.poy2, active: true})
	return f
}

// TestFgChain_PoyDtyMel covers (a) POY self, (b) DTY via upstream POY, (e) MEL ordering.
func (s *FgGoldenSuite) TestFgChain_PoyDtyMel() {
	f := s.fgFixture()
	_, newR, _ := s.chains()
	stats := s.autofill(newR, []int64{f.poy, f.poy2, f.dty, f.mel, f.melBoth})

	check := func(pid int64, orion string, spin uuid.UUID) {
		txt, sid, by := s.mbCode(pid)
		s.Require().Equal(fgGoldenPrefix+orion, txt, "product %d", pid)
		s.Require().True(sid.Valid)
		s.Require().Equal(spin, sid.UUID)
		s.Require().Equal(mbsource.FilledBy, by)
	}
	check(f.poy, "FG-POY", f.fgPoy) // (a) beats the newer Spinning LEGACY pick
	check(f.poy2, "FG-POY2", f.fgPoy2)
	check(f.dty, "FG-POY", f.fgPoy)   // (b) upstream POY
	check(f.mel, "FG-POY2", f.fgPoy2) // (e) only the 2nd POY has an S2 FG spin
	txt, _, _ := s.mbCode(f.melBoth)  // (e) both have S1 FG spins -> first in seq order (POY2)
	s.Require().Equal(fgGoldenPrefix+"FG-POY2-S1", txt)
	s.Require().Equal(5, stats.BySource[mbsource.SourceMBSpinFg])
}

// TestFgChain_SupersededFgSpinFallsBackToOldRule: a superseded FG spin is ignored.
func (s *FgGoldenSuite) TestFgChain_SupersededFgSpinFallsBackToOldRule() {
	p := fgGoldenPrefix
	poy := s.product("POY", "POY", p+"S1", false)
	legacy := s.spin(fgSpinOpt{shade: p + "S1", status: "Spinning", orion: "LEGACY", active: true})
	s.spin(fgSpinOpt{shade: p + "S1", status: "Spinning", orion: "FG-OLD", ageDays: 30, fg: poy, active: true, supersededBy: &legacy})
	_, newR, _ := s.chains()
	stats := s.autofill(newR, []int64{poy})
	txt, sid, _ := s.mbCode(poy)
	s.Require().Equal(p+"LEGACY", txt)
	s.Require().Equal(legacy, sid.UUID)
	s.Require().Equal(1, stats.BySource[mbsource.SourceMBSpin])
}

func (s *FgGoldenSuite) linkService() *mbsourceautofill.LinkService {
	_, newR, fgP := s.chains()
	store := postgres.NewMBSourceAutoFillStore(s.db)
	svc := mbsourceautofill.New(store, newR, postgres.NewParameterRepository(s.db))
	return mbsourceautofill.NewLinkService(postgres.NewMBSourceLinkReader(s.db), newR, fgP, store, svc)
}

// TestRelink_OverwritesManualForOneProductOnly_LockedRejected covers Re-link (ApplyForce).
func (s *FgGoldenSuite) TestRelink_OverwritesManualForOneProductOnly_LockedRejected() {
	f := s.fgFixture()
	for _, pid := range []int64{f.manual, f.dty} {
		_, err := s.db.ExecContext(s.ctx, `INSERT INTO cost_product_parameter (cpp_product_sys_id, cpp_param_id, cpp_value_text,
			cpp_filled_by, cpp_created_by) VALUES ($1, $2, 'MANUAL-PICK', 'user', 'user')`, pid, s.paramID(mbsource.ParamSPCode))
		s.Require().NoError(err)
	}
	ls := s.linkService()

	before, err := ls.Explain(s.ctx, f.manual)
	s.Require().NoError(err)
	s.Require().Equal(mbsourceautofill.LinkFgMatch, before.LinkSource)
	s.Require().Equal("MANUAL-PICK", before.CurrentParamValue)
	s.Require().False(before.ParamMatches)

	link, err := ls.Relink(s.ctx, f.manual, "relinker")
	s.Require().NoError(err)
	s.Require().Equal(mbsourceautofill.LinkFgMatch, link.LinkSource)
	s.Require().True(link.ParamMatches)
	txt, sid, by := s.mbCode(f.manual)
	s.Require().Equal(fgGoldenPrefix+"FG-POY", txt)
	s.Require().Equal(f.fgPoy, sid.UUID)
	s.Require().Equal(mbsource.FilledBy, by)

	other, _, otherBy := s.mbCode(f.dty)
	s.Require().Equal("MANUAL-PICK", other, "only the re-linked product is written")
	s.Require().Equal("user", otherBy)

	_, err = ls.Relink(s.ctx, f.lock, "relinker")
	s.Require().ErrorIs(err, costproductparameter.ErrProductLocked)
	lockTxt, _, _ := s.mbCode(f.lock)
	s.Require().Empty(lockTxt)
}

// TestExplain_Sources covers GetCostProductMbLink: FG_MATCH / SHADE_ONLY / SUPERBA / NONE + candidates.
func (s *FgGoldenSuite) TestExplain_Sources() {
	f := s.fgFixture()
	p := fgGoldenPrefix
	shadeOnly := s.product("SHONLY", "DTY", p+"s1", false) // no route -> no candidate
	sup := s.product("SUP", "FG", p+"sup", false)
	s.superba(p + "SUP")
	noShade := s.product("NOSH", "FG", "", false)
	noMatch := s.product("NOM", "FG", p+"nomatch", false)
	ls := s.linkService()

	fg, err := ls.Explain(s.ctx, f.mel)
	s.Require().NoError(err)
	s.Require().Equal(mbsourceautofill.LinkFgMatch, fg.LinkSource)
	s.Require().Equal([]int64{f.poy, f.poy2}, fg.CandidatePoyIDs)
	s.Require().Equal(f.poy2, fg.FgProductID)
	s.Require().Equal(p+"POY2", fg.FgProductCode)
	s.Require().Equal(f.fgPoy2.String(), fg.SpinID)
	s.Require().Equal(p+"FG-POY2", fg.SpinCode)

	so, err := ls.Explain(s.ctx, shadeOnly)
	s.Require().NoError(err)
	s.Require().Equal(mbsourceautofill.LinkShadeOnly, so.LinkSource)
	s.Require().Equal(f.legacy.String(), so.SpinID)
	s.Require().Empty(so.CandidatePoyIDs)

	sb, err := ls.Explain(s.ctx, sup)
	s.Require().NoError(err)
	s.Require().Equal(mbsourceautofill.LinkSuperba, sb.LinkSource)

	for _, pid := range []int64{noShade, noMatch} {
		n, err := ls.Explain(s.ctx, pid)
		s.Require().NoError(err)
		s.Require().Equal(mbsourceautofill.LinkNone, n.LinkSource)
		s.Require().NotEmpty(n.Reason)
	}

	_, err = ls.Explain(s.ctx, -1)
	s.Require().ErrorIs(err, mbsourceautofill.ErrLinkProductNotFound)
}
