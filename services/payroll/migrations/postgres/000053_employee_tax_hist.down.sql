-- [T033n] PERINGATAN: down menghapus seluruh riwayat status pajak; up ulang hanya mem-backfill status terkini
-- (berlaku 1 Jan tahun berjalan), riwayat sungguhan tidak kembali.
DROP TRIGGER IF EXISTS trg_employee_tax_hist_upd ON payroll.pyr_employee;
DROP TRIGGER IF EXISTS trg_employee_tax_hist ON payroll.pyr_employee;
DROP FUNCTION IF EXISTS payroll.fn_employee_tax_hist();
DROP TABLE IF EXISTS payroll.pyr_employee_tax_hist;
