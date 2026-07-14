package postgres

import (
	"database/sql"
	"os"
	"testing"
)

func TestZZCheckMBParams(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	db, err := sql.Open("postgres", "postgres://finance:finance123@localhost:5434/finance_db?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query("SELECT param_code, deleted_at FROM mst_parameter WHERE param_code IN ('IS_BOUGHTOUT','MACHINE_MB_FIXED_TOTAL','MB_COMPOSITION_VERSION','MB_WASTE')")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	found := 0
	for rows.Next() {
		var code string
		var deletedAt sql.NullString
		if scanErr := rows.Scan(&code, &deletedAt); scanErr != nil {
			t.Fatal(scanErr)
		}
		t.Logf("found: %s deleted_at=%v", code, deletedAt)
		found++
	}
	t.Logf("total found: %d", found)
}
