-- H035: koreksi 000596. Sumbu x HR_OT_TREND = periode, jadi baris tabel adalah bulan, bukan dept; drill mengirim
-- "202608" sebagai group_1 → kosong → viewer menampilkan CTA "Upload Data". Drill dimatikan, judul dikoreksi.
-- Rincian per dept lewat chip filter Dept (000597). Idempotent.
BEGIN;

UPDATE bi_dashboard
SET drill_enabled = FALSE,
    max_drill_level = 1,
    layout_config = '{"secondary_charts":[{"title":"Jam OT per Bulan × Tipe OT (ikut filter dept)","chart_type":"data_table","chart_config":{"number_format":"raw","decimals":1},"span":"full","drill_enabled":false}]}'::jsonb
WHERE dashboard_code = 'HR_OT_TREND';

COMMIT;
