package grpc

import (
	"context"

	payrollv1 "github.com/mutugading/goapps-backend/gen/payroll/v1"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/component"
)

// Permission codes for employee components and level-change (T008b). Reading
// follows employee.view; changing attachments or level follows employee.update.
func init() {
	const svc = "/payroll.v1.PayrollService/"
	for m, p := range map[string]string{
		"GetEmployeeComponents": permEmployeeView, "PreviewLevelChange": permEmployeeView,
		"PutEmployeeComponents": permEmployeeUpdate, "ApplyLevelChange": permEmployeeUpdate,
	} {
		rpcPermissions[svc+m] = p
	}
}

func attachmentPB(a component.Attachment) *payrollv1.EmployeeComponent {
	return &payrollv1.EmployeeComponent{
		ComponentSysid: a.Sysid, ComponentCode: a.Code, ComponentName: a.Name, Sts: a.Sts, Tax: int32(a.Tax), //nolint:gosec // smallint
		Amount: a.Amount, TaxPaidCompany: a.TaxPaidCompany, FrzFlag: int32(a.Frz), Source: a.Source, //nolint:gosec // smallint
		ValidFrom: int32(a.ValidFrom), ValidTo: int32(a.ValidTo), //nolint:gosec // yymm
	}
}

func attachmentsPB(in []component.Attachment) []*payrollv1.EmployeeComponent {
	out := make([]*payrollv1.EmployeeComponent, len(in))
	for i, a := range in {
		out[i] = attachmentPB(a)
	}
	return out
}

func attachmentFromInput(in *payrollv1.EmployeeComponentInput) component.Attachment {
	return component.Attachment{
		Sysid: in.GetComponentSysid(), Sts: in.GetSts(), Tax: int(in.GetTax()), Amount: in.GetAmount(),
		TaxPaidCompany: in.GetTaxPaidCompany(), Frz: int(in.GetFrzFlag()),
		ValidFrom: int(in.GetValidFrom()), ValidTo: int(in.GetValidTo()),
	}
}

func changesPB(in []component.Change) []*payrollv1.ComponentChange {
	out := make([]*payrollv1.ComponentChange, len(in))
	for i, c := range in {
		pb := &payrollv1.ComponentChange{Action: c.Action, ComponentSysid: c.Sysid, ComponentCode: c.Code, ComponentName: c.Name}
		if c.Before != nil {
			pb.Before = attachmentPB(*c.Before)
		}
		if c.After != nil {
			pb.After = attachmentPB(*c.After)
		}
		out[i] = pb
	}
	return out
}

// GetEmployeeComponents lists the components attached to an employee.
func (h *MasterHandler) GetEmployeeComponents(ctx context.Context, r *payrollv1.GetEmployeeComponentsRequest) (*payrollv1.GetEmployeeComponentsResponse, error) {
	id, items, err := h.svc.GetEmployeeComponents(ctx, r.GetNik(), i16(r.GetCompanyId()), r.GetDivision())
	if err != nil {
		return nil, fail(ctx, err)
	}
	return &payrollv1.GetEmployeeComponentsResponse{Base: ok("OK"), EmployeeId: id, Items: attachmentsPB(items)}, nil
}

// PutEmployeeComponents replaces the attachment set of an employee.
func (h *MasterHandler) PutEmployeeComponents(ctx context.Context, r *payrollv1.PutEmployeeComponentsRequest) (*payrollv1.PutEmployeeComponentsResponse, error) {
	items := make([]component.Attachment, len(r.GetItems()))
	for i, it := range r.GetItems() {
		items[i] = attachmentFromInput(it)
	}
	id, out, err := h.svc.PutEmployeeComponents(ctx, r.GetNik(), i16(r.GetCompanyId()), r.GetDivision(), items, caller(ctx).IP)
	if err != nil {
		return nil, fail(ctx, err)
	}
	return &payrollv1.PutEmployeeComponentsResponse{Base: ok("Employee components saved"), EmployeeId: id, Items: attachmentsPB(out)}, nil
}

// PreviewLevelChange returns the component diff of a level change.
func (h *MasterHandler) PreviewLevelChange(ctx context.Context, r *payrollv1.PreviewLevelChangeRequest) (*payrollv1.PreviewLevelChangeResponse, error) {
	lc, err := h.svc.PreviewLevelChange(ctx, r.GetNik(), i16(r.GetCompanyId()), r.GetDivision(), r.GetNewLevel(), int(r.GetEffectiveYymm()))
	if err != nil {
		return nil, fail(ctx, err)
	}
	return &payrollv1.PreviewLevelChangeResponse{Base: ok("OK"), CurrentLevel: lc.CurrentLevel, NewLevel: lc.NewLevel,
		Changes: changesPB(lc.Changes), SkippedManual: lc.SkippedManual}, nil
}

// ApplyLevelChange applies a level change under the employee If-Match.
func (h *MasterHandler) ApplyLevelChange(ctx context.Context, r *payrollv1.ApplyLevelChangeRequest) (*payrollv1.ApplyLevelChangeResponse, error) {
	v, err := ifMatch(r.GetIfMatch())
	if err != nil {
		return nil, fail(ctx, err)
	}
	emp, lc, err := h.svc.ApplyLevelChange(ctx, r.GetNik(), i16(r.GetCompanyId()), r.GetDivision(), r.GetNewLevel(), int(r.GetEffectiveYymm()), v, caller(ctx))
	if err != nil {
		return nil, fail(ctx, err)
	}
	return &payrollv1.ApplyLevelChangeResponse{Base: ok("Level changed"), Data: employeePB(emp),
		Changes: changesPB(lc.Changes), SkippedManual: lc.SkippedManual}, nil
}
