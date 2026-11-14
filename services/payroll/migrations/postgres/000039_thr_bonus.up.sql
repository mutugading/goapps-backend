-- [T024a] D-56 (opsi A): DDL 04§19 THR & Bonus apa adanya, dengan adaptasi:
--   * aktor (created_by/approved_by/decided_by/closed_by/started_by) varchar(64), bukan 30 (D-54, 000037);
--   * pyr_thr_detail.ptd_component_sysid NULLABLE (THR id 8/800/36 tidak punya padanan master, TB6; wajib
--     ptd_meta.legacy_unmapped=true). PK komposit lama tidak bisa memuat NULL: diganti ptd_row_id + unique parsial;
--   * v_pyr_thr_detail.td_jenis = ptd_type::text (enum comp_type sudah 'Tunjangan'/'Potongan'; CASE 'T' di spec salah);
--   * view security_invoker (pola 000016/000018), RLS p_scope/p_parent (D-15), p_etl_* LEGACY (pola 000031),
--     guard CLOSED fn_guard_thr_closed (pola 000038).
-- Tabel skema/input/override ikut dibuat tetapi kosong sampai B-THR.
CREATE TABLE payroll.pyr_thr_scheme (
  pts_id bigserial PRIMARY KEY,
  pts_company_id smallint NOT NULL REFERENCES payroll.pyr_company,
  pts_kind varchar(5) NOT NULL CHECK (pts_kind IN ('THR','BONUS')),
  pts_division varchar(30),                                      -- NULL = semua divisi
  pts_name varchar(100) NOT NULL, pts_version int NOT NULL,
  pts_valid_from payroll.yymm NOT NULL, pts_valid_to payroll.yymm,
  pts_eligibility_expr text NOT NULL DEFAULT 'EMP.sts_payroll == 2',
  pts_factor_expr text,
  pts_net_rounding smallint NOT NULL DEFAULT -2,                 -- round(net, -2), selisih → kolom rounding
  pts_pph_method varchar(20) NOT NULL DEFAULT 'TER_GROSS' CHECK (pts_pph_method IN ('TER_GROSS','TER_INCREMENTAL','NONE')),  -- 14 §2.5
  pts_subsidy_enabled boolean NOT NULL DEFAULT true,
  pts_status varchar(20) NOT NULL DEFAULT 'DRAFT' CHECK (pts_status IN ('DRAFT','PENDING_APPROVAL','ACTIVE','RETIRED')),
  pts_change_reason text,
  created_by varchar(64) NOT NULL, approved_by varchar(64), approved_at timestamptz,
  CHECK (approved_by IS NULL OR approved_by <> created_by),
  UNIQUE NULLS NOT DISTINCT (pts_company_id, pts_kind, pts_division, pts_version)
);
CREATE TABLE payroll.pyr_thr_scheme_component (
  ptsc_scheme_id bigint NOT NULL REFERENCES payroll.pyr_thr_scheme ON DELETE CASCADE,
  ptsc_component_sysid bigint NOT NULL REFERENCES payroll.pyr_component,
  ptsc_expression text NOT NULL, ptsc_remark_template text, ptsc_serial int NOT NULL DEFAULT 0, ptsc_tax boolean NOT NULL,
  PRIMARY KEY (ptsc_scheme_id, ptsc_component_sysid)
);
CREATE TABLE payroll.pyr_thr_scheme_param (
  ptsp_scheme_id bigint NOT NULL REFERENCES payroll.pyr_thr_scheme ON DELETE CASCADE,
  ptsp_key varchar(40) NOT NULL,
  ptsp_value numeric(20,6), ptsp_tier jsonb,                           -- tier: [{"gte":20,"value":200000}, …]
  PRIMARY KEY (ptsp_scheme_id, ptsp_key)
);
CREATE TABLE payroll.pyr_thr_event (
  pte_id bigserial PRIMARY KEY,
  pte_company_id smallint NOT NULL REFERENCES payroll.pyr_company, pte_division payroll.division NOT NULL,
  pte_kind varchar(5) NOT NULL CHECK (pte_kind IN ('THR','BONUS')),
  pte_yymm payroll.yymm NOT NULL, pte_event_date date NOT NULL, pte_pay_date date,
  pte_description varchar(255) NOT NULL,
  pte_scheme_id bigint REFERENCES payroll.pyr_thr_scheme,
  pte_decision varchar(10) NOT NULL DEFAULT 'GIVEN' CHECK (pte_decision IN ('GIVEN','NOT_GIVEN')),
  pte_decision_note text, pte_decided_by varchar(64),
  pte_status varchar(12) NOT NULL DEFAULT 'DRAFT'
         CHECK (pte_status IN ('DRAFT','CALCULATED','APPROVED','CLOSED','CANCELLED')),
  pte_active_run_id bigint,
  pte_pay_with_payroll boolean NOT NULL DEFAULT false,           -- fase lanjut (14 §2.7)
  created_by varchar(64) NOT NULL, approved_by varchar(64), approved_at timestamptz,
  closed_by varchar(64), closed_at timestamptz, pte_reopen_reason text,
  CHECK (pte_decision = 'GIVEN' OR (pte_scheme_id IS NULL AND pte_active_run_id IS NULL)),
  UNIQUE (pte_company_id, pte_division, pte_kind, pte_yymm)
);
CREATE TABLE payroll.pyr_thr_input (                 -- pengganti MGT_THR_ADDITION (T41, T42, P3, …)
  pti_event_id bigint NOT NULL REFERENCES payroll.pyr_thr_event,
  pti_employee_id bigint NOT NULL REFERENCES payroll.pyr_employee (pe_id),
  pti_component_sysid bigint NOT NULL REFERENCES payroll.pyr_component,
  pti_amount payroll.money_idr NOT NULL, pti_remarks varchar(255),
  pti_source varchar(10) NOT NULL DEFAULT 'MANUAL' CHECK (pti_source IN ('MANUAL','UPLOAD','MIGRASI')),
  created_by varchar(64), created_at timestamptz DEFAULT now(),
  PRIMARY KEY (pti_event_id, pti_employee_id, pti_component_sysid)
);
CREATE TABLE payroll.pyr_thr_override (
  pto_event_id bigint NOT NULL REFERENCES payroll.pyr_thr_event,
  pto_employee_id bigint NOT NULL REFERENCES payroll.pyr_employee (pe_id),
  pto_action varchar(8) NOT NULL CHECK (pto_action IN ('INCLUDE','EXCLUDE','FACTOR','AMOUNT')),
  pto_component_sysid bigint REFERENCES payroll.pyr_component,       -- wajib untuk AMOUNT
  pto_value numeric(20,6), pto_reason text NOT NULL,
  created_by varchar(64) NOT NULL, approved_by varchar(64),
  CHECK (approved_by IS NULL OR approved_by <> created_by),
  CHECK (pto_action <> 'AMOUNT' OR pto_component_sysid IS NOT NULL),
  UNIQUE NULLS NOT DISTINCT (pto_event_id, pto_employee_id, pto_action, pto_component_sysid)
);
CREATE TABLE payroll.pyr_thr_run (
  ptr_id bigserial PRIMARY KEY,
  ptr_event_id bigint NOT NULL REFERENCES payroll.pyr_thr_event,
  ptr_kind varchar(8) NOT NULL DEFAULT 'NORMAL' CHECK (ptr_kind IN ('NORMAL','LEGACY')),
  ptr_scheme_id bigint REFERENCES payroll.pyr_thr_scheme, ptr_rule_set_id int REFERENCES payroll.pyr_rule_set,
  ptr_status payroll.run_status NOT NULL,
  ptr_input_hash text, ptr_emp_count int, ptr_tot_tunjangan payroll.money_idr, ptr_tot_potongan payroll.money_idr,
  ptr_warnings jsonb, ptr_started_by varchar(64), ptr_started_at timestamptz NOT NULL DEFAULT now(), ptr_finished_at timestamptz
);
CREATE UNIQUE INDEX uix_ptr_one_active ON payroll.pyr_thr_run (ptr_event_id) WHERE ptr_status = 'ACTIVE';
ALTER TABLE payroll.pyr_thr_event ADD FOREIGN KEY (pte_active_run_id) REFERENCES payroll.pyr_thr_run;

CREATE TABLE payroll.pyr_thr_head (                -- ≈ MGT_THR_MAIN
  pth_run_id bigint NOT NULL REFERENCES payroll.pyr_thr_run ON DELETE CASCADE,
  pth_employee_id bigint NOT NULL REFERENCES payroll.pyr_employee (pe_id),
  pth_nik varchar(15) NOT NULL, pth_nama varchar(255), pth_dept varchar(255), pth_emp_type varchar(50),
  pth_date_join date, pth_tax_code varchar(5),
  pth_gapok payroll.money_idr, pth_wy numeric(10,4), pth_factor numeric(10,4),
  pth_tot_tunjangan payroll.money_idr NOT NULL, pth_tot_potongan payroll.money_idr NOT NULL,
  pth_rounding payroll.money_idr NOT NULL,
  pth_net_pay payroll.money_idr GENERATED ALWAYS AS (pth_tot_tunjangan - pth_tot_potongan - pth_rounding) STORED,
  PRIMARY KEY (pth_run_id, pth_employee_id)
);
CREATE TABLE payroll.pyr_thr_detail (                  -- ≈ MGT_THR_DETAIL
  ptd_row_id bigserial PRIMARY KEY,                                    -- D-56: sysid nullable, jadi tidak bisa jadi PK
  ptd_run_id bigint NOT NULL REFERENCES payroll.pyr_thr_run ON DELETE CASCADE,
  ptd_employee_id bigint NOT NULL REFERENCES payroll.pyr_employee (pe_id),
  ptd_nik varchar(15) NOT NULL,
  ptd_component_sysid bigint REFERENCES payroll.pyr_component,         -- NULL = legacy tanpa padanan master (D-56)
  ptd_type payroll.comp_type NOT NULL, ptd_id int NOT NULL,           -- kode legacy (TD_JENIS / TD_NO)
  ptd_description varchar(255), ptd_amount payroll.money_idr NOT NULL,
  ptd_basis_amount payroll.money_idr, ptd_rate numeric(18,6), ptd_tax boolean,
  ptd_remarks varchar(1000), ptd_meta jsonb,                          -- override, rule subsidi, dsb.
  CHECK (ptd_component_sysid IS NOT NULL OR ptd_meta ->> 'legacy_unmapped' = 'true')
);
CREATE INDEX ix_ptd_run_emp ON payroll.pyr_thr_detail (ptd_run_id, ptd_employee_id, ptd_type, ptd_id);
CREATE UNIQUE INDEX uix_ptd_comp ON payroll.pyr_thr_detail (ptd_run_id, ptd_employee_id, ptd_component_sysid) WHERE ptd_component_sysid IS NOT NULL;

-- Subsidi PPh THR/Bonus memakai pyr_subsidy_rule yang sama (14 §2.5)
ALTER TABLE payroll.pyr_subsidy_rule
  ADD COLUMN psr_applies_to varchar(6) NOT NULL DEFAULT 'SALARY' CHECK (psr_applies_to IN ('SALARY','THR','BONUS'));
-- basis_fallback §16 diperluas: 'LATEST_BEFORE' = yymm terakhir < periode berjalan (fallback legacy FNC_MGT_THR_PPH)

-- View kompatibilitas: hanya run ACTIVE; dibaca PPh Desember (06 §6.2) dan PPHSLIP01
CREATE VIEW payroll.v_pyr_thr_main WITH (security_barrier) AS
SELECT e.pte_yymm tm_yymm, initcap(e.pte_kind) tm_type, h.pth_nik tm_nik, e.pte_division tm_division,
       e.pte_description tm_description, e.pte_event_date tm_date, h.pth_nama tm_nama, h.pth_gapok tm_gapok,
       h.pth_tot_tunjangan tm_tot_tunjangan, h.pth_tot_potongan tm_tot_potongan, h.pth_rounding tm_rounding,
       h.pth_net_pay tm_tot_thr, e.pte_status = 'CLOSED' AS is_closed, h.pth_employee_id
FROM payroll.pyr_thr_head h
JOIN payroll.pyr_thr_event e ON e.pte_active_run_id = h.pth_run_id;

CREATE VIEW payroll.v_pyr_thr_detail WITH (security_barrier) AS
SELECT e.pte_yymm td_yymm, initcap(e.pte_kind) td_type, l.ptd_nik td_nik,
       l.ptd_type::text td_jenis, l.ptd_id td_no,
       e.pte_division td_division, l.ptd_description td_description, l.ptd_amount td_amount, l.ptd_tax td_tax,
       e.pte_status = 'CLOSED' AS is_closed, l.ptd_employee_id
FROM payroll.pyr_thr_detail l
JOIN payroll.pyr_thr_event e ON e.pte_active_run_id = l.ptd_run_id;

ALTER VIEW payroll.v_pyr_thr_main   SET (security_invoker = true);
ALTER VIEW payroll.v_pyr_thr_detail SET (security_invoker = true);

-- ---------------------------------------------------------------------------
-- RLS (D-15, I20)
-- ---------------------------------------------------------------------------
DO $$
DECLARE t record;
BEGIN
  FOR t IN SELECT * FROM (VALUES
    ('pyr_thr_scheme', 'pts_company_id', 'pts_division'),
    ('pyr_thr_event',  'pte_company_id', 'pte_division')
  ) AS v(tbl, co, dv) LOOP
    EXECUTE format('ALTER TABLE payroll.%I ENABLE ROW LEVEL SECURITY', t.tbl);
    EXECUTE format(
      'CREATE POLICY p_scope ON payroll.%I TO payroll_app, payroll_ro '
      'USING (payroll.fn_in_scope(%I, %I)) WITH CHECK (payroll.fn_in_scope(%I, %I))',
      t.tbl, t.co, t.dv, t.co, t.dv);
  END LOOP;
  FOR t IN SELECT * FROM (VALUES
    ('pyr_thr_scheme_component', 'payroll.pyr_thr_scheme s WHERE s.pts_id = ptsc_scheme_id'),
    ('pyr_thr_scheme_param',     'payroll.pyr_thr_scheme s WHERE s.pts_id = ptsp_scheme_id'),
    ('pyr_thr_input',            'payroll.pyr_thr_event e WHERE e.pte_id = pti_event_id'),
    ('pyr_thr_override',         'payroll.pyr_thr_event e WHERE e.pte_id = pto_event_id'),
    ('pyr_thr_run',              'payroll.pyr_thr_event e WHERE e.pte_id = ptr_event_id'),
    ('pyr_thr_head',             'payroll.pyr_thr_run r WHERE r.ptr_id = pth_run_id'),
    ('pyr_thr_detail',           'payroll.pyr_thr_run r WHERE r.ptr_id = ptd_run_id')
  ) AS v(tbl, parent) LOOP
    EXECUTE format('ALTER TABLE payroll.%I ENABLE ROW LEVEL SECURITY', t.tbl);
    EXECUTE format(
      'CREATE POLICY p_parent ON payroll.%I TO payroll_app, payroll_ro '
      'USING (EXISTS (SELECT 1 FROM %s)) WITH CHECK (EXISTS (SELECT 1 FROM %s))',
      t.tbl, t.parent, t.parent);
  END LOOP;
END $$;

-- ---------------------------------------------------------------------------
-- Grant
-- ---------------------------------------------------------------------------

-- payroll_etl (pola 000031): hanya event CLOSED + run LEGACY + anaknya. Head/detail INSERT saja (tidak bisa
-- membaca nominal, I10). Event/run boleh dibaca untuk idempotensi; pte_active_run_id di-set sesudah run dibuat.

CREATE POLICY p_etl_sel ON payroll.pyr_thr_event FOR SELECT TO payroll_etl USING (true);
CREATE POLICY p_etl_ins ON payroll.pyr_thr_event FOR INSERT TO payroll_etl
  WITH CHECK (pte_status = 'CLOSED' AND pte_decision = 'GIVEN' AND pte_scheme_id IS NULL);
CREATE POLICY p_etl_upd ON payroll.pyr_thr_event FOR UPDATE TO payroll_etl
  USING (EXISTS (SELECT 1 FROM payroll.pyr_thr_run r WHERE r.ptr_event_id = pte_id AND r.ptr_kind = 'LEGACY'))
  WITH CHECK (true);
CREATE POLICY p_etl_sel ON payroll.pyr_thr_run FOR SELECT TO payroll_etl USING (true);
CREATE POLICY p_etl_ins ON payroll.pyr_thr_run FOR INSERT TO payroll_etl WITH CHECK (ptr_kind = 'LEGACY');
CREATE POLICY p_etl_upd ON payroll.pyr_thr_run FOR UPDATE TO payroll_etl USING (ptr_kind = 'LEGACY') WITH CHECK (ptr_kind = 'LEGACY');
CREATE POLICY p_etl_ins ON payroll.pyr_thr_head FOR INSERT TO payroll_etl
  WITH CHECK (EXISTS (SELECT 1 FROM payroll.pyr_thr_run r WHERE r.ptr_id = pth_run_id AND r.ptr_kind = 'LEGACY'));
CREATE POLICY p_etl_ins ON payroll.pyr_thr_detail FOR INSERT TO payroll_etl
  WITH CHECK (EXISTS (SELECT 1 FROM payroll.pyr_thr_run r WHERE r.ptr_id = ptd_run_id AND r.ptr_kind = 'LEGACY'));

-- payroll_app tidak boleh membuat/ubah run LEGACY atau event langsung CLOSED (hanya ETL; review T024a).
CREATE POLICY p_no_legacy_ins ON payroll.pyr_thr_run AS RESTRICTIVE FOR INSERT TO payroll_app WITH CHECK (ptr_kind <> 'LEGACY');
CREATE POLICY p_no_legacy_upd ON payroll.pyr_thr_run AS RESTRICTIVE FOR UPDATE TO payroll_app USING (ptr_kind <> 'LEGACY') WITH CHECK (ptr_kind <> 'LEGACY');
CREATE POLICY p_no_closed_ins ON payroll.pyr_thr_event AS RESTRICTIVE FOR INSERT TO payroll_app WITH CHECK (pte_status <> 'CLOSED');

-- ---------------------------------------------------------------------------
-- Guard CLOSED (lapis kedua; pola 000038). Event CLOSED tidak boleh diubah, begitu pula run/head/detail/
-- input/override-nya, KECUALI event yang menyangkut run LEGACY (histori Oracle diisi ETL sesudah event
-- dibuat CLOSED dan tidak bisa di-reopen).
-- ---------------------------------------------------------------------------
CREATE FUNCTION payroll.fn_guard_thr_closed() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path = payroll, pg_catalog AS $$
DECLARE v_ev bigint; v_sts varchar; v_legacy boolean := false;
BEGIN
  IF TG_TABLE_NAME = 'pyr_thr_event' THEN
    v_ev := COALESCE(OLD.pte_id, NEW.pte_id);
    IF TG_OP = 'INSERT' THEN RETURN NEW; END IF;
    v_sts := OLD.pte_status;
  ELSIF TG_TABLE_NAME = 'pyr_thr_run' THEN
    v_ev := COALESCE(NEW.ptr_event_id, OLD.ptr_event_id);
  ELSIF TG_TABLE_NAME = 'pyr_thr_head' THEN
    SELECT r.ptr_event_id INTO v_ev FROM payroll.pyr_thr_run r WHERE r.ptr_id = COALESCE(NEW.pth_run_id, OLD.pth_run_id);
  ELSIF TG_TABLE_NAME = 'pyr_thr_detail' THEN
    SELECT r.ptr_event_id INTO v_ev FROM payroll.pyr_thr_run r WHERE r.ptr_id = COALESCE(NEW.ptd_run_id, OLD.ptd_run_id);
  ELSIF TG_TABLE_NAME = 'pyr_thr_input' THEN
    v_ev := COALESCE(NEW.pti_event_id, OLD.pti_event_id);
  ELSIF TG_TABLE_NAME = 'pyr_thr_override' THEN
    v_ev := COALESCE(NEW.pto_event_id, OLD.pto_event_id);
  END IF;
  IF v_sts IS NULL THEN
    SELECT pte_status INTO v_sts FROM payroll.pyr_thr_event WHERE pte_id = v_ev;
  END IF;
  IF v_sts = 'CLOSED' THEN
    -- Pengecualian histori LEGACY hanya untuk ETL. session_user (bukan current_user: SECURITY DEFINER) = login
    -- sebenarnya; payroll_app tidak bisa memicunya walau membuat run LEGACY (juga ditolak p_no_legacy_*).
    IF session_user = 'payroll_etl' THEN
      SELECT EXISTS (SELECT 1 FROM payroll.pyr_thr_run l WHERE l.ptr_event_id = v_ev AND l.ptr_kind = 'LEGACY') INTO v_legacy;
      IF TG_TABLE_NAME = 'pyr_thr_run' AND TG_OP <> 'DELETE' THEN -- run LEGACY pertama (belum ada barisnya)
        IF NEW.ptr_kind = 'LEGACY' THEN v_legacy := true; END IF;
      END IF;
      IF v_legacy AND TG_TABLE_NAME NOT IN ('pyr_thr_input', 'pyr_thr_override') THEN
        RETURN COALESCE(NEW, OLD);
      END IF;
    END IF;
    RAISE EXCEPTION 'Event THR/Bonus % sudah CLOSED, data tidak boleh diubah', v_ev USING ERRCODE = 'P0001';
  END IF;
  RETURN COALESCE(NEW, OLD);
END $$;

CREATE TRIGGER trg_guard_thr_event    BEFORE UPDATE OR DELETE ON payroll.pyr_thr_event FOR EACH ROW EXECUTE FUNCTION payroll.fn_guard_thr_closed();
CREATE TRIGGER trg_guard_thr_run      BEFORE INSERT OR UPDATE OR DELETE ON payroll.pyr_thr_run      FOR EACH ROW EXECUTE FUNCTION payroll.fn_guard_thr_closed();
CREATE TRIGGER trg_guard_thr_head     BEFORE INSERT OR UPDATE OR DELETE ON payroll.pyr_thr_head     FOR EACH ROW EXECUTE FUNCTION payroll.fn_guard_thr_closed();
CREATE TRIGGER trg_guard_thr_detail   BEFORE INSERT OR UPDATE OR DELETE ON payroll.pyr_thr_detail   FOR EACH ROW EXECUTE FUNCTION payroll.fn_guard_thr_closed();
CREATE TRIGGER trg_guard_thr_input    BEFORE INSERT OR UPDATE OR DELETE ON payroll.pyr_thr_input    FOR EACH ROW EXECUTE FUNCTION payroll.fn_guard_thr_closed();
CREATE TRIGGER trg_guard_thr_override BEFORE INSERT OR UPDATE OR DELETE ON payroll.pyr_thr_override FOR EACH ROW EXECUTE FUNCTION payroll.fn_guard_thr_closed();
