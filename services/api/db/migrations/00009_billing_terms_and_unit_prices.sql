-- +goose Up

-- Billing terms and the unit price history (decision 32). A lease is billed every 1, 3, 6 or 12
-- months; a unit lists the terms it offers, each with its own price, and a price change adds a
-- row to an append-only history. The rent of a period is the price for the lease's term in force
-- on the day its invoice is made, so the unit's usual rent and the lease's own rent go.

-- The terms a unit offers, in months, ascending and without repeats (the application keeps
-- them so). A lease keeps its term when the unit later stops offering it.
ALTER TABLE unit ADD COLUMN terms smallint[];
-- Every unit so far was rented by the month.
UPDATE unit SET terms = '{1}';
ALTER TABLE unit
    ALTER COLUMN terms SET NOT NULL,
    ADD CONSTRAINT unit_terms_known CHECK (terms <@ '{1,3,6,12}'::smallint[] AND cardinality(terms) >= 1);

-- The term a lease is billed by, fixed when it is created. Every lease so far is monthly.
ALTER TABLE lease ADD COLUMN term_months smallint NOT NULL DEFAULT 1
    CHECK (term_months IN (1, 3, 6, 12));
ALTER TABLE lease ALTER COLUMN term_months DROP DEFAULT;

-- One price of a unit for one term, from effective_from on. Append-only: a change is a new row.
-- The price in force on a day is the row with the latest effective_from on or before it, and on
-- the same effective_from the latest recorded one (the highest seq). The creator is the member
-- who recorded it; rows moved over by this migration have none.
CREATE TABLE unit_price (
    id                       uuid        PRIMARY KEY,
    seq                      bigint      GENERATED ALWAYS AS IDENTITY UNIQUE,
    organization_id          uuid        NOT NULL,
    unit_id                  uuid        NOT NULL,
    term_months              smallint    NOT NULL CHECK (term_months IN (1, 3, 6, 12)),
    amount                   bigint      NOT NULL CHECK (amount > 0),
    effective_from           date        NOT NULL,
    created_by_staff_user_id uuid,
    created_at               timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (unit_id, organization_id) REFERENCES unit (id, organization_id),
    FOREIGN KEY (organization_id, created_by_staff_user_id) REFERENCES membership (organization_id, staff_user_id)
);
CREATE INDEX unit_price_lookup ON unit_price (organization_id, unit_id, term_months, effective_from DESC, seq DESC);

REVOKE UPDATE, DELETE, TRUNCATE ON unit_price FROM cashflow_app;
CREATE TRIGGER unit_price_append_only BEFORE UPDATE OR DELETE ON unit_price
    FOR EACH ROW EXECUTE FUNCTION reject_change();
CREATE TRIGGER unit_price_no_truncate BEFORE TRUNCATE ON unit_price
    FOR EACH STATEMENT EXECUTE FUNCTION reject_change();

ALTER TABLE unit_price ENABLE ROW LEVEL SECURITY;
ALTER TABLE unit_price FORCE ROW LEVEL SECURITY;
CREATE POLICY unit_price_org ON unit_price USING (organization_id = app_org_id());

-- Each unit's monthly price so far: its active lease's rent, else its usual rent, from the day
-- the unit was created. No invoice exists yet, so no earlier price was ever billed. A unit with
-- neither gets no price; a lease on it needs one first.
INSERT INTO unit_price (id, organization_id, unit_id, term_months, amount, effective_from)
SELECT gen_random_uuid(), u.organization_id, u.id, 1, coalesce(l.rent_amount, u.default_rent),
       (u.created_at AT TIME ZONE 'Asia/Jakarta')::date
FROM unit u
LEFT JOIN lease l ON l.unit_id = u.id AND l.status = 'active'
WHERE coalesce(l.rent_amount, u.default_rent) IS NOT NULL
ORDER BY u.created_at, u.id;

ALTER TABLE unit DROP COLUMN default_rent;
ALTER TABLE lease DROP COLUMN rent_amount;

-- +goose Down
ALTER TABLE lease ADD COLUMN rent_amount bigint CHECK (rent_amount > 0);
ALTER TABLE unit ADD COLUMN default_rent bigint CHECK (default_rent > 0);
-- Back to one monthly rent per lease: the price in force today for its term divided by the
-- term's months, at least Rp1 (Rp1 too when there is no price).
UPDATE lease l SET rent_amount = greatest(1, (
    SELECT p.amount / l.term_months FROM unit_price p
    WHERE p.unit_id = l.unit_id AND p.term_months = l.term_months
      AND p.effective_from <= (now() AT TIME ZONE 'Asia/Jakarta')::date
    ORDER BY p.effective_from DESC, p.seq DESC LIMIT 1));
UPDATE lease SET rent_amount = 1 WHERE rent_amount IS NULL;
ALTER TABLE lease ALTER COLUMN rent_amount SET NOT NULL;
UPDATE unit u SET default_rent = (
    SELECT p.amount FROM unit_price p
    WHERE p.unit_id = u.id AND p.term_months = 1
      AND p.effective_from <= (now() AT TIME ZONE 'Asia/Jakarta')::date
    ORDER BY p.effective_from DESC, p.seq DESC LIMIT 1);
DROP TABLE unit_price;
ALTER TABLE lease DROP COLUMN term_months;
ALTER TABLE unit DROP COLUMN terms;
