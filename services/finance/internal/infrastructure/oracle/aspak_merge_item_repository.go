package oracle

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
)

// mergeItemMaxRows is the safety cap on rows read per call; more is an error, never a silent cut.
const mergeItemMaxRows = 20000

// mergeItemQuery lists distinct (merge item, shade) pairs merged since :1. SELECT-only; the date
// is bound as YYYY-MM-DD text. Denier/filament are fetched as TM9 text to keep decimals exact.
const mergeItemQuery = `SELECT DISTINCT MRG.MERGE_ITEM_ORION, ITEM_DESCR||ITEM_SPECS ITEM_NAME, MRG.SHADE_CODE,
       NVL(SH.SHADE_COLOR, NVL(MRG.SHADE_COLOR,'NATURAL')) SHADE_NAME,
       TO_CHAR(MERGE_DENIER,'TM9','NLS_NUMERIC_CHARACTERS=''.,'''), TO_CHAR(MERGE_FILAMENT,'TM9','NLS_NUMERIC_CHARACTERS=''.,''')
  FROM ASPAK.MMSMERGE MRG, ASPAK.MMSITEM, ASPAK.SHADEMAS SH
 WHERE MERGE_PUT_DATE >= TO_DATE(:1,'YYYY-MM-DD')
   AND MERGE_ITEM_CODE = ITEM_CODE
   AND MRG.SHADE_CODE = SH.SHADE_CODE`

// ASPAKMergeItemRepository reads merge items from ASPAK through the read-only guard.
type ASPAKMergeItemRepository struct {
	q ReadOnlyQuerier
}

var _ costproductmaster.EtlSource = (*ASPAKMergeItemRepository)(nil)

// NewASPAKMergeItemRepository constructs the reader.
func NewASPAKMergeItemRepository(q ReadOnlyQuerier) *ASPAKMergeItemRepository {
	return &ASPAKMergeItemRepository{q: q}
}

// ListMergeItems returns merge rows with MERGE_PUT_DATE >= from.
func (r *ASPAKMergeItemRepository) ListMergeItems(ctx context.Context, from time.Time) (out []costproductmaster.EtlSourceRow, err error) {
	if r == nil || r.q == nil {
		return nil, ErrNoReadConnection
	}
	rows, err := r.q.QueryRO(ctx, mergeItemQuery, from.Format(time.DateOnly))
	if err != nil {
		return nil, fmt.Errorf("query ASPAK.MMSMERGE: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close rows: %w", cerr)
		}
	}()
	for rows.Next() {
		if len(out) >= mergeItemMaxRows {
			return nil, fmt.Errorf("ASPAK.MMSMERGE returned more than %d rows; narrow from_date", mergeItemMaxRows)
		}
		var item, name, shade, shadeName, denier, fil sql.NullString
		if err := rows.Scan(&item, &name, &shade, &shadeName, &denier, &fil); err != nil {
			return nil, fmt.Errorf("scan ASPAK.MMSMERGE: %w", err)
		}
		out = append(out, costproductmaster.EtlSourceRow{
			ItemCode:  strings.TrimSpace(item.String),
			ItemName:  strings.TrimSpace(name.String),
			ShadeCode: strings.TrimSpace(shade.String),
			ShadeName: strings.TrimSpace(shadeName.String),
			Denier:    strings.TrimSpace(denier.String),
			Filament:  strings.TrimSpace(fil.String),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ASPAK.MMSMERGE: %w", err)
	}
	return out, nil
}
