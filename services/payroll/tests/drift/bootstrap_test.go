// Package drift guards copies that must stay identical across repos.
package drift

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// copyHeaderLines is the number of comment lines the in-repo copy adds on top of the workspace original.
const copyHeaderLines = 2

// TestBootstrapSQLNotDrifted compares services/payroll/deploy/bootstrap.sql (the in-repo copy used by CI and the
// testcontainers tests) with goapps/deploy/payroll/bootstrap.sql (the workspace original used by the dev stack).
// The original only exists in the workspace checkout, so the test is skipped in CI (repo alone).
func TestBootstrapSQLNotDrifted(t *testing.T) {
	copyPath, err := filepath.Abs("../../deploy/bootstrap.sql")
	if err != nil {
		t.Fatal(err)
	}
	copyData, err := os.ReadFile(copyPath) //nolint:gosec // fixed in-repo path
	if err != nil {
		t.Fatalf("in-repo copy: %v", err)
	}
	orig := findWorkspaceOriginal(t)
	if orig == "" {
		t.Skip("goapps/deploy/payroll/bootstrap.sql not found above the repo (CI checkout): drift check skipped")
	}
	origData, err := os.ReadFile(orig) //nolint:gosec // path found by walking up from the test dir
	if err != nil {
		t.Fatal(err)
	}
	body := copyData
	for i := 0; i < copyHeaderLines; i++ {
		_, rest, ok := bytes.Cut(body, []byte("\n"))
		if !ok {
			t.Fatal("in-repo copy is shorter than its header")
		}
		body = rest
	}
	if !bytes.Equal(body, origData) {
		t.Fatalf("%s differs from %s (apart from the %d header lines): change both together", copyPath, orig, copyHeaderLines)
	}
}

// findWorkspaceOriginal walks up from the working directory looking for deploy/payroll/bootstrap.sql.
func findWorkspaceOriginal(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		cand := filepath.Join(dir, "deploy", "payroll", "bootstrap.sql")
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}
