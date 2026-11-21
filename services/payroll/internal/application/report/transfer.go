package report

import (
	"strconv"

	"github.com/xuri/excelize/v2"
)

// transfer writes the F-2 `transfer` sheet: one line per payable employee, Σ Amount = Σ DETAIL of the bank file.
// The account is a number (as in the bank file), so the sheet is as sensitive as the DB itself.
func (s *sheetWriter) transfer(d RecoData) {
	s.start(SheetTransfer)
	s.put("BANK TRANSFER")
	s.put("Processing Month " + strconv.Itoa(d.YYMM))
	s.put()
	s.put("No.", "Processing Month", "NIK", "Nama", "Dept", "Employee Status", "Level Status", "MGT MB Contract", "Bank Account", "Amount")
	for i, r := range d.Transfer.Rows {
		acc, err := strconv.ParseInt(r.Account, 10, 64) // digits only and <= 15 by bankIssues
		if err != nil {
			s.err = err
			return
		}
		s.put(i+1, d.YYMM, r.NIK, r.Nama, optStr(r.Dept), optStr(r.Status), optStr(r.LevelStatus), optStr(r.MGTMB), acc, r.Net)
	}
	s.transferStyles(len(d.Transfer.Rows))
}

// transferStyles formats Bank Account as a plain number (no scientific notation) and Amount as #,##0.
func (s *sheetWriter) transferStyles(n int) {
	if s.err != nil || n == 0 {
		return
	}
	for _, c := range []struct {
		col    string
		numFmt int
	}{{"I", 1}, {"J", 3}} {
		st, err := s.f.NewStyle(&excelize.Style{NumFmt: c.numFmt})
		if err != nil {
			s.err = err
			return
		}
		if err := s.f.SetCellStyle(SheetTransfer, c.col+"5", c.col+strconv.Itoa(4+n), st); err != nil {
			s.err = err
			return
		}
	}
}

// transferPreCheck lists employees left out of the transfer sheet (no account number).
func (s *sheetWriter) transferPreCheck(d RecoData) {
	if len(d.Transfer.Issues) == 0 {
		return
	}
	s.put()
	s.put("Transfer: tidak masuk sheet transfer", "Nama", "Alasan")
	for _, i := range d.Transfer.Issues {
		s.put(i.NIK, i.Nama, i.Reason)
	}
}
