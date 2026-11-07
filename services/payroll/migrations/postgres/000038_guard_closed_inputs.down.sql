-- [T023c] Balikan 000038: guard CLOSED kembali hanya untuk head/detail/addition (000018).
DROP TRIGGER IF EXISTS trg_guard_tax_adj ON payroll.pyr_tax_adj;
DROP TRIGGER IF EXISTS trg_guard_attd_days ON payroll.pyr_attd_days;
DROP TRIGGER IF EXISTS trg_guard_att_snapshot ON payroll.pyr_att_snapshot;
DROP TRIGGER IF EXISTS trg_guard_att_snap_row ON payroll.pyr_att_snapshot_row;

CREATE OR REPLACE FUNCTION payroll.fn_guard_closed() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path = payroll, pg_catalog AS $$
DECLARE v_co smallint; v_yymm int; v_div text; v_sts payroll.period_status;
BEGIN
  IF TG_TABLE_NAME = 'pyr_salary_head' THEN
    SELECT r.pr_company_id, r.pr_yymm, r.pr_division INTO v_co, v_yymm, v_div FROM payroll.pyr_run r WHERE r.pr_id = COALESCE(NEW.psh_run_id, OLD.psh_run_id);
  ELSIF TG_TABLE_NAME = 'pyr_salary_detail' THEN
    SELECT r.pr_company_id, r.pr_yymm, r.pr_division INTO v_co, v_yymm, v_div FROM payroll.pyr_run r WHERE r.pr_id = COALESCE(NEW.psd_run_id, OLD.psd_run_id);
  ELSIF TG_TABLE_NAME = 'pyr_addition' THEN
    v_co := COALESCE(NEW.pa_company_id, OLD.pa_company_id);
    v_yymm := COALESCE(NEW.pa_yymm, OLD.pa_yymm); v_div := COALESCE(NEW.pa_division, OLD.pa_division);
  END IF;
  SELECT pp_status INTO v_sts FROM payroll.pyr_period WHERE pp_company_id = v_co AND pp_yymm = v_yymm AND pp_division = v_div;
  IF v_sts = 'CLOSED' THEN
    RAISE EXCEPTION 'Periode % % % sudah CLOSED, data tidak boleh diubah', v_co, v_yymm, v_div USING ERRCODE = 'P0001';
  END IF;
  RETURN COALESCE(NEW, OLD);
END $$;
