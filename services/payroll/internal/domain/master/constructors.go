package master

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Payroll master entities are exported structs on purpose (D-79): they are read models for the repository and
// the gRPC mappers, and ScopedTx (*sql.Tx) is part of the repository contract. The entities that clients write
// through the API (currency, company, division) get a validating constructor so the invariants live in the domain
// and not only in proto validation or SQL CHECKs. Employee input is normalized in the application layer because it
// needs the caller's scope and the active rule set (D-79).

// ValidationError names the field that broke an invariant; the application layer maps it to apperr.CodeValidation.
type ValidationError struct {
	Field string
	Msg   string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Msg }

// IsValidation reports whether err is (or wraps) a ValidationError and returns it.
func IsValidation(err error) (*ValidationError, bool) {
	var ve *ValidationError
	if errors.As(err, &ve) {
		return ve, true
	}
	return nil, false
}

func invalid(field, format string, a ...any) error {
	return &ValidationError{Field: field, Msg: fmt.Sprintf(format, a...)}
}

var currencyCodeRE = regexp.MustCompile(`^[A-Z]{3}$`)

const (
	maxCompanyCode  = 10
	maxCompanyName  = 200
	maxDivisionCode = 30
	maxDivisionName = 100
	maxCompanyNPWP  = 30
	// DefaultCurrency is the base currency of a company that names none.
	DefaultCurrency = "IDR"
)

// NewCurrency validates a currency (pyr_currency): ISO 3-letter code, minor unit 0..4, take-home rounding -6..4.
func NewCurrency(code string, minorUnit, takehomeRound int) (Currency, error) {
	if !currencyCodeRE.MatchString(code) {
		return Currency{}, invalid("code", "code harus kode ISO 3 huruf kapital")
	}
	if minorUnit < 0 || minorUnit > 4 {
		return Currency{}, invalid("minor_unit", "minor_unit harus 0..4")
	}
	if takehomeRound < -6 || takehomeRound > 4 {
		return Currency{}, invalid("takehome_round", "takehome_round harus -6..4")
	}
	return Currency{Code: code, MinorUnit: minorUnit, TakehomeRound: takehomeRound}, nil
}

// NewCompany validates a company (pyr_company). An empty base currency means IDR. The NPWP is validated by the
// caller (it is PII, passed beside the entity, never inside it).
func NewCompany(code, name, baseCurrency string) (Company, error) {
	code, name = strings.TrimSpace(code), strings.TrimSpace(name)
	if code == "" || len(code) > maxCompanyCode {
		return Company{}, invalid("code", "code wajib, maksimal %d karakter", maxCompanyCode)
	}
	if name == "" || len(name) > maxCompanyName {
		return Company{}, invalid("name", "name wajib, maksimal %d karakter", maxCompanyName)
	}
	if baseCurrency == "" {
		baseCurrency = DefaultCurrency
	}
	if !currencyCodeRE.MatchString(baseCurrency) {
		return Company{}, invalid("base_currency", "base_currency harus kode ISO 3 huruf kapital")
	}
	return Company{Code: code, Name: name, BaseCurrency: baseCurrency, Active: true}, nil
}

// ValidateCompanyNPWP rejects an over-long NPWP and the mask a client echoes back after reading masked PII (D-17).
func ValidateCompanyNPWP(npwp string) error {
	if npwp == MaskedValue {
		return invalid("npwp", "npwp tidak boleh berisi nilai mask")
	}
	if len(npwp) > maxCompanyNPWP {
		return invalid("npwp", "npwp maksimal %d karakter", maxCompanyNPWP)
	}
	return nil
}

// NewDivision validates a division (pyr_division_ref).
func NewDivision(companyID int16, code, name string) (Division, error) {
	code, name = strings.TrimSpace(code), strings.TrimSpace(name)
	if companyID <= 0 {
		return Division{}, invalid("company_id", "company_id harus > 0")
	}
	if code == "" || len(code) > maxDivisionCode {
		return Division{}, invalid("code", "code wajib, maksimal %d karakter", maxDivisionCode)
	}
	if len(name) > maxDivisionName {
		return Division{}, invalid("name", "name maksimal %d karakter", maxDivisionName)
	}
	return Division{CompanyID: companyID, Code: code, Name: name}, nil
}
