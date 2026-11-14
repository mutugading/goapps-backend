-- [T019k] cmd/recon (role payroll_ro, D-41) memuat input dry-run lewat loader yang sama dengan run; untuk
-- periode yang memuat YTD (mis. 202512, PPh tahunan) loader memanggil fn_ytd_out_of_scope. Fungsi hanya
-- mengembalikan jumlah penempatan per NIK dalam scope pemanggil, tanpa nominal, jadi aman untuk role baca.
-- [T028n2] grant dipindah ke deploy/grants.sql (D-66 C(2)); tidak ada DDL tersisa.
SELECT 1;
