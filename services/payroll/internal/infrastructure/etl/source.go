// Package etl loads legacy Oracle MGTDAT master data into the payroll DB (08§1, D-22).
// Source files are .psv spools (pipe separated, first line = column names); the Source
// interface lets a direct Oracle reader replace them without touching the loaders.
package etl

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/rs/zerolog/log"
)

// Legacy table names (also the .psv file base name, case-insensitive).
const (
	TableComponentMaster   = "MGT_PAYROLL_COMPNT_MASTER"
	TableComponentTemplate = "MGT_PAYROLL_COMPNT_TEMPLATE"
	TableEmployee          = "MGT_PAYROLL_EMP"
	TableEmployeeComponent = "MGT_PAYROLL_COMPNT_EMP"
	TableHRISEmployee      = "HM_EMP_DATA" // opsional: HMEMD_NIK, HMEMD_SYS_ID (13§3.5)
)

// Table is one source table: upper-cased column names and string cells ("" = NULL).
type Table struct {
	Name string
	Cols []string
	Rows [][]string
	// Notes are reader-level data-quality remarks (e.g. non-UTF-8 bytes repaired), surfaced as report warnings.
	Notes []string
}

// Source reads a legacy table. Implementations must be read-only.
type Source interface {
	Read(table string) (*Table, error)
}

// PSVDir reads <dir>/<TABLE>.psv spool files.
type PSVDir struct{ Dir string }

// Read implements Source.
func (p PSVDir) Read(table string) (*Table, error) {
	path := filepath.Join(p.Dir, table+".psv")
	if _, err := os.Stat(path); err != nil {
		path = filepath.Join(p.Dir, strings.ToLower(table)+".psv")
	}
	f, err := os.Open(path) //nolint:gosec // path from operator-supplied dir
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", table, err)
	}
	defer closeQuiet(f)

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	t := &Table{Name: table}
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line, fixed := decodeLine(sc.Bytes())
		line = strings.TrimRight(line, "\r")
		if fixed {
			t.Notes = append(t.Notes, fmt.Sprintf("%s baris %d: byte non-UTF-8 didecode sebagai Latin-1", table, lineNo))
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		cells := strings.Split(line, "|")
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		if t.Cols == nil {
			for i := range cells {
				cells[i] = strings.ToUpper(cells[i])
			}
			t.Cols = cells
			continue
		}
		if len(cells) != len(t.Cols) {
			return nil, fmt.Errorf("%s: baris %d punya %d kolom, header %d", table, len(t.Rows)+2, len(cells), len(t.Cols))
		}
		t.Rows = append(t.Rows, cells)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", table, err)
	}
	if t.Cols == nil {
		return nil, fmt.Errorf("%s: berkas kosong (header tidak ada)", table)
	}
	return t, nil
}

// decodeLine returns b as UTF-8. Bytes that are not valid UTF-8 (legacy Oracle spools, e.g. 0xA0 in
// an email) are mapped as Latin-1 runes instead of failing the load (sqlstate 22021); fixed reports it.
func decodeLine(b []byte) (string, bool) {
	if utf8.Valid(b) {
		return string(b), false
	}
	var sb strings.Builder
	for len(b) > 0 {
		r, n := utf8.DecodeRune(b)
		if r == utf8.RuneError && n == 1 {
			r = rune(b[0])
		}
		sb.WriteRune(r)
		b = b[n:]
	}
	return sb.String(), true
}

// Col returns the index of a column or an error naming the missing one.
func (t *Table) Col(name string) (int, error) {
	for i, c := range t.Cols {
		if c == name {
			return i, nil
		}
	}
	return -1, fmt.Errorf("%s: kolom %s tidak ada", t.Name, name)
}

// Checksum is a sha256 over the sorted rows, independent of spool order.
func (t *Table) Checksum() string {
	lines := make([]string, len(t.Rows))
	for i, r := range t.Rows {
		lines[i] = strings.Join(r, "|")
	}
	sort.Strings(lines)
	h := sha256.New()
	for _, l := range lines {
		_, _ = h.Write([]byte(l + "\n"))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// closeQuiet closes a read-only handle and logs (not drops) a failure, satisfying errcheck.
func closeQuiet(c io.Closer) {
	if err := c.Close(); err != nil {
		log.Warn().Err(err).Msg("etl: close failed")
	}
}
