-- Migration 000067: Access/rollout for CPR Submit+StartReview merge (P3-T7).
--
-- The Cost Product Request "Submit" and "Start review" actions are being merged
-- into a single action gated solely by `finance.product.request.review`
-- (see docs/superpowers/specs/2026-07-06-product-request-workflow-revamp-design.md
-- section 3 B3, and plan.md P3-T5/P3-T7). Any role/user that currently holds
-- `finance.product.request.submit` but NOT `finance.product.request.review`
-- would lose the ability to perform the merged action once the permission
-- check is narrowed in the delivery layer.
--
-- Audit of role_permissions (as seeded by 000050_cpr_roles_and_permissions.up.sql,
-- confirmed against the local dev IAM database on 2026-07-07) found exactly one
-- role in this situation:
--   CPR_SUBMITTER — has .submit (+ .view/.create + route.view) but not .review.
--   Assigned to test user `marketingmgr` (alongside CPR_REQUESTER).
-- All other roles holding .submit (CPR_ADMIN, SUPER_ADMIN) already hold .review
-- too, so they are unaffected. No direct user_permissions grants of .submit
-- exist independent of a role.
--
-- This migration grants `finance.product.request.review` to CPR_SUBMITTER so its
-- holders retain the ability to perform the merged Submit+StartReview action.
-- It does not touch any other role.

BEGIN;

INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r CROSS JOIN mst_permission p
WHERE r.role_code = 'CPR_SUBMITTER'
  AND r.is_active = TRUE AND p.is_active = TRUE
  AND p.permission_code = 'finance.product.request.review'
ON CONFLICT (role_id, permission_id) DO NOTHING;

COMMIT;
