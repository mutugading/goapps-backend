-- H035: pilih departemen di Tren OT. Viewer menampilkan chip bila chart_config.filter_chips_group1 terisi
-- (parser hanya meneruskan kunci ini; daftar sebenarnya diganti nilai distinct HR_OT_HOURS saat dibuka).
-- group_1 = dept, group_2 = tipe OT. Pivot, KPI tipe, dan tabel mengikuti pilihan; KPI cross_ratio
-- (jam OT/karyawan, % lembur) memakai filter dept karena HR_HEADCOUNT & HR_OT_EMP_COUNT ber-group_1 dept sama.
-- Daftar awal diambil dari fakta yang ada; DB kosong → tetap diisi satu nilai agar chip aktif. Idempotent.
BEGIN;

UPDATE bi_dashboard
SET chart_config = (chart_config - 'filter_chips' - 'filter_chips_group1_label' - 'filter_chips_group2_label')
      || jsonb_build_object('filter_chips_group1', COALESCE(
           (SELECT jsonb_agg(g ORDER BY g) FROM (SELECT DISTINCT group_1 AS g FROM bi_fact_metric
             WHERE type = 'HR_OT_HOURS' AND group_1 IS NOT NULL) x),
           '["Finance"]'::jsonb))
WHERE dashboard_code = 'HR_OT_TREND'
  AND NOT (chart_config ? 'filter_chips_group1');

COMMIT;
