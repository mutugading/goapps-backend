-- T033d (review #3c): the app-level "already exists" checks race under REPEATABLE READ (two concurrent creates both see
-- nothing), so the database is the arbiter. One live (not CANCELLED/SUPERSEDED) slip batch per company/division/doc/period,
-- and one FINAL GL export per run and journal type. Before applying on a database with data, check for old duplicates
-- (the index fails otherwise): SELECT pdb_company_id,pdb_division,pdb_doc_type,pdb_yymm,count(*) FROM payroll.pyr_distribution_batch
-- WHERE pdb_yymm IS NOT NULL AND pdb_status NOT IN ('CANCELLED','SUPERSEDED') GROUP BY 1,2,3,4 HAVING count(*)>1; likewise
-- (pge_run_id,pge_journal_type) WHERE pge_status='FINAL'. THR batches (pdb_yymm NULL) are not covered yet (no THR flow). A violation (23505) is mapped to a domain error by the services.
CREATE UNIQUE INDEX IF NOT EXISTS uix_pdb_active ON payroll.pyr_distribution_batch (pdb_company_id, pdb_division, pdb_doc_type, pdb_yymm)
  WHERE pdb_yymm IS NOT NULL AND pdb_status NOT IN ('CANCELLED','SUPERSEDED');
CREATE UNIQUE INDEX IF NOT EXISTS uix_pge_final ON payroll.pyr_gl_export (pge_run_id, pge_journal_type)
  WHERE pge_status = 'FINAL';
