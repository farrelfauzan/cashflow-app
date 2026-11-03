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
INSERT INTO invoice_line (id, organization_id, invoice_id, kind, charge_type_id, label, amount, metered_charge_id)
VALUES (@id, @organization_id, @invoice_id, @kind, @charge_type_id, @label, @amount, @metered_charge_id)
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

-- name: GetTenantOverdue :one
-- What a tenant still owes on invoices due before today (Asia/Jakarta, from the injected clock):
-- debt from the day after the due date.
SELECT coalesce(sum(outstanding), 0)::bigint AS overdue
FROM invoice_balance
WHERE organization_id = @organization_id AND tenant_id = @tenant_id
  AND outstanding > 0 AND due_date < @today::date;

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
       l.term_months, l.planned_end_date, l.end_date, l.status, l.collector_staff_user_id
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

-- Manual payments and corrections (T-052) -----------------------------------------------------

-- name: InsertPaymentIfNew :one
-- A payment with a source_ref already recorded in the organization for the same source inserts
-- nothing and returns no row; the caller then loads it with GetPaymentBySourceRef. A manual
-- payment's source_ref is the client's requestId.
INSERT INTO payment (id, organization_id, tenant_id, amount, method, received_at, source, source_ref,
                     recorded_by_staff_user_id, note)
VALUES (@id, @organization_id, @tenant_id, @amount, @method, @received_at, @source, @source_ref,
        @recorded_by_staff_user_id, @note)
ON CONFLICT (organization_id, source, source_ref) WHERE source_ref IS NOT NULL DO NOTHING
RETURNING *;

-- name: GetPaymentBySourceRef :one
SELECT * FROM payment
WHERE organization_id = @organization_id AND source = @source AND source_ref = @source_ref;

-- name: GetPayment :one
SELECT * FROM payment
WHERE organization_id = @organization_id AND id = @id;

-- name: InsertCorrectionIfNew :one
-- A correction made from the API, keyed by the client's requestId: one already recorded in the
-- organization under it inserts nothing and returns no row. The not-negative trigger runs before
-- the conflict check, so the caller looks the requestId up first (GetCorrectionByRequestID).
-- new_amount is the amount the request named, when it named one instead of a difference.
INSERT INTO correction (id, organization_id, invoice_id, payment_id, amount_delta, reason,
                        created_by_staff_user_id, request_id, new_amount)
VALUES (@id, @organization_id, @invoice_id, @payment_id, @amount_delta, @reason,
        @created_by_staff_user_id, @request_id, sqlc.narg('new_amount'))
ON CONFLICT (organization_id, request_id) WHERE request_id IS NOT NULL DO NOTHING
RETURNING *;

-- name: GetInvoiceBilled :one
-- What an invoice bills now: its amount plus its corrections. The caller holds the tenant lock,
-- so no other correction or allocation can change it before the caller's own insert.
SELECT billed FROM invoice_balance
WHERE organization_id = @organization_id AND invoice_id = @invoice_id;

-- name: GetPaymentNet :one
-- What a payment counts now: its amount plus its corrections. Read under the tenant lock.
SELECT (p.amount + coalesce((SELECT sum(c.amount_delta) FROM correction c WHERE c.payment_id = p.id), 0))::bigint AS net
FROM payment p
WHERE p.organization_id = @organization_id AND p.id = @id;

-- name: GetCorrectionByRequestID :one
SELECT * FROM correction
WHERE organization_id = @organization_id AND request_id = @request_id;

-- name: GetInvoiceScope :one
-- An invoice with what authorizing a change to it needs: its tenant, its lease's property and
-- collector.
SELECT i.id, i.tenant_id, i.lease_id, u.property_id, l.collector_staff_user_id
FROM invoice i
JOIN lease l ON l.organization_id = i.organization_id AND l.id = i.lease_id
JOIN unit u ON u.organization_id = l.organization_id AND u.id = l.unit_id
WHERE i.organization_id = @organization_id AND i.id = @id;

-- name: ListTenantLeaseScopes :many
-- Every lease of a tenant, active or ended, with its property and collector, for authorizing
-- an action on the tenant's money through each of them.
SELECT l.id, u.property_id, l.collector_staff_user_id
FROM lease l
JOIN unit u ON u.organization_id = l.organization_id AND u.id = l.unit_id
WHERE l.organization_id = @organization_id AND l.tenant_id = @tenant_id
ORDER BY l.id;

-- name: GetStaffRef :one
-- The member id and name of a staff user in the organization.
SELECT m.id AS member_id, su.name
FROM membership m
JOIN staff_user su ON su.id = m.staff_user_id
WHERE m.organization_id = @organization_id AND m.staff_user_id = @staff_user_id;

-- name: ListAccountInvoices :many
-- A tenant's invoices with their balances and where they are, oldest period first.
SELECT b.invoice_id, b.lease_id, u.label AS unit_label, p.name AS property_name,
       b.period, b.period_end, b.due_date, b.amount, b.billed, b.paid, b.outstanding, b.status,
       i.created_at
FROM invoice_balance b
JOIN invoice i ON i.organization_id = b.organization_id AND i.id = b.invoice_id
JOIN lease l ON l.organization_id = b.organization_id AND l.id = b.lease_id
JOIN unit u ON u.organization_id = l.organization_id AND u.id = l.unit_id
JOIN property p ON p.organization_id = u.organization_id AND p.id = u.property_id
WHERE b.organization_id = @organization_id AND b.tenant_id = @tenant_id
ORDER BY b.period, b.seq;

-- name: ListAccountInvoiceLines :many
-- The lines of every invoice of a tenant, in the order they were written.
SELECT il.id, il.invoice_id, il.kind, il.label, il.amount, il.metered_charge_id
FROM invoice_line il
JOIN invoice i ON i.organization_id = il.organization_id AND i.id = il.invoice_id
WHERE il.organization_id = @organization_id AND i.tenant_id = @tenant_id
ORDER BY il.seq;

-- name: ListAccountCorrections :many
-- Every correction of a tenant's invoices and payments, oldest first, with the member who made
-- it.
SELECT c.id, c.invoice_id, c.payment_id, c.amount_delta, c.reason, c.created_at,
       m.id AS created_by_member_id, su.name AS created_by_name
FROM correction c
JOIN membership m ON m.organization_id = c.organization_id AND m.staff_user_id = c.created_by_staff_user_id
JOIN staff_user su ON su.id = c.created_by_staff_user_id
LEFT JOIN invoice i ON i.organization_id = c.organization_id AND i.id = c.invoice_id
LEFT JOIN payment p ON p.organization_id = c.organization_id AND p.id = c.payment_id
WHERE c.organization_id = @organization_id AND (i.tenant_id = @tenant_id OR p.tenant_id = @tenant_id)
ORDER BY c.seq;

-- name: ListAccountPayments :many
-- A tenant's payments with their net amount and the member who recorded each (none for a
-- gateway payment), oldest received first.
SELECT p.id, p.amount,
       (p.amount + coalesce((SELECT sum(c.amount_delta) FROM correction c WHERE c.payment_id = p.id), 0))::bigint AS net,
       p.method, p.received_at, p.note, p.created_at,
       m.id AS recorded_by_member_id, su.name AS recorded_by_name
FROM payment p
LEFT JOIN membership m ON m.organization_id = p.organization_id AND m.staff_user_id = p.recorded_by_staff_user_id
LEFT JOIN staff_user su ON su.id = p.recorded_by_staff_user_id
WHERE p.organization_id = @organization_id AND p.tenant_id = @tenant_id
ORDER BY p.received_at, p.seq;

-- name: ListAccountAllocations :many
-- What each payment of a tenant holds on each invoice now (the sum of its allocation rows),
-- non-zero only, oldest invoice first.
SELECT a.payment_id, a.invoice_id, sum(a.amount)::bigint AS amount
FROM payment_allocation a
JOIN invoice i ON i.organization_id = a.organization_id AND i.id = a.invoice_id
WHERE a.organization_id = @organization_id AND a.tenant_id = @tenant_id
GROUP BY a.payment_id, a.invoice_id, i.period, i.seq
HAVING sum(a.amount) <> 0
ORDER BY i.period, i.seq, a.payment_id;
