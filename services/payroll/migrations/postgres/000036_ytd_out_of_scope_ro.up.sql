-- [T019k] cmd/recon (role payroll_ro, D-41) memuat input dry-run lewat loader yang sama dengan run; untuk
-- periode yang memuat YTD (mis. 202512, PPh tahunan) loader memanggil fn_ytd_out_of_scope. Fungsi hanya
-- mengembalikan jumlah penempatan per NIK dalam scope pemanggil, tanpa nominal, jadi aman untuk role baca.
GRANT EXECUTE ON FUNCTION payroll.fn_ytd_out_of_scope(smallint, varchar, int, int, text[], int) TO payroll_ro;
