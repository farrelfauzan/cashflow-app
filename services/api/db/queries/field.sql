-- The karyawan's field app (slice 6, S6-2): "Tagihan hari ini" and the cash they hold. Read-only,
-- in an organization scope (store.WithOrgSnapshot). Who may see a lease is the policy's decision,
-- made in Go for every lease of the organization; the queries get the visible leases in lease_ids.
-- A cash payment counts net of its corrections. Today's cash is field cash only: the payments
-- recordCashPayment recorded, which have a receipt row (payment_receipt) and can take a photo; cash
-- an admin records through recordPayment is not listed here.

-- name: ListFieldToday :many
-- One page of "Tagihan hari ini": of the tenants @collector_id collects from on an active lease, all
-- of whose leases, current or ended, are visible (in lease_ids), (a) those who owe on an invoice due
-- on or before @until (overdue, due today, or due within the coming days), oldest unpaid due date
-- first, then (b) those not in (a) from whom @collector_id recorded cash in [@day_start, @day_end)
-- (paid today), by name; the tenant id breaks ties. With each: what they owe in all, the part of it
-- overdue on @today, and that cash. CountFieldToday has the same CTE and WHERE: change both
-- together.
WITH mine AS (
    -- The tenants @collector_id collects from on an active lease, all of whose leases, current or
    -- ended, are visible (in lease_ids). due: they owe on an invoice due on or before @until.
    -- A tenant who owes nothing (a (b) row) has no unpaid due date: it reads 0001-01-01 here, and
    -- the service uses it only when outstanding > 0.
    SELECT b.tenant_id, t.name AS tenant_name, b.outstanding,
           coalesce(b.oldest_unpaid_due_date, DATE '0001-01-01')::date AS oldest_unpaid_due_date,
           (b.outstanding > 0 AND b.oldest_unpaid_due_date <= @until::date)::boolean AS due,
           (SELECT coalesce(sum(ib.outstanding), 0) FROM invoice_balance ib
            WHERE ib.organization_id = b.organization_id AND ib.tenant_id = b.tenant_id
              AND ib.outstanding > 0 AND ib.due_date < @today::date)::bigint AS overdue,
           (SELECT coalesce(sum(p.amount + (SELECT coalesce(sum(c.amount_delta), 0) FROM correction c
                                            WHERE c.organization_id = p.organization_id AND c.payment_id = p.id)), 0)
            FROM payment p
            WHERE p.organization_id = b.organization_id AND p.tenant_id = b.tenant_id AND p.method = 'cash'
              AND EXISTS (SELECT 1 FROM payment_receipt r WHERE r.organization_id = p.organization_id AND r.payment_id = p.id)
              AND p.recorded_by_staff_user_id = @collector_id
              AND p.received_at >= @day_start AND p.received_at < @day_end)::bigint AS cash_today,
           (SELECT count(*) FROM payment p
            WHERE p.organization_id = b.organization_id AND p.tenant_id = b.tenant_id AND p.method = 'cash'
              AND EXISTS (SELECT 1 FROM payment_receipt r WHERE r.organization_id = p.organization_id AND r.payment_id = p.id)
              AND p.recorded_by_staff_user_id = @collector_id
              AND p.received_at >= @day_start AND p.received_at < @day_end)::bigint AS cash_today_count
    FROM tenant_balance b
    JOIN tenant t ON t.organization_id = b.organization_id AND t.id = b.tenant_id
    WHERE b.organization_id = @organization_id
      AND EXISTS (SELECT 1 FROM lease l
                  WHERE l.organization_id = b.organization_id AND l.tenant_id = b.tenant_id
                    AND l.status = 'active' AND l.collector_staff_user_id = @collector_id)
      AND NOT EXISTS (SELECT 1 FROM lease l
                      WHERE l.organization_id = b.organization_id AND l.tenant_id = b.tenant_id
                        AND NOT (l.id = ANY(@lease_ids::uuid[])))
)
SELECT tenant_id, tenant_name, outstanding, oldest_unpaid_due_date, due, overdue, cash_today, cash_today_count
FROM mine
WHERE due OR cash_today_count > 0
ORDER BY due DESC,
         CASE WHEN due THEN oldest_unpaid_due_date END,
         CASE WHEN NOT due THEN tenant_name END,
         tenant_id
LIMIT @page_limit OFFSET @page_offset;

-- name: CountFieldToday :one
-- Every row ListFieldToday would list, on all pages.
WITH mine AS (
    -- The tenants @collector_id collects from on an active lease, all of whose leases, current or
    -- ended, are visible (in lease_ids). due: they owe on an invoice due on or before @until.
    -- A tenant who owes nothing (a (b) row) has no unpaid due date: it reads 0001-01-01 here, and
    -- the service uses it only when outstanding > 0.
    SELECT b.tenant_id, t.name AS tenant_name, b.outstanding,
           coalesce(b.oldest_unpaid_due_date, DATE '0001-01-01')::date AS oldest_unpaid_due_date,
           (b.outstanding > 0 AND b.oldest_unpaid_due_date <= @until::date)::boolean AS due,
           (SELECT coalesce(sum(ib.outstanding), 0) FROM invoice_balance ib
            WHERE ib.organization_id = b.organization_id AND ib.tenant_id = b.tenant_id
              AND ib.outstanding > 0 AND ib.due_date < @today::date)::bigint AS overdue,
           (SELECT coalesce(sum(p.amount + (SELECT coalesce(sum(c.amount_delta), 0) FROM correction c
                                            WHERE c.organization_id = p.organization_id AND c.payment_id = p.id)), 0)
            FROM payment p
            WHERE p.organization_id = b.organization_id AND p.tenant_id = b.tenant_id AND p.method = 'cash'
              AND EXISTS (SELECT 1 FROM payment_receipt r WHERE r.organization_id = p.organization_id AND r.payment_id = p.id)
              AND p.recorded_by_staff_user_id = @collector_id
              AND p.received_at >= @day_start AND p.received_at < @day_end)::bigint AS cash_today,
           (SELECT count(*) FROM payment p
            WHERE p.organization_id = b.organization_id AND p.tenant_id = b.tenant_id AND p.method = 'cash'
              AND EXISTS (SELECT 1 FROM payment_receipt r WHERE r.organization_id = p.organization_id AND r.payment_id = p.id)
              AND p.recorded_by_staff_user_id = @collector_id
              AND p.received_at >= @day_start AND p.received_at < @day_end)::bigint AS cash_today_count
    FROM tenant_balance b
    JOIN tenant t ON t.organization_id = b.organization_id AND t.id = b.tenant_id
    WHERE b.organization_id = @organization_id
      AND EXISTS (SELECT 1 FROM lease l
                  WHERE l.organization_id = b.organization_id AND l.tenant_id = b.tenant_id
                    AND l.status = 'active' AND l.collector_staff_user_id = @collector_id)
      AND NOT EXISTS (SELECT 1 FROM lease l
                      WHERE l.organization_id = b.organization_id AND l.tenant_id = b.tenant_id
                        AND NOT (l.id = ANY(@lease_ids::uuid[])))
)
SELECT count(*)::bigint
FROM mine
WHERE due OR cash_today_count > 0;

-- name: ListFieldCashToday :many
-- The cash payments @collector_id recorded from the given tenants in [@day_start, @day_end), each
-- net of its corrections, oldest recorded first.
SELECT p.id, p.tenant_id, p.created_at,
       (p.amount + (SELECT coalesce(sum(c.amount_delta), 0) FROM correction c
                    WHERE c.organization_id = p.organization_id AND c.payment_id = p.id))::bigint AS net
FROM payment p
WHERE p.organization_id = @organization_id AND p.tenant_id = ANY(@tenant_ids::uuid[]) AND p.method = 'cash'
  AND EXISTS (SELECT 1 FROM payment_receipt r WHERE r.organization_id = p.organization_id AND r.payment_id = p.id)
  AND p.recorded_by_staff_user_id = @collector_id
  AND p.received_at >= @day_start AND p.received_at < @day_end
ORDER BY p.tenant_id, p.created_at, p.seq;

-- name: ListFieldTodayUnits :many
-- The units of the given tenants' active leases that @collector_id collects, by property then unit.
SELECT l.tenant_id, u.label AS unit_label, p.name AS property_name, p.address AS property_address
FROM lease l
JOIN unit u ON u.organization_id = l.organization_id AND u.id = l.unit_id
JOIN property p ON p.organization_id = u.organization_id AND p.id = u.property_id
WHERE l.organization_id = @organization_id AND l.tenant_id = ANY(@tenant_ids::uuid[])
  AND l.status = 'active' AND l.collector_staff_user_id = @collector_id
ORDER BY l.tenant_id, p.name, u.label, l.id;

-- name: GetFieldCash :one
-- The cash @staff_user_id holds: every cash payment they recorded, net of its corrections, and how
-- many. Cash deposits (S6-3) keep only the payments no deposit covers yet: they add that one
-- condition to the WHERE (a NOT EXISTS on the deposit's payment link).
SELECT coalesce(sum(p.amount + (SELECT coalesce(sum(c.amount_delta), 0) FROM correction c
                                WHERE c.organization_id = p.organization_id AND c.payment_id = p.id)), 0)::bigint AS amount,
       count(*)::bigint AS payments
FROM payment p
WHERE p.organization_id = @organization_id AND p.method = 'cash'
  AND p.recorded_by_staff_user_id = @staff_user_id;
