-- Runs in an organization scope (store.WithOrgTx). invoice, invoice_line, payment,
-- payment_allocation and correction are append-only money rows: there is no UPDATE or DELETE
-- here, the app role has no privilege for them, and a trigger rejects them for every role.

-- name: CreateInvoice :one
-- An invoice's lines are inserted after it in the same transaction; at commit they must add up
-- to amount.
INSERT INTO invoice (id, organization_id, lease_id, tenant_id, period, period_end, due_date, amount)
VALUES (@id, @organization_id, @lease_id, @tenant_id, @period, @period_end, @due_date, @amount)
RETURNING *;

-- name: CreateInvoiceLine :one
INSERT INTO invoice_line (id, organization_id, invoice_id, kind, charge_type_id, label, amount)
VALUES (@id, @organization_id, @invoice_id, @kind, @charge_type_id, @label, @amount)
RETURNING *;

-- name: CreatePayment :one
INSERT INTO payment (id, organization_id, tenant_id, amount, method, received_at, source, source_ref,
                     recorded_by_staff_user_id, note)
VALUES (@id, @organization_id, @tenant_id, @amount, @method, @received_at, @source, @source_ref,
        @recorded_by_staff_user_id, @note)
RETURNING *;

-- name: CreatePaymentAllocation :exec
INSERT INTO payment_allocation (id, organization_id, payment_id, invoice_id, tenant_id, amount)
VALUES (@id, @organization_id, @payment_id, @invoice_id, @tenant_id, @amount);

-- name: CreateCorrection :one
-- Exactly one of invoice_id and payment_id is set. A trigger refuses a correction that would
-- make the invoice's billed amount or the payment's net amount negative.
INSERT INTO correction (id, organization_id, invoice_id, payment_id, amount_delta, reason, created_by_staff_user_id)
VALUES (@id, @organization_id, @invoice_id, @payment_id, @amount_delta, @reason, @created_by_staff_user_id)
RETURNING *;

-- name: LockTenantForAllocation :one
-- Serializes allocation per tenant. FOR NO KEY UPDATE, not FOR UPDATE: inserting a payment or
-- an invoice takes FOR KEY SHARE on the tenant through its foreign key, which FOR UPDATE would
-- wait on, so two transactions that each inserted a payment and then locked would deadlock.
SELECT id FROM tenant
WHERE organization_id = @organization_id AND id = @tenant_id
FOR NO KEY UPDATE;

-- name: ListAllocationPayments :many
-- A tenant's payments with their net amount (amount plus its corrections), oldest first.
SELECT p.id, p.seq, p.received_at,
       (p.amount + coalesce((SELECT sum(c.amount_delta) FROM correction c WHERE c.payment_id = p.id), 0))::bigint AS net
FROM payment p
WHERE p.organization_id = @organization_id AND p.tenant_id = @tenant_id
ORDER BY p.received_at, p.seq;

-- name: ListAllocationInvoices :many
-- A tenant's invoices with what each bills after corrections, cancelled ones included, oldest
-- period first.
SELECT invoice_id, seq, period, billed
FROM invoice_balance
WHERE organization_id = @organization_id AND tenant_id = @tenant_id
ORDER BY period, seq;

-- name: ListTenantAllocations :many
-- Every allocation row of a tenant, in the order they were written.
SELECT seq, payment_id, invoice_id, amount
FROM payment_allocation
WHERE organization_id = @organization_id AND tenant_id = @tenant_id
ORDER BY seq;

-- name: ListInvoiceBalancesByTenant :many
SELECT * FROM invoice_balance
WHERE organization_id = @organization_id AND tenant_id = @tenant_id
ORDER BY period, seq;

-- name: GetTenantBalance :one
SELECT * FROM tenant_balance
WHERE organization_id = @organization_id AND tenant_id = @tenant_id;

-- name: ListInvoiceLines :many
SELECT * FROM invoice_line
WHERE organization_id = @organization_id AND invoice_id = @invoice_id
ORDER BY seq;

-- name: ActiveOrganizationIDs :many
-- Every organization's id, for scheduled jobs that then run one task per organization. Callable
-- without an organization scope; the only cross-organization read.
SELECT id::uuid AS id FROM active_organization_ids() AS id;
