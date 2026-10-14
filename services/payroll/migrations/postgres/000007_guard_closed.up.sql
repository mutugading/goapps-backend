-- [T003] 04§9 (sumber: spec 04, baris dipotong apa adanya; D-04)
-- ---------------------------------------------------------------------------
-- 9. GUARD PERIODE CLOSED (pengganti TRG_*_DEL) — lapis kedua setelah service
-- ---------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION payroll.fn_guard_closed() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path = payroll, pg_catalog AS $$
DECLARE v_yymm int; v_div text; v_sts payroll.period_status;
BEGIN
  IF TG_TABLE_NAME = 'pyr_salary_head' THEN
    SELECT r.pr_yymm, r.pr_division INTO v_yymm, v_div FROM payroll.pyr_run r WHERE r.pr_id = COALESCE(NEW.psh_run_id, OLD.psh_run_id);
  ELSIF TG_TABLE_NAME = 'pyr_salary_detail' THEN
    SELECT r.pr_yymm, r.pr_division INTO v_yymm, v_div FROM payroll.pyr_run r WHERE r.pr_id = COALESCE(NEW.psd_run_id, OLD.psd_run_id);
  ELSIF TG_TABLE_NAME = 'pyr_addition' THEN
    v_yymm := COALESCE(NEW.pa_yymm, OLD.pa_yymm); v_div := COALESCE(NEW.pa_division, OLD.pa_division);
  END IF;
  SELECT pp_status INTO v_sts FROM payroll.pyr_period WHERE pp_yymm = v_yymm AND pp_division = v_div;
  IF v_sts = 'CLOSED' THEN
    RAISE EXCEPTION 'Periode % % sudah CLOSED, data tidak boleh diubah', v_yymm, v_div USING ERRCODE = 'P0001';
  END IF;
  RETURN COALESCE(NEW, OLD);
END $$;
CREATE TRIGGER trg_guard_line     BEFORE INSERT OR UPDATE OR DELETE ON payroll.pyr_salary_detail   FOR EACH ROW EXECUTE FUNCTION payroll.fn_guard_closed();
CREATE TRIGGER trg_guard_header   BEFORE INSERT OR UPDATE OR DELETE ON payroll.pyr_salary_head FOR EACH ROW EXECUTE FUNCTION payroll.fn_guard_closed();
CREATE TRIGGER trg_guard_addition BEFORE INSERT OR UPDATE OR DELETE ON payroll.pyr_addition       FOR EACH ROW EXECUTE FUNCTION payroll.fn_guard_closed();
REVOKE ALL ON FUNCTION payroll.fn_guard_closed() FROM PUBLIC;

