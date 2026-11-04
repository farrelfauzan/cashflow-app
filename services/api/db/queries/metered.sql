-- Runs in an organization scope (store.WithOrgTx). metered_charge rows are never deleted, and one
-- that an invoice line bills is never updated (a trigger rejects it); fixes are corrections on
-- the invoice.

-- Expected metered types on a lease ------------------------------------------------------------

-- name: ListLeaseMeteredTypes :many
-- The metered types the given leases pay for, with their names, by lease then type name.
SELECT lmt.lease_id, lmt.charge_type_id, ct.name AS charge_type_name
FROM lease_metered_type lmt
JOIN charge_type ct ON ct.organization_id = lmt.organization_id AND ct.id = lmt.charge_type_id
WHERE lmt.organization_id = @organization_id AND lmt.lease_id = ANY(@lease_ids::uuid[])
ORDER BY lmt.lease_id, ct.name COLLATE natural_sort, ct.id;

-- name: DeleteLeaseMeteredTypesExcept :exec
-- Removes the lease's metered types not in keep. A lease term, not a money row: the amounts
-- already recorded stay and are still billed.
DELETE FROM lease_metered_type
WHERE organization_id = @organization_id AND lease_id = @lease_id
  AND NOT (charge_type_id = ANY(@keep::uuid[]));

-- name: AddLeaseMeteredType :exec
INSERT INTO lease_metered_type (id, organization_id, lease_id, charge_type_id)
VALUES (@id, @organization_id, @lease_id, @charge_type_id)
ON CONFLICT (lease_id, charge_type_id) DO NOTHING;

-- Recording an amount ---------------------------------------------------------------------------

-- name: LockMeteredCharge :one
-- The lease's amount for one type and month, locked for the rest of the transaction, and whether
-- an invoice line bills it. Taken after the lease lock (see the lock order in the billing
-- package).
SELECT mc.id,
       EXISTS (SELECT 1 FROM invoice_line il
               WHERE il.organization_id = mc.organization_id AND il.metered_charge_id = mc.id) AS billed
FROM metered_charge mc
WHERE mc.organization_id = @organization_id AND mc.lease_id = @lease_id
  AND mc.charge_type_id = @charge_type_id AND mc.usage_month = @usage_month
FOR UPDATE OF mc;

-- name: CreateMeteredCharge :exec
INSERT INTO metered_charge (id, organization_id, lease_id, charge_type_id, usage_month, amount,
                            meter_start, meter_end, recorded_by_staff_user_id, created_at, updated_at)
VALUES (@id, @organization_id, @lease_id, @charge_type_id, @usage_month, @amount,
        @meter_start, @meter_end, @recorded_by_staff_user_id, @now, @now);

-- name: UpdateMeteredCharge :execrows
-- Changes an amount no invoice line bills yet; the trigger rejects a billed one.
UPDATE metered_charge
SET amount = @amount, meter_start = @meter_start, meter_end = @meter_end,
    recorded_by_staff_user_id = @recorded_by_staff_user_id, updated_at = @now
WHERE organization_id = @organization_id AND id = @id;

-- Reading amounts -------------------------------------------------------------------------------

-- name: GetMeteredCharge :one
-- One amount with its type, who recorded it last and the invoice that bills it, if any.
SELECT mc.id, mc.lease_id, mc.charge_type_id, ct.name AS charge_type_name, mc.usage_month, mc.amount,
       mc.meter_start, mc.meter_end, m.id AS recorded_by_member_id, su.name AS recorded_by_name,
       mc.created_at, mc.updated_at, il.invoice_id AS billed_invoice_id, i.period AS billed_period, i.kind AS billed_kind
FROM metered_charge mc
JOIN charge_type ct ON ct.organization_id = mc.organization_id AND ct.id = mc.charge_type_id
JOIN membership m ON m.organization_id = mc.organization_id AND m.staff_user_id = mc.recorded_by_staff_user_id
JOIN staff_user su ON su.id = mc.recorded_by_staff_user_id
LEFT JOIN invoice_line il ON il.organization_id = mc.organization_id AND il.metered_charge_id = mc.id
LEFT JOIN invoice i ON i.organization_id = il.organization_id AND i.id = il.invoice_id
WHERE mc.organization_id = @organization_id AND mc.id = @id;

-- name: ListLeaseMeteredChargePage :many
-- One page of a lease's amounts, the newest month first, then by type name. CountLeaseMeteredCharges
-- has the same WHERE: change both together.
SELECT mc.id, mc.lease_id, mc.charge_type_id, ct.name AS charge_type_name, mc.usage_month, mc.amount,
       mc.meter_start, mc.meter_end, m.id AS recorded_by_member_id, su.name AS recorded_by_name,
       mc.created_at, mc.updated_at, il.invoice_id AS billed_invoice_id, i.period AS billed_period, i.kind AS billed_kind
FROM metered_charge mc
JOIN charge_type ct ON ct.organization_id = mc.organization_id AND ct.id = mc.charge_type_id
JOIN membership m ON m.organization_id = mc.organization_id AND m.staff_user_id = mc.recorded_by_staff_user_id
JOIN staff_user su ON su.id = mc.recorded_by_staff_user_id
LEFT JOIN invoice_line il ON il.organization_id = mc.organization_id AND il.metered_charge_id = mc.id
LEFT JOIN invoice i ON i.organization_id = il.organization_id AND i.id = il.invoice_id
WHERE mc.organization_id = @organization_id AND mc.lease_id = @lease_id
  AND (sqlc.narg('charge_type_id')::uuid IS NULL OR mc.charge_type_id = sqlc.narg('charge_type_id'))
  AND (sqlc.narg('usage_month')::date IS NULL OR mc.usage_month = sqlc.narg('usage_month'))
ORDER BY mc.usage_month DESC, ct.name COLLATE natural_sort, mc.id
LIMIT @page_limit OFFSET @page_offset;

-- name: CountLeaseMeteredCharges :one
SELECT count(*)
FROM metered_charge mc
WHERE mc.organization_id = @organization_id AND mc.lease_id = @lease_id
  AND (sqlc.narg('charge_type_id')::uuid IS NULL OR mc.charge_type_id = sqlc.narg('charge_type_id'))
  AND (sqlc.narg('usage_month')::date IS NULL OR mc.usage_month = sqlc.narg('usage_month'));

-- Billing ---------------------------------------------------------------------------------------

-- name: LockUnbilledMeteredCharges :many
-- The lease's amounts no invoice line bills yet, locked, oldest month first, then by type name.
-- Generation takes it after the lease and unit locks and bills each one on the newest invoice it
-- makes. FOR UPDATE makes an edit in progress commit first, so the line snapshots the amount
-- that is stored.
SELECT mc.id, mc.charge_type_id, ct.name AS charge_type_name, mc.usage_month, mc.amount
FROM metered_charge mc
JOIN charge_type ct ON ct.organization_id = mc.organization_id AND ct.id = mc.charge_type_id
WHERE mc.organization_id = @organization_id AND mc.lease_id = @lease_id
  AND NOT EXISTS (SELECT 1 FROM invoice_line il
                  WHERE il.organization_id = mc.organization_id AND il.metered_charge_id = mc.id)
ORDER BY mc.usage_month, ct.name COLLATE natural_sort, mc.id
FOR UPDATE OF mc;

-- name: ListInvoicePeriodsOfLeases :many
-- The period starts that already have a regular invoice, for each of the given leases. A final
-- invoice bills no period of the term.
SELECT lease_id, period FROM invoice
WHERE organization_id = @organization_id AND lease_id = ANY(@lease_ids::uuid[]) AND kind = 'regular'
ORDER BY lease_id, period;

-- Missing readings (Penagihan: "meteran belum dicatat") -----------------------------------------

-- name: ListMissingMeterLeasePage :many
-- One page of the active leases among lease_ids, started on or before month_last, that expect a
-- metered type with no amount recorded for usage_month. By tenant name, then unit, then lease.
-- CountMissingMeterLeases has the same WHERE: change both together.
SELECT l.id AS lease_id, l.tenant_id, t.name AS tenant_name, p.name AS property_name, u.label AS unit_label,
       l.start_date, l.entered_on, l.billed_from, l.term_months, l.planned_end_date, l.end_date
FROM lease l
JOIN tenant t ON t.organization_id = l.organization_id AND t.id = l.tenant_id
JOIN unit u ON u.organization_id = l.organization_id AND u.id = l.unit_id
JOIN property p ON p.organization_id = u.organization_id AND p.id = u.property_id
WHERE l.organization_id = @organization_id AND l.id = ANY(@lease_ids::uuid[])
  AND l.status = 'active' AND l.start_date <= @month_last::date
  AND (sqlc.narg('property_id')::uuid IS NULL OR u.property_id = sqlc.narg('property_id'))
  AND EXISTS (
      SELECT 1 FROM lease_metered_type lmt
      WHERE lmt.organization_id = l.organization_id AND lmt.lease_id = l.id
        AND NOT EXISTS (SELECT 1 FROM metered_charge mc
                        WHERE mc.organization_id = lmt.organization_id AND mc.lease_id = lmt.lease_id
                          AND mc.charge_type_id = lmt.charge_type_id AND mc.usage_month = @usage_month::date))
ORDER BY t.name COLLATE natural_sort, u.label COLLATE natural_sort, l.id
LIMIT @page_limit OFFSET @page_offset;

-- name: CountMissingMeterLeases :one
SELECT count(*)
FROM lease l
JOIN unit u ON u.organization_id = l.organization_id AND u.id = l.unit_id
WHERE l.organization_id = @organization_id AND l.id = ANY(@lease_ids::uuid[])
  AND l.status = 'active' AND l.start_date <= @month_last::date
  AND (sqlc.narg('property_id')::uuid IS NULL OR u.property_id = sqlc.narg('property_id'))
  AND EXISTS (
      SELECT 1 FROM lease_metered_type lmt
      WHERE lmt.organization_id = l.organization_id AND lmt.lease_id = l.id
        AND NOT EXISTS (SELECT 1 FROM metered_charge mc
                        WHERE mc.organization_id = lmt.organization_id AND mc.lease_id = lmt.lease_id
                          AND mc.charge_type_id = lmt.charge_type_id AND mc.usage_month = @usage_month::date));

-- name: ListMissingMeterTypes :many
-- For each of the given leases, the metered types it expects with no amount for usage_month, by
-- type name.
SELECT lmt.lease_id, lmt.charge_type_id, ct.name AS charge_type_name
FROM lease_metered_type lmt
JOIN charge_type ct ON ct.organization_id = lmt.organization_id AND ct.id = lmt.charge_type_id
WHERE lmt.organization_id = @organization_id AND lmt.lease_id = ANY(@lease_ids::uuid[])
  AND NOT EXISTS (SELECT 1 FROM metered_charge mc
                  WHERE mc.organization_id = lmt.organization_id AND mc.lease_id = lmt.lease_id
                    AND mc.charge_type_id = lmt.charge_type_id AND mc.usage_month = @usage_month::date)
ORDER BY lmt.lease_id, ct.name COLLATE natural_sort, ct.id;
