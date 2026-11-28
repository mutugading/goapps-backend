-- [T023c] D-54: fn_guard_closed juga menjaga input periode yang dibaca loader: pyr_tax_adj, pyr_attd_days,
-- pyr_att_snapshot, pyr_att_snapshot_row. Pengecualian:
--   (a) UPDATE snapshot yang hanya mengubah pas_locked (approve/reopen, D-40);
--   (b) snapshot (dan barisnya) periode yang punya run LEGACY: sejarah Oracle diisi ETL att-legacy /
--       att-legacy-fail sesudah salary-legacy menutup periode (D-33), dan periode itu tidak bisa di-reopen (D-40).
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

CREATE TRIGGER trg_guard_tax_adj      BEFORE INSERT OR UPDATE OR DELETE ON payroll.pyr_tax_adj          FOR EACH ROW EXECUTE FUNCTION payroll.fn_guard_closed();
CREATE TRIGGER trg_guard_attd_days    BEFORE INSERT OR UPDATE OR DELETE ON payroll.pyr_attd_days        FOR EACH ROW EXECUTE FUNCTION payroll.fn_guard_closed();
CREATE TRIGGER trg_guard_att_snapshot BEFORE INSERT OR UPDATE OR DELETE ON payroll.pyr_att_snapshot     FOR EACH ROW EXECUTE FUNCTION payroll.fn_guard_closed();
CREATE TRIGGER trg_guard_att_snap_row BEFORE INSERT OR UPDATE OR DELETE ON payroll.pyr_att_snapshot_row FOR EACH ROW EXECUTE FUNCTION payroll.fn_guard_closed();
