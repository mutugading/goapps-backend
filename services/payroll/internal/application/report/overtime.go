package report

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

// SheetOvertime is the F-2 `OT payroll` sheet (SALARY003, 11§2.2).
const SheetOvertime = "OT payroll"

// otTypes fixes the column order: psh_jam_lembur key, label, and the multiplier that turned actual hours into paid hours.
var otTypes = []struct {
	Key, Label string
	Mult       int64 // lembur1 is 1.5 and Executive NAS is ×2: see overtimeMult
	NAS        bool
}{
	{"lembur1", "Biasa 1", 0, false}, {"lembur2", "Biasa 2", 2, false},
	{"nas1", "Libnas 1", 2, true}, {"nas2", "Libnas 2", 3, true}, {"nas3", "Libnas 3", 4, true},
	{"off1", "Off 1", 2, false}, {"off2", "Off 2", 3, false}, {"off3", "Off 3", 4, false},
	{"nasp1", "Libnas Pendek 1", 2, false}, {"nasp2", "Libnas Pendek 2", 3, false}, {"nasp3", "Libnas Pendek 3", 4, false},
}

// overtimeMult is the legacy multiplier of an OT column; Executive NAS columns are all ×2 (D11).
func overtimeMult(i int, executive bool) decimal.Decimal {
	t := otTypes[i]
	switch {
	case i == 0:
		return decimal.RequireFromString("1.5")
	case t.NAS && executive:
		return decimal.NewFromInt(2)
	}
	return decimal.NewFromInt(t.Mult)
}

// OTRow is one NIK of the overtime report: paid hours as frozen in psh_jam_lembur, actual hours = paid ÷ multiplier,
// rupiah = round(paid × rate) per column (as the slip).
type OTRow struct {
	NIK, Nama, Dept, Level string
	Rate                   decimal.Decimal
	Paid, Actual, Rupiah   [11]decimal.Decimal
}

// TotalPaid sums the paid hours of the 11 columns.
func (r OTRow) TotalPaid() decimal.Decimal { return sumDec(r.Paid[:]) }

// TotalActual sums the actual hours of the 11 columns.
func (r OTRow) TotalActual() decimal.Decimal { return sumDec(r.Actual[:]) }

// TotalRupiah sums the rupiah of the 11 columns.
func (r OTRow) TotalRupiah() decimal.Decimal { return sumDec(r.Rupiah[:]) }

func sumDec(v []decimal.Decimal) decimal.Decimal {
	t := decimal.Zero
	for _, x := range v {
		t = t.Add(x)
	}
	return t
}

// BuildOTRow derives actual hours and rupiah from the frozen paid hours.
func BuildOTRow(nik, nama, dept, level string, rate decimal.Decimal, jam map[string]decimal.Decimal) OTRow {
	r := OTRow{NIK: nik, Nama: nama, Dept: dept, Level: level, Rate: rate}
	exec := strings.EqualFold(strings.TrimSpace(level), "executive")
	for i, t := range otTypes {
		p := jam[t.Key]
		r.Paid[i] = p
		r.Actual[i] = p.Div(overtimeMult(i, exec)).Round(2)
		r.Rupiah[i] = roundMoney(p.Mul(rate))
	}
	return r
}

func loadOvertime(ctx context.Context, tx *sql.Tx, q Query, runID int64) ([]OTRow, error) {
	rows, err := tx.QueryContext(ctx, `SELECT h.psh_nik, COALESCE(h.psh_nama,''), COALESCE(h.psh_dept,''), COALESCE(e.pe_level,''),
  COALESCE(h.psh_rate_perjam::text,'0'), h.psh_jam_lembur::text
FROM payroll.pyr_salary_head h
LEFT JOIN payroll.pyr_employee e ON e.pe_company_id = $2 AND e.pe_nik = h.psh_nik AND e.pe_division = h.psh_division
WHERE h.psh_run_id = $1 AND h.psh_jam_lembur IS NOT NULL
ORDER BY COALESCE(h.psh_dept,''), h.psh_nik`, runID, q.CompanyID)
	if err != nil {
		return nil, fmt.Errorf("read overtime: %w", err)
	}
	defer closeRows(rows)
	var out []OTRow
	for rows.Next() {
		var nik, nama, dept, level, rate, raw string
		if err := rows.Scan(&nik, &nama, &dept, &level, &rate, &raw); err != nil {
			return nil, fmt.Errorf("scan overtime: %w", err)
		}
		rt, err := decimal.NewFromString(rate)
		if err != nil {
			return nil, fmt.Errorf("rate %s: %w", nik, err)
		}
		jam, err := parseJam(raw)
		if err != nil {
			return nil, fmt.Errorf("jam_lembur %s: %w", nik, err)
		}
		r := BuildOTRow(nik, nama, dept, level, rt, jam)
		if r.TotalPaid().IsZero() {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *sheetWriter) overtime(d RecoData) {
	s.start(SheetOvertime)
	s.put("Employee Overtime Report")
	s.put("Processing Month " + fmt.Sprint(d.YYMM))
	s.put()
	h := make([]any, 0, 5+2*len(otTypes)+2+len(otTypes)+1)
	h = append(h, "No.", hdrNIK, hdrNama, hdrDept, "Rate / Jam")
	for _, t := range otTypes {
		h = append(h, "Jam Actual "+t.Label, "Jam Paid "+t.Label)
	}
	h = append(h, "Total Jam Actual", "Total Jam Paid")
	for _, t := range otTypes {
		h = append(h, "Rp "+t.Label)
	}
	h = append(h, "Total Rp")
	s.put(h...)
	for i, r := range d.Overtime {
		v := make([]any, 0, 5+2*len(otTypes)+2+len(otTypes)+1)
		v = append(v, i+1, r.NIK, r.Nama, optStr(r.Dept), r.Rate)
		for j := range otTypes {
			v = append(v, nz(r.Actual[j]), nz(r.Paid[j]))
		}
		v = append(v, r.TotalActual(), r.TotalPaid())
		for j := range otTypes {
			v = append(v, nz(r.Rupiah[j]))
		}
		v = append(v, r.TotalRupiah())
		s.put(v...)
	}
}
