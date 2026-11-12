package recon

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"

	"github.com/shopspring/decimal"
)

// Fields of ATT_DERIVED (05§5 option A): the recap column of VIEW_ATT_REKAP_HM and the pasr_data key of the value
// payroll derives itself (written by etl.RunAttSnapshot, keys kept in sync by tests/etl TestAttDerived).
const (
	DerivedShift = "shift"
	DerivedOtiw  = "otiw_decr"
)

var derivedKeys = []struct{ field, mpar, derived string }{
	{DerivedShift, "MPAR_SHIFT", "derived_shift"},
	{DerivedOtiw, "MPAR_OTIW_DECR", "derived_otiw_decr"},
}

// AttDerivedRow is one derived value next to the HRIS view value of one NIK. NULL counts as 0 (the view sums).
type AttDerivedRow struct {
	NIK     string
	Field   string // DerivedShift | DerivedOtiw
	MPAR    *decimal.Decimal
	Derived *decimal.Decimal
}

// AttDerivedDiff is an ATT_DERIVED finding: the derived value differs from MPAR_*. It is reported next to the line
// diffs, not as a line: engine v1 calculates with MPAR_* (paritas), so the difference never moves an amount, and
// it does not fail the v1 parity criteria (09§4). It must be zero before the derived values replace MPAR_* (05§5).
type AttDerivedDiff struct {
	NIK      string           `json:"nik"`
	Field    string           `json:"field"`
	MPAR     *decimal.Decimal `json:"mpar"`
	Derived  *decimal.Decimal `json:"derived"`
	Delta    decimal.Decimal  `json:"delta"` // derived − mpar
	Excluded string           `json:"excluded,omitempty"`
}

// addDerived records the ATT_DERIVED differences; excluded NIKs (D-47) are listed but not counted.
func (rep *DiffReport) addDerived(rows []AttDerivedRow, excluded map[string]string) {
	for _, r := range rows {
		delta := orZero(r.Derived).Sub(orZero(r.MPAR))
		if delta.IsZero() {
			continue
		}
		d := AttDerivedDiff{NIK: r.NIK, Field: r.Field, MPAR: r.MPAR, Derived: r.Derived, Delta: delta, Excluded: excluded[r.NIK]}
		if d.Excluded == "" {
			rep.Summary.Categories[CatAttDerived]++
		}
		rep.AttDerived = append(rep.AttDerived, d)
	}
	sort.SliceStable(rep.AttDerived, func(i, j int) bool {
		a, b := rep.AttDerived[i], rep.AttDerived[j]
		if a.NIK != b.NIK {
			return a.NIK < b.NIK
		}
		return a.Field < b.Field
	})
}

// LoadAttDerived reads the derived values next to MPAR_* from the newest synced (non-LEGACY) snapshot of the period:
// both come from the same sync, so the difference is the eligibility/formula only (05§5). Returns that snapshot
// id, 0 when the period has no synced snapshot with derived values (ATT_DERIVED then stays 0). NIKs without
// derived keys (no daily rows, D-60) are not checked. The newest synced snapshot is used whatever the target run
// was calculated from: MPAR and derived come from the same sync, and DerivedSnapshotID names it in the report.
func LoadAttDerived(ctx context.Context, tx *sql.Tx, companyID int16, division string, yymm int) (int64, []AttDerivedRow, error) {
	var snap int64
	err := tx.QueryRowContext(ctx, `SELECT s.pas_snapshot_id FROM payroll.pyr_att_snapshot s
WHERE s.pas_company_id = $1 AND s.pas_division = $2 AND s.pas_yymm = $3 AND s.pas_source_batch_ids <> '{}'
  AND EXISTS (SELECT 1 FROM payroll.pyr_att_snapshot_row r WHERE r.pasr_snapshot_id = s.pas_snapshot_id
              AND r.pasr_kind = 'REKAP' AND r.pasr_data ? 'derived_shift')
ORDER BY s.pas_snapshot_id DESC LIMIT 1`, companyID, division, yymm).Scan(&snap)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil, nil
	}
	if err != nil {
		return 0, nil, fmt.Errorf("derived snapshot: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `SELECT pasr_nik, pasr_data->>'MPAR_SHIFT', pasr_data->>'derived_shift',
  pasr_data->>'MPAR_OTIW_DECR', pasr_data->>'derived_otiw_decr'
FROM payroll.pyr_att_snapshot_row WHERE pasr_snapshot_id = $1 AND pasr_kind = 'REKAP' AND pasr_data ? 'derived_shift'
ORDER BY pasr_nik`, snap)
	if err != nil {
		return 0, nil, fmt.Errorf("derived att: %w", err)
	}
	defer closeRows(rows)
	var out []AttDerivedRow
	for rows.Next() {
		var nik string
		vals := make([]sql.NullString, 2*len(derivedKeys))
		if err := rows.Scan(&nik, &vals[0], &vals[1], &vals[2], &vals[3]); err != nil {
			return 0, nil, fmt.Errorf("scan derived att: %w", err)
		}
		for i, k := range derivedKeys {
			m, err := nullDec(vals[2*i])
			if err != nil {
				return 0, nil, fmt.Errorf("%s %s: %w", nik, k.mpar, err)
			}
			d, err := nullDec(vals[2*i+1])
			if err != nil {
				return 0, nil, fmt.Errorf("%s %s: %w", nik, k.derived, err)
			}
			out = append(out, AttDerivedRow{NIK: nik, Field: k.field, MPAR: m, Derived: d})
		}
	}
	return snap, out, rows.Err()
}

func nullDec(s sql.NullString) (*decimal.Decimal, error) {
	if !s.Valid {
		return nil, nil //nolint:nilnil // NULL is a valid value
	}
	d, err := decimal.NewFromString(s.String)
	if err != nil {
		return nil, err
	}
	return &d, nil
}
