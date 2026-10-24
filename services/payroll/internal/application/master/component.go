package master

import (
	"context"
	"database/sql"
	"slices"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/apperr"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/audit"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/component"
)

// WithComponents enables the component master and template use cases (T008).
func (s *Service) WithComponents(r component.Repository) *Service {
	s.comp = r
	return s
}

const templateAmountScale = 6 // numeric(18,6)

var maxTemplateAmount = decimal.RequireFromString("1000000000000")

func validYYMM(v int) bool { return v >= 200001 && v <= 209912 && v%100 >= 1 && v%100 <= 12 }

// validateComponent checks the rules the proto constraints cannot express.
func validateComponent(c component.Component) error {
	if !validYYMM(c.ValidFrom) {
		return validation("valid_from", "valid_from harus yymm yang valid (mis. 202610)")
	}
	if c.ValidTo != 0 && (!validYYMM(c.ValidTo) || c.ValidTo < c.ValidFrom) {
		return validation("valid_to", "valid_to harus yymm yang valid dan >= valid_from")
	}
	for _, d := range c.ScopeDivisions {
		if strings.ContainsAny(d, ", ") {
			return validation("scope_divisions", "kode divisi tidak boleh berisi koma/spasi: "+d)
		}
	}
	if c.BaseSysid != 0 && c.BaseSysid == c.Sysid {
		return validation("base_component_sysid", "komponen tidak boleh menjadi pasangan dirinya sendiri")
	}
	return nil
}

const maxBaseChain = 32 // guards the cycle walk against a corrupt chain.

// checkRefs verifies scope divisions and the base component inside tx. prevBase is the base the
// component had before (0 on create): a base that is RETIRED is only tolerated when unchanged.
func (s *Service) checkRefs(ctx context.Context, tx *sql.Tx, c component.Component, prevBase int64) error {
	missing, err := s.comp.MissingDivisions(ctx, tx, c.CompanyID, c.ScopeDivisions)
	if err != nil {
		return err
	}
	if len(missing) > 0 {
		return validation("scope_divisions", "divisi tidak ada di company: "+strings.Join(missing, ","))
	}
	if c.BaseSysid == 0 {
		return nil
	}
	base, err := s.comp.Get(ctx, tx, c.BaseSysid)
	if err != nil {
		return validation("base_component_sysid", "komponen pasangan tidak ditemukan")
	}
	if base.CompanyID != c.CompanyID {
		return validation("base_component_sysid", "komponen pasangan harus se-company")
	}
	if base.Status == component.StatusRetired && c.BaseSysid != prevBase {
		return validation("base_component_sysid", "komponen pasangan RETIRED: "+base.Code)
	}
	return s.checkBaseCycle(ctx, tx, c, base)
}

// checkBaseCycle follows the base chain from base and rejects a path that returns to c (A<->B, A->B->C->A).
func (s *Service) checkBaseCycle(ctx context.Context, tx *sql.Tx, c component.Component, base component.Component) error {
	cur := base
	for i := 0; i < maxBaseChain; i++ {
		if cur.BaseSysid == 0 {
			return nil
		}
		if cur.BaseSysid == c.Sysid && c.Sysid != 0 {
			return validation("base_component_sysid", "siklus komponen pasangan terdeteksi lewat "+cur.Code)
		}
		next, err := s.comp.Get(ctx, tx, cur.BaseSysid)
		if err != nil {
			return nil //nolint:nilerr // an invisible/missing link ends the walk; it cannot loop back to c within scope
		}
		cur = next
	}
	return validation("base_component_sysid", "rantai komponen pasangan terlalu panjang atau berputar")
}

// sameSet compares two string lists ignoring order.
func sameSet(a, b []string) bool {
	x, y := slices.Clone(a), slices.Clone(b)
	slices.Sort(x)
	slices.Sort(y)
	return slices.Equal(x, y)
}

// guardedChanges lists the fields of a used component that must not change (T008c): they alter how
// existing attachments and CLOSED runs read.
func guardedChanges(before, after component.Component) []string {
	var out []string
	if before.TaxClass != after.TaxClass {
		out = append(out, "tax_class")
	}
	if before.Currency != after.Currency {
		out = append(out, "currency")
	}
	if before.ValidFrom != after.ValidFrom {
		out = append(out, "valid_from")
	}
	if before.ValidTo != after.ValidTo {
		out = append(out, "valid_to")
	}
	if !sameSet(before.ScopeDivisions, after.ScopeDivisions) {
		out = append(out, "scope_divisions")
	}
	return out
}

// guardUsed refuses changes of guarded fields once the component is attached or booked in a CLOSED run.
func (s *Service) guardUsed(ctx context.Context, tx *sql.Tx, before, after component.Component) error {
	changed := guardedChanges(before, after)
	if len(changed) == 0 {
		return nil
	}
	u, err := s.comp.InUse(ctx, tx, before.Sysid)
	if err != nil {
		return err
	}
	if !u.Used() {
		return nil
	}
	return apperr.New(apperr.CodeConflict, "komponen sudah dipakai; field tidak boleh diubah: "+strings.Join(changed, ", "),
		map[string]string{"fields": strings.Join(changed, ","), "attachments": strconv.FormatInt(u.Attachments, 10), "closed_run": strconv.FormatBool(u.ClosedRun)})
}

// CreateComponent creates an ACTIVE component (no activation flow in 07; D-20).
func (s *Service) CreateComponent(ctx context.Context, c component.Component, ip string) (out component.Component, err error) {
	if err = inCompany(ctx, c.CompanyID); err != nil {
		return out, err
	}
	if err = validateComponent(c); err != nil {
		return out, err
	}
	c.Status = component.StatusActive
	err = s.db.ScopedTx(ctx, func(tx *sql.Tx) error {
		if e := s.checkRefs(ctx, tx, c, 0); e != nil {
			return e
		}
		var e error
		if out, e = s.comp.Create(ctx, tx, c); e != nil {
			return toAppErr(e, "komponen")
		}
		return s.audit.Write(ctx, tx, audit.Entry{CompanyID: out.CompanyID, Action: "CREATE", Entity: "pyr_component",
			EntityKey: strconv.FormatInt(out.Sysid, 10), After: out, IP: ip})
	})
	return out, err
}

// GetComponent reads a component in the caller's scope.
func (s *Service) GetComponent(ctx context.Context, sysid int64) (out component.Component, err error) {
	err = s.db.ScopedTx(ctx, func(tx *sql.Tx) error {
		var e error
		out, e = s.comp.Get(ctx, tx, sysid)
		return toAppErr(e, "komponen")
	})
	return out, err
}

// ListComponents lists components with a keyset cursor (the last returned sysid).
func (s *Service) ListComponents(ctx context.Context, f component.Filter, companyID int16, cursor string) (out []component.Component, next string, err error) {
	if companyID != 0 {
		if err = inCompany(ctx, companyID); err != nil {
			return nil, "", err
		}
		f.CompanyIDs = []int16{companyID}
	} else {
		f.CompanyIDs = scopeCompanies(ctx)
	}
	if f.Limit <= 0 {
		f.Limit = 50
	}
	if cursor != "" {
		if f.AfterSysid, err = strconv.ParseInt(cursor, 10, 64); err != nil || f.AfterSysid < 0 {
			return nil, "", validation("cursor", "cursor tidak valid")
		}
	}
	want := f.Limit
	f.Limit++
	err = s.db.ScopedTx(ctx, func(tx *sql.Tx) error {
		var e error
		out, e = s.comp.List(ctx, tx, f)
		return e
	})
	if err != nil {
		return nil, "", err
	}
	if len(out) > want {
		out = out[:want]
		next = strconv.FormatInt(out[len(out)-1].Sysid, 10)
	}
	return out, next, nil
}

// UpdateComponent updates the writable fields under If-Match. RETIRED components are frozen.
func (s *Service) UpdateComponent(ctx context.Context, c component.Component, ifMatch int, ip string) (out component.Component, err error) {
	err = s.db.ScopedTx(ctx, func(tx *sql.Tx) error {
		if e := s.comp.Lock(ctx, tx, c.Sysid); e != nil {
			return toAppErr(e, "komponen")
		}
		before, e := s.comp.Get(ctx, tx, c.Sysid)
		if e != nil {
			return toAppErr(e, "komponen")
		}
		if e = inCompany(ctx, before.CompanyID); e != nil {
			return e
		}
		if before.Status == component.StatusRetired {
			return apperr.New(apperr.CodeConflict, "komponen RETIRED tidak dapat diubah", nil)
		}
		c.CompanyID = before.CompanyID
		if e = validateComponent(c); e != nil {
			return e
		}
		if e = s.checkRefs(ctx, tx, c, before.BaseSysid); e != nil {
			return e
		}
		if e = s.guardUsed(ctx, tx, before, c); e != nil {
			return e
		}
		if out, e = s.comp.Update(ctx, tx, c, ifMatch); e != nil {
			return toAppErr(e, "komponen")
		}
		return s.audit.Write(ctx, tx, audit.Entry{CompanyID: out.CompanyID, Action: "UPDATE", Entity: "pyr_component",
			EntityKey: strconv.FormatInt(out.Sysid, 10), Before: before, After: out, IP: ip})
	})
	return out, err
}

// RetireComponent retires a component; its sysid stays valid for history (12§2).
func (s *Service) RetireComponent(ctx context.Context, sysid int64, ifMatch int, ip string) (out component.Component, err error) {
	err = s.db.ScopedTx(ctx, func(tx *sql.Tx) error {
		if e := s.comp.Lock(ctx, tx, sysid); e != nil {
			return toAppErr(e, "komponen")
		}
		before, e := s.comp.Get(ctx, tx, sysid)
		if e != nil {
			return toAppErr(e, "komponen")
		}
		if e = inCompany(ctx, before.CompanyID); e != nil {
			return e
		}
		if before.Status == component.StatusRetired {
			return apperr.New(apperr.CodeConflict, "komponen sudah RETIRED", nil)
		}
		if out, e = s.comp.SetStatus(ctx, tx, sysid, component.StatusRetired, ifMatch); e != nil {
			return toAppErr(e, "komponen")
		}
		return s.audit.Write(ctx, tx, audit.Entry{CompanyID: out.CompanyID, Action: "RETIRE", Entity: "pyr_component",
			EntityKey: strconv.FormatInt(out.Sysid, 10), Before: before, After: out, IP: ip})
	})
	return out, err
}

// GetTemplate lists the template of a level; rows of components outside the
// caller's scope are not visible (D-20).
func (s *Service) GetTemplate(ctx context.Context, level string) (out []component.TemplateItem, err error) {
	err = s.db.ScopedTx(ctx, func(tx *sql.Tx) error {
		var e error
		out, e = s.comp.GetTemplate(ctx, tx, level)
		return e
	})
	return out, err
}

// normalizeTemplate validates items and canonicalizes amounts.
func normalizeTemplate(items []component.TemplateItem) ([]component.TemplateItem, error) {
	seen := map[int64]bool{}
	out := slices.Clone(items)
	for i := range out {
		it := &out[i]
		if seen[it.Sysid] {
			return nil, validation("items", "komponen dobel di template: "+strconv.FormatInt(it.Sysid, 10))
		}
		seen[it.Sysid] = true
		if it.Amount == "" {
			continue
		}
		d, err := decimal.NewFromString(it.Amount)
		if err != nil || d.Abs().GreaterThanOrEqual(maxTemplateAmount) {
			return nil, validation("items.amount", "amount bukan desimal yang valid: "+it.Amount)
		}
		if d.Exponent() < -templateAmountScale {
			return nil, validation("items.amount", "amount maksimal 6 desimal: "+it.Amount)
		}
		it.Amount = d.String()
	}
	return out, nil
}

// templateCompanies checks every item's component is visible and not RETIRED and
// returns the companies the template touches (D-20).
func (s *Service) templateCompanies(ctx context.Context, tx *sql.Tx, items []component.TemplateItem) (map[int16]bool, error) {
	companies := map[int16]bool{}
	for _, it := range items {
		c, err := s.comp.Get(ctx, tx, it.Sysid)
		if err != nil {
			return nil, validation("items.component_sysid", "komponen tidak ditemukan: "+strconv.FormatInt(it.Sysid, 10))
		}
		if c.Status == component.StatusRetired {
			return nil, validation("items.component_sysid", "komponen RETIRED tidak boleh masuk template: "+c.Code)
		}
		companies[c.CompanyID] = true
	}
	return companies, nil
}

// PutTemplate replaces the template of a level. Every component must be visible
// to the caller (their company in scope) and not RETIRED; one audit row is
// written per affected company (D-20).
func (s *Service) PutTemplate(ctx context.Context, level string, items []component.TemplateItem, ip string) (out []component.TemplateItem, err error) {
	items, err = normalizeTemplate(items)
	if err != nil {
		return nil, err
	}
	auditCo, err := auditCompany(ctx)
	if err != nil {
		return nil, err
	}
	err = s.db.ScopedTx(ctx, func(tx *sql.Tx) error {
		companies, e := s.templateCompanies(ctx, tx, items)
		if e != nil {
			return e
		}
		companies[auditCo] = true
		before, e := s.comp.GetTemplate(ctx, tx, level)
		if e != nil {
			return e
		}
		if e = s.comp.ReplaceTemplate(ctx, tx, level, items); e != nil {
			return toAppErr(e, "template")
		}
		if out, e = s.comp.GetTemplate(ctx, tx, level); e != nil {
			return e
		}
		for co := range companies {
			if e = s.audit.Write(ctx, tx, audit.Entry{CompanyID: co, Action: "UPDATE", Entity: "pyr_component_template",
				EntityKey: level, Before: before, After: out, IP: ip}); e != nil {
				return e
			}
		}
		return nil
	})
	return out, err
}
