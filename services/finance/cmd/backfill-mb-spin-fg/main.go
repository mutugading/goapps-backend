// Command backfill-mb-spin-fg links legacy live MB spins to their FG POY product
// (mb-spin-fg-poy-link P6, design §6 + Revisi 3 "Backfill final").
//
// Modes:
//
//	(default) --dry-run [--out file.csv]
//	    Read-only. Classifies every live spin with the P-13 matcher and writes a CSV
//	    with action APPLY / REVIEW / SKIP (+ reason). Nothing is written to the DB.
//	--apply --input reviewed.csv [--out result.csv] [--actor name]
//	    Processes ONLY rows whose action is APPLY (reviewers turn REVIEW into APPLY
//	    by hand). One transaction per row; every condition is re-checked under row
//	    locks; an FG already set is never overwritten; reruns are idempotent.
//	--report
//	    Reserved for P6-T2 (spin-move report); not implemented here.
//
// The command never recalculates costs, never runs autofill/Relink and never
// touches cost_product_parameter.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strconv"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/config"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

const (
	cmdName      = "backfill-mb-spin-fg"
	defaultActor = cmdName
)

var errNotImplemented = errors.New("--report is not implemented yet (P6-T2)")

type options struct {
	dryRun bool
	apply  bool
	report bool
	input  string
	out    string
	actor  string
}

func main() {
	opts, err := parseFlags(os.Args[1:])
	if err != nil {
		log.Fatalf("%s: %v", cmdName, err)
	}
	if err := run(context.Background(), opts); err != nil {
		log.Fatalf("%s: %v", cmdName, err)
	}
}

func parseFlags(args []string) (options, error) {
	var o options
	fs := flag.NewFlagSet(cmdName, flag.ContinueOnError)
	fs.BoolVar(&o.dryRun, "dry-run", false, "classify only, write CSV, no DB writes (default mode)")
	fs.BoolVar(&o.apply, "apply", false, "apply APPLY rows of --input (one tx per row)")
	fs.BoolVar(&o.report, "report", false, "spin-move report (P6-T2, not implemented yet)")
	fs.StringVar(&o.input, "input", "", "reviewed CSV for --apply")
	fs.StringVar(&o.out, "out", "", "output CSV path (default: timestamped file in cwd)")
	fs.StringVar(&o.actor, "actor", defaultActor, "updated_by value for --apply")
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	return o, validateOptions(&o)
}

func validateOptions(o *options) error {
	modes := 0
	for _, m := range []bool{o.dryRun, o.apply, o.report} {
		if m {
			modes++
		}
	}
	if modes > 1 {
		return errors.New("--dry-run, --apply and --report are mutually exclusive")
	}
	if o.apply && o.input == "" {
		return errors.New("--apply requires --input reviewed.csv")
	}
	if !o.apply && !o.report {
		o.dryRun = true
	}
	if o.apply && o.actor == "" {
		return errors.New("--actor must not be empty")
	}
	return nil
}

func defaultOut(kind string) string {
	return fmt.Sprintf("%s-%s-%s.csv", cmdName, kind, time.Now().Format("20060102-150405"))
}

func run(ctx context.Context, o options) error {
	if o.report {
		return errNotImplemented
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	db, err := postgres.NewConnection(&cfg.Database)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer logClose("database", db)
	repo := postgres.NewMBSpinFgBackfillRepository(db)

	if o.apply {
		return runApply(ctx, repo, o)
	}
	return runDryRun(ctx, repo, o)
}

type candidateLister interface {
	ListBackfillCandidates(ctx context.Context) ([]mbspin.BackfillCandidate, error)
}

type rowApplier interface {
	ApplyBackfillRow(ctx context.Context, in mbspin.BackfillApplyRequest) (mbspin.BackfillApplyOutcome, error)
}

func runDryRun(ctx context.Context, repo candidateLister, o options) error {
	cands, err := repo.ListBackfillCandidates(ctx)
	if err != nil {
		return fmt.Errorf("list candidates: %w", err)
	}
	rows := mbspin.ClassifyBackfillAll(cands)
	out := o.out
	if out == "" {
		out = defaultOut("dryrun")
	}
	if err := writeFile(out, func(w io.Writer) error { return writeDryRunCSV(w, rows) }); err != nil {
		return err
	}
	log.Printf("dry-run: %d live spins classified -> %s (no DB writes)", len(rows), out)
	logCounts("bucket (P-13, before overrides)", mbspin.CountBackfillBuckets(rows))
	logCounts("action/reason", mbspin.CountBackfillRows(rows))
	return nil
}

func runApply(ctx context.Context, repo rowApplier, o options) error {
	f, err := os.Open(o.input) //nolint:gosec // operator-supplied reviewed CSV path for a one-shot CLI
	if err != nil {
		return fmt.Errorf("open input: %w", err)
	}
	in, err := parseApplyCSV(f)
	logClose("input", f)
	if err != nil {
		return fmt.Errorf("parse input: %w", err)
	}
	results, err := applyEntries(ctx, repo, in.Entries, o.actor)
	if err != nil {
		return err
	}
	out := o.out
	if out == "" {
		out = defaultOut("apply")
	}
	if err := writeFile(out, func(w io.Writer) error { return writeApplyResultCSV(w, results) }); err != nil {
		return err
	}
	log.Printf("apply: %d APPLY rows processed -> %s", len(results), out)
	logCounts("ignored (non-APPLY action)", in.Ignored)
	logCounts("result/reason", countResults(results))
	return nil
}

// applyEntries applies each entry in its own transaction (inside the repository).
// Malformed entries and per-row database errors become FAILED rows; only a
// cancelled context aborts the run.
func applyEntries(ctx context.Context, repo rowApplier, entries []applyEntry, actor string) ([]applyResultRow, error) {
	results := make([]applyResultRow, 0, len(entries))
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return results, fmt.Errorf("apply aborted: %w", err)
		}
		results = append(results, applyOne(ctx, repo, e, actor))
	}
	return results, nil
}

func applyOne(ctx context.Context, repo rowApplier, e applyEntry, actor string) applyResultRow {
	row := applyResultRow{SpinID: e.RawSpin, PoySys: e.RawPoy}
	if e.Err != nil {
		row.Outcome = mbspin.BackfillApplyOutcome{
			Result: mbspin.BackfillResultFailed, Reason: mbspin.BackfillReasonMalformedRow,
			Message: fmt.Sprintf("line %d: %v", e.Line, e.Err),
		}
		return row
	}
	row.SpinID, row.PoySys = e.SpinID, strconv.FormatInt(e.PoySys, 10)
	out, err := repo.ApplyBackfillRow(ctx, mbspin.BackfillApplyRequest{SpinID: e.SpinID, PoyProductSysID: e.PoySys, Actor: actor})
	if err != nil {
		log.Printf("line %d spin %s: FAILED: %v", e.Line, e.SpinID, err)
		out = mbspin.BackfillApplyOutcome{Result: mbspin.BackfillResultFailed, Reason: mbspin.BackfillReasonDBError, Message: err.Error()}
	}
	row.Outcome = out
	return row
}

func countResults(rows []applyResultRow) map[string]int {
	out := make(map[string]int)
	for _, r := range rows {
		out[string(r.Outcome.Result)+"/"+r.Outcome.Reason]++
	}
	return out
}

func logCounts(title string, counts map[string]int) {
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	log.Printf("summary %s:", title)
	for _, k := range keys {
		log.Printf("  %-55s %d", k, counts[k])
	}
}

func writeFile(path string, fn func(io.Writer) error) (err error) {
	f, err := os.Create(path) //nolint:gosec // operator-supplied output path for a one-shot CLI
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer func() {
		if cErr := f.Close(); cErr != nil && err == nil {
			err = fmt.Errorf("close %s: %w", path, cErr)
		}
	}()
	return fn(f)
}

type closer interface{ Close() error }

func logClose(name string, c closer) {
	if err := c.Close(); err != nil {
		log.Printf("WARN: failed to close %s: %v", name, err)
	}
}
