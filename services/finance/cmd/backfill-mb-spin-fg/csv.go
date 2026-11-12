package main

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

// Column names shared by the dry-run CSV and the reviewed input CSV.
const (
	colAction  = "action"
	colSpinID  = "mbs_id"
	colPoySys  = "poy_product_sys_id"
	boolTrue   = "true"
	boolFalse  = "false"
	actionMiss = "missing"
)

// dryRunHeader is the fixed dry-run CSV header (P6 context pack T1).
var dryRunHeader = []string{
	colAction, "reason", "bucket", colSpinID, "mbh_id", "is_root", "orion_item_code", "mgt_name",
	"shade_code", "final_product", "norm_spec", "n_spec_shade", "n_shade_only",
	colPoySys, "poy_product_code", "poy_product_name", "poy_erp_item_code", "conflict_spin_id",
}

// applyResultHeader is the fixed apply-result CSV header.
var applyResultHeader = []string{colSpinID, colPoySys, "result", "reason", "head_updated", "message"}

func boolStr(b bool) string {
	if b {
		return boolTrue
	}
	return boolFalse
}

func dryRunRecord(r mbspin.BackfillRow) []string {
	c := r.Candidate
	poyID, poyCode, poyName, poyItem := "", "", "", ""
	if r.Poy != nil {
		poyID = strconv.FormatInt(r.Poy.ProductSysID, 10)
		poyCode, poyName, poyItem = r.Poy.ProductCode, r.Poy.ProductName, r.Poy.ErpItemCode
	}
	return []string{
		string(r.Action), r.Reason, r.Bucket, c.SpinID, c.HeadID, boolStr(c.IsRoot), c.OrionItemCode, c.MgtName,
		c.ShadeCode, c.FinalProduct, c.NormSpec,
		strconv.FormatInt(c.NSpecShade, 10), strconv.FormatInt(c.NShadeOnly, 10),
		poyID, poyCode, poyName, poyItem, r.ConflictSpinID,
	}
}

// writeDryRunCSV writes the header and one record per classified row.
func writeDryRunCSV(w io.Writer, rows []mbspin.BackfillRow) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(dryRunHeader); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	for _, r := range rows {
		if err := cw.Write(dryRunRecord(r)); err != nil {
			return fmt.Errorf("write row %s: %w", r.Candidate.SpinID, err)
		}
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		return fmt.Errorf("flush csv: %w", err)
	}
	return nil
}

// applyEntry is one APPLY row of the reviewed CSV (Err set when malformed).
type applyEntry struct {
	Line    int
	SpinID  string
	PoySys  int64
	RawSpin string
	RawPoy  string
	Err     error
}

// applyInput is the parsed reviewed CSV.
type applyInput struct {
	Entries []applyEntry
	// Ignored counts non-APPLY rows by their (trimmed, upper-cased) action.
	Ignored map[string]int
}

// parseApplyCSV reads the reviewed CSV. Required columns: action, mbs_id,
// poy_product_sys_id (any order, extra columns ignored). Only rows whose action
// is exactly APPLY after TRIM+UPPER become entries; a malformed APPLY row is kept
// with Err set so the caller reports it as FAILED and continues. A bad header or
// unreadable file is a fatal error.
func parseApplyCSV(r io.Reader) (applyInput, error) {
	in := applyInput{Ignored: map[string]int{}}
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	header, err := cr.Read()
	if err != nil {
		return in, fmt.Errorf("read header: %w", err)
	}
	idx, err := headerIndex(header)
	if err != nil {
		return in, err
	}
	line := 1
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			return in, nil
		}
		line++
		if err != nil {
			var pe *csv.ParseError
			if errors.As(err, &pe) {
				in.Entries = append(in.Entries, applyEntry{Line: line, Err: fmt.Errorf("unparseable csv row: %w", err)})
				continue
			}
			return in, fmt.Errorf("read line %d: %w", line, err)
		}
		action := strings.ToUpper(strings.TrimSpace(field(rec, idx[colAction])))
		if action != string(mbspin.BackfillActionApply) {
			if action == "" {
				action = actionMiss
			}
			in.Ignored[action]++
			continue
		}
		in.Entries = append(in.Entries, parseApplyEntry(line, field(rec, idx[colSpinID]), field(rec, idx[colPoySys])))
	}
}

func headerIndex(header []string) (map[string]int, error) {
	idx := make(map[string]int, len(header))
	for i, h := range header {
		name := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(h, "\uFEFF")))
		if _, dup := idx[name]; !dup {
			idx[name] = i
		}
	}
	for _, req := range []string{colAction, colSpinID, colPoySys} {
		if _, ok := idx[req]; !ok {
			return nil, fmt.Errorf("input csv: missing required column %q", req)
		}
	}
	return idx, nil
}

func field(rec []string, i int) string {
	if i < 0 || i >= len(rec) {
		return ""
	}
	return rec[i]
}

func parseApplyEntry(line int, rawSpin, rawPoy string) applyEntry {
	e := applyEntry{Line: line, RawSpin: strings.TrimSpace(rawSpin), RawPoy: strings.TrimSpace(rawPoy)}
	id, err := uuid.Parse(e.RawSpin)
	if err != nil {
		e.Err = fmt.Errorf("invalid mbs_id %q", e.RawSpin)
		return e
	}
	e.SpinID = id.String()
	poy, err := strconv.ParseInt(e.RawPoy, 10, 64)
	if err != nil || poy <= 0 {
		e.Err = fmt.Errorf("invalid poy_product_sys_id %q", e.RawPoy)
		return e
	}
	e.PoySys = poy
	return e
}

// applyResultRow is one line of the apply-result CSV.
type applyResultRow struct {
	SpinID  string
	PoySys  string
	Outcome mbspin.BackfillApplyOutcome
}

func applyResultRecord(r applyResultRow) []string {
	return []string{r.SpinID, r.PoySys, string(r.Outcome.Result), r.Outcome.Reason, boolStr(r.Outcome.HeadUpdated), r.Outcome.Message}
}

// writeApplyResultCSV writes the apply-result CSV.
func writeApplyResultCSV(w io.Writer, rows []applyResultRow) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(applyResultHeader); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	for _, r := range rows {
		if err := cw.Write(applyResultRecord(r)); err != nil {
			return fmt.Errorf("write result row: %w", err)
		}
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		return fmt.Errorf("flush csv: %w", err)
	}
	return nil
}

// reportHeader is the fixed spin-move report CSV header (P6 context pack T2).
var reportHeader = []string{
	"product_sys_id", "product_code", "product_name", "type_code", "shade_code", "is_locked",
	"current_sp_code", "current_spin_id", "new_sp_code", "new_spin_id", "fg_product_id", "change",
}

func uuidStr(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

func reportRecord(r reportRow) []string {
	p := r.Product
	return []string{
		strconv.FormatInt(p.ProductSysID, 10), p.ProductCode, p.ProductName, p.TypeCode, p.ShadeCode, boolStr(p.IsLocked),
		r.CurrentSPCode, uuidStr(r.CurrentSpinID), r.NewSPCode, uuidStr(r.NewSpinID),
		strconv.FormatInt(r.FgProductID, 10), r.Change,
	}
}

// writeReportCSV writes the spin-move report CSV (header only when no product moves).
func writeReportCSV(w io.Writer, rows []reportRow) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(reportHeader); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	for _, r := range rows {
		if err := cw.Write(reportRecord(r)); err != nil {
			return fmt.Errorf("write report row: %w", err)
		}
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		return fmt.Errorf("flush csv: %w", err)
	}
	return nil
}
