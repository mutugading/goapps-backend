package report

import (
	"database/sql"
	"sort"

	"github.com/rs/zerolog/log"
)

func closeRows(rows *sql.Rows) {
	if err := rows.Close(); err != nil {
		log.Warn().Err(err).Msg("failed to close rows")
	}
}

// sortComps orders by pcm_serial, then type (T before P), then id.
func sortComps(cs []Comp) {
	sort.Slice(cs, func(i, j int) bool {
		a, b := cs[i], cs[j]
		if a.Serial != b.Serial {
			return a.Serial < b.Serial
		}
		if a.Type != b.Type {
			return a.Type == "T"
		}
		return a.ID < b.ID
	})
}
