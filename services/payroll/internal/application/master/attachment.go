package master

import (
	"context"
	"database/sql"
	"strconv"

	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/apperr"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/audit"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/component"
	domain "github.com/mutugading/goapps-backend/services/payroll/internal/domain/master"
)

// WithAttachments enables employee component attachment and level-change (T008b).
func (s *Service) WithAttachments(r component.AttachmentRepository) *Service {
	s.att = r
	return s
}

// errLevelEmpty means a level has neither template rows nor a level rule.
var errLevelEmpty = apperr.New(apperr.CodeValidation, "level tidak punya template maupun aturan", map[string]string{"field": "new_level"})

// LevelChange is the diff of a level-change preview or apply.
type LevelChange struct {
	CurrentLevel, NewLevel string
	Changes                []component.Change
	SkippedManual          []string
}

func keyOf(e domain.Employee) component.EmployeeKey {
	return component.EmployeeKey{CompanyID: e.CompanyID, NIK: e.NIK, Division: e.Division}
}

// resolveEmployee resolves a current/old NIK to exactly one employee row and locks it.
func (s *Service) resolveEmployee(ctx context.Context, tx *sql.Tx, nik string, companyID int16, division string, lock bool) (domain.Employee, error) {
	ids, err := s.repo.ResolveEmployeeIDs(ctx, tx, nik, companyID, division)
	if err != nil {
		return domain.Employee{}, err
	}
	switch len(ids) {
	case 0:
		return domain.Employee{}, apperr.New(apperr.CodeNotFound, "karyawan tidak ditemukan", nil)
	case 1:
	default:
		return domain.Employee{}, apperr.New(apperr.CodeConflict, "NIK ambigu: cocok ke lebih dari satu karyawan", map[string]string{"nik": nik})
	}
	if lock {
		if err = s.repo.LockEmployee(ctx, tx, ids[0]); err != nil {
			return domain.Employee{}, toAppErr(err, "karyawan")
		}
	}
	e, err := s.repo.GetEmployeeByID(ctx, tx, ids[0], domain.PII{EncKey: s.enc, HMACKey: s.hmac})
	return e, toAppErr(err, "karyawan")
}

// GetEmployeeComponents lists the attachments of one employee.
func (s *Service) GetEmployeeComponents(ctx context.Context, nik string, companyID int16, division string) (empID int64, out []component.Attachment, err error) {
	err = s.db.ScopedTx(ctx, func(tx *sql.Tx) error {
		e, e2 := s.resolveEmployee(ctx, tx, nik, companyID, division, false)
		if e2 != nil {
			return e2
		}
		empID = e.ID
		out, e2 = s.att.List(ctx, tx, keyOf(e))
		return e2
	})
	return empID, out, err
}

func validateAttachmentInput(in component.Attachment) (component.Attachment, error) {
	if in.Amount != "" {
		d, err := decimal.NewFromString(in.Amount)
		if err != nil || d.Abs().GreaterThanOrEqual(maxTemplateAmount) {
			return in, validation("items.amount", "amount bukan desimal yang valid: "+in.Amount)
		}
		if d.Exponent() < -templateAmountScale {
			return in, validation("items.amount", "amount maksimal 6 desimal: "+in.Amount)
		}
		in.Amount = d.String()
	}
	if in.Frz == 0 {
		in.Frz = 2
	}
	if in.ValidFrom != 0 && !validYYMM(in.ValidFrom) || in.ValidTo != 0 && !validYYMM(in.ValidTo) ||
		in.ValidTo != 0 && in.ValidTo < in.ValidFrom {
		return in, validation("items.valid_from", "valid_from/valid_to harus yymm yang valid dan valid_to >= valid_from")
	}
	return in, nil
}

// PutEmployeeComponents replaces the attachment set: listed components not yet
// attached become MANUAL, attached ones keep their source, unlisted ones are detached.
func (s *Service) PutEmployeeComponents(ctx context.Context, nik string, companyID int16, division string, items []component.Attachment, ip string) (empID int64, out []component.Attachment, err error) {
	seen := map[int64]bool{}
	sysids := make([]int64, len(items))
	for i := range items {
		if seen[items[i].Sysid] {
			return 0, nil, validation("items", "komponen dobel: "+strconv.FormatInt(items[i].Sysid, 10))
		}
		seen[items[i].Sysid] = true
		sysids[i] = items[i].Sysid
		if items[i], err = validateAttachmentInput(items[i]); err != nil {
			return 0, nil, err
		}
	}
	err = s.db.ScopedTx(ctx, func(tx *sql.Tx) error {
		e, e2 := s.resolveEmployee(ctx, tx, nik, companyID, division, true)
		if e2 != nil {
			return e2
		}
		empID = e.ID
		k := keyOf(e)
		catalog, e2 := s.att.CatalogBySysid(ctx, tx, e.CompanyID, e.Division, sysids)
		if e2 != nil {
			return e2
		}
		before, e2 := s.att.List(ctx, tx, k)
		if e2 != nil {
			return e2
		}
		if e2 = s.guardOpenPeriods(ctx, tx, k, items); e2 != nil {
			return e2
		}
		if e2 = s.applyPut(ctx, tx, k, before, items, catalog); e2 != nil {
			return e2
		}
		if out, e2 = s.att.List(ctx, tx, k); e2 != nil {
			return e2
		}
		return s.audit.Write(ctx, tx, audit.Entry{CompanyID: e.CompanyID, Division: e.Division, Action: "UPDATE",
			Entity: "pyr_employee_component", EntityKey: strconv.FormatInt(e.ID, 10), Before: before, After: out, IP: ip})
	})
	return empID, out, err
}

// resolvePut fills the display fields of one PUT item and decides its source. An empty
// valid period inherits the stored one, so a GET -> PUT round trip is stable. A hand-edited
// TEMPLATE row becomes MANUAL so level-change never overwrites it again.
func resolvePut(it component.Attachment, cur component.Attachment, isHeld bool, c component.CatalogEntry, inCatalog bool) (component.Attachment, error) {
	switch {
	case inCatalog:
		it.Name = c.Name
		if it.Sts == "" {
			it.Sts = c.DefaultSts
		}
	case isHeld: // attached already: stays editable even if the component was retired since.
		it.Name = cur.Name
		if it.Sts == "" {
			it.Sts = cur.Sts
		}
	default:
		return it, validation("items.component_sysid", "komponen tidak ditemukan/aktif untuk karyawan ini: "+strconv.FormatInt(it.Sysid, 10))
	}
	it.Source = component.SourceManual
	if !isHeld {
		return it, nil
	}
	if it.ValidFrom == 0 && it.ValidTo == 0 {
		it.ValidFrom, it.ValidTo = cur.ValidFrom, cur.ValidTo
	}
	it.Source = cur.Source
	if cur.Source == component.SourceTemplate && !sameAttachment(cur, it) {
		it.Source = component.SourceManual
	}
	return it, nil
}

func unchanged(cur, it component.Attachment) bool {
	return sameAttachment(cur, it) && it.Source == cur.Source && it.ValidFrom == cur.ValidFrom && it.ValidTo == cur.ValidTo
}

func (s *Service) applyPut(ctx context.Context, tx *sql.Tx, k component.EmployeeKey, before, items []component.Attachment, catalog map[int64]component.CatalogEntry) error {
	held := make(map[int64]component.Attachment, len(before))
	for _, b := range before {
		held[b.Sysid] = b
	}
	listed := map[int64]bool{}
	actor, snapYYMM := actorOf(ctx), s.currentYYMM()
	for _, it := range items {
		listed[it.Sysid] = true
		cur, isHeld := held[it.Sysid]
		c, inCatalog := catalog[it.Sysid]
		it, err := resolvePut(it, cur, isHeld, c, inCatalog)
		if err != nil {
			return err
		}
		if isHeld && unchanged(cur, it) {
			continue
		}
		if isHeld {
			if err = s.att.Snapshot(ctx, tx, k, it.Sysid, snapYYMM); err != nil {
				return err
			}
		}
		if err = s.att.Upsert(ctx, tx, k, it, actor); err != nil {
			return toAppErr(err, "komponen karyawan")
		}
	}
	return s.detachUnlisted(ctx, tx, k, before, listed, snapYYMM)
}

// detachUnlisted snapshots and deletes the attachments a PUT did not list.
func (s *Service) detachUnlisted(ctx context.Context, tx *sql.Tx, k component.EmployeeKey, before []component.Attachment, listed map[int64]bool, yymm int) error {
	for _, b := range before {
		if listed[b.Sysid] {
			continue
		}
		if err := s.att.Snapshot(ctx, tx, k, b.Sysid, yymm); err != nil {
			return err
		}
		if err := s.att.Delete(ctx, tx, k, b.Sysid); err != nil {
			return err
		}
	}
	return nil
}

// guardOpenPeriods refuses a PUT that touches a CLOSED period (04§9): the snapshot period
// and every explicit valid_from must be open.
func (s *Service) guardOpenPeriods(ctx context.Context, tx *sql.Tx, k component.EmployeeKey, items []component.Attachment) error {
	yymms := map[int]bool{s.currentYYMM(): true}
	for _, it := range items {
		if it.ValidFrom != 0 {
			yymms[it.ValidFrom] = true
		}
	}
	for y := range yymms {
		closed, err := s.att.PeriodClosed(ctx, tx, k, y)
		if err != nil {
			return err
		}
		if closed {
			return apperr.PeriodClosed(y, k.Division)
		}
	}
	return nil
}

// currentYYMM is the period a PUT snapshot is filed under; the period must be open.
func (s *Service) currentYYMM() int {
	t := s.now()
	return t.Year()*100 + int(t.Month())
}

// buildLevelPlan computes the diff of moving an employee to newLevel inside tx.
// lenient (CreateEmployee, D-21) skips rule components the company lacks and reports them in Missing.
func (s *Service) buildLevelPlan(ctx context.Context, tx *sql.Tx, e domain.Employee, newLevel string, yymm int, lenient bool) (levelPlan, error) {
	k := keyOf(e)
	current, err := s.att.List(ctx, tx, k)
	if err != nil {
		return levelPlan{}, err
	}
	template, err := s.att.TemplateFor(ctx, tx, newLevel, e.CompanyID, e.Division)
	if err != nil {
		return levelPlan{}, err
	}
	rule, err := s.att.LevelRule(ctx, tx, newLevel)
	if err != nil {
		return levelPlan{}, err
	}
	if rule == nil && len(template) == 0 {
		return levelPlan{}, errLevelEmpty
	}
	catalog, err := s.att.CatalogByCode(ctx, tx, e.CompanyID, e.Division, ruleCodes())
	if err != nil {
		return levelPlan{}, err
	}
	want, missing, err := desire(template, rule, catalog, lenient)
	if err != nil {
		return levelPlan{}, err
	}
	p := planLevel(current, want, yymm)
	p.Missing = missing
	return p, nil
}

// PreviewLevelChange returns the diff without writing anything.
func (s *Service) PreviewLevelChange(ctx context.Context, nik string, companyID int16, division, newLevel string, yymm int) (out LevelChange, err error) {
	if !validYYMM(yymm) {
		return out, validation("effective_yymm", "effective_yymm harus yymm yang valid")
	}
	err = s.db.ScopedTx(ctx, func(tx *sql.Tx) error {
		e, e2 := s.resolveEmployee(ctx, tx, nik, companyID, division, false)
		if e2 != nil {
			return e2
		}
		if blocked, e3 := s.att.PeriodLockedFrom(ctx, tx, keyOf(e), yymm); e3 != nil {
			return e3
		} else if blocked != 0 {
			return apperr.PeriodClosed(blocked, e.Division)
		}
		p, e2 := s.buildLevelPlan(ctx, tx, e, newLevel, yymm, false)
		if e2 != nil {
			return e2
		}
		out = LevelChange{CurrentLevel: e.Level, NewLevel: newLevel, Changes: p.Changes, SkippedManual: p.SkippedManual}
		return nil
	})
	return out, err
}

// ApplyLevelChange applies the diff under the employee If-Match (replaces TRG_EMP_MAST UPDATE).
// The effective period must not be CLOSED. Replaced/removed rows are snapshotted to _hist.
func (s *Service) ApplyLevelChange(ctx context.Context, nik string, companyID int16, division, newLevel string, yymm, ifMatch int, cl Caller) (emp domain.Employee, lc LevelChange, err error) {
	if !validYYMM(yymm) {
		return emp, lc, validation("effective_yymm", "effective_yymm harus yymm yang valid")
	}
	err = s.db.ScopedTx(ctx, func(tx *sql.Tx) error {
		e, e2 := s.resolveEmployee(ctx, tx, nik, companyID, division, true)
		if e2 != nil {
			return e2
		}
		k := keyOf(e)
		// Same level: nothing to change, no version bump, no audit. Deliberately before the If-Match
		// and period checks, since nothing is written.
		if newLevel == e.Level {
			emp = e
			lc = LevelChange{CurrentLevel: e.Level, NewLevel: newLevel}
			return nil
		}
		if blocked, e3 := s.att.PeriodLockedFrom(ctx, tx, k, yymm); e3 != nil {
			return e3
		} else if blocked != 0 {
			return apperr.PeriodClosed(blocked, e.Division)
		}
		p, e2 := s.buildLevelPlan(ctx, tx, e, newLevel, yymm, false)
		if e2 != nil {
			return e2
		}
		actor := actorOf(ctx)
		if e2 = s.writePlan(ctx, tx, k, p, yymm, actor); e2 != nil {
			return e2
		}
		if e2 = s.att.SetEmployeeLevel(ctx, tx, e.ID, newLevel, actor, ifMatch); e2 != nil {
			return toAppErr(e2, "karyawan")
		}
		if emp, e2 = s.repo.GetEmployeeByID(ctx, tx, e.ID, s.pii(cl)); e2 != nil {
			return e2
		}
		lc = LevelChange{CurrentLevel: e.Level, NewLevel: newLevel, Changes: p.Changes, SkippedManual: p.SkippedManual}
		return s.audit.Write(ctx, tx, audit.Entry{CompanyID: e.CompanyID, Division: e.Division, Action: "LEVEL_CHANGE",
			Entity: "pyr_employee", EntityKey: strconv.FormatInt(e.ID, 10), Before: e.Level, After: lc, IP: cl.IP})
	})
	return emp, lc, err
}

func (s *Service) writePlan(ctx context.Context, tx *sql.Tx, k component.EmployeeKey, p levelPlan, yymm int, actor string) error {
	for _, c := range p.Changes {
		if c.Before != nil {
			if err := s.att.Snapshot(ctx, tx, k, c.Sysid, yymm); err != nil {
				return err
			}
		}
		switch c.Action {
		case component.ActionRemove:
			if err := s.att.Delete(ctx, tx, k, c.Sysid); err != nil {
				return err
			}
		default:
			if err := s.att.Upsert(ctx, tx, k, *c.After, actor); err != nil {
				return toAppErr(err, "komponen karyawan")
			}
		}
	}
	return nil
}

// attachFromLevel copies the level template (plus level rule) to a new employee, like
// TRG_EMP_MAST INSERT (02§5.1). A level without template or rule attaches nothing.
func (s *Service) attachFromLevel(ctx context.Context, tx *sql.Tx, e domain.Employee, actor string) error {
	if s.att == nil || e.Level == "" {
		return nil
	}
	yymm := s.currentYYMM()
	if len(e.DateJoin) >= 7 {
		if y, err := strconv.Atoi(e.DateJoin[:4]); err == nil {
			if m, err := strconv.Atoi(e.DateJoin[5:7]); err == nil && validYYMM(y*100+m) {
				yymm = y*100 + m
			}
		}
	}
	// A CLOSED join period must not receive valid_from (04§9): leave it unset instead. Only CLOSED is
	// checked here (DRAFT/APPROVED still accept a new attachment); level-change is stricter (non-OPEN).
	if closed, err := s.att.PeriodClosed(ctx, tx, keyOf(e), yymm); err != nil {
		return err
	} else if closed {
		yymm = 0
	}
	p, err := s.buildLevelPlan(ctx, tx, e, e.Level, yymm, true)
	if err == errLevelEmpty { //nolint:errorlint // sentinel compared by identity; apperr.Is matches by code
		return nil // level has no template/rule: nothing to attach.
	}
	if err != nil {
		return err
	}
	if len(p.Missing) > 0 {
		log.Warn().Int16("company_id", e.CompanyID).Str("nik", e.NIK).Str("level", e.Level).
			Strs("missing", p.Missing).Msg("level rule components absent in company; skipped (D-21)")
	}
	return s.writePlan(ctx, tx, keyOf(e), p, yymm, actor)
}
