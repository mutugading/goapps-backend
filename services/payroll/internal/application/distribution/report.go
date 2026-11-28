package distribution

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"

	"github.com/xuri/excelize/v2"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/report"
)

// ItemRow is one line of the batch report. The address is masked.
type ItemRow struct {
	ItemID                             int64
	NIK, Nama, Email, Status           string
	Attempts                           int
	SentAt, LastError, Message, Resend string
}

// BatchReport is the report of one batch.
type BatchReport struct {
	BatchID                      int64
	YYMM                         int
	Division, Status             string
	Total, Sent, Failed, Blocked int
	Rows                         []ItemRow
}

// Report reads the batch and its items (masked addresses) and writes the audit row.
func (s *Service) Report(ctx context.Context, q report.Query, batchID int64) (BatchReport, error) {
	var br BatchReport
	err := s.DB.ScopedTx(ctx, func(tx *sql.Tx) error {
		br = BatchReport{BatchID: batchID}
		if err := recount(ctx, tx, batchID); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `SELECT pdb_yymm, pdb_division::text, pdb_status, COALESCE(pdb_total,0), pdb_sent, pdb_failed, pdb_blocked
FROM payroll.pyr_distribution_batch WHERE pdb_id=$1 AND pdb_company_id=$2 AND pdb_division=$3`, batchID, q.CompanyID, q.Division).
			Scan(&br.YYMM, &br.Division, &br.Status, &br.Total, &br.Sent, &br.Failed, &br.Blocked); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("batch %d tidak ada (atau di luar scope)", batchID)
			}
			return fmt.Errorf("read batch: %w", err)
		}
		rows, err := tx.QueryContext(ctx, `SELECT i.pdi_id, i.pdi_nik, COALESCE(e.pe_nama,''), COALESCE(i.pdi_email_to,''), i.pdi_status, i.pdi_attempts,
 COALESCE(to_char(i.pdi_sent_at AT TIME ZONE 'UTC','YYYY-MM-DD HH24:MI:SS'),''), COALESCE(i.pdi_last_error,''), COALESCE(i.pdi_message_id,''),
 COALESCE(i.pdi_resend_reason,'')
FROM payroll.pyr_distribution_item i LEFT JOIN payroll.pyr_employee e ON e.pe_id=i.pdi_employee_id
WHERE i.pdi_batch_id=$1 ORDER BY i.pdi_nik, i.pdi_id`, batchID)
		if err != nil {
			return fmt.Errorf("read items: %w", err)
		}
		defer closeRows(rows)
		for rows.Next() {
			var r ItemRow
			var email string
			if err := rows.Scan(&r.ItemID, &r.NIK, &r.Nama, &email, &r.Status, &r.Attempts, &r.SentAt, &r.LastError, &r.Message, &r.Resend); err != nil {
				return err
			}
			if email != "" {
				r.Email = MaskEmail(email)
			}
			br.Rows = append(br.Rows, r)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		return s.audit(ctx, tx, q.CompanyID, q.Division, "DISTRIBUTION_REPORT", batchID, map[string]any{"baris": len(br.Rows)})
	})
	return br, err
}

// WriteReport writes the batch report workbook (sheet "item"; the PII it holds is NIK, name, masked address).
func WriteReport(br BatchReport, w io.Writer) error {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }() //nolint:errcheck // in-memory workbook
	const sh = "item"
	if err := f.SetSheetName("Sheet1", sh); err != nil {
		return err
	}
	rows := [][]any{
		{"Batch", br.BatchID, "Periode", br.YYMM, "Divisi", br.Division, "Status", br.Status},
		{"Total", br.Total, "Terkirim", br.Sent, "Gagal", br.Failed, "Diblokir", br.Blocked},
		{},
		{"Item", "NIK", "Nama", "Email (masked)", "Status", "Percobaan", "Terkirim", "Kesalahan terakhir", "Message-ID", "Alasan kirim ulang"},
	}
	for _, r := range br.Rows {
		rows = append(rows, []any{r.ItemID, r.NIK, r.Nama, r.Email, r.Status, r.Attempts, r.SentAt, r.LastError, r.Message, r.Resend})
	}
	for i, row := range rows {
		cell, err := excelize.CoordinatesToCellName(1, i+1)
		if err != nil {
			return err
		}
		if len(row) == 0 {
			continue
		}
		if err := f.SetSheetRow(sh, cell, &row); err != nil {
			return err
		}
	}
	return f.Write(w)
}
