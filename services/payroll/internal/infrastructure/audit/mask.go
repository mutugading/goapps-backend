// Package audit implements the pyr_audit_log writer and PII masking.
package audit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

const masked = "***"

// allowedKeys are the JSON keys (lower-case, exact) whose values may be stored in
// pyr_audit_log as-is. Everything else is replaced with "***" at any depth, so a
// new field is masked until someone decides it is safe (allowlist, T033g1). Keys
// come from the Go field names of the audited structs (no json tag) and from the
// map payloads built by the application services.
var allowedKeys = toSet(
	// shared identifiers and bookkeeping
	"id", "sysid", "code", "name", "version", "status", "companyid", "division", "level",
	"createdby", "createdat", "updatedby", "updatedat", "action", "before", "after",
	// master: currency, company, division (07§1)
	"minorunit", "takehomeround", "basecurrency", "active",
	// master: employee, non-sensitive work data (personal data is denied below)
	"nik", "nama", "deptid", "dept", "sectionid", "section", "designationid", "designation", "grade",
	"taxid", "stsemp", "stspayroll", "stsbonus", "datejoin", "resigndate", "marriedsts", "bank", "bankbranch",
	"orionsubacnt", "idmesin", "cutitahunan", "paycurrency", "employmentstatus", "costgroup", "prevemployeeid",
	"hrissysid", "placementreason",
	// component, template, attachment, level change
	"type", "scopedivisions", "currency", "defaultsts", "taxclass", "subsidyeligible", "fixincome",
	"includeinestimate", "tags", "serial", "basesysid", "validfrom", "validto", "sts", "tax", "amount",
	"taxpaidcompany", "frz", "source", "currentlevel", "newlevel", "changes", "skippedmanual",
	// period, run commit
	"active_run_id", "reason", "run_id", "superseded_run_id", "yymm", "rule_set_id", "snapshot_id", "input_hash",
	"emp_count", "line_count", "tot_tunjangan", "tot_potongan", "warnings",
	// report export (counts and flags only)
	"jenis", "divisi", "baris", "allow_open", "pra_cek", "pra_cek_transfer", "tanpa_password", "dilewati",
	"filter_nik",
	// distribution (tujuan is already MaskEmail'd by the caller)
	"total", "queued", "tanpa_email", "email_ganda", "sent", "failed", "blocked", "pending", "jumlah", "tujuan",
	"item", "asal", "alasan", "terkirim", "tertunda", "run", "run_aktif",
)

// deniedKeys document the PII fields of audited structs that are masked on purpose
// (review PR #283, D-17). Masking does not depend on this list (anything not allowed
// is masked); the test uses it to force a decision for every new struct field.
var deniedKeys = toSet(
	"npwp", "ktp", "bank_acct", "bankacntname", "email", "birthdate", "birthplace", "religion", "sex",
)

func toSet(keys ...string) map[string]struct{} {
	m := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		m[k] = struct{}{}
	}
	return m
}

func isAllowed(key string) bool {
	_, ok := allowedKeys[strings.ToLower(key)]
	return ok
}

// MaskJSON marshals v and replaces the value of every key outside the allowlist
// (at any depth) with "***". A nil v yields nil (stored as SQL NULL).
func MaskJSON(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal audit payload: %w", err)
	}
	var tree any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber() // keep int64 ids exact
	if err := dec.Decode(&tree); err != nil {
		return nil, fmt.Errorf("decode audit payload: %w", err)
	}
	out, err := json.Marshal(maskTree(tree))
	if err != nil {
		return nil, fmt.Errorf("encode audit payload: %w", err)
	}
	return out, nil
}

func maskTree(n any) any {
	switch t := n.(type) {
	case map[string]any:
		for k, v := range t {
			if !isAllowed(k) {
				t[k] = masked
				continue
			}
			t[k] = maskTree(v)
		}
	case []any:
		for i, v := range t {
			t[i] = maskTree(v)
		}
	}
	return n
}
