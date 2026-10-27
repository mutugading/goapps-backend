package grpc

import (
	"context"

	payrollv1 "github.com/mutugading/goapps-backend/gen/payroll/v1"
	app "github.com/mutugading/goapps-backend/services/payroll/internal/application/master"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/component"
)

// Permission codes for the component master and level template (T008, 000100).
const (
	permComponentView   = "payroll.master.component.view"
	permComponentCreate = "payroll.master.component.create"
	permComponentUpdate = "payroll.master.component.update"
	permTemplateView    = "payroll.master.template.view"
	permTemplateUpdate  = "payroll.master.template.update"
)

func init() {
	const svc = "/payroll.v1.PayrollService/"
	for m, p := range map[string]string{
		"CreateComponent": permComponentCreate, "GetComponent": permComponentView, "ListComponents": permComponentView,
		"UpdateComponent": permComponentUpdate, "RetireComponent": permComponentUpdate,
		"GetComponentTemplate": permTemplateView, "PutComponentTemplate": permTemplateUpdate,
	} {
		rpcPermissions[svc+m] = p
	}
}

func componentPB(c component.Component) *payrollv1.Component {
	return &payrollv1.Component{
		Sysid: c.Sysid, CompanyId: int32(c.CompanyID), Code: c.Code, Type: c.Type, Name: c.Name,
		ScopeDivisions: c.ScopeDivisions, Currency: c.Currency, DefaultSts: c.DefaultSts, TaxClass: c.TaxClass,
		SubsidyEligible: c.SubsidyEligible, FixIncome: c.FixIncome, IncludeInEstimate: c.IncludeInEstimate,
		Tags: c.Tags, Serial: int32(c.Serial), BaseComponentSysid: c.BaseSysid, Status: c.Status, //nolint:gosec // validated >= 0, small
		ValidFrom: int32(c.ValidFrom), ValidTo: int32(c.ValidTo), Etag: app.ETag(c.Version), //nolint:gosec // yymm
	}
}

func componentFromInput(in *payrollv1.ComponentInput) component.Component {
	return component.Component{
		Name: in.GetName(), ScopeDivisions: in.GetScopeDivisions(), Currency: in.GetCurrency(), DefaultSts: in.GetDefaultSts(),
		TaxClass: in.GetTaxClass(), SubsidyEligible: in.GetSubsidyEligible(), FixIncome: in.GetFixIncome(),
		IncludeInEstimate: in.GetIncludeInEstimate(), Tags: in.GetTags(), Serial: int(in.GetSerial()),
		BaseSysid: in.GetBaseComponentSysid(), ValidFrom: int(in.GetValidFrom()), ValidTo: int(in.GetValidTo()),
	}
}

func templateItemPB(it component.TemplateItem) *payrollv1.TemplateItem {
	return &payrollv1.TemplateItem{ComponentSysid: it.Sysid, Sts: it.Sts, Tax: int32(it.Tax), Amount: it.Amount, //nolint:gosec // smallint
		TaxPaidCompany: it.TaxPaidCompany, ComponentCode: it.Code, ComponentName: it.Name}
}

func templateItemsPB(items []component.TemplateItem) []*payrollv1.TemplateItem {
	out := make([]*payrollv1.TemplateItem, len(items))
	for i, it := range items {
		out[i] = templateItemPB(it)
	}
	return out
}

// CreateComponent creates a component.
func (h *MasterHandler) CreateComponent(ctx context.Context, r *payrollv1.CreateComponentRequest) (*payrollv1.CreateComponentResponse, error) {
	c := componentFromInput(r.GetInput())
	c.CompanyID, c.Code, c.Type = i16(r.GetCompanyId()), r.GetCode(), r.GetType()
	out, err := h.svc.CreateComponent(ctx, c, caller(ctx).IP)
	if err != nil {
		return nil, fail(ctx, err)
	}
	return &payrollv1.CreateComponentResponse{Base: ok("Component created"), Data: componentPB(out)}, nil
}

// GetComponent reads a component.
func (h *MasterHandler) GetComponent(ctx context.Context, r *payrollv1.GetComponentRequest) (*payrollv1.GetComponentResponse, error) {
	out, err := h.svc.GetComponent(ctx, r.GetSysid())
	if err != nil {
		return nil, fail(ctx, err)
	}
	return &payrollv1.GetComponentResponse{Base: ok("OK"), Data: componentPB(out)}, nil
}

// ListComponents lists components with a keyset cursor.
func (h *MasterHandler) ListComponents(ctx context.Context, r *payrollv1.ListComponentsRequest) (*payrollv1.ListComponentsResponse, error) {
	out, next, err := h.svc.ListComponents(ctx, component.Filter{
		Type: r.GetType(), Status: r.GetStatus(), Q: r.GetQ(), Limit: int(r.GetLimit()),
	}, i16(r.GetCompanyId()), r.GetCursor())
	if err != nil {
		return nil, fail(ctx, err)
	}
	resp := &payrollv1.ListComponentsResponse{Base: ok("OK"), NextCursor: next}
	for _, c := range out {
		resp.Data = append(resp.Data, componentPB(c))
	}
	return resp, nil
}

// UpdateComponent updates a component under If-Match.
func (h *MasterHandler) UpdateComponent(ctx context.Context, r *payrollv1.UpdateComponentRequest) (*payrollv1.UpdateComponentResponse, error) {
	v, err := ifMatch(r.GetIfMatch())
	if err != nil {
		return nil, fail(ctx, err)
	}
	c := componentFromInput(r.GetInput())
	c.Sysid = r.GetSysid()
	out, err := h.svc.UpdateComponent(ctx, c, v, caller(ctx).IP)
	if err != nil {
		return nil, fail(ctx, err)
	}
	return &payrollv1.UpdateComponentResponse{Base: ok("Component updated"), Data: componentPB(out)}, nil
}

// RetireComponent retires a component under If-Match.
func (h *MasterHandler) RetireComponent(ctx context.Context, r *payrollv1.RetireComponentRequest) (*payrollv1.RetireComponentResponse, error) {
	v, err := ifMatch(r.GetIfMatch())
	if err != nil {
		return nil, fail(ctx, err)
	}
	out, err := h.svc.RetireComponent(ctx, r.GetSysid(), v, caller(ctx).IP)
	if err != nil {
		return nil, fail(ctx, err)
	}
	return &payrollv1.RetireComponentResponse{Base: ok("Component retired"), Data: componentPB(out)}, nil
}

// GetComponentTemplate reads the template of a level.
func (h *MasterHandler) GetComponentTemplate(ctx context.Context, r *payrollv1.GetComponentTemplateRequest) (*payrollv1.GetComponentTemplateResponse, error) {
	out, err := h.svc.GetTemplate(ctx, r.GetLevel())
	if err != nil {
		return nil, fail(ctx, err)
	}
	return &payrollv1.GetComponentTemplateResponse{Base: ok("OK"), Level: r.GetLevel(), Items: templateItemsPB(out)}, nil
}

// PutComponentTemplate replaces the template of a level.
func (h *MasterHandler) PutComponentTemplate(ctx context.Context, r *payrollv1.PutComponentTemplateRequest) (*payrollv1.PutComponentTemplateResponse, error) {
	items := make([]component.TemplateItem, len(r.GetItems()))
	for i, it := range r.GetItems() {
		items[i] = component.TemplateItem{Sysid: it.GetComponentSysid(), Sts: it.GetSts(), Tax: int(it.GetTax()),
			Amount: it.GetAmount(), TaxPaidCompany: it.GetTaxPaidCompany()}
	}
	out, err := h.svc.PutTemplate(ctx, r.GetLevel(), items, caller(ctx).IP)
	if err != nil {
		return nil, fail(ctx, err)
	}
	return &payrollv1.PutComponentTemplateResponse{Base: ok("Template updated"), Level: r.GetLevel(), Items: templateItemsPB(out)}, nil
}
