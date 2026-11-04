package recon

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/run"
	"github.com/mutugading/goapps-backend/services/payroll/internal/engine"
)

// PGSource reads the LEGACY run and the legacy snapshot from Postgres inside a scoped transaction (D-41).
type PGSource struct {
	DB    run.TxRunner
	Rules run.RulesFunc
	// FormulaOf derives the formula lookup of the loaded rule set; nil = group by component key.
	FormulaOf func(engine.RuleSet) FormulaFunc
}

// Candidates implements Source.
func (s PGSource) Candidates(ctx context.Context, q SourceQuery) ([]Candidate, error) {
	rq := run.Query{CompanyID: q.CompanyID, Division: q.Division, YYMM: q.YYMM}
	var out []Candidate
	err := s.DB.ScopedTx(ctx, func(tx *sql.Tx) error {
		runID, err := legacyRun(ctx, tx, rq)
		if err != nil {
			return err
		}
		rules, err := s.Rules(ctx, tx, rq, q.RuleSetID)
		if err != nil {
			return fmt.Errorf("load rules: %w", err)
		}
		loaded, err := run.NewLoader().Load(ctx, tx, rq, rules)
		if err != nil {
			return fmt.Errorf("load inputs: %w", err)
		}
		lines, err := legacyLines(ctx, tx, runID)
		if err != nil {
			return err
		}
		heads, err := legacyHeads(ctx, tx, runID)
		if err != nil {
			return err
		}
		formulaOf := FormulaFunc(func(engine.CompKey) string { return "" })
		if s.FormulaOf != nil {
			formulaOf = s.FormulaOf(rules.Set)
		}
		out = buildCandidates(q.Division, loaded.Inputs, lines, heads, formulaOf)
		return nil
	})
	return out, err
}

func legacyRun(ctx context.Context, tx *sql.Tx, q run.Query) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx, `SELECT pr_id FROM payroll.pyr_run
WHERE pr_company_id = $1 AND pr_yymm = $2 AND pr_division = $3 AND pr_kind = 'LEGACY' AND pr_status = 'ACTIVE'
  AND pr_parent_run_id IS NULL`, q.CompanyID, q.YYMM, q.Division).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("%w: %s %d", ErrNoLegacyRun, q.Division, q.YYMM)
	}
	if err != nil {
		return 0, fmt.Errorf("find legacy run: %w", err)
	}
	return id, nil
}

func closeRows(rows *sql.Rows) {
	if err := rows.Close(); err != nil {
		log.Warn().Err(err).Msg("failed to close rows")
	}
}

func legacyLines(ctx context.Context, tx *sql.Tx, runID int64) (map[string][]ExpectedLine, error) {
	rows, err := tx.QueryContext(ctx, `SELECT psd_nik, psd_type::text, psd_id, psd_amount::text
FROM payroll.pyr_salary_detail WHERE psd_run_id = $1 ORDER BY psd_nik, psd_type, psd_id`, runID)
	if err != nil {
		return nil, fmt.Errorf("load legacy detail: %w", err)
	}
	defer closeRows(rows)
	out := map[string][]ExpectedLine{}
	for rows.Next() {
		var nik, typ string
		var l ExpectedLine
		var amt sql.NullString
		if err := rows.Scan(&nik, &typ, &l.ID, &amt); err != nil {
			return nil, fmt.Errorf("scan legacy detail: %w", err)
		}
		l.Type = "T"
		if typ == "Potongan" || typ == "P" {
			l.Type = "P"
		}
		if amt.Valid {
			l.Amount = &amt.String
		}
		out[nik] = append(out[nik], l)
	}
	return out, rows.Err()
}

func legacyHeads(ctx context.Context, tx *sql.Tx, runID int64) (map[string]*ExpectedHead, error) {
	rows, err := tx.QueryContext(ctx, `SELECT psh_nik, psh_tot_tunjangan::text, psh_tot_potongan::text, psh_rounding::text
FROM payroll.pyr_salary_head WHERE psh_run_id = $1`, runID)
	if err != nil {
		return nil, fmt.Errorf("load legacy head: %w", err)
	}
	defer closeRows(rows)
	out := map[string]*ExpectedHead{}
	for rows.Next() {
		var nik string
		var h ExpectedHead
		if err := rows.Scan(&nik, &h.TotTunjangan, &h.TotPotongan, &h.Rounding); err != nil {
			return nil, fmt.Errorf("scan legacy head: %w", err)
		}
		out[nik] = &h
	}
	return out, rows.Err()
}

// buildCandidates pairs every input with its legacy lines; a NIK without a legacy result is left out.
func buildCandidates(div string, inputs []engine.Input, lines map[string][]ExpectedLine,
	heads map[string]*ExpectedHead, formulaOf FormulaFunc) []Candidate {
	out := make([]Candidate, 0, len(inputs))
	for _, in := range inputs {
		ls, ok := lines[in.Emp.NIK]
		if !ok {
			continue
		}
		c := Candidate{Division: div, Input: in, Expected: Expected{Lines: ls, Head: heads[in.Emp.NIK]},
			Formulas: map[string]bool{}}
		for _, l := range ls {
			key := engine.CompKey{Type: engine.CompType(l.Type), ID: l.ID}
			f := formulaOf(key)
			if f == "" {
				f = key.String()
			}
			c.Formulas[f] = true
		}
		out = append(out, c)
	}
	return out
}
