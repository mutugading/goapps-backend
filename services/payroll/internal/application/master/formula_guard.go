package master

import (
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/component"
	"github.com/mutugading/goapps-backend/services/payroll/internal/engine"
)

// Formulas are code-only (D-76): a component id that engine.SpecOf does not know makes the whole run of
// that NIK INCOMPLETE. The guards below stop such a component upstream (T033f2), where HR can still react.

// formulaMapped reports whether the engine has a formula for the component. typ is "T" or "P"; code is
// the pc_code ("T080"). A code that does not parse is treated as unmapped.
func formulaMapped(typ, code string) bool {
	k, err := engine.ParseCompKey(typ, code)
	if err != nil {
		return false
	}
	_, _, ok := engine.SpecOf(k)
	return ok
}

// codeMapped is formulaMapped for a code whose type is its first letter (attachment catalog rows).
func codeMapped(code string) bool {
	if code == "" {
		return false
	}
	return formulaMapped(code[:1], code)
}

// FormulaWarning returns a non-empty warning when the component has no formula in the engine, so the
// delivery layer can tell HR right after CreateComponent. The component is still created (D-76 (1)).
func FormulaWarning(c component.Component) string {
	if formulaMapped(c.Type, c.Code) {
		return ""
	}
	return "peringatan: komponen " + c.Code + " belum ada formula di engine; belum bisa dipasang ke karyawan/template sampai dipetakan di kode"
}

func noFormulaError(field, code string) error {
	return validation(field, "komponen "+code+" belum ada formula di engine; minta developer memetakannya dulu")
}

// guardNewAttachment refuses to attach a component the engine cannot calculate. Already attached rows
// stay editable and removable, so a GET -> PUT round trip never fails on legacy data.
func guardNewAttachment(isHeld bool, code string) error {
	if isHeld || codeMapped(code) {
		return nil
	}
	return noFormulaError("items.component_sysid", code)
}

// guardPlanAdds handles level-change adds of components without an engine formula. Strict mode (preview and
// apply) refuses the plan; lenient mode (CreateEmployee, D-21) drops those adds and reports their codes in
// Missing, so onboarding is never blocked by a template row HR can still fix.
func guardPlanAdds(p levelPlan, level string, lenient bool) (levelPlan, error) {
	kept := p.Changes[:0:0]
	for _, c := range p.Changes {
		if c.Action != component.ActionAdd || codeMapped(c.Code) {
			kept = append(kept, c)
			continue
		}
		if !lenient {
			return levelPlan{}, validation("level", "komponen "+c.Code+" di template level "+level+
				" belum ada formula di engine; hapus dari template atau minta developer memetakannya")
		}
		p.Missing = append(p.Missing, c.Code)
	}
	p.Changes = kept
	return p, nil
}
