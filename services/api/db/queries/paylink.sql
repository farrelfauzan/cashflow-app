-- Tenant payment links (slice 3, decision 39) and the bank account the payment page shows
-- (decision 38).

-- name: LookupPaymentLink :one
-- Maps a token hash to its link's organization and id, with no organization scope, through the
-- SECURITY DEFINER function payment_link_lookup. Revoked links are found too; whether a link
-- still works is decided inside the organization scope.
SELECT l.organization_id::uuid AS organization_id, l.link_id::uuid AS link_id
FROM payment_link_lookup(@token_hash::bytea) AS l;

-- name: InsertPaymentLink :exec
INSERT INTO payment_link (id, organization_id, invoice_id, tenant_id, token_hash, created_at, created_by_staff_user_id)
VALUES (@id, @organization_id, @invoice_id, @tenant_id, @token_hash, @created_at, sqlc.narg('created_by_staff_user_id'));

-- name: RevokePaymentLinks :execrows
-- Revokes every link of an invoice not revoked yet; returns how many it revoked.
UPDATE payment_link SET revoked_at = @revoked_at
WHERE organization_id = @organization_id AND invoice_id = @invoice_id AND revoked_at IS NULL;

-- name: GetPaymentLink :one
SELECT id, invoice_id, tenant_id, revoked_at
FROM payment_link
WHERE organization_id = @organization_id AND id = @id;

-- name: GetInvoiceSettlement :one
-- What an invoice still owes, and when it was last settled: the newest allocation or correction
-- on it, or the invoice's own creation when nothing touched it yet. Once nothing is owed that is
-- the moment it became paid (or was cancelled by a correction), from which its payment links stay
-- valid for 30 days.
SELECT b.outstanding,
       coalesce(greatest(
           (SELECT max(a.created_at) FROM payment_allocation a
            WHERE a.organization_id = b.organization_id AND a.invoice_id = b.invoice_id),
           (SELECT max(c.created_at) FROM correction c
            WHERE c.organization_id = b.organization_id AND c.invoice_id = b.invoice_id)
       ), i.created_at)::timestamptz AS settled_at
FROM invoice_balance b
JOIN invoice i ON i.organization_id = b.organization_id AND i.id = b.invoice_id
WHERE b.organization_id = @organization_id AND b.invoice_id = @invoice_id;

-- name: UpsertBankAccount :one
INSERT INTO organization_bank_account (
    organization_id, bank_name, account_number, account_holder, created_at, updated_at, updated_by_staff_user_id
) VALUES (
    @organization_id, @bank_name, @account_number, @account_holder, @now, @now, sqlc.narg('updated_by_staff_user_id')
)
ON CONFLICT (organization_id) DO UPDATE
SET bank_name                = EXCLUDED.bank_name,
    account_number           = EXCLUDED.account_number,
    account_holder           = EXCLUDED.account_holder,
    updated_at               = EXCLUDED.updated_at,
    updated_by_staff_user_id = EXCLUDED.updated_by_staff_user_id
RETURNING *;

-- name: GetBankAccount :one
SELECT * FROM organization_bank_account WHERE organization_id = @organization_id;
