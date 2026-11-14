package etl

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/shopspring/decimal"
)

// TableFxRate is the Orion exchange-rate table read by SALARY001 for the GL journal (11§4.4, D-63).
const TableFxRate = "FM_EXCHANGE_RATE"

// fxMaxScale is the scale of pyr_fx_rate.pfr_rate (migration 000043); a rate with more decimals is rejected, never cut.
const fxMaxScale = 12

// FxRateOptions controls the BOOK rate load. Only IDR→USD rate type 'B' (BOOK) is loaded, which is what SALARY001
// reads (CER_EXG_RATE_TYPE='B', IDR→USD). ExpectRows = 0 means unchecked; it counts the selected IDR→USD BOOK rows.
type FxRateOptions struct {
	ExpectRows int
}

// RunFxRate loads FM_EXCHANGE_RATE (IDR→USD, BOOK) into pyr_fx_rate in ONE transaction (D-63 kurs).
// rate_date = CER_EFF_FRM_DT; source = UPLOAD_ORION. Rows that are not a positive decimal with at most 12 decimals or
// have no valid date are reported as Exceptions. Months with more than one BOOK rate are Warnings: the legacy lookup
// by month (TO_CHAR(CER_EFF_FRM_DT,'RRRRMM')) returns 0 for them. Idempotent: upsert on the primary key.
func RunFxRate(ctx context.Context, db *sql.DB, src Source, o FxRateOptions) (TableReport, error) {
	t, err := src.Read(TableFxRate)
	if err != nil {
		return TableReport{}, err
	}
	rep := TableReport{Table: t.Name, SrcRows: len(t.Rows), Checksum: t.Checksum()}
	ix, err := bindCols(t, []string{"CER_EFF_FRM_DT", "CER_CONV_FM_CURR_CODE", "CER_CONV_TO_CURR_CODE", "CER_EXG_RATE_TYPE", "CER_EXG_RATE"})
	if err != nil {
		return rep, err
	}
	rows, selected, perMonth := parseFxRows(t, ix, &rep)
	if o.ExpectRows > 0 && selected != o.ExpectRows {
		return rep, fmt.Errorf("%s: %d baris IDR→USD BOOK, diharapkan %d", TableFxRate, selected, o.ExpectRows)
	}
	months := make([]string, 0, len(perMonth))
	for m, n := range perMonth {
		if n > 1 {
			months = append(months, m)
		}
	}
	sort.Strings(months)
	for _, m := range months {
		rep.Warnings = append(rep.Warnings, fmt.Sprintf("%s: %d kurs BOOK dalam satu bulan (legacy SALARY001 memakai 0)", m, perMonth[m]))
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return rep, err
	}
	defer func() { _ = tx.Rollback() }() //nolint:errcheck // no-op after Commit
	for _, r := range rows {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO payroll.pyr_fx_rate (pfr_rate_date, pfr_from_ccy, pfr_to_ccy, pfr_rate_type, pfr_rate, pfr_source, created_by)
VALUES ($1::date,'IDR','USD','BOOK',$2::numeric,'UPLOAD_ORION','etl:fx-rate')
ON CONFLICT (pfr_rate_date, pfr_from_ccy, pfr_to_ccy, pfr_rate_type)
DO UPDATE SET pfr_rate=EXCLUDED.pfr_rate, pfr_source=EXCLUDED.pfr_source`, r.date, r.rate.String()); err != nil {
			return rep, fmt.Errorf("kurs %s: %w", r.date, err)
		}
	}
	rep.DstRows = len(rows)
	if err := tx.Commit(); err != nil {
		return rep, err
	}
	return rep, nil
}

type fxRow struct {
	date string
	rate decimal.Decimal
}

// parseFxRows selects IDR→USD BOOK rows, validates them (bad ones become Exceptions) and counts rates per month.
func parseFxRows(t *Table, ix map[string]int, rep *TableReport) (rows []fxRow, selected int, perMonth map[string]int) {
	rows = make([]fxRow, 0, len(t.Rows))
	perMonth = map[string]int{}
	for i, r := range t.Rows {
		c := func(n string) string { return strings.TrimSpace(r[ix[n]]) }
		if !strings.EqualFold(c("CER_CONV_FM_CURR_CODE"), "IDR") || !strings.EqualFold(c("CER_CONV_TO_CURR_CODE"), "USD") ||
			!strings.EqualFold(c("CER_EXG_RATE_TYPE"), "B") {
			continue
		}
		selected++
		d, derr := parseDate(c("CER_EFF_FRM_DT"))
		if derr != nil || d == "" {
			rep.Exceptions = append(rep.Exceptions, fmt.Sprintf("%s baris %d: CER_EFF_FRM_DT %q tidak valid", t.Name, i+2, c("CER_EFF_FRM_DT")))
			continue
		}
		rate, rerr := decimal.NewFromString(c("CER_EXG_RATE"))
		if rerr != nil || !rate.IsPositive() || -rate.Exponent() > fxMaxScale {
			rep.Exceptions = append(rep.Exceptions, fmt.Sprintf("%s baris %d: CER_EXG_RATE %q bukan desimal positif ≤ %d desimal", t.Name, i+2, c("CER_EXG_RATE"), fxMaxScale))
			continue
		}
		rows = append(rows, fxRow{d, rate})
		perMonth[d[:7]]++
	}
	return rows, selected, perMonth
}
