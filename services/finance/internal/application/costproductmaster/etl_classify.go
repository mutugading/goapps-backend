package costproductmaster

import (
	"fmt"
	"sort"
	"strings"

	"github.com/shopspring/decimal"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
)

// ETL parameter codes written (fill-empty) for newly inserted products.
const (
	etlParamDenier    = "DENIER"
	etlParamFilament  = "NO_OF_FILAMENTS"
	etlParamActDenier = "ACT_DENIER"
	etlMBToken        = "MB"
)

var etlParamCodes = []string{etlParamDenier, etlParamFilament, etlParamActDenier}

// etlMerged is one deduped (item, shade) source row.
type etlMerged struct {
	src domain.EtlSourceRow
	key string
}

// etlDedupe merges duplicate Oracle rows by normalized (item, shade); the first non-empty
// denier/filament wins. MB rows are dropped and counted. Output is sorted by row key.
func etlDedupe(rows []domain.EtlSourceRow) (out []etlMerged, skippedMB int) {
	idx := make(map[string]int, len(rows))
	for _, r := range rows {
		r = etlTrimRow(r)
		if etlTypeToken(r.ItemName) == etlMBToken {
			skippedMB++
			continue
		}
		k := domain.EtlRowKey(r.ItemCode, r.ShadeCode)
		i, ok := idx[k]
		if !ok {
			idx[k] = len(out)
			out = append(out, etlMerged{src: r, key: k})
			continue
		}
		cur := &out[i].src
		if cur.Denier == "" {
			cur.Denier = r.Denier
		}
		if cur.Filament == "" {
			cur.Filament = r.Filament
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].key < out[b].key })
	return out, skippedMB
}

func etlTrimRow(r domain.EtlSourceRow) domain.EtlSourceRow {
	r.ItemCode = strings.TrimSpace(r.ItemCode)
	r.ItemName = strings.TrimSpace(r.ItemName)
	r.ShadeCode = strings.TrimSpace(r.ShadeCode)
	r.ShadeName = strings.TrimSpace(r.ShadeName)
	r.Denier = strings.TrimSpace(r.Denier)
	r.Filament = strings.TrimSpace(r.Filament)
	return r
}

// etlTypeToken is the first space-separated token of the item name, upper-cased.
func etlTypeToken(itemName string) string {
	f := strings.Fields(itemName)
	if len(f) == 0 {
		return ""
	}
	return strings.ToUpper(f[0])
}

// etlKeys builds the lookup keys for rows that have both item and shade.
func etlKeys(rows []etlMerged) []domain.EtlKey {
	keys := make([]domain.EtlKey, 0, len(rows))
	for _, m := range rows {
		if m.src.ItemCode == "" || m.src.ShadeCode == "" {
			continue
		}
		keys = append(keys, domain.EtlKey{
			Item:      domain.EtlNorm(m.src.ItemCode),
			Shade:     domain.EtlNorm(m.src.ShadeCode),
			NameUpper: domain.EtlNorm(m.src.ItemName),
		})
	}
	return keys
}

// etlClassify applies the FINAL classification order (design Revisi 3).
func etlClassify(m etlMerged, ms domain.EtlMatchSet, types map[string]int32) domain.EtlRunRow {
	s := m.src
	row := domain.EtlRunRow{
		RowKey: m.key, ItemCode: s.ItemCode, ItemName: s.ItemName,
		ShadeCode: s.ShadeCode, ShadeName: s.ShadeName, TypeCode: etlTypeToken(s.ItemName),
	}
	if s.ItemCode == "" || s.ShadeCode == "" {
		row.Status, row.Note = domain.EtlInvalidType, "item/shade kosong"
		return row
	}
	if a := ms.ByItemShade[m.key]; len(a) > 0 {
		row.MatchedIDs = a
		row.Status = domain.EtlExists
		if len(a) > 1 {
			row.Status = domain.EtlExistsMulti
		}
		return row
	}
	typeID, ok := types[row.TypeCode]
	if !ok {
		row.Status, row.Note = domain.EtlInvalidType, "tipe produk tidak ada di master aktif: "+row.TypeCode
		return row
	}
	row.TypeID = typeID
	if c := ms.ByNameShade[domain.EtlNameShadeKey(s.ItemName, s.ShadeCode)]; len(c) > 0 {
		row.Status, row.MatchedIDs = domain.EtlPossibleDuplicate, c
		row.Note = "nama+shade sudah ada dengan item ERP berbeda/kosong"
		return row
	}
	row.Selectable = true
	if sib := ms.ByItem[domain.EtlNorm(s.ItemCode)]; len(sib) > 0 {
		row.Status, row.MatchedIDs = domain.EtlNewShade, sib
		return row
	}
	row.Status = domain.EtlNewItem
	return row
}

// etlParseNum returns the canonical decimal text of v, or "" when empty / not a non-negative number.
func etlParseNum(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	d, err := decimal.NewFromString(v)
	if err != nil || d.IsNegative() {
		return ""
	}
	return d.String()
}

// etlProposeParams fills Denier/Filament/ActDenier of a selectable row (fill-empty, new product only).
// NEW_SHADE copies from the first sibling (id ASC) holding a value, falling back to Oracle for
// DENIER/NO_OF_FILAMENTS; ACT_DENIER is never taken from Oracle.
func etlProposeParams(row *domain.EtlRunRow, src domain.EtlSourceRow, sib map[int64]map[string]string) {
	pick := func(code string) (val string, from int64) {
		for _, id := range row.MatchedIDs { // ascending
			if v := etlParseNum(sib[id][code]); v != "" {
				return v, id
			}
		}
		return "", 0
	}
	var notes []string
	set := func(dst *string, code, oracle, label string) {
		if row.Status == domain.EtlNewShade {
			if v, id := pick(code); v != "" {
				*dst = v
				notes = append(notes, fmt.Sprintf("%s: sibling #%d", label, id))
				return
			}
		}
		if oracle != "" {
			if v := etlParseNum(oracle); v != "" {
				*dst = v
				notes = append(notes, label+": oracle")
			}
		}
	}
	set(&row.Denier, etlParamDenier, src.Denier, "denier")
	set(&row.Filament, etlParamFilament, src.Filament, "filament")
	set(&row.ActDenier, etlParamActDenier, "", "act_denier")
	row.Note = strings.Join(notes, "; ")
}

func etlCount(c *domain.EtlCounts, status string) {
	c.Total++
	switch status {
	case domain.EtlExists:
		c.Exists++
	case domain.EtlExistsMulti:
		c.ExistsMulti++
	case domain.EtlNewShade:
		c.NewShade++
	case domain.EtlNewItem:
		c.NewItem++
	case domain.EtlPossibleDuplicate:
		c.PossibleDuplicate++
	case domain.EtlInvalidType:
		c.InvalidType++
	}
}

func etlIsExisting(status string) bool {
	return status == domain.EtlExists || status == domain.EtlExistsMulti
}

func etlMatchesSearch(r domain.EtlRunRow, q string) bool {
	if q == "" {
		return true
	}
	for _, f := range []string{r.ItemCode, r.ItemName, r.ShadeCode, r.ShadeName} {
		if strings.Contains(strings.ToLower(f), q) {
			return true
		}
	}
	return false
}
