-- Runs in an organization scope (store.WithOrgTx). unit_price is append-only (decision 32):
-- there is no UPDATE or DELETE here, and a trigger rejects them.

-- name: GetPriceInForce :one
-- The price of a unit for a term in force on a day: the row with the latest effective date on
-- or before it, the latest recorded one on the same date.
SELECT amount FROM unit_price
WHERE organization_id = @organization_id AND unit_id = @unit_id AND term_months = @term_months
  AND effective_from <= @day::date
ORDER BY effective_from DESC, seq DESC
LIMIT 1;

-- name: ListPricesInForce :many
-- For each of the units, the price per term in force on a day (see GetPriceInForce). A term
-- without one has no row.
SELECT DISTINCT ON (unit_id, term_months) unit_id, term_months, amount
FROM unit_price
WHERE organization_id = @organization_id AND unit_id = ANY(@unit_ids::uuid[]) AND effective_from <= @day::date
ORDER BY unit_id, term_months, effective_from DESC, seq DESC;

-- name: ListNextPrices :many
-- For each of the units, per term, the next price after a day: the earliest effective date
-- after it, the latest recorded one on that date.
SELECT DISTINCT ON (unit_id, term_months) unit_id, term_months, amount, effective_from
FROM unit_price
WHERE organization_id = @organization_id AND unit_id = ANY(@unit_ids::uuid[]) AND effective_from > @day::date
ORDER BY unit_id, term_months, effective_from, seq DESC;

-- name: ListUnitPrices :many
-- A unit's whole price history, the latest effective date first, with the name of the member
-- who recorded each row (none for rows moved over by the migration).
SELECT p.id, p.term_months, p.amount, p.effective_from, p.created_at, p.created_by_staff_user_id,
       u.name AS created_by_name
FROM unit_price p
LEFT JOIN staff_user u ON u.id = p.created_by_staff_user_id
WHERE p.organization_id = @organization_id AND p.unit_id = @unit_id
ORDER BY p.effective_from DESC, p.seq DESC;

-- name: CreateUnitPrice :one
INSERT INTO unit_price (id, organization_id, unit_id, term_months, amount, effective_from, created_by_staff_user_id)
VALUES (@id, @organization_id, @unit_id, @term_months, @amount, @effective_from, @created_by_staff_user_id)
RETURNING *;
