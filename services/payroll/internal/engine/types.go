package engine

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/shopspring/decimal"
)

// CompType is the component type: T (tunjangan/allowance) or P (potongan/deduction).
type CompType string

// Component types.
const (
	CompT CompType = "T"
	CompP CompType = "P"
)

// Valid reports whether t is a known component type.
func (t CompType) Valid() bool { return t == CompT || t == CompP }

// CompKey identifies a component by legacy (type, id).
type CompKey struct {
	Type CompType
	ID   int
}

// String renders the key as e.g. "T001".
func (k CompKey) String() string { return fmt.Sprintf("%s%03d", k.Type, k.ID) }

// Less orders keys T before P, then by id (06§9).
func (k CompKey) Less(o CompKey) bool {
	if k.Type != o.Type {
		return k.Type == CompT
	}
	return k.ID < o.ID
}

// Period is a payroll period. Start is the 26th of the previous month, End the 25th.
type Period struct {
	YYMM  int // e.g. 202610
	Start time.Time
	End   time.Time
}

// Year returns the calendar year of the period.
func (p Period) Year() int { return p.YYMM / 100 }

// Month returns the calendar month (1..12) of the period.
func (p Period) Month() int { return p.YYMM % 100 }

// IsJanuary reports whether the period is January.
func (p Period) IsJanuary() bool { return p.Month() == 1 }

// IsDecember reports whether the period is December (annual PPh reconciliation).
func (p Period) IsDecember() bool { return p.Month() == 12 }

// Employee holds the employee attributes the engine needs.
type Employee struct {
	NIK       string
	Name      string
	Level     string // e.g. executive / staff / operator, as mapped in the rule set
	TaxStatus string // PTKP code, e.g. "K/1"
	Dept      string // pe_dept_id, matched by pyr_subsidy_rule.psr_depts
	HasNPWP   bool
	BirthDate *time.Time
	JoinDate  *time.Time // pe_date_join; decides the first full month of the P0 basis (D-30)
	GapokSts  string     // F (full) / D (prorate) for the base salary
}

// EmpComponent is an employee component amount (COMPNT_EMP, frz=2 & sts_payroll=2).
type EmpComponent struct {
	Key    CompKey
	Amount decimal.Decimal
	Sts    string // 'F' full / 'D' prorate
	Tax    bool
	// TaxPaidCompany is pcm_tax_paid_company = Y: PPh on it is borne by the company (P5, 06§6.3).
	TaxPaidCompany bool
	// TaxSubsidy is pcm_tax_subsidy = Y: the component forms the PPh subsidy basis (P0, 06§6.4).
	TaxSubsidy bool
	// SubsidyOT marks an overtime component (provisional v2 basis leaves it out, D-30); the id list
	// in isOTSubsidyID applies when the master does not set it.
	SubsidyOT bool
}

// AttRekap is a frozen attendance recap row (MPAR_* columns), keyed by column name
// in lower case, e.g. "day_work", "pay_tot", "jam_lembur_n1".
type AttRekap map[string]decimal.Decimal

// Get returns the column value, or zero when absent (legacy NVL(x, 0)).
func (r AttRekap) Get(col string) decimal.Decimal {
	if v, ok := r[col]; ok {
		return v
	}
	return decimal.Zero
}

// AttRekapAdd is the additional recap (day_work_add, pay_tot_add, shift_add).
type AttRekapAdd = AttRekap

// SchRekap is the schedule recap (pyr_stg_sch_rekap) keyed by schedule code, e.g. "NW", "NWS".
type SchRekap = AttRekap

// Addition is one pyr_addition amount for a component.
type Addition struct {
	Amount decimal.Decimal
	Remark string
	Source string // pa_source: MANUAL / UPLOAD / LOAN / MIGRASI (LOAN_DOUBLE, 15§2.4)
}

// LoanDue is a loan installment due in this period (15§2.4).
type LoanDue struct {
	LoanID    int64
	Seq       int
	Principal decimal.Decimal
	Interest  decimal.Decimal
	Remark    string // e.g. "Cicilan Pinjaman LN/SOLO/2026/0001 Ke 3 Dari 10"
}

// DailyFails holds counts derived from daily attendance (05§7).
type DailyFails struct {
	PremiFail int
	BerasFail int
}

// YTDTax is the year-to-date tax basis, January up to the previous period, aggregated by the
// loader per (company, tax year, npwp_hash) (13§3.3). Legacy December sources (D-29):
//   - Gross: Σ detail tax=2 with tax_paid_company=N except no 75, minus Σ P4 and P8, plus
//     THR/Bonus Tunjangan of earlier months.
//   - Deduct: Σ P14, P18, P22, P23 (JHT/pension, incl. BPJS Naker debt).
//   - TaxPaid: Σ P1 + THR PPh (TD_NO 1) + Bonus PPh (TD_NO 1, 3) + P7 additions.
type YTDTax struct {
	Gross   decimal.Decimal
	Deduct  decimal.Decimal
	TaxPaid decimal.Decimal
	Months  int
}

// YTDCompanyTax is the December P5 basis January up to the previous period, aggregated by the
// loader like YTDTax. Legacy FNC_MGT_PAYROLL_PPH_COMPANY uses a composition of its own (D-31):
//   - Gross: Σ detail tax=2 (T and P, every id incl. 75 and company-paid rows).
//   - CompanyPaid: Σ detail tax=2 with tax_paid_company=Y.
//   - Deduct: Σ P4, P14, P18.
//   - ThrBonus: Σ THR and Bonus Tunjangan of the whole tax year, December included.
//   - P5Paid: Σ P5 (rule v2 subtracts it, 06§6.3).
type YTDCompanyTax struct {
	Gross       decimal.Decimal
	CompanyPaid decimal.Decimal
	Deduct      decimal.Decimal
	ThrBonus    decimal.Decimal
	P5Paid      decimal.Decimal
}

// TaxAdj is pyr_tax_adj of the period (legacy MGT_PAYROLL_TAX_ADJ, December only):
// Tunjangan rows add to the annual gross, Potongan rows to the annual deduction.
type TaxAdj struct {
	Earn   decimal.Decimal
	Deduct decimal.Decimal
}

// RuleSet is a versioned rule set (03§2.3). Implementations must be immutable.
type RuleSet interface {
	ID() int
	// LegacyCompat is true for rule v1 that reproduces legacy behavior (K3).
	LegacyCompat() bool
	// Constant returns a rule constant such as DAYS_MONTH or HOURS_MONTH.
	Constant(key string) (decimal.Decimal, bool)
}

// Input is everything Calculate needs for one employee. It carries no I/O handles.
type Input struct {
	Period     Period
	Division   string
	Emp        Employee
	Components []EmpComponent
	Att        AttRekap    // nil = no recap (D2); empty non-nil = recap with all columns zero
	AttAdd     AttRekapAdd // nil = no additional recap
	Sch        SchRekap    // nil = no schedule recap
	Workdays   *int
	Additions  map[CompKey][]Addition
	Loans      []LoanDue
	Daily      DailyFails
	YTD        YTDTax
	TaxAdj     TaxAdj
	Thr        ThrTax        // THR/bonus of the same tax month; zero outside THR months
	Tax        TaxTables     // EMPTAX/PAYROLLSETUP of the rule set (loaded with Rules, not queried)
	YTDCompany YTDCompanyTax // December P5 basis (D-31)
	Subsidy    Subsidy       // P0 rules, override and basis (12§4)
	Rules      RuleSet
}

// HasRekap reports whether the employee has an attendance recap. Legacy mode skips
// the header when it does not (D2). An empty but non-nil recap counts as present.
func (in Input) HasRekap() bool { return in.Att != nil }

// Input validation errors.
var (
	ErrNoRules          = errors.New("engine: input has no rule set")
	ErrBadPeriod        = errors.New("engine: invalid period")
	ErrNoNIK            = errors.New("engine: employee NIK is empty")
	ErrBadComponentType = errors.New("engine: invalid component type")
	ErrDupComponent     = errors.New("engine: duplicate component")
)

// Validate checks structural invariants of the input.
func (in Input) Validate() error {
	if in.Rules == nil {
		return ErrNoRules
	}
	if m := in.Period.Month(); in.Period.YYMM < 190001 || m < 1 || m > 12 {
		return fmt.Errorf("%w: yymm %d", ErrBadPeriod, in.Period.YYMM)
	}
	if !in.Period.Start.IsZero() && !in.Period.End.IsZero() && in.Period.End.Before(in.Period.Start) {
		return fmt.Errorf("%w: end before start", ErrBadPeriod)
	}
	if in.Emp.NIK == "" {
		return ErrNoNIK
	}
	seen := make(map[CompKey]struct{}, len(in.Components))
	for _, c := range in.Components {
		if !c.Key.Type.Valid() {
			return fmt.Errorf("%w: %q", ErrBadComponentType, c.Key.Type)
		}
		if _, dup := seen[c.Key]; dup {
			return fmt.Errorf("%w: %s", ErrDupComponent, c.Key)
		}
		seen[c.Key] = struct{}{}
	}
	return nil
}

// Line is one calculated component (pyr_salary_detail).
// Meta is a map, so Line is not comparable: compare fields, never Line values with ==.
type Line struct {
	Key    CompKey
	Amount *decimal.Decimal // already ROUND(0); nil only for legacy NULL results (D10)
	Base   decimal.Decimal  // calculation basis (audit)
	Qty    decimal.Decimal  // days / hours / shifts
	Rate   decimal.Decimal
	Tax    bool
	// TaxPaidCompany and TaxSubsidy are copied from the component (P5/P0 read them).
	TaxPaidCompany bool
	TaxSubsidy     bool
	Remark         string            // replaces legacy *_DESCR
	Meta           map[string]string // psd_meta, e.g. the subsidy rule applied (12§4)
	Formula        string            // formula code, e.g. "PRORATE_30"
}

// AmountOrZero returns the amount, treating NULL as zero (legacy NVL in totals).
func (l Line) AmountOrZero() decimal.Decimal {
	if l.Amount == nil {
		return decimal.Zero
	}
	return *l.Amount
}

// Header is the per-employee summary (pyr_salary_head, 06§7).
type Header struct {
	BasicPay          *decimal.Decimal // A(T,1); nil when the component is absent (legacy NULL)
	TunjJabatan       *decimal.Decimal // A(T,2)
	TunjRumah         *decimal.Decimal // A(T,3)
	TunjKesejahteraan *decimal.Decimal // A(T,7)
	TunjTransport     *decimal.Decimal // A(T,4)
	RatePerJam        *decimal.Decimal // unrounded; nil = legacy NULL; storage rounds to money_idr
	TotTunjangan      decimal.Decimal
	TotPotongan       decimal.Decimal
	Rounding          decimal.Decimal
	TakeHome          decimal.Decimal
	// JamLembur is recap hours × legacy multiplier, keyed lembur_1 … lembur_nasp_3 (psh_jam_lembur).
	JamLembur map[string]decimal.Decimal
	Remarks   string
}

// Result is the output of Calculate for one employee.
type Result struct {
	NIK      string
	Lines    []Line  // sorted by (type, id)
	Header   *Header // nil when legacy mode skips the header (D2)
	Warnings []string
	// Incomplete is true when some component was skipped or evaluated with the default
	// because its formula is not implemented yet. Callers must not persist such a result.
	Incomplete bool
}

// SortLines orders lines deterministically by (type, id) (06§9).
func SortLines(lines []Line) {
	sort.SliceStable(lines, func(i, j int) bool { return lines[i].Key.Less(lines[j].Key) })
}
