-- [T033e] review #3a: guard CLOSED memeriksa kunci periode/event dari OLD **dan** NEW. Sebelumnya COALESCE(NEW, OLD)
-- hanya memeriksa NEW, sehingga UPDATE yang memindahkan baris dari periode CLOSED ke periode OPEN (key-move) lolos.
-- Pengecualian (a) pas_locked-only dan (b) snapshot legacy (000038), serta pengecualian ETL LEGACY THR (000039),
-- dipertahankan dan dievaluasi per kunci.
CREATE OR REPLACE FUNCTION payroll.fn_guard_closed() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path = payroll, pg_catalog AS $$
DECLARE
  v_rows jsonb[] := ARRAY[]::jsonb[]; v_row jsonb;
  v_co smallint; v_yymm int; v_div text; v_sts payroll.period_status; v_snap bool := false;
BEGIN
  IF TG_TABLE_NAME = 'pyr_att_snapshot' AND TG_OP = 'UPDATE'
     AND (to_jsonb(NEW) - 'pas_locked') = (to_jsonb(OLD) - 'pas_locked') THEN
    RETURN NEW; -- (a) lock/unlock oleh approve/reopen
  END IF;
  IF TG_OP IN ('UPDATE', 'DELETE') THEN v_rows := v_rows || to_jsonb(OLD); END IF;
  IF TG_OP IN ('INSERT', 'UPDATE') THEN v_rows := v_rows || to_jsonb(NEW); END IF;
  FOREACH v_row IN ARRAY v_rows LOOP
    v_co := NULL; v_yymm := NULL; v_div := NULL; v_snap := false;
    IF TG_TABLE_NAME = 'pyr_salary_head' THEN
      SELECT r.pr_company_id, r.pr_yymm, r.pr_division INTO v_co, v_yymm, v_div FROM payroll.pyr_run r WHERE r.pr_id = (v_row->>'psh_run_id')::bigint;
    ELSIF TG_TABLE_NAME = 'pyr_salary_detail' THEN
      SELECT r.pr_company_id, r.pr_yymm, r.pr_division INTO v_co, v_yymm, v_div FROM payroll.pyr_run r WHERE r.pr_id = (v_row->>'psd_run_id')::bigint;
    ELSIF TG_TABLE_NAME = 'pyr_addition' THEN
      v_co := (v_row->>'pa_company_id')::smallint; v_yymm := (v_row->>'pa_yymm')::int; v_div := v_row->>'pa_division';
    ELSIF TG_TABLE_NAME = 'pyr_tax_adj' THEN
      v_co := (v_row->>'pta_company_id')::smallint; v_yymm := (v_row->>'pta_yymm')::int; v_div := v_row->>'pta_division';
    ELSIF TG_TABLE_NAME = 'pyr_attd_days' THEN
      v_co := (v_row->>'pad_company_id')::smallint; v_yymm := (v_row->>'pad_yymm')::int; v_div := v_row->>'pad_division';
    ELSIF TG_TABLE_NAME = 'pyr_att_snapshot' THEN
      v_co := (v_row->>'pas_company_id')::smallint; v_yymm := (v_row->>'pas_yymm')::int; v_div := v_row->>'pas_division';
      v_snap := true;
    ELSIF TG_TABLE_NAME = 'pyr_att_snapshot_row' THEN
      SELECT s.pas_company_id, s.pas_yymm, s.pas_division INTO v_co, v_yymm, v_div
      FROM payroll.pyr_att_snapshot s WHERE s.pas_snapshot_id = (v_row->>'pasr_snapshot_id')::bigint;
      v_snap := true;
    END IF;
    SELECT pp_status INTO v_sts FROM payroll.pyr_period WHERE pp_company_id = v_co AND pp_yymm = v_yymm AND pp_division = v_div;
    IF v_sts = 'CLOSED' THEN
      IF v_snap AND EXISTS (SELECT 1 FROM payroll.pyr_run l WHERE l.pr_company_id = v_co AND l.pr_yymm = v_yymm
                            AND l.pr_division = v_div AND l.pr_kind = 'LEGACY') THEN
        CONTINUE; -- (b) snapshot sejarah legacy
      END IF;
      RAISE EXCEPTION 'Periode % % % sudah CLOSED, data tidak boleh diubah', v_co, v_yymm, v_div USING ERRCODE = 'P0001';
    END IF;
  END LOOP;
  RETURN COALESCE(NEW, OLD);
END $$;

CREATE OR REPLACE FUNCTION payroll.fn_guard_thr_closed() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path = payroll, pg_catalog AS $$
DECLARE
  v_rows jsonb[] := ARRAY[]::jsonb[]; v_row jsonb; v_new_legacy boolean := false;
  v_ev bigint; v_sts varchar; v_legacy boolean;
BEGIN
  IF TG_TABLE_NAME = 'pyr_thr_event' AND TG_OP = 'INSERT' THEN RETURN NEW; END IF;
  IF TG_TABLE_NAME = 'pyr_thr_event' THEN
    v_rows := ARRAY[to_jsonb(OLD)]; -- status event dinilai dari OLD
  ELSE
    IF TG_OP IN ('UPDATE', 'DELETE') THEN v_rows := v_rows || to_jsonb(OLD); END IF;
    IF TG_OP IN ('INSERT', 'UPDATE') THEN v_rows := v_rows || to_jsonb(NEW); END IF;
  END IF;
  -- run LEGACY pertama (belum ada barisnya): dinilai dari NEW untuk INSERT/UPDATE, seperti 000039.
  IF TG_TABLE_NAME = 'pyr_thr_run' AND TG_OP <> 'DELETE' THEN v_new_legacy := NEW.ptr_kind = 'LEGACY'; END IF;
  FOREACH v_row IN ARRAY v_rows LOOP
    v_sts := NULL; v_ev := NULL;
    IF TG_TABLE_NAME = 'pyr_thr_event' THEN
      v_ev := (v_row->>'pte_id')::bigint; v_sts := v_row->>'pte_status';
    ELSIF TG_TABLE_NAME = 'pyr_thr_run' THEN
      v_ev := (v_row->>'ptr_event_id')::bigint;
    ELSIF TG_TABLE_NAME = 'pyr_thr_head' THEN
      SELECT r.ptr_event_id INTO v_ev FROM payroll.pyr_thr_run r WHERE r.ptr_id = (v_row->>'pth_run_id')::bigint;
    ELSIF TG_TABLE_NAME = 'pyr_thr_detail' THEN
      SELECT r.ptr_event_id INTO v_ev FROM payroll.pyr_thr_run r WHERE r.ptr_id = (v_row->>'ptd_run_id')::bigint;
    ELSIF TG_TABLE_NAME = 'pyr_thr_input' THEN
      v_ev := (v_row->>'pti_event_id')::bigint;
    ELSIF TG_TABLE_NAME = 'pyr_thr_override' THEN
      v_ev := (v_row->>'pto_event_id')::bigint;
    END IF;
    IF v_sts IS NULL THEN
      SELECT pte_status INTO v_sts FROM payroll.pyr_thr_event WHERE pte_id = v_ev;
    END IF;
    IF v_sts = 'CLOSED' THEN
      -- Pengecualian histori LEGACY hanya untuk ETL (session_user, bukan current_user: SECURITY DEFINER).
      IF session_user = 'payroll_etl' THEN
        SELECT EXISTS (SELECT 1 FROM payroll.pyr_thr_run l WHERE l.ptr_event_id = v_ev AND l.ptr_kind = 'LEGACY') INTO v_legacy;
        IF v_new_legacy THEN v_legacy := true; END IF;
        IF v_legacy AND TG_TABLE_NAME NOT IN ('pyr_thr_input', 'pyr_thr_override') THEN
          CONTINUE;
        END IF;
      END IF;
      RAISE EXCEPTION 'Event THR/Bonus % sudah CLOSED, data tidak boleh diubah', v_ev USING ERRCODE = 'P0001';
    END IF;
  END LOOP;
  RETURN COALESCE(NEW, OLD);
END $$;
