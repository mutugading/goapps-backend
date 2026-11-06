package grpc

import (
	"context"
	"errors"
	"fmt"
	"time"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	payrollv1 "github.com/mutugading/goapps-backend/gen/payroll/v1"
	"github.com/mutugading/goapps-backend/services/payroll/internal/application/period"
	"github.com/mutugading/goapps-backend/services/payroll/internal/application/run"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/apperr"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/scope"
)

// T022b: run and period RPCs (07§4–5, D-51). Order per RPC: permission (interceptor) → scope check
// (before any query) → service. Nominals are masked without payroll.salary.amount.view (I23).
const (
	keyField           = "field"
	keyDivision        = "division"
	permRunDry         = "payroll.run.dryrun.execute"
	permRunCommit      = "payroll.run.salary.commit"
	permPeriodApprove  = "payroll.period.salary.approve"
	permPeriodClose    = "payroll.period.salary.close"
	permPeriodReopen   = "payroll.period.salary.reopen"
	permReportView     = "payroll.report.data.view"
	permSalaryAmount   = "payroll.salary.amount.view"
	masked             = "***"
	runTimeout         = 120 * time.Second // D-51: CreateRun is synchronous
	dateLayout         = "2006-01-02"
	runWarningFmt      = "[%s] %s"
	runWarningFmtNIK   = "[%s] %s: %s"
	defaultRunLinePage = 100
)

// methodTimeouts raises the server timeout of the synchronous run RPC above the default (TimeoutInterceptor).
var methodTimeouts = map[string]time.Duration{"/payroll.v1.PayrollService/CreateRun": runTimeout}

func init() {
	const svc = "/payroll.v1.PayrollService/"
	for m, p := range map[string]string{
		"CreateRun": permRunDry, // commit mode additionally needs permRunCommit (checked in CreateRun)
		"GetRun":    permReportView, "ListRuns": permReportView, "ListRunLines": permReportView, "ListPeriods": permReportView,
		"ApprovePeriod": permPeriodApprove, "ClosePeriod": permPeriodClose, "ReopenPeriod": permPeriodReopen,
	} {
		rpcPermissions[svc+m] = p
	}
}

// RunHandler implements the run and period RPCs of PayrollService.
type RunHandler struct {
	runs    *run.Service
	periods *period.Service
	reader  run.Reader
}

// NewRunHandler creates a RunHandler.
func NewRunHandler(runs *run.Service, periods *period.Service, reader run.Reader) *RunHandler {
	return &RunHandler{runs: runs, periods: periods, reader: reader}
}

// PayrollHandler combines the master-data and run handlers into one PayrollServiceServer.
type PayrollHandler struct {
	*MasterHandler
	*RunHandler
}

// NewPayrollHandler combines the handlers; the embedded Unimplemented servers are shadowed by the real methods.
func NewPayrollHandler(m *MasterHandler, r *RunHandler) *PayrollHandler {
	return &PayrollHandler{MasterHandler: m, RunHandler: r}
}

func scopeOf(ctx context.Context) ([]scope.Scope, bool) {
	id, found := scope.FromContext(ctx)
	return id.Scopes, found
}

func identityOf(ctx context.Context) (string, bool) {
	id, found := scope.FromContext(ctx)
	return id.UserID, found
}

// canSeeAmounts follows the SUPER_ADMIN bypass of PermissionInterceptor and MasterHandler.caller (I23).
func canSeeAmounts(ctx context.Context) bool {
	return IsSuperAdmin(ctx) || HasPermission(ctx, permSalaryAmount)
}

// mask returns v, or "***" when the caller may not see nominals.
func mask(see bool, v string) string {
	if see {
		return v
	}
	return masked
}

// requireDivision refuses a division outside the caller scope before anything is queried (I22).
func requireDivision(ctx context.Context, companyID int16, division string) error {
	return run.InScope(ctx, companyID, division)
}

// warningsPB keeps code and NIK; the free-text message can embed nominals (net, bruto), so it is masked
// without payroll.salary.amount.view (I23).
func warningsPB(in []run.Issue, see bool) []string {
	out := make([]string, len(in))
	for i, w := range in {
		msg := mask(see, w.Message)
		if w.NIK != "" {
			out[i] = fmt.Sprintf(runWarningFmtNIK, w.Code, w.NIK, msg)
		} else {
			out[i] = fmt.Sprintf(runWarningFmt, w.Code, msg)
		}
	}
	return out
}

func runPB(r run.Info, see bool) *payrollv1.Run {
	out := &payrollv1.Run{
		RunId: r.ID, Yymm: int32(r.YYMM), Division: r.Division, ProcessDate: r.PayDate, Status: r.Status, //nolint:gosec // validated yymm
		EmployeeCount: int32(r.EmpCount), LineCount: int32(r.LineCount), //nolint:gosec // bounded by employees
		TotalTunjangan: mask(see, r.TotT), TotalPotongan: mask(see, r.TotP), RuleSetId: int32(r.RuleSetID), //nolint:gosec // small id
		SnapshotId: r.SnapshotID, NikFilter: r.NIKFilter, Warnings: warningsPB(r.Warnings, see), StartedBy: r.StartedBy,
		StartedAt: r.StartedAt.UTC().Format(time.RFC3339), DurationMs: r.DurationMs,
	}
	if !r.FinishedAt.IsZero() {
		out.FinishedAt = r.FinishedAt.UTC().Format(time.RFC3339)
	}
	return out
}

func linePB(l run.LineInfo, see bool) *payrollv1.RunLine {
	return &payrollv1.RunLine{Nik: l.NIK, CompType: l.CompType, CompId: int32(l.CompID), Description: l.Description, //nolint:gosec // small id
		Amount: mask(see, l.Amount), Base: mask(see, l.Base), Qty: l.Qty, Rate: mask(see, l.Rate), Formula: l.Formula, Remarks: l.Remarks}
}

func periodPB(p run.PeriodInfo) *payrollv1.Period {
	return &payrollv1.Period{Yymm: int32(p.YYMM), Division: p.Division, DateFrom: p.DateFrom, DateTo: p.DateTo, Status: p.Status, //nolint:gosec // validated yymm
		ActiveRunId: p.ActiveRunID, ApprovedBy: p.ApprovedBy, ApprovedAt: p.ApprovedAt, ClosedBy: p.ClosedBy, ClosedAt: p.ClosedAt,
		ReopenReason: p.ReopenReason}
}

// CreateRun runs the division synchronously: dry returns the totals per employee, commit persists the run.
func (h *RunHandler) CreateRun(ctx context.Context, r *payrollv1.CreateRunRequest) (*payrollv1.CreateRunResponse, error) {
	company, division, yymm := i16(r.GetCompanyId()), r.GetDivision(), int(r.GetYymm())
	if err := requireDivision(ctx, company, division); err != nil {
		return nil, fail(ctx, err)
	}
	payDate, err := validateCreateRun(ctx, r)
	if err != nil {
		return nil, fail(ctx, err)
	}
	commit := r.GetMode() == payrollv1.RunMode_RUN_MODE_COMMIT
	ruleSet, err := h.runs.ResolveRuleSet(ctx, company, division, yymm)
	if err != nil {
		return nil, fail(ctx, err)
	}
	see := canSeeAmounts(ctx)
	if !commit {
		out, err := h.runs.Dry(ctx, run.DryCommand{CompanyID: company, Division: division, YYMM: yymm, RuleSetID: ruleSet, NIKs: r.GetNik()})
		if err != nil {
			return nil, fail(ctx, err)
		}
		return &payrollv1.CreateRunResponse{Base: ok("Dry run completed"), Data: dryRunPB(ctx, r, ruleSet, out, see),
			Employees: employeesPB(out.Employees, see)}, nil
	}
	cm, err := h.runs.Commit(ctx, run.CommitCommand{CompanyID: company, Division: division, YYMM: yymm, RuleSetID: ruleSet,
		PayDate: payDate, IP: caller(ctx).IP})
	if err != nil {
		return nil, fail(ctx, err)
	}
	info, err := h.reader.GetRun(ctx, cm.RunID)
	if err != nil {
		return nil, fail(ctx, err)
	}
	info.Warnings = cm.Warnings
	return &payrollv1.CreateRunResponse{Base: ok("Run committed"), Data: runPB(info, see)}, nil
}

// validateCreateRun checks the request before the rule set is resolved (400 on failure) and returns the pay date.
// process_date is required to commit and ignored by dry; commit per NIK is not supported yet (B-UI3).
func validateCreateRun(ctx context.Context, r *payrollv1.CreateRunRequest) (time.Time, error) {
	yymm := int(r.GetYymm())
	if m := yymm % 100; yymm < 190001 || m < 1 || m > 12 {
		return time.Time{}, apperr.New(apperr.CodeValidation, "yymm tidak valid", map[string]string{keyField: "yymm"})
	}
	commit := r.GetMode() == payrollv1.RunMode_RUN_MODE_COMMIT
	if !commit && r.GetMode() != payrollv1.RunMode_RUN_MODE_DRY {
		return time.Time{}, apperr.New(apperr.CodeValidation, "mode wajib dry atau commit", map[string]string{keyField: "mode"})
	}
	if commit && !IsSuperAdmin(ctx) && !HasPermission(ctx, permRunCommit) {
		return time.Time{}, apperr.New(apperr.CodePermissionDenied, "permission denied: requires "+permRunCommit, nil)
	}
	if commit && len(r.GetNik()) > 0 {
		return time.Time{}, apperr.New(apperr.CodeValidation, "commit per NIK belum didukung; pakai mode dry", map[string]string{keyField: "nik"})
	}
	if !commit && r.GetProcessDate() == "" {
		return time.Time{}, nil
	}
	payDate, err := time.Parse(dateLayout, r.GetProcessDate())
	if err != nil {
		return time.Time{}, apperr.New(apperr.CodeValidation, "process_date harus YYYY-MM-DD", map[string]string{keyField: "process_date"})
	}
	return payDate, nil
}

func dryRunPB(ctx context.Context, r *payrollv1.CreateRunRequest, ruleSet int, out run.DryResult, see bool) *payrollv1.Run {
	actor := ""
	if id, found := identityOf(ctx); found {
		actor = id
	}
	return runPB(run.Info{YYMM: int(r.GetYymm()), Division: r.GetDivision(), PayDate: r.GetProcessDate(), Status: "DRY",
		EmpCount: out.Summary.EmpCount, LineCount: out.Summary.LineCount, TotT: out.Summary.TotalT.StringFixed(2),
		TotP: out.Summary.TotalP.StringFixed(2), RuleSetID: ruleSet, SnapshotID: out.SnapshotID, NIKFilter: r.GetNik(),
		Warnings: out.Warnings, StartedBy: actor, StartedAt: time.Now().Add(-out.Duration), FinishedAt: time.Now(),
		DurationMs: out.Duration.Milliseconds()}, see)
}

func employeesPB(in []run.EmployeeTotal, see bool) []*payrollv1.RunEmployeeTotal {
	out := make([]*payrollv1.RunEmployeeTotal, len(in))
	for i, e := range in {
		out[i] = &payrollv1.RunEmployeeTotal{Nik: e.NIK, Name: e.Name, TotalTunjangan: mask(see, e.TotT),
			TotalPotongan: mask(see, e.TotP), TakeHome: mask(see, e.TakeHome)}
	}
	return out
}

// GetRun reads the summary of one run. A run outside the caller scope is invisible (404): the division of an
// unknown id cannot be checked before the query without leaking that the id exists.
func (h *RunHandler) GetRun(ctx context.Context, r *payrollv1.GetRunRequest) (*payrollv1.GetRunResponse, error) {
	out, err := h.reader.GetRun(ctx, r.GetRunId())
	if err != nil {
		return nil, fail(ctx, err)
	}
	return &payrollv1.GetRunResponse{Base: ok("OK"), Data: runPB(out, canSeeAmounts(ctx))}, nil
}

// ListRuns lists runs of the caller scope; an explicit division outside the scope is 403.
func (h *RunHandler) ListRuns(ctx context.Context, r *payrollv1.ListRunsRequest) (*payrollv1.ListRunsResponse, error) {
	if err := h.requireListDivision(ctx, r.GetDivision()); err != nil {
		return nil, fail(ctx, err)
	}
	out, err := h.reader.ListRuns(ctx, int(r.GetYymm()), r.GetDivision())
	if err != nil {
		return nil, fail(ctx, err)
	}
	see := canSeeAmounts(ctx)
	resp := &payrollv1.ListRunsResponse{Base: ok("OK")}
	for _, x := range out {
		resp.Data = append(resp.Data, runPB(x, see))
	}
	return resp, nil
}

// ListRunLines pages the lines of a run (see GetRun for the scope behavior).
func (h *RunHandler) ListRunLines(ctx context.Context, r *payrollv1.ListRunLinesRequest) (*payrollv1.ListRunLinesResponse, error) {
	page, size := int(r.GetPage()), int(r.GetPageSize())
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = defaultRunLinePage
	}
	lines, total, err := h.reader.ListLines(ctx, r.GetRunId(), r.GetNik(), page, size)
	if err != nil {
		return nil, fail(ctx, err)
	}
	see := canSeeAmounts(ctx)
	resp := &payrollv1.ListRunLinesResponse{Base: ok("OK")}
	for _, l := range lines {
		resp.Data = append(resp.Data, linePB(l, see))
	}
	pages := (total + int64(size) - 1) / int64(size)
	resp.Pagination = &commonv1.PaginationResponse{CurrentPage: int32(page), PageSize: int32(size), TotalItems: total, TotalPages: int32(pages)} //nolint:gosec // bounded
	return resp, nil
}

// ListPeriods lists the periods of the caller scope.
func (h *RunHandler) ListPeriods(ctx context.Context, r *payrollv1.ListPeriodsRequest) (*payrollv1.ListPeriodsResponse, error) {
	if err := h.requireListDivision(ctx, r.GetDivision()); err != nil {
		return nil, fail(ctx, err)
	}
	out, err := h.reader.ListPeriods(ctx, r.GetDivision())
	if err != nil {
		return nil, fail(ctx, err)
	}
	resp := &payrollv1.ListPeriodsResponse{Base: ok("OK")}
	for _, p := range out {
		resp.Data = append(resp.Data, periodPB(p))
	}
	return resp, nil
}

// requireListDivision: an empty filter means "all in scope"; an explicit division must be in scope in any company.
func (h *RunHandler) requireListDivision(ctx context.Context, division string) error {
	if division == "" {
		return nil
	}
	id, found := scopeOf(ctx)
	if !found {
		return run.InScope(ctx, 0, division)
	}
	for _, sc := range id {
		if sc.Division() == division {
			return nil
		}
	}
	return apperr.New(apperr.CodeDivisionForbidden, "divisi di luar scope pengguna", map[string]string{keyDivision: division})
}

// periodCommand builds the command. The period RPCs carry no company_id (T022a), so the company is the one of
// the caller scope that has the division AND a period (yymm, division). More than one match is ambiguous (D-53).
func (h *RunHandler) periodCommand(ctx context.Context, yymm int32, division, reason string) (period.Command, error) {
	scopes, found := scopeOf(ctx)
	if !found {
		return period.Command{}, run.InScope(ctx, 0, division)
	}
	var company int16
	inScope := false
	for _, sc := range scopes {
		if sc.Division() != division {
			continue
		}
		inScope = true
		if _, err := h.reader.GetPeriod(ctx, sc.CompanyID(), int(yymm), division); err != nil {
			if ae := new(apperr.Error); errors.As(err, &ae) && ae.Code == apperr.CodeNotFound {
				continue
			}
			return period.Command{}, err
		}
		if company != 0 {
			return period.Command{}, apperr.New(apperr.CodeValidation, "divisi ada di lebih dari satu company; periode ambigu", map[string]string{keyField: "division"})
		}
		company = sc.CompanyID()
	}
	if !inScope {
		return period.Command{}, apperr.DivisionForbidden(0, division)
	}
	if company == 0 {
		return period.Command{}, apperr.New(apperr.CodeNotFound, "periode tidak ditemukan", map[string]string{keyDivision: division})
	}
	return period.Command{CompanyID: company, Division: division, YYMM: int(yymm), Reason: reason, IP: caller(ctx).IP}, nil
}

func (h *RunHandler) transition(ctx context.Context, yymm int32, division, reason string,
	do func(context.Context, period.Command) error) (*payrollv1.Period, error) {
	cmd, err := h.periodCommand(ctx, yymm, division, reason)
	if err != nil {
		return nil, err
	}
	if err := do(ctx, cmd); err != nil {
		return nil, err
	}
	p, err := h.reader.GetPeriod(ctx, cmd.CompanyID, cmd.YYMM, cmd.Division)
	if err != nil {
		return nil, err
	}
	return periodPB(p), nil
}

// ApprovePeriod approves the period (needs an ACTIVE run).
func (h *RunHandler) ApprovePeriod(ctx context.Context, r *payrollv1.ApprovePeriodRequest) (*payrollv1.ApprovePeriodResponse, error) {
	p, err := h.transition(ctx, r.GetYymm(), r.GetDivision(), "", h.periods.Approve)
	if err != nil {
		return nil, fail(ctx, err)
	}
	return &payrollv1.ApprovePeriodResponse{Base: ok("Period approved"), Data: p}, nil
}

// ClosePeriod closes an approved period.
func (h *RunHandler) ClosePeriod(ctx context.Context, r *payrollv1.ClosePeriodRequest) (*payrollv1.ClosePeriodResponse, error) {
	p, err := h.transition(ctx, r.GetYymm(), r.GetDivision(), "", h.periods.Close)
	if err != nil {
		return nil, fail(ctx, err)
	}
	return &payrollv1.ClosePeriodResponse{Base: ok("Period closed"), Data: p}, nil
}

// ReopenPeriod reopens an approved/closed period with a reason of at least 10 characters.
func (h *RunHandler) ReopenPeriod(ctx context.Context, r *payrollv1.ReopenPeriodRequest) (*payrollv1.ReopenPeriodResponse, error) {
	p, err := h.transition(ctx, r.GetYymm(), r.GetDivision(), r.GetReason(), h.periods.Reopen)
	if err != nil {
		return nil, fail(ctx, err)
	}
	return &payrollv1.ReopenPeriodResponse{Base: ok("Period reopened"), Data: p}, nil
}
