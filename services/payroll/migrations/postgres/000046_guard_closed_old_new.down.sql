-- [T033e] Balikan 000046: guard CLOSED kembali hanya memeriksa NEW (COALESCE(NEW, OLD)), versi 000038 dan 000039.
CREATE OR REPLACE FUNCTION payroll.fn_guard_closed() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path = payroll, pg_catalog AS $$
DECLARE v_co smallint; v_yymm int; v_div text; v_sts payroll.period_status; v_snap bool := false;
BEGIN
  IF TG_TABLE_NAME = 'pyr_salary_head' THEN
    SELECT r.pr_company_id, r.pr_yymm, r.pr_division INTO v_co, v_yymm, v_div FROM payroll.pyr_run r WHERE r.pr_id = COALESCE(NEW.psh_run_id, OLD.psh_run_id);
  ELSIF TG_TABLE_NAME = 'pyr_salary_detail' THEN
    SELECT r.pr_company_id, r.pr_yymm, r.pr_division INTO v_co, v_yymm, v_div FROM payroll.pyr_run r WHERE r.pr_id = COALESCE(NEW.psd_run_id, OLD.psd_run_id);
  ELSIF TG_TABLE_NAME = 'pyr_addition' THEN
    v_co := COALESCE(NEW.pa_company_id, OLD.pa_company_id);
    v_yymm := COALESCE(NEW.pa_yymm, OLD.pa_yymm); v_div := COALESCE(NEW.pa_division, OLD.pa_division);
  ELSIF TG_TABLE_NAME = 'pyr_tax_adj' THEN
    v_co := COALESCE(NEW.pta_company_id, OLD.pta_company_id);
    v_yymm := COALESCE(NEW.pta_yymm, OLD.pta_yymm); v_div := COALESCE(NEW.pta_division, OLD.pta_division);
  ELSIF TG_TABLE_NAME = 'pyr_attd_days' THEN
    v_co := COALESCE(NEW.pad_company_id, OLD.pad_company_id);
    v_yymm := COALESCE(NEW.pad_yymm, OLD.pad_yymm); v_div := COALESCE(NEW.pad_division, OLD.pad_division);
  ELSIF TG_TABLE_NAME = 'pyr_att_snapshot' THEN
    IF TG_OP = 'UPDATE' AND (to_jsonb(NEW) - 'pas_locked') = (to_jsonb(OLD) - 'pas_locked') THEN
      RETURN NEW; -- (a) lock/unlock oleh approve/reopen
    END IF;
    v_co := COALESCE(NEW.pas_company_id, OLD.pas_company_id);
    v_yymm := COALESCE(NEW.pas_yymm, OLD.pas_yymm); v_div := COALESCE(NEW.pas_division, OLD.pas_division);
    v_snap := true;
  ELSIF TG_TABLE_NAME = 'pyr_att_snapshot_row' THEN
    SELECT s.pas_company_id, s.pas_yymm, s.pas_division INTO v_co, v_yymm, v_div
    FROM payroll.pyr_att_snapshot s WHERE s.pas_snapshot_id = COALESCE(NEW.pasr_snapshot_id, OLD.pasr_snapshot_id);
    v_snap := true;
  END IF;
  SELECT pp_status INTO v_sts FROM payroll.pyr_period WHERE pp_company_id = v_co AND pp_yymm = v_yymm AND pp_division = v_div;
  IF v_sts = 'CLOSED' THEN
    IF v_snap AND EXISTS (SELECT 1 FROM payroll.pyr_run l WHERE l.pr_company_id = v_co AND l.pr_yymm = v_yymm
                          AND l.pr_division = v_div AND l.pr_kind = 'LEGACY') THEN
      RETURN COALESCE(NEW, OLD); -- (b) snapshot sejarah legacy
    END IF;
    RAISE EXCEPTION 'Periode % % % sudah CLOSED, data tidak boleh diubah', v_co, v_yymm, v_div USING ERRCODE = 'P0001';
  END IF;
  RETURN COALESCE(NEW, OLD);
END $$;

CREATE OR REPLACE FUNCTION payroll.fn_guard_thr_closed() RETURNS trigger
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
