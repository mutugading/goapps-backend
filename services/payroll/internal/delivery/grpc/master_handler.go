package grpc

import (
	"context"
	"errors"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	payrollv1 "github.com/mutugading/goapps-backend/gen/payroll/v1"
	app "github.com/mutugading/goapps-backend/services/payroll/internal/application/master"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/apperr"
	domain "github.com/mutugading/goapps-backend/services/payroll/internal/domain/master"
)

// Permission codes (IAM 4-segment, D-05; docs/permissions.md).
const (
	permEmployeeView   = "payroll.master.employee.view"
	permEmployeeCreate = "payroll.master.employee.create"
	permEmployeeUpdate = "payroll.master.employee.update"
)

func init() {
	const svc = "/payroll.v1.PayrollService/"
	for m, p := range map[string]string{
		"ListCurrencies": permEmployeeView, "GetCompany": permEmployeeView, "ListCompanies": permEmployeeView,
		"ListDivisions": permEmployeeView, "GetEmployee": permEmployeeView, "ListEmployees": permEmployeeView,
		"CreateCurrency": permEmployeeCreate, "CreateCompany": permEmployeeCreate,
		"CreateDivision": permEmployeeCreate, "CreateEmployee": permEmployeeCreate,
		"UpdateCurrency": permEmployeeUpdate, "UpdateCompany": permEmployeeUpdate,
		"UpdateDivision": permEmployeeUpdate, "UpdateEmployee": permEmployeeUpdate,
	} {
		rpcPermissions[svc+m] = p
	}
}

// MasterHandler implements the master-data RPCs of PayrollService.
type MasterHandler struct {
	payrollv1.UnimplementedPayrollServiceServer
	svc *app.Service
}

// NewMasterHandler creates a MasterHandler.
func NewMasterHandler(svc *app.Service) *MasterHandler { return &MasterHandler{svc: svc} }

func ok(msg string) *commonv1.BaseResponse {
	return &commonv1.BaseResponse{IsSuccess: true, StatusCode: "200", Message: msg}
}

// fail converts err to a status error; unknown errors are logged and hidden.
func fail(_ context.Context, err error) error {
	if err == nil {
		return nil
	}
	var ae *apperr.Error
	if errors.As(err, &ae) {
		return ToStatusError(err)
	}
	if _, isStatus := status.FromError(err); isStatus {
		return err
	}
	log.Error().Err(err).Msg("master handler error")
	return status.Error(codes.Internal, "internal error")
}

// caller builds the per-request facts. PII is revealed only to holders of
// payroll.master.employee.update (D-17); SUPER_ADMIN counts as a holder.
func caller(ctx context.Context) app.Caller {
	c := app.Caller{RevealPII: IsSuperAdmin(ctx) || HasPermission(ctx, permEmployeeUpdate)}
	if p, found := peer.FromContext(ctx); found && p.Addr != nil {
		c.IP = hostOnly(p.Addr.String())
	}
	return c
}

func hostOnly(addr string) string {
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[:i]
		}
	}
	return addr
}

func ifMatch(v string) (int, error) { return app.ParseIfMatch(v) }

func i16(v int32) int16 { return int16(v) } //nolint:gosec // validated gt 0 / small ids

// ── mappers ──────────────────────────────────────────────────────────────────

func currencyPB(c domain.Currency) *payrollv1.Currency {
	return &payrollv1.Currency{Code: c.Code, MinorUnit: int32(c.MinorUnit), TakehomeRound: int32(c.TakehomeRound), Etag: app.ETag(c.Version)} //nolint:gosec // small
}

func companyPB(c domain.Company) *payrollv1.Company {
	return &payrollv1.Company{CompanyId: int32(c.ID), Code: c.Code, Name: c.Name, Npwp: c.NPWP,
		BaseCurrency: c.BaseCurrency, Active: c.Active, Etag: app.ETag(c.Version)}
}

func divisionPB(d domain.Division) *payrollv1.Division {
	return &payrollv1.Division{CompanyId: int32(d.CompanyID), Code: d.Code, Name: d.Name, Etag: app.ETag(d.Version)}
}

func employeePB(e domain.Employee) *payrollv1.Employee {
	return &payrollv1.Employee{
		Id: e.ID, Nik: e.NIK, CompanyId: int32(e.CompanyID), Division: e.Division, Nama: e.Nama, Email: e.Email,
		DeptId: e.DeptID, Dept: e.Dept, SectionId: e.SectionID, Section: e.Section,
		DesignationId: e.DesignationID, Designation: e.Designation, Grade: e.Grade, Level: e.Level, TaxId: e.TaxID,
		StsEmp: int32(e.StsEmp), StsPayroll: int32(e.StsPayroll), StsBonus: int32(e.StsBonus), //nolint:gosec // smallint
		DateJoin: e.DateJoin, ResignDate: e.ResignDate, BirthDate: e.BirthDate, BirthPlace: e.BirthPlace,
		Sex: e.Sex, MarriedSts: e.MarriedSts, Religion: e.Religion,
		Npwp: e.NPWP, Ktp: e.KTP, Bank: e.Bank, BankBranch: e.BankBranch, BankAcnt: e.BankAcnt, BankAcntName: e.BankAcntName,
		OrionSubAcnt: e.OrionSubAcnt, IdMesin: e.IDMesin, CutiTahunan: e.CutiTahunan,
		PayCurrency: e.PayCurrency, EmploymentStatus: e.EmploymentStatus, CostGroup: e.CostGroup,
		HrisSysId: e.HrisSysID, PrevEmployeeId: e.PrevEmployeeID, PlacementReason: e.PlacementReason,
		Etag:  app.ETag(e.Version),
		Audit: &commonv1.AuditInfo{CreatedAt: e.CreatedAt, CreatedBy: e.CreatedBy, UpdatedAt: e.UpdatedAt, UpdatedBy: e.UpdatedBy},
	}
}

func employeeFromInput(in *payrollv1.EmployeeInput) (domain.Employee, app.EmployeeSecrets) {
	return domain.Employee{
		Nama: in.GetNama(), Email: in.GetEmail(), DeptID: in.GetDeptId(), Dept: in.GetDept(),
		SectionID: in.GetSectionId(), Section: in.GetSection(), DesignationID: in.GetDesignationId(), Designation: in.GetDesignation(),
		Grade: in.GetGrade(), Level: in.GetLevel(), TaxID: in.GetTaxId(),
		StsEmp: int(in.GetStsEmp()), StsPayroll: int(in.GetStsPayroll()), StsBonus: int(in.GetStsBonus()),
		DateJoin: in.GetDateJoin(), ResignDate: in.GetResignDate(), BirthDate: in.GetBirthDate(), BirthPlace: in.GetBirthPlace(),
		Sex: in.GetSex(), MarriedSts: in.GetMarriedSts(), Religion: in.GetReligion(),
		Bank: in.GetBank(), BankBranch: in.GetBankBranch(), BankAcntName: in.GetBankAcntName(),
		OrionSubAcnt: in.GetOrionSubAcnt(), IDMesin: in.GetIdMesin(), CutiTahunan: in.GetCutiTahunan(),
		PayCurrency: in.GetPayCurrency(), EmploymentStatus: in.GetEmploymentStatus(), CostGroup: in.GetCostGroup(),
		HrisSysID: in.GetHrisSysId(),
	}, app.EmployeeSecrets{NPWP: in.GetNpwp(), KTP: in.GetKtp(), BankAcnt: in.GetBankAcnt()}
}

// ── Currency ─────────────────────────────────────────────────────────────────

// CreateCurrency creates a currency.
func (h *MasterHandler) CreateCurrency(ctx context.Context, r *payrollv1.CreateCurrencyRequest) (*payrollv1.CreateCurrencyResponse, error) {
	out, err := h.svc.CreateCurrency(ctx, domain.Currency{Code: r.GetCode(), MinorUnit: int(r.GetMinorUnit()), TakehomeRound: int(r.GetTakehomeRound())}, caller(ctx).IP)
	if err != nil {
		return nil, fail(ctx, err)
	}
	return &payrollv1.CreateCurrencyResponse{Base: ok("Currency created"), Data: currencyPB(out)}, nil
}

// ListCurrencies lists currencies.
func (h *MasterHandler) ListCurrencies(ctx context.Context, _ *payrollv1.ListCurrenciesRequest) (*payrollv1.ListCurrenciesResponse, error) {
	out, err := h.svc.ListCurrencies(ctx)
	if err != nil {
		return nil, fail(ctx, err)
	}
	resp := &payrollv1.ListCurrenciesResponse{Base: ok("OK")}
	for _, c := range out {
		resp.Data = append(resp.Data, currencyPB(c))
	}
	return resp, nil
}

// UpdateCurrency updates a currency under If-Match.
func (h *MasterHandler) UpdateCurrency(ctx context.Context, r *payrollv1.UpdateCurrencyRequest) (*payrollv1.UpdateCurrencyResponse, error) {
	v, err := ifMatch(r.GetIfMatch())
	if err != nil {
		return nil, fail(ctx, err)
	}
	out, err := h.svc.UpdateCurrency(ctx, domain.Currency{Code: r.GetCode(), MinorUnit: int(r.GetMinorUnit()), TakehomeRound: int(r.GetTakehomeRound())}, v, caller(ctx).IP)
	if err != nil {
		return nil, fail(ctx, err)
	}
	return &payrollv1.UpdateCurrencyResponse{Base: ok("Currency updated"), Data: currencyPB(out)}, nil
}

// ── Company ──────────────────────────────────────────────────────────────────

// CreateCompany creates a company.
func (h *MasterHandler) CreateCompany(ctx context.Context, r *payrollv1.CreateCompanyRequest) (*payrollv1.CreateCompanyResponse, error) {
	out, err := h.svc.CreateCompany(ctx, domain.Company{Code: r.GetCode(), Name: r.GetName(), BaseCurrency: r.GetBaseCurrency()}, r.GetNpwp(), caller(ctx))
	if err != nil {
		return nil, fail(ctx, err)
	}
	return &payrollv1.CreateCompanyResponse{Base: ok("Company created"), Data: companyPB(out)}, nil
}

// GetCompany reads a company.
func (h *MasterHandler) GetCompany(ctx context.Context, r *payrollv1.GetCompanyRequest) (*payrollv1.GetCompanyResponse, error) {
	out, err := h.svc.GetCompany(ctx, i16(r.GetCompanyId()), caller(ctx))
	if err != nil {
		return nil, fail(ctx, err)
	}
	return &payrollv1.GetCompanyResponse{Base: ok("OK"), Data: companyPB(out)}, nil
}

// ListCompanies lists companies.
func (h *MasterHandler) ListCompanies(ctx context.Context, _ *payrollv1.ListCompaniesRequest) (*payrollv1.ListCompaniesResponse, error) {
	out, err := h.svc.ListCompanies(ctx, caller(ctx))
	if err != nil {
		return nil, fail(ctx, err)
	}
	resp := &payrollv1.ListCompaniesResponse{Base: ok("OK")}
	for _, c := range out {
		resp.Data = append(resp.Data, companyPB(c))
	}
	return resp, nil
}

// UpdateCompany updates a company under If-Match.
func (h *MasterHandler) UpdateCompany(ctx context.Context, r *payrollv1.UpdateCompanyRequest) (*payrollv1.UpdateCompanyResponse, error) {
	v, err := ifMatch(r.GetIfMatch())
	if err != nil {
		return nil, fail(ctx, err)
	}
	out, err := h.svc.UpdateCompany(ctx, domain.Company{ID: i16(r.GetCompanyId()), Name: r.GetName(), Active: r.GetActive()}, r.GetNpwp(), v, caller(ctx))
	if err != nil {
		return nil, fail(ctx, err)
	}
	return &payrollv1.UpdateCompanyResponse{Base: ok("Company updated"), Data: companyPB(out)}, nil
}

// ── Division ─────────────────────────────────────────────────────────────────

// CreateDivision creates a division.
func (h *MasterHandler) CreateDivision(ctx context.Context, r *payrollv1.CreateDivisionRequest) (*payrollv1.CreateDivisionResponse, error) {
	out, err := h.svc.CreateDivision(ctx, domain.Division{CompanyID: i16(r.GetCompanyId()), Code: r.GetCode(), Name: r.GetName()}, caller(ctx).IP)
	if err != nil {
		return nil, fail(ctx, err)
	}
	return &payrollv1.CreateDivisionResponse{Base: ok("Division created"), Data: divisionPB(out)}, nil
}

// ListDivisions lists the divisions of a company.
func (h *MasterHandler) ListDivisions(ctx context.Context, r *payrollv1.ListDivisionsRequest) (*payrollv1.ListDivisionsResponse, error) {
	out, err := h.svc.ListDivisions(ctx, i16(r.GetCompanyId()))
	if err != nil {
		return nil, fail(ctx, err)
	}
	resp := &payrollv1.ListDivisionsResponse{Base: ok("OK")}
	for _, d := range out {
		resp.Data = append(resp.Data, divisionPB(d))
	}
	return resp, nil
}

// UpdateDivision updates a division under If-Match.
func (h *MasterHandler) UpdateDivision(ctx context.Context, r *payrollv1.UpdateDivisionRequest) (*payrollv1.UpdateDivisionResponse, error) {
	v, err := ifMatch(r.GetIfMatch())
	if err != nil {
		return nil, fail(ctx, err)
	}
	out, err := h.svc.UpdateDivision(ctx, domain.Division{CompanyID: i16(r.GetCompanyId()), Code: r.GetCode(), Name: r.GetName()}, v, caller(ctx).IP)
	if err != nil {
		return nil, fail(ctx, err)
	}
	return &payrollv1.UpdateDivisionResponse{Base: ok("Division updated"), Data: divisionPB(out)}, nil
}

// ── Employee ─────────────────────────────────────────────────────────────────

// CreateEmployee creates an employee.
func (h *MasterHandler) CreateEmployee(ctx context.Context, r *payrollv1.CreateEmployeeRequest) (*payrollv1.CreateEmployeeResponse, error) {
	e, sec := employeeFromInput(r.GetInput())
	e.CompanyID, e.Division, e.NIK = i16(r.GetCompanyId()), r.GetDivision(), r.GetNik()
	out, err := h.svc.CreateEmployee(ctx, e, sec, caller(ctx))
	if err != nil {
		return nil, fail(ctx, err)
	}
	return &payrollv1.CreateEmployeeResponse{Base: ok("Employee created"), Data: employeePB(out)}, nil
}

// GetEmployee resolves a current or old NIK.
func (h *MasterHandler) GetEmployee(ctx context.Context, r *payrollv1.GetEmployeeRequest) (*payrollv1.GetEmployeeResponse, error) {
	out, err := h.svc.GetEmployee(ctx, r.GetNik(), i16(r.GetCompanyId()), caller(ctx))
	if err != nil {
		return nil, fail(ctx, err)
	}
	return &payrollv1.GetEmployeeResponse{Base: ok("OK"), Data: employeePB(out)}, nil
}

// ListEmployees lists employees with a keyset cursor.
func (h *MasterHandler) ListEmployees(ctx context.Context, r *payrollv1.ListEmployeesRequest) (*payrollv1.ListEmployeesResponse, error) {
	out, next, err := h.svc.ListEmployees(ctx, domain.EmployeeFilter{
		CompanyID: i16(r.GetCompanyId()), Division: r.GetDivision(), StsPayroll: int(r.GetStsPayroll()),
		Q: r.GetQ(), Limit: int(r.GetLimit()),
	}, r.GetCursor(), caller(ctx))
	if err != nil {
		return nil, fail(ctx, err)
	}
	resp := &payrollv1.ListEmployeesResponse{Base: ok("OK"), NextCursor: next}
	for _, e := range out {
		resp.Data = append(resp.Data, employeePB(e))
	}
	return resp, nil
}

// UpdateEmployee updates an employee under If-Match.
func (h *MasterHandler) UpdateEmployee(ctx context.Context, r *payrollv1.UpdateEmployeeRequest) (*payrollv1.UpdateEmployeeResponse, error) {
	v, err := ifMatch(r.GetIfMatch())
	if err != nil {
		return nil, fail(ctx, err)
	}
	e, sec := employeeFromInput(r.GetInput())
	out, err := h.svc.UpdateEmployee(ctx, r.GetId(), e, sec, v, caller(ctx))
	if err != nil {
		return nil, fail(ctx, err)
	}
	return &payrollv1.UpdateEmployeeResponse{Base: ok("Employee updated"), Data: employeePB(out)}, nil
}
