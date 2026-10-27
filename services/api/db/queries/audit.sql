-- Runs in an organization scope (store.WithOrgTx). There is no UPDATE or DELETE: the table is
-- append-only and the app role has no privilege to change rows.

-- name: InsertAuditLog :exec
INSERT INTO audit_log (id, organization_id, actor_staff_user_id, action, resource, decision, reason, detail, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: ListAuditLog :many
SELECT * FROM audit_log
WHERE organization_id = $1
ORDER BY seq DESC
LIMIT $2;
