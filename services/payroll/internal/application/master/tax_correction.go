package master

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/apperr"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/audit"
	domain "github.com/mutugading/goapps-backend/services/payroll/internal/domain/master"
)

const (
	actTaxCorrect   = "TAX_STATUS_CORRECT"
	maxReasonLength = 500
)

// wib is the calendar of the tax-status history (000053/000054 use Asia/Jakarta).
var wib = time.FixedZone("WIB", 7*60*60)

// TaxCorrection is a backdated correction of a wrongly entered tax status (D-82 (5), T033n2).
type TaxCorrection struct {
	TaxID     string
	ValidFrom string // YYYY-MM-DD, current or previous tax year, not in the future (WIB)
	Reason    string
}

// validate checks the request against the clock; the DB function repeats the date and period checks.
func (c *TaxCorrection) validate(now time.Time) error {
	c.TaxID, c.Reason = strings.TrimSpace(c.TaxID), strings.TrimSpace(c.Reason)
	if c.TaxID == "" {
		return validation("tax_id", "tax_id wajib diisi")
	}
	if c.Reason == "" || len(c.Reason) > maxReasonLength {
		return validation("reason", "alasan koreksi wajib diisi (maks 500 karakter)")
	}
	if c.ValidFrom == "" {
		return validation("valid_from", "valid_from wajib diisi")
	}
	if err := validDate("valid_from", c.ValidFrom); err != nil {
		return err
	}
	d, err := time.ParseInLocation("2006-01-02", c.ValidFrom, wib)
	if err != nil {
		return validation("valid_from", "valid_from bukan tanggal valid")
	}
	today := now.In(wib)
	if d.After(today) || d.Year() < today.Year()-1 {
		return validation("valid_from", "valid_from harus di tahun pajak berjalan atau tahun lalu, dan tidak di masa depan")
	}
	return nil
}

// CorrectEmployeeTaxStatus records a wrongly entered tax status as valid from c.ValidFrom (D-82 (5)): the annual
// PTKP (status on 1 January) then uses it. Permission (payroll.master.taxstatus.update) is checked by the
// interceptor; scope, If-Match, the tax-status code and a final tax year are checked here and in the DB function.
func (s *Service) CorrectEmployeeTaxStatus(ctx context.Context, id int64, c TaxCorrection, ifMatch int, cl Caller) (out domain.Employee, err error) {
	if err = c.validate(s.now()); err != nil {
		return out, err
	}
	err = s.db.ScopedTx(ctx, func(tx *sql.Tx) error {
		if e := s.repo.LockEmployee(ctx, tx, id); e != nil {
			return toAppErr(e, "karyawan")
		}
		before, e := s.repo.GetEmployeeByID(ctx, tx, id, domain.PII{EncKey: s.enc, HMACKey: s.hmac})
		if e != nil {
			return toAppErr(e, "karyawan")
		}
		if before.Version != ifMatch {
			return toAppErr(domain.ErrVersionMismatch, "karyawan")
		}
		if e = s.checkTaxStatus(ctx, tx, c.TaxID); e != nil {
			return e
		}
		hist, e := s.repo.CorrectTaxStatus(ctx, tx, id, c.TaxID, c.ValidFrom, c.Reason)
		if e != nil {
			return taxCorrectionErr(e, before)
		}
		if out, e = s.repo.GetEmployeeByID(ctx, tx, id, s.pii(cl)); e != nil {
			return toAppErr(e, "karyawan")
		}
		return s.audit.Write(ctx, tx, audit.Entry{CompanyID: out.CompanyID, Division: out.Division, Action: actTaxCorrect,
			Entity: entEmployee, EntityKey: strconv.FormatInt(id, 10),
			Before: map[string]any{"taxid": before.TaxID, "changes": hist}, // keys from the audit allowlist (mask.go)
			After:  map[string]string{"taxid": out.TaxID, "validfrom": c.ValidFrom, "reason": c.Reason}, IP: cl.IP})
	})
	return out, err
}

func taxCorrectionErr(err error, emp domain.Employee) error {
	if errors.Is(err, domain.ErrTaxYearFinal) {
		return apperr.New(apperr.CodePeriodClosed, "tahun pajak sudah final (periode Desember/resign APPROVED atau CLOSED); status pajak tidak boleh dikoreksi",
			map[string]string{"division": emp.Division})
	}
	return toAppErr(err, "karyawan")
}
