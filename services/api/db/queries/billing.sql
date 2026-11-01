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

-- Invoice generation (slice 2) ----------------------------------------------------------------

-- name: InsertInvoiceIfMissing :one
-- Generation's insert: a period the lease already has an invoice for inserts nothing and
-- returns no row, so a rerun creates nothing. created_at comes from the injected clock: the day
-- an invoice was made decides which unit prices may still be recorded (CreateUnitPrice).
INSERT INTO invoice (id, organization_id, lease_id, tenant_id, period, period_end, due_date, amount, created_at)
VALUES (@id, @organization_id, @lease_id, @tenant_id, @period, @period_end, @due_date, @amount, @created_at)
ON CONFLICT (lease_id, period) DO NOTHING
RETURNING id;

-- name: ListActiveLeaseIDs :many
-- The leases the generate task looks at, in a fixed order.
SELECT id FROM lease
WHERE organization_id = @organization_id AND status = 'active'
ORDER BY id;

-- name: LockLeaseForBilling :one
-- Locks one lease for generation and returns what generation needs. FOR NO KEY UPDATE OF the
-- lease only, the same lock the lease service takes (LockLease), so generation and a change of
-- the lease's dates run one after the other and each sees the other's committed rows. It does not
-- conflict with the FOR KEY SHARE that inserting an invoice takes on the lease.
SELECT l.id, l.tenant_id, l.unit_id, u.property_id, l.start_date, l.entered_on, l.billed_from,
       l.term_months, l.planned_end_date, l.end_date, l.status
FROM lease l
JOIN unit u ON u.organization_id = l.organization_id AND u.id = l.unit_id
WHERE l.organization_id = @organization_id AND l.id = @id
FOR NO KEY UPDATE OF l;

-- name: LockUnitForBilling :one
-- Taken by generation before it reads the unit's price, so a price recorded at the same moment
-- (CreateUnitPrice locks the unit FOR NO KEY UPDATE) waits and then sees the new invoice.
SELECT id FROM unit
WHERE organization_id = @organization_id AND id = @id
FOR SHARE;

-- name: ListLeaseInvoicePeriods :many
SELECT period FROM invoice
WHERE organization_id = @organization_id AND lease_id = @lease_id
ORDER BY period;

-- name: ListLeaseInvoicesForChange :many
-- A lease's invoices, oldest period first, with what each bills and owes and its latest
-- correction (an empty reason and 0 when it has none), for cancelling and restoring invoices
-- when the lease's end changes.
SELECT b.invoice_id, b.period, b.amount, b.billed, b.outstanding,
       coalesce(lc.reason, '')::text AS last_reason, coalesce(lc.amount_delta, 0)::bigint AS last_delta
FROM invoice_balance b
LEFT JOIN LATERAL (
    SELECT c.reason, c.amount_delta FROM correction c
    WHERE c.invoice_id = b.invoice_id
    ORDER BY c.seq DESC
    LIMIT 1
) lc ON true
WHERE b.organization_id = @organization_id AND b.lease_id = @lease_id
ORDER BY b.period, b.seq;

-- name: LeaseHasInvoices :one
SELECT EXISTS (
    SELECT 1 FROM invoice WHERE organization_id = @organization_id AND lease_id = @lease_id
)::boolean AS has_invoices;

-- name: NewestInvoiceCreatedAtForUnit :one
-- When the newest invoice of any lease of the unit was made; no row when it has none.
SELECT i.created_at
FROM invoice i
JOIN lease l ON l.organization_id = i.organization_id AND l.id = i.lease_id
WHERE i.organization_id = @organization_id AND l.unit_id = @unit_id
ORDER BY i.created_at DESC
LIMIT 1;
