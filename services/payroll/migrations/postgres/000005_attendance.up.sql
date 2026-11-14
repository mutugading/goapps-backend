-- [T003] 04§7 (sumber: spec 04, baris dipotong apa adanya; D-04)
-- ---------------------------------------------------------------------------
-- 7. ABSENSI — staging (ditulis ETL) & snapshot (dibekukan saat run)
-- ---------------------------------------------------------------------------
CREATE TABLE payroll_stg.pyr_stg_sync_batch (
  psb_id bigserial PRIMARY KEY,
  psb_source varchar(40) NOT NULL,            -- 'MGTHRIS.VIEW_ATT_REKAP_HM' | 'MGTHRIS.HM_ATT_DAILY' | …
  psb_yymm int, psb_division varchar(30),
  psb_started_at timestamptz NOT NULL DEFAULT now(), psb_finished_at timestamptz,
  psb_rows_in int, psb_rows_upserted int, psb_rows_deleted int, psb_checksum text,
  psb_status varchar(10) NOT NULL DEFAULT 'RUNNING', psb_error text
);
CREATE TABLE payroll_stg.pyr_stg_att_rekap_hm (       -- 1:1 kolom VIEW_ATT_REKAP_HM / MGT_PAYROLL_ATT_REKAP
  psar_yymm int NOT NULL, psar_nik varchar(15) NOT NULL, psar_division varchar(30) NOT NULL,
  psar_nama varchar(200),
  psar_day_work numeric(6,2), psar_lembur_1 numeric(8,2), psar_lembur_2 numeric(8,2),
  psar_lembur_nas_1 numeric(8,2), psar_lembur_nas_2 numeric(8,2), psar_lembur_nas_3 numeric(8,2), psar_lembur_nas_tot numeric(8,2),
  psar_lembur_off_1 numeric(8,2), psar_lembur_off_2 numeric(8,2), psar_lembur_off_3 numeric(8,2), psar_lembur_off_tot numeric(8,2),
  psar_lembur_nasp_1 numeric(8,2), psar_lembur_nasp_2 numeric(8,2), psar_lembur_nasp_3 numeric(8,2), psar_lembur_nasp_tot numeric(8,2),
  psar_cuti numeric(6,2), psar_alpa numeric(6,2), psar_ijin numeric(6,2), psar_pay_tot numeric(6,2), psar_shift numeric(6,2),
  psar_dayoff numeric(6,2), psar_dirumahkan numeric(6,2),
  psar_sakit_75 numeric(6,2), psar_sakit_50 numeric(6,2), psar_sakit_25 numeric(6,2), psar_otiw_decr numeric(6,2),
  psar_row_hash text NOT NULL,                 -- md5 kolom → deteksi perubahan
  psar_batch_id bigint NOT NULL REFERENCES payroll_stg.pyr_stg_sync_batch,
  psar_synced_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (psar_yymm, psar_nik, psar_division)
);
CREATE TABLE payroll_stg.pyr_stg_att_daily (          -- harian untuk PREMI_FAIL/BERAS_FAIL (ganti akses langsung MGTHRIS)
  psad_work_date date NOT NULL, psad_nik varchar(15) NOT NULL, psad_division varchar(30) NOT NULL,
  psad_schedule_code varchar(10), psad_att_code varchar(10), -- kode yg dicocokkan ke pyr_batal_premi
  psad_check_in timestamptz, psad_check_out timestamptz, psad_work_hours numeric(5,2),
  psad_row_hash text NOT NULL, psad_batch_id bigint NOT NULL REFERENCES payroll_stg.pyr_stg_sync_batch,
  PRIMARY KEY (psad_work_date, psad_nik, psad_division)
);
CREATE TABLE payroll_stg.pyr_stg_att_rekap_add (LIKE payroll_stg.pyr_stg_att_rekap_hm INCLUDING DEFAULTS);
ALTER TABLE payroll_stg.pyr_stg_att_rekap_add ADD PRIMARY KEY (psar_yymm, psar_nik, psar_division);
CREATE TABLE payroll_stg.pyr_stg_sch_rekap (pssr_yymm int, pssr_nik varchar(15), pssr_schedule varchar(5), pssr_division varchar(30), pssr_no_sch numeric(6,2),
                                   pssr_batch_id bigint, PRIMARY KEY (pssr_yymm, pssr_nik, pssr_schedule, pssr_division));

CREATE TABLE payroll.pyr_att_snapshot (
  pas_snapshot_id bigserial PRIMARY KEY,
  pas_yymm payroll.yymm NOT NULL, pas_division payroll.division NOT NULL,
  pas_source_batch_ids bigint[] NOT NULL, pas_checksum text NOT NULL,
  pas_locked boolean NOT NULL DEFAULT false,
  created_by varchar(30), created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE payroll.pyr_att_snapshot_row (
  pasr_snapshot_id bigint REFERENCES payroll.pyr_att_snapshot ON DELETE CASCADE,
  pasr_kind varchar(10) NOT NULL DEFAULT 'REKAP',       -- REKAP | ADD
  pasr_nik varchar(15) NOT NULL,
  pasr_data jsonb NOT NULL,                             -- kolom MPAR_* dibekukan
  pasr_premi_fail int, pasr_beras_fail int,                  -- turunan dari pyr_stg_att_daily
  PRIMARY KEY (pasr_snapshot_id, pasr_kind, pasr_nik)
);

