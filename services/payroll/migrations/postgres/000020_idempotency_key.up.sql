-- [T007e] Idempotency-Key: hasil POST disimpan 24 jam (07 Konvensi umum; D-19).
-- Per user (bukan per company/divisi); scope_hash (scope + permission) menolak replay bila hak user berubah;
-- claim_id mencegah request lama yang macet menimpa klaim pengambil-alih.
CREATE TABLE payroll.pyr_idempotency_key (
  pik_user_id      varchar(64)  NOT NULL,
  pik_method       varchar(200) NOT NULL,
  pik_key          varchar(128) NOT NULL,
  pik_request_hash char(64)     NOT NULL,
  pik_scope_hash   char(64)     NOT NULL,
  pik_claim_id     uuid         NOT NULL DEFAULT gen_random_uuid(),
  pik_status       varchar(12)  NOT NULL CHECK (pik_status IN ('IN_PROGRESS', 'DONE')),
  pik_response     bytea,
  pik_created_at   timestamptz  NOT NULL DEFAULT now(),
  pik_expires_at   timestamptz  NOT NULL DEFAULT now() + interval '24 hours',
  PRIMARY KEY (pik_user_id, pik_method, pik_key)
);
CREATE INDEX ix_pyr_idempotency_expires ON payroll.pyr_idempotency_key (pik_expires_at);

ALTER TABLE payroll.pyr_idempotency_key ENABLE ROW LEVEL SECURITY;
CREATE POLICY p_self ON payroll.pyr_idempotency_key TO payroll_app
  USING (pik_user_id = current_setting('app.user_id', true))
  WITH CHECK (pik_user_id = current_setting('app.user_id', true));
GRANT SELECT, INSERT, UPDATE, DELETE ON payroll.pyr_idempotency_key TO payroll_app;
GRANT SELECT ON payroll.pyr_idempotency_key TO payroll_ro;
