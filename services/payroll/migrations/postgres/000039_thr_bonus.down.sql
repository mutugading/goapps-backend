-- [T024a] Balikan 000039 (urutan terbalik; CASCADE tidak dipakai supaya sisa objek ketahuan).
DROP VIEW IF EXISTS payroll.v_pyr_thr_detail;
DROP VIEW IF EXISTS payroll.v_pyr_thr_main;
ALTER TABLE payroll.pyr_subsidy_rule DROP COLUMN IF EXISTS psr_applies_to;
ALTER TABLE payroll.pyr_thr_event DROP CONSTRAINT IF EXISTS pyr_thr_event_pte_active_run_id_fkey;
DROP TABLE IF EXISTS payroll.pyr_thr_detail, payroll.pyr_thr_head, payroll.pyr_thr_run, payroll.pyr_thr_override,
  payroll.pyr_thr_input, payroll.pyr_thr_event, payroll.pyr_thr_scheme_param, payroll.pyr_thr_scheme_component,
  payroll.pyr_thr_scheme;
DROP FUNCTION IF EXISTS payroll.fn_guard_thr_closed();
