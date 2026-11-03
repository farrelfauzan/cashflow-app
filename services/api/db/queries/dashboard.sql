-- The Supervisor's dashboard and Penagihan (T-053). Read-only, in an organization scope
-- (store.WithOrgTx). Who may see a lease is the policy's decision, made in Go for every lease of
-- the organization; each query gets the visible leases in lease_ids and counts only the invoices
-- of those leases. A month's invoices are those whose period starts in [month_start, month_end).

-- name: SumMonthInvoices :one
-- What the month's visible invoices bill, what is paid on them and what they still owe, how many
-- there are (cancelled ones included) and how many tenants still owe something on one of them.
SELECT coalesce(sum(b.billed), 0)::bigint                                 AS billed,
       coalesce(sum(b.paid), 0)::bigint                                   AS paid,
       coalesce(sum(b.outstanding), 0)::bigint                            AS outstanding,
       count(*)::bigint                                                   AS invoice_count,
       count(DISTINCT b.tenant_id) FILTER (WHERE b.outstanding > 0)::bigint AS tenants_outstanding
FROM invoice_balance b
JOIN lease l ON l.organization_id = b.organization_id AND l.id = b.lease_id
JOIN unit u ON u.organization_id = l.organization_id AND u.id = l.unit_id
WHERE b.organization_id = @organization_id
  AND b.lease_id = ANY(@lease_ids::uuid[])
  AND b.period >= @month_start::date AND b.period < @month_end::date
  AND (sqlc.narg('property_id')::uuid IS NULL OR u.property_id = sqlc.narg('property_id'));

-- name: SumOverdue :one
-- Everything the visible invoices of any month still owe after their due date (debt from the day
-- after it; today is the Asia/Jakarta day from the injected clock), and how many tenants owe it.
SELECT coalesce(sum(b.outstanding), 0)::bigint AS overdue,
       count(DISTINCT b.tenant_id)::bigint     AS tenants
FROM invoice_balance b
WHERE b.organization_id = @organization_id
  AND b.lease_id = ANY(@lease_ids::uuid[])
  AND b.outstanding > 0 AND b.due_date < @today::date;

-- name: ListTenantArrears :many
-- The tenants in arrears on visible invoices, the oldest debt first, then the most owed.
SELECT b.tenant_id, t.name AS tenant_name,
       sum(b.outstanding)::bigint AS overdue,
       min(b.due_date)::date      AS oldest_unpaid_due_date
FROM invoice_balance b
JOIN tenant t ON t.organization_id = b.organization_id AND t.id = b.tenant_id
WHERE b.organization_id = @organization_id
  AND b.lease_id = ANY(@lease_ids::uuid[])
  AND b.outstanding > 0 AND b.due_date < @today::date
GROUP BY b.tenant_id, t.name
ORDER BY min(b.due_date), sum(b.outstanding) DESC, b.tenant_id
LIMIT @row_limit;

-- name: ListArrearsUnits :many
-- The units of some tenants' visible overdue invoices, by property then unit.
SELECT DISTINCT b.tenant_id, p.name AS property_name, u.label AS unit_label
FROM invoice_balance b
JOIN lease l ON l.organization_id = b.organization_id AND l.id = b.lease_id
JOIN unit u ON u.organization_id = l.organization_id AND u.id = l.unit_id
JOIN property p ON p.organization_id = u.organization_id AND p.id = u.property_id
WHERE b.organization_id = @organization_id
  AND b.lease_id = ANY(@lease_ids::uuid[])
  AND b.tenant_id = ANY(@tenant_ids::uuid[])
  AND b.outstanding > 0 AND b.due_date < @today::date
ORDER BY b.tenant_id, p.name, u.label;

-- name: GetOldestOverdue :many
-- The visible invoice of any month still owed after its due date with the oldest due date, in
-- the property if one is given; ties go to the tenant's name, then the invoice id. No row when
-- nothing is overdue.
SELECT b.tenant_id, t.name AS tenant_name, p.name AS property_name, u.label AS unit_label, b.due_date
FROM invoice_balance b
JOIN tenant t ON t.organization_id = b.organization_id AND t.id = b.tenant_id
JOIN lease l ON l.organization_id = b.organization_id AND l.id = b.lease_id
JOIN unit u ON u.organization_id = l.organization_id AND u.id = l.unit_id
JOIN property p ON p.organization_id = u.organization_id AND p.id = u.property_id
WHERE b.organization_id = @organization_id
  AND b.lease_id = ANY(@lease_ids::uuid[])
  AND b.outstanding > 0 AND b.due_date < @today::date
  AND (sqlc.narg('property_id')::uuid IS NULL OR u.property_id = sqlc.narg('property_id'))
ORDER BY b.due_date, t.name, b.invoice_id
LIMIT 1;

-- name: ListRecentCorrections :many
-- The newest visible corrections with the member who made each, the tenant, and the corrected
-- invoice's period and unit or the corrected payment's received moment. A correction on an
-- invoice is visible with the invoice's lease. A correction on a payment is visible when every
-- lease the payment's tenant has or had is visible, or, for a tenant with no lease, when
-- no_lease_visible (the organization-wide grant) is set.
SELECT c.id, c.invoice_id, c.payment_id, c.amount_delta, c.reason, c.created_at,
       m.id AS created_by_member_id, su.name AS created_by_name,
       t.id AS tenant_id, t.name AS tenant_name,
       i.period AS invoice_period, pr.name AS property_name, u.label AS unit_label,
       pay.received_at
FROM correction c
JOIN membership m ON m.organization_id = c.organization_id AND m.staff_user_id = c.created_by_staff_user_id
JOIN staff_user su ON su.id = c.created_by_staff_user_id
LEFT JOIN invoice i ON i.organization_id = c.organization_id AND i.id = c.invoice_id
LEFT JOIN lease l ON l.organization_id = i.organization_id AND l.id = i.lease_id
LEFT JOIN unit u ON u.organization_id = l.organization_id AND u.id = l.unit_id
LEFT JOIN property pr ON pr.organization_id = u.organization_id AND pr.id = u.property_id
LEFT JOIN payment pay ON pay.organization_id = c.organization_id AND pay.id = c.payment_id
JOIN tenant t ON t.organization_id = c.organization_id AND t.id = coalesce(i.tenant_id, pay.tenant_id)
WHERE c.organization_id = @organization_id
  AND (
      (i.id IS NOT NULL AND i.lease_id = ANY(@lease_ids::uuid[]))
      OR (pay.id IS NOT NULL
          AND NOT EXISTS (
              SELECT 1 FROM lease tl
              WHERE tl.organization_id = c.organization_id AND tl.tenant_id = pay.tenant_id
                AND NOT (tl.id = ANY(@lease_ids::uuid[])))
          AND (@no_lease_visible::boolean
               OR EXISTS (SELECT 1 FROM lease tl
                          WHERE tl.organization_id = c.organization_id AND tl.tenant_id = pay.tenant_id)))
  )
ORDER BY c.created_at DESC, c.seq DESC
LIMIT @row_limit;

-- name: ListInvoicePage :many
-- One page of the month's visible invoices matching the filters. statuses is the invoice
-- statuses to keep; overdue_only keeps only what is owed after its due date, of any month (the
-- month is ignored, as debt is not tied to one). CountInvoicePage has the same WHERE: change
-- both together.
SELECT b.invoice_id, b.tenant_id, t.name AS tenant_name, b.lease_id, u.property_id,
       p.name AS property_name, u.label AS unit_label,
       b.period, b.period_end, b.due_date, b.amount, b.billed, b.paid, b.outstanding, b.status,
       i.created_at,
       (l.status = 'active' AND l.planned_end_date IS NOT NULL
        AND l.planned_end_date < @today::date)::boolean AS past_planned_end
FROM invoice_balance b
JOIN invoice i ON i.organization_id = b.organization_id AND i.id = b.invoice_id
JOIN tenant t ON t.organization_id = b.organization_id AND t.id = b.tenant_id
JOIN lease l ON l.organization_id = b.organization_id AND l.id = b.lease_id
JOIN unit u ON u.organization_id = l.organization_id AND u.id = l.unit_id
JOIN property p ON p.organization_id = u.organization_id AND p.id = u.property_id
WHERE b.organization_id = @organization_id
  AND b.lease_id = ANY(@lease_ids::uuid[])
  AND (@overdue_only::boolean OR (b.period >= @month_start::date AND b.period < @month_end::date))
  AND (sqlc.narg('property_id')::uuid IS NULL OR u.property_id = sqlc.narg('property_id'))
  AND (sqlc.narg('q')::text IS NULL
       OR t.name ILIKE sqlc.narg('q') OR u.label ILIKE sqlc.narg('q') OR p.name ILIKE sqlc.narg('q'))
  AND b.status = ANY(@statuses::text[])
  AND (NOT @overdue_only::boolean OR (b.outstanding > 0 AND b.due_date < @today::date))
ORDER BY CASE WHEN @sort::text = 'dueDate' THEN b.due_date END,
         CASE WHEN @sort::text = '-dueDate' THEN b.due_date END DESC,
         CASE WHEN @sort::text = 'tenantName' THEN t.name END,
         CASE WHEN @sort::text = '-outstanding' THEN b.outstanding END DESC,
         b.invoice_id
LIMIT @page_limit OFFSET @page_offset;

-- name: CountInvoicePage :one
-- Every invoice ListInvoicePage would return on any page: keep the WHERE the same.
SELECT count(*)
FROM invoice_balance b
JOIN tenant t ON t.organization_id = b.organization_id AND t.id = b.tenant_id
JOIN lease l ON l.organization_id = b.organization_id AND l.id = b.lease_id
JOIN unit u ON u.organization_id = l.organization_id AND u.id = l.unit_id
JOIN property p ON p.organization_id = u.organization_id AND p.id = u.property_id
WHERE b.organization_id = @organization_id
  AND b.lease_id = ANY(@lease_ids::uuid[])
  AND (@overdue_only::boolean OR (b.period >= @month_start::date AND b.period < @month_end::date))
  AND (sqlc.narg('property_id')::uuid IS NULL OR u.property_id = sqlc.narg('property_id'))
  AND (sqlc.narg('q')::text IS NULL
       OR t.name ILIKE sqlc.narg('q') OR u.label ILIKE sqlc.narg('q') OR p.name ILIKE sqlc.narg('q'))
  AND b.status = ANY(@statuses::text[])
  AND (NOT @overdue_only::boolean OR (b.outstanding > 0 AND b.due_date < @today::date));

-- name: ListLinesOfInvoices :many
-- The lines of some invoices, in the order they were written.
SELECT il.id, il.invoice_id, il.kind, il.label, il.amount, il.metered_charge_id
FROM invoice_line il
WHERE il.organization_id = @organization_id AND il.invoice_id = ANY(@invoice_ids::uuid[])
ORDER BY il.seq;
