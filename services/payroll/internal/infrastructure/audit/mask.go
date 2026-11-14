// Package audit implements the pyr_audit_log writer and PII masking.
package audit

import (
	"encoding/json"
	"fmt"
	"strings"
)

const masked = "***"

// piiKeys are JSON keys (lower-case substrings) whose values are never logged.
var piiKeys = []string{"npwp", "ktp", "nik_ktp", "rekening", "bank_acct", "account_no", "password", "secret", "token"}

func isPII(key string) bool {
	k := strings.ToLower(key)
	for _, p := range piiKeys {
		if strings.Contains(k, p) {
			return true
		}
	}
	return false
}

// MaskJSON marshals v and replaces values of PII keys (at any depth) with "***".
// A nil v yields nil (stored as SQL NULL).
func MaskJSON(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal audit payload: %w", err)
	}
	var tree any
	if err := json.Unmarshal(raw, &tree); err != nil {
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
			if isPII(k) {
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
