// Package master implements the master-data use cases (T007): validation,
// optimistic concurrency, PII handling and audit around master.Repository.
package master

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/apperr"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/audit"
	domain "github.com/mutugading/goapps-backend/services/payroll/internal/domain/master"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/scope"
)

// TxRunner runs fn in a scoped transaction (postgres.DB.ScopedTx).
type TxRunner interface {
	ScopedTx(ctx context.Context, fn func(tx *sql.Tx) error) error
}

// Service is the master-data application service.
type Service struct {
	db    TxRunner
	repo  domain.Repository
	audit audit.Writer
	enc   string
	hmac  []byte
}

// NewService creates a Service. encKey/hmacKey protect PII columns (D-17).
func NewService(db TxRunner, repo domain.Repository, w audit.Writer, encKey, hmacKey string) *Service {
	return &Service{db: db, repo: repo, audit: w, enc: encKey, hmac: []byte(hmacKey)}
}

// Caller carries per-request facts the delivery layer extracts from the context.
type Caller struct {
	RevealPII bool // holds payroll.master.employee.update (or SUPER_ADMIN)
	IP        string
}

func (s *Service) pii(c Caller) domain.PII {
	return domain.PII{EncKey: s.enc, HMACKey: s.hmac, Reveal: c.RevealPII}
}

// ETag formats a row version as an ETag value.
func ETag(version int) string { return strconv.Itoa(version) }

// ParseIfMatch parses an If-Match value (optionally quoted) into a version.
func ParseIfMatch(v string) (int, error) {
	n, err := strconv.Atoi(strings.Trim(strings.TrimSpace(v), `"`))
	if err != nil || n < 1 {
		return 0, apperr.New(apperr.CodeValidation, "If-Match tidak valid", map[string]string{"field": "if_match"})
	}
	return n, nil
}

// toAppErr maps repository sentinels to apperr; unknown errors pass through.
func toAppErr(err error, what string) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, domain.ErrNotFound):
		return apperr.New(apperr.CodeNotFound, what+" tidak ditemukan", nil)
	case errors.Is(err, domain.ErrVersionMismatch):
		return apperr.New(apperr.CodeVersionMismatch, what+" sudah berubah; muat ulang lalu coba lagi", nil)
	case errors.Is(err, domain.ErrDuplicate):
		return apperr.New(apperr.CodeConflict, what+" sudah ada", nil)
	case errors.Is(err, domain.ErrReference):
		return apperr.New(apperr.CodeValidation, "referensi "+what+" tidak valid", nil)
	}
	return err
}

func validation(field, msg string) error {
	return apperr.New(apperr.CodeValidation, msg, map[string]string{"field": field})
}

// inScope rejects a (company, division) pair outside the caller's scope (D-06).
func inScope(ctx context.Context, companyID int16, division string) error {
	id, ok := scope.FromContext(ctx)
	if !ok {
		return scope.ErrNoIdentity
	}
	for _, sc := range id.Scopes {
		if sc.CompanyID() == companyID && sc.Division() == division {
			return nil
		}
	}
	return apperr.DivisionForbidden(companyID, division)
}

// inCompany rejects a company in which the caller holds no scope at all (D-18).
// Company and division masters carry no RLS (D-15), so this is the only guard.
func inCompany(ctx context.Context, companyID int16) error {
	id, ok := scope.FromContext(ctx)
	if !ok {
		return scope.ErrNoIdentity
	}
	for _, sc := range id.Scopes {
		if sc.CompanyID() == companyID {
			return nil
		}
	}
	return apperr.DivisionForbidden(companyID, "")
}

func companyInScope(ctx context.Context, companyID int16) bool {
	return inCompany(ctx, companyID) == nil
}

// ── Currency ─────────────────────────────────────────────────────────────────

// CreateCurrency creates a currency.
func (s *Service) CreateCurrency(ctx context.Context, c domain.Currency, ip string) (out domain.Currency, err error) {
	err = s.db.ScopedTx(ctx, func(tx *sql.Tx) error {
		var e error
		if out, e = s.repo.CreateCurrency(ctx, tx, c); e != nil {
			return toAppErr(e, "mata uang")
		}
		return s.audit.Write(ctx, tx, audit.Entry{CompanyID: firstScopeCompany(ctx), Action: "CREATE", Entity: "pyr_currency", EntityKey: out.Code, After: out, IP: ip})
	})
	return out, err
}

// ListCurrencies lists currencies.
func (s *Service) ListCurrencies(ctx context.Context) (out []domain.Currency, err error) {
	err = s.db.ScopedTx(ctx, func(tx *sql.Tx) error {
		var e error
		out, e = s.repo.ListCurrencies(ctx, tx)
		return e
	})
	return out, err
}

// UpdateCurrency updates a currency under If-Match.
func (s *Service) UpdateCurrency(ctx context.Context, c domain.Currency, ifMatch int, ip string) (out domain.Currency, err error) {
	err = s.db.ScopedTx(ctx, func(tx *sql.Tx) error {
		if e := s.repo.LockCurrency(ctx, tx, c.Code); e != nil {
			return toAppErr(e, "mata uang")
		}
		before, e := s.repo.GetCurrency(ctx, tx, c.Code)
		if e != nil {
			return toAppErr(e, "mata uang")
		}
		if out, e = s.repo.UpdateCurrency(ctx, tx, c, ifMatch); e != nil {
			return toAppErr(e, "mata uang")
		}
		return s.audit.Write(ctx, tx, audit.Entry{CompanyID: firstScopeCompany(ctx), Action: "UPDATE", Entity: "pyr_currency", EntityKey: out.Code, Before: before, After: out, IP: ip})
	})
	return out, err
}

// ── Company ──────────────────────────────────────────────────────────────────

// CreateCompany creates a company.
func (s *Service) CreateCompany(ctx context.Context, c domain.Company, npwp string, cl Caller) (out domain.Company, err error) {
	if npwp == domain.MaskedValue {
		return out, validation("npwp", "npwp tidak boleh berisi nilai mask")
	}
	if c.BaseCurrency == "" {
		c.BaseCurrency = "IDR"
	}
	err = s.db.ScopedTx(ctx, func(tx *sql.Tx) error {
		var e error
		if out, e = s.repo.CreateCompany(ctx, tx, c, npwp, s.pii(cl)); e != nil {
			return toAppErr(e, "company")
		}
		// The new company is not yet in app.scopes, so RLS would reject an audit row under
		// its own id; record it under the caller's first scoped company instead.
		return s.audit.Write(ctx, tx, audit.Entry{CompanyID: firstScopeCompany(ctx), Action: "CREATE", Entity: "pyr_company", EntityKey: out.Code, After: out, IP: cl.IP})
	})
	return out, err
}

// GetCompany reads a company.
func (s *Service) GetCompany(ctx context.Context, id int16, cl Caller) (out domain.Company, err error) {
	if err = inCompany(ctx, id); err != nil {
		return out, err
	}
	err = s.db.ScopedTx(ctx, func(tx *sql.Tx) error {
		var e error
		out, e = s.repo.GetCompany(ctx, tx, id, s.pii(cl))
		return toAppErr(e, "company")
	})
	return out, err
}

// ListCompanies lists companies.
func (s *Service) ListCompanies(ctx context.Context, cl Caller) (out []domain.Company, err error) {
	err = s.db.ScopedTx(ctx, func(tx *sql.Tx) error {
		var e error
		all, e := s.repo.ListCompanies(ctx, tx, s.pii(cl))
		for _, c := range all {
			if companyInScope(ctx, c.ID) {
				out = append(out, c)
			}
		}
		return e
	})
	return out, err
}

// UpdateCompany updates a company under If-Match.
func (s *Service) UpdateCompany(ctx context.Context, c domain.Company, npwp string, ifMatch int, cl Caller) (out domain.Company, err error) {
	if npwp == domain.MaskedValue {
		return out, validation("npwp", "npwp tidak boleh berisi nilai mask")
	}
	if err = inCompany(ctx, c.ID); err != nil {
		return out, err
	}
	err = s.db.ScopedTx(ctx, func(tx *sql.Tx) error {
		if e := s.repo.LockCompany(ctx, tx, c.ID); e != nil {
			return toAppErr(e, "company")
		}
		before, e := s.repo.GetCompany(ctx, tx, c.ID, s.pii(cl))
		if e != nil {
			return toAppErr(e, "company")
		}
		if out, e = s.repo.UpdateCompany(ctx, tx, c, npwp, ifMatch, s.pii(cl)); e != nil {
			return toAppErr(e, "company")
		}
		return s.audit.Write(ctx, tx, audit.Entry{CompanyID: out.ID, Action: "UPDATE", Entity: "pyr_company", EntityKey: out.Code, Before: before, After: out, IP: cl.IP})
	})
	return out, err
}

// ── Division ─────────────────────────────────────────────────────────────────

// CreateDivision creates a division.
func (s *Service) CreateDivision(ctx context.Context, d domain.Division, ip string) (out domain.Division, err error) {
	if err = inCompany(ctx, d.CompanyID); err != nil {
		return out, err
	}
	err = s.db.ScopedTx(ctx, func(tx *sql.Tx) error {
		var e error
		if out, e = s.repo.CreateDivision(ctx, tx, d); e != nil {
			return toAppErr(e, "divisi")
		}
		return s.audit.Write(ctx, tx, audit.Entry{CompanyID: out.CompanyID, Action: "CREATE", Entity: "pyr_division_ref", EntityKey: out.Code, After: out, IP: ip}) // company-level: the new division is not yet in app.scopes (RLS WITH CHECK)
	})
	return out, err
}

// ListDivisions lists the divisions of a company.
func (s *Service) ListDivisions(ctx context.Context, companyID int16) (out []domain.Division, err error) {
	if err = inCompany(ctx, companyID); err != nil {
		return nil, err
	}
	err = s.db.ScopedTx(ctx, func(tx *sql.Tx) error {
		var e error
		out, e = s.repo.ListDivisions(ctx, tx, companyID)
		return e
	})
	return out, err
}

// UpdateDivision updates a division under If-Match.
func (s *Service) UpdateDivision(ctx context.Context, d domain.Division, ifMatch int, ip string) (out domain.Division, err error) {
	if err = inCompany(ctx, d.CompanyID); err != nil {
		return out, err
	}
	err = s.db.ScopedTx(ctx, func(tx *sql.Tx) error {
		if e := s.repo.LockDivision(ctx, tx, d.CompanyID, d.Code); e != nil {
			return toAppErr(e, "divisi")
		}
		before, e := s.repo.GetDivision(ctx, tx, d.CompanyID, d.Code)
		if e != nil {
			return toAppErr(e, "divisi")
		}
		if out, e = s.repo.UpdateDivision(ctx, tx, d, ifMatch); e != nil {
			return toAppErr(e, "divisi")
		}
		return s.audit.Write(ctx, tx, audit.Entry{CompanyID: out.CompanyID, Action: "UPDATE", Entity: "pyr_division_ref", EntityKey: out.Code, Before: before, After: out, IP: ip})
	})
	return out, err
}

// ── Employee ─────────────────────────────────────────────────────────────────

// EmployeeSecrets are the plaintext PII values of a write; empty on update = keep.
type EmployeeSecrets struct{ NPWP, KTP, BankAcnt string }

// validate rejects the mask echoed back by a client that read masked PII (D-17);
// storing it would overwrite the real value with "***".
func (s EmployeeSecrets) validate() error {
	for f, v := range map[string]string{"npwp": s.NPWP, "ktp": s.KTP, "bank_acnt": s.BankAcnt} {
		if v == domain.MaskedValue {
			return validation(f, f+" tidak boleh berisi nilai mask")
		}
	}
	return nil
}

var (
	maxCuti = decimal.RequireFromString("9999.9") // numeric(5,1); rounding 9999.95 would overflow
	dateRE  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	ccyRE   = regexp.MustCompile(`^[A-Z]{3}$`)
)

func validDate(field, v string) error {
	if v == "" {
		return nil
	}
	if !dateRE.MatchString(v) {
		return validation(field, field+" harus YYYY-MM-DD")
	}
	if _, err := time.Parse("2006-01-02", v); err != nil {
		return validation(field, field+" bukan tanggal valid")
	}
	return nil
}

// normalize validates employee input and applies defaults.
func normalize(e *domain.Employee) error {
	for f, v := range map[string]string{"date_join": e.DateJoin, "resign_date": e.ResignDate, "birth_date": e.BirthDate} {
		if err := validDate(f, v); err != nil {
			return err
		}
	}
	if e.PayCurrency == "" {
		e.PayCurrency = "IDR"
	}
	if !ccyRE.MatchString(e.PayCurrency) {
		return validation("pay_currency", "pay_currency harus kode ISO 3 huruf")
	}
	if e.CutiTahunan != "" {
		d, err := decimal.NewFromString(e.CutiTahunan)
		if err != nil || d.IsNegative() || d.GreaterThan(maxCuti) {
			return validation("cuti_tahunan", "cuti_tahunan harus desimal 0..9999.9")
		}
		e.CutiTahunan = d.Round(1).String() // numeric(5,1), half-up (D-07)
	}
	return nil
}

// CreateEmployee creates an employee inside the caller's scope.
func (s *Service) CreateEmployee(ctx context.Context, e domain.Employee, sec EmployeeSecrets, cl Caller) (out domain.Employee, err error) {
	if err = sec.validate(); err != nil {
		return out, err
	}
	if err = inScope(ctx, e.CompanyID, e.Division); err != nil {
		return out, err
	}
	if err = normalize(&e); err != nil {
		return out, err
	}
	actor := actorOf(ctx)
	err = s.db.ScopedTx(ctx, func(tx *sql.Tx) error {
		var e2 error
		if out, e2 = s.repo.CreateEmployee(ctx, tx, e, sec.NPWP, sec.KTP, sec.BankAcnt, actor, s.pii(cl)); e2 != nil {
			return toAppErr(e2, "karyawan")
		}
		return s.audit.Write(ctx, tx, audit.Entry{CompanyID: out.CompanyID, Division: out.Division, Action: "CREATE",
			Entity: "pyr_employee", EntityKey: strconv.FormatInt(out.ID, 10), After: out, IP: cl.IP})
	})
	return out, err
}

// GetEmployee resolves a current/old NIK to a single employee (13§3.1).
func (s *Service) GetEmployee(ctx context.Context, nik string, companyID int16, division string, cl Caller) (out domain.Employee, err error) {
	err = s.db.ScopedTx(ctx, func(tx *sql.Tx) error {
		ids, e := s.repo.ResolveEmployeeIDs(ctx, tx, nik, companyID, division)
		if e != nil {
			return e
		}
		switch len(ids) {
		case 0:
			return apperr.New(apperr.CodeNotFound, "karyawan tidak ditemukan", nil)
		case 1:
			out, e = s.repo.GetEmployeeByID(ctx, tx, ids[0], s.pii(cl))
			return toAppErr(e, "karyawan")
		default:
			return apperr.New(apperr.CodeConflict, "NIK ambigu: cocok ke lebih dari satu karyawan", map[string]string{"nik": nik})
		}
	})
	return out, err
}

// ListEmployees lists employees with a keyset cursor (the last returned id).
func (s *Service) ListEmployees(ctx context.Context, f domain.EmployeeFilter, cursor string, cl Caller) (out []domain.Employee, next string, err error) {
	if f.Limit <= 0 {
		f.Limit = 50
	}
	if cursor != "" {
		if f.AfterID, err = strconv.ParseInt(cursor, 10, 64); err != nil || f.AfterID < 0 {
			return nil, "", validation("cursor", "cursor tidak valid")
		}
	}
	want := f.Limit
	f.Limit++ // one extra row tells whether another page exists
	err = s.db.ScopedTx(ctx, func(tx *sql.Tx) error {
		var e error
		out, e = s.repo.ListEmployees(ctx, tx, f, s.pii(cl))
		return e
	})
	if err != nil {
		return nil, "", err
	}
	if len(out) > want {
		out = out[:want]
		next = strconv.FormatInt(out[len(out)-1].ID, 10)
	}
	return out, next, nil
}

// UpdateEmployee updates an employee under If-Match.
func (s *Service) UpdateEmployee(ctx context.Context, id int64, e domain.Employee, sec EmployeeSecrets, ifMatch int, cl Caller) (out domain.Employee, err error) {
	if err = sec.validate(); err != nil {
		return out, err
	}
	if err = normalize(&e); err != nil {
		return out, err
	}
	actor := actorOf(ctx)
	err = s.db.ScopedTx(ctx, func(tx *sql.Tx) error {
		if e2 := s.repo.LockEmployee(ctx, tx, id); e2 != nil {
			return toAppErr(e2, "karyawan")
		}
		before, e2 := s.repo.GetEmployeeByID(ctx, tx, id, domain.PII{EncKey: s.enc, HMACKey: s.hmac})
		if e2 != nil {
			return toAppErr(e2, "karyawan")
		}
		if out, e2 = s.repo.UpdateEmployee(ctx, tx, id, e, sec.NPWP, sec.KTP, sec.BankAcnt, actor, ifMatch, s.pii(cl)); e2 != nil {
			return toAppErr(e2, "karyawan")
		}
		return s.audit.Write(ctx, tx, audit.Entry{CompanyID: out.CompanyID, Division: out.Division, Action: "UPDATE",
			Entity: "pyr_employee", EntityKey: strconv.FormatInt(id, 10), Before: before, After: out, IP: cl.IP})
	})
	return out, err
}

func firstScopeCompany(ctx context.Context) int16 {
	if id, ok := scope.FromContext(ctx); ok && len(id.Scopes) > 0 {
		return id.Scopes[0].CompanyID()
	}
	return 0
}

func actorOf(ctx context.Context) string {
	if id, ok := scope.FromContext(ctx); ok {
		return id.UserID
	}
	return ""
}
