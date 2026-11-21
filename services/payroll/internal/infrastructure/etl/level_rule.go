package etl

import (
	"context"
	"database/sql"
	"fmt"
)

// LevelRuleRow mirrors a pyr_level_rule row (02§5.1, TRG_MGT_PAYROLL_EMP_MAST).
type LevelRuleRow struct {
	Level        string
	Group        string
	OtinworkRate string
	OtinworkSts  int
	ShiftadjSts  int
	TunjshiftSts int
	TunjshiftRt  string
	RateOTBasis  string // T1_7_55 | T1_2_3_4 | T1 (06§2: Executive sum T1-4, lainnya T1,7,55)
}

// LevelRules is the 02§5.1 table, one row per PE_LEVEL named in the legacy trigger.
// JAKARTA uses T1 as RATE_OT basis (06§2); that is per division, so it stays an engine concern.
func LevelRules() []LevelRuleRow {
	var out []LevelRuleRow
	add := func(grp, rate string, otSts, adjSts, tsSts int, tsRate, basis string, levels ...string) {
		for _, l := range levels {
			out = append(out, LevelRuleRow{l, grp, rate, otSts, adjSts, tsSts, tsRate, basis})
		}
	}
	add("SHIFT", "0.0877", 2, 1, 2, "2200", "T1_7_55", "Operator Kontrak Shift I,II,III", "Operator Shift I,II,III")
	add("GS", "0.075", 2, 2, 1, "2200", "T1_7_55", "Operator General Shift", "Operator PKWT GS Less 3 Month", "Operator Kontrak GS")
	add("DRIVER", "0.9017", 2, 1, 2, "2200", "T1_7_55", "Driver")
	add("DIRUMAHKAN", "0", 1, 1, 1, "2200", "T1_7_55", "Dirumahkan")
	add("STAFF", "0", 1, 1, 1, "3300", "T1_7_55", "Driver HO", "Staff HO", "Office Boy", "Solo Asmen", "Driver Bandung", "Staff Bandung", "OB Bandung")
	add("STAFF", "0", 1, 1, 1, "3300", "T1_2_3_4", "Executive")
	return out
}

// LoadLevelRules upserts LevelRules and returns the number of rows written.
func LoadLevelRules(ctx context.Context, tx *sql.Tx) (int, error) {
	rules := LevelRules()
	for _, r := range rules {
		_, err := tx.ExecContext(ctx, `
INSERT INTO payroll.pyr_level_rule (plr_level, plr_grp, plr_otinwork_rate, plr_otinwork_sts, plr_shiftadj_sts,
  plr_tunjshift_sts, plr_tunjshift_rate, plr_rate_ot_basis)
VALUES ($1,$2,$3::numeric,$4,$5,$6,$7::numeric,$8)
ON CONFLICT (plr_level) DO UPDATE SET plr_grp=EXCLUDED.plr_grp, plr_otinwork_rate=EXCLUDED.plr_otinwork_rate,
  plr_otinwork_sts=EXCLUDED.plr_otinwork_sts, plr_shiftadj_sts=EXCLUDED.plr_shiftadj_sts,
  plr_tunjshift_sts=EXCLUDED.plr_tunjshift_sts, plr_tunjshift_rate=EXCLUDED.plr_tunjshift_rate,
  plr_rate_ot_basis=EXCLUDED.plr_rate_ot_basis`,
			r.Level, r.Group, r.OtinworkRate, r.OtinworkSts, r.ShiftadjSts, r.TunjshiftSts, r.TunjshiftRt, r.RateOTBasis)
		if err != nil {
			return 0, fmt.Errorf("level_rule %q: %w", r.Level, err)
		}
	}
	return len(rules), nil
}
