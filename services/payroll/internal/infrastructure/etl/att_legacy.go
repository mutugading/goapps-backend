package etl

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/rs/zerolog/log"
)

// Legacy attendance recap tables (T009d, 08§2): one frozen snapshot per (company, yymm, division).
const (
	TableAttRekap    = "MGT_PAYROLL_ATT_REKAP"
	TableAttRekapAdd = "MGT_PAYROLL_ATT_REKAP_ADD"
	TableSchRekap    = "MGT_PAYROLL_SCH_REKAP"

	snapKindRekap = "REKAP"
	snapKindAdd   = "ADD"
	snapKindSch   = "SCH"
)

// AttLegacyOptions controls one legacy attendance snapshot load. Expect* = 0 means unchecked (08§2: 68925/870/3996).
type AttLegacyOptions struct {
	CompanyID                       int16
	ExpectRekap, ExpectAdd, ExpectS int
}

type snapKey struct {
	yymm     string
	division string
}

// snapRow is one pyr_att_snapshot_row to write; src keeps the raw source line for the snapshot checksum.
type snapRow struct {
	kind, nik string
	data      map[string]any
	src       string
	sched     map[string]any // SCH only: alias of data["SCHEDULES"]
}

var plainNumber = regexp.MustCompile(`^-?\d*\.?\d+$`)

// jsonValue keeps numeric cells exact (json.Number, never float) and everything else as text; "" = null.
func jsonValue(v string) any {
	switch {
	case v == "":
		return nil
	case plainNumber.MatchString(v):
		s := v
		if strings.HasPrefix(s, ".") {
			s = "0" + s
		} else if strings.HasPrefix(s, "-.") {
			s = "-0" + s[1:]
		}
		return json.Number(s)
	}
	return v
}

// groupRekap collects REKAP/ADD rows: every source column except the key columns becomes a pasr_data key.
func groupRekap(src *Table, kind, prefix string, groups map[snapKey][]snapRow) (TableReport, error) {
	keyCols := []string{prefix + "YYMM", prefix + "NIK", prefix + "DIVISION"}
	rep, err := loadTable(src, keyCols, func(r []string, ix map[string]int, _ int) (string, error) {
		k := snapKey{r[ix[keyCols[0]]], r[ix[keyCols[2]]]}
		data := map[string]any{}
		for i, c := range src.Cols {
			if i != ix[keyCols[0]] && i != ix[keyCols[1]] && i != ix[keyCols[2]] {
				data[c] = jsonValue(r[i])
			}
		}
		groups[k] = append(groups[k], snapRow{kind: kind, nik: r[ix[keyCols[1]]], data: data, src: strings.Join(r, "|")})
		return "", nil
	})
	return rep, err
}

// groupSch folds MGT_PAYROLL_SCH_REKAP (many rows per NIK) into one SCH row per NIK.
func groupSch(src *Table, groups map[snapKey][]snapRow) (TableReport, error) {
	cols := []string{"MPSR_YYMM", "MPSR_NIK", "MPSR_SCHEDULE", "MPSR_NO_SCH", "MPSR_DIVISION"}
	type nikKey struct {
		snapKey
		nik string
	}
	byNik := map[nikKey]*snapRow{}
	var order []nikKey
	rep, err := loadTable(src, cols, func(r []string, ix map[string]int, _ int) (string, error) {
		k := nikKey{snapKey{r[ix["MPSR_YYMM"]], r[ix["MPSR_DIVISION"]]}, r[ix["MPSR_NIK"]]}
		row := byNik[k]
		if row == nil {
			sched := map[string]any{}
			row = &snapRow{kind: snapKindSch, nik: k.nik, data: map[string]any{"SCHEDULES": sched}, sched: sched}
			byNik[k] = row
			order = append(order, k)
		}
		row.sched[r[ix["MPSR_SCHEDULE"]]] = jsonValue(r[ix["MPSR_NO_SCH"]])
		row.src += strings.Join(r, "|") + "\n"
		return "", nil
	})
	for _, k := range order {
		groups[k.snapKey] = append(groups[k.snapKey], *byNik[k])
	}
	return rep, err
}

// snapChecksum is order independent: sha256 over the sorted raw source lines of the snapshot.
func snapChecksum(rows []snapRow) string {
	lines := make([]string, len(rows))
	for i, r := range rows {
		lines[i] = r.kind + "|" + r.src
	}
	sort.Strings(lines)
	h := sha256.New()
	for _, l := range lines {
		_, _ = h.Write([]byte(l + "\n"))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// writeSnapshots upserts one locked LEGACY snapshot per group and its rows. pasr_premi_fail/beras_fail stay NULL:
// the historical source of those figures is still an open question (TASKS T009d).
func writeSnapshots(ctx context.Context, tx *sql.Tx, company int16, groups map[snapKey][]snapRow) error {
	keys := make([]snapKey, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].yymm != keys[j].yymm {
			return keys[i].yymm < keys[j].yymm
		}
		return keys[i].division < keys[j].division
	})
	for _, k := range keys {
		rows := groups[k]
		var id int64
		err := tx.QueryRowContext(ctx, `
INSERT INTO payroll.pyr_att_snapshot (pas_company_id, pas_yymm, pas_division, pas_source_batch_ids, pas_checksum, pas_locked, created_by)
VALUES ($1,$2,$3,'{}',$4,true,'etl')
ON CONFLICT (pas_company_id, pas_yymm, pas_division) WHERE pas_source_batch_ids = '{}'
DO UPDATE SET pas_checksum = EXCLUDED.pas_checksum, pas_locked = true
RETURNING pas_snapshot_id`, company, k.yymm, k.division, snapChecksum(rows)).Scan(&id)
		if err != nil {
			return fmt.Errorf("snapshot %s/%s: %w", k.yymm, k.division, err)
		}
		for _, r := range rows {
			js, err := json.Marshal(r.data)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `
INSERT INTO payroll.pyr_att_snapshot_row (pasr_snapshot_id, pasr_kind, pasr_nik, pasr_data) VALUES ($1,$2,$3,$4::jsonb)
ON CONFLICT (pasr_snapshot_id, pasr_kind, pasr_nik) DO UPDATE SET pasr_data = EXCLUDED.pasr_data`,
				id, r.kind, r.nik, string(js)); err != nil {
				return fmt.Errorf("snapshot %s/%s nik %s %s: %w", k.yymm, k.division, r.nik, r.kind, err)
			}
		}
	}
	return nil
}

// RunAttLegacy loads the historical attendance recaps into LEGACY snapshots in ONE transaction (idempotent).
// The reports count source rows vs rows folded into snapshots; the snapshot count is logged as pyr_att_snapshot.
func RunAttLegacy(ctx context.Context, db *sql.DB, src Source, o AttLegacyOptions) ([]TableReport, error) {
	rk, err := src.Read(TableAttRekap)
	if err != nil {
		return nil, err
	}
	ad, err := src.Read(TableAttRekapAdd)
	if err != nil {
		return nil, err
	}
	sc, err := src.Read(TableSchRekap)
	if err != nil {
		return nil, err
	}
	groups := map[snapKey][]snapRow{}
	reps := make([]TableReport, 0, 3)
	for _, f := range []func() (TableReport, error){
		func() (TableReport, error) { return groupRekap(rk, snapKindRekap, "MPAR_", groups) },
		func() (TableReport, error) { return groupRekap(ad, snapKindAdd, "MPARA_", groups) },
		func() (TableReport, error) { return groupSch(sc, groups) },
	} {
		r, err := f()
		if err != nil {
			return reps, err
		}
		reps = append(reps, r)
	}
	if err := checkAttExpected(reps, o); err != nil {
		return reps, err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			log.Warn().Err(rbErr).Msg("etl: rollback failed")
		}
	}()
	if err := writeSnapshots(ctx, tx, o.CompanyID, groups); err != nil {
		return reps, err
	}
	for _, r := range reps {
		if err := logReport(ctx, tx, o.CompanyID, r); err != nil {
			return reps, err
		}
	}
	return reps, tx.Commit()
}

func checkAttExpected(reps []TableReport, o AttLegacyOptions) error {
	want := map[string]int{TableAttRekap: o.ExpectRekap, TableAttRekapAdd: o.ExpectAdd, TableSchRekap: o.ExpectS}
	for _, r := range reps {
		if n := want[r.Table]; n > 0 && r.SrcRows != n {
			return fmt.Errorf("%s: %d baris, diharapkan %d", r.Table, r.SrcRows, n)
		}
	}
	return nil
}
