-- +goose Up

-- A unit may offer one custom term besides 1, 3, 6 and 12 months (decision 33): any month count
-- from 2 to 60 the admin types. It lives in unit.terms with the standard ones, so unit_price and
-- lease keep keying on term_months; only the allowed values widen. Existing rows all hold
-- standard terms and pass the new checks unchanged.

-- valid_unit_terms holds for a unit's offered terms: at least one, each 1 to 60 months, no month
-- count twice, and at most one outside the standard terms.
-- +goose StatementBegin
CREATE FUNCTION valid_unit_terms(terms smallint[]) RETURNS boolean LANGUAGE sql IMMUTABLE AS $$
    SELECT cardinality(terms) >= 1
       AND NOT EXISTS (SELECT 1 FROM unnest(terms) AS t (m) WHERE m IS NULL OR m < 1 OR m > 60)
       AND (SELECT count(DISTINCT m) FROM unnest(terms) AS t (m)) = cardinality(terms)
       AND (SELECT count(*) FROM unnest(terms) AS t (m) WHERE m <> ALL ('{1,3,6,12}'::smallint[])) <= 1
$$;
-- +goose StatementEnd

ALTER TABLE unit
    DROP CONSTRAINT unit_terms_known,
    ADD CONSTRAINT unit_terms_valid CHECK (valid_unit_terms(terms));

ALTER TABLE lease
    DROP CONSTRAINT lease_term_months_check,
    ADD CONSTRAINT lease_term_months_check CHECK (term_months BETWEEN 1 AND 60);

ALTER TABLE unit_price
    DROP CONSTRAINT unit_price_term_months_check,
    ADD CONSTRAINT unit_price_term_months_check CHECK (term_months BETWEEN 1 AND 60);

-- +goose Down
-- Back to the standard terms only. This fails while any unit, lease or price uses a custom
-- term: those rows have no meaning without decision 33 and are not rewritten here.
ALTER TABLE unit_price
    DROP CONSTRAINT unit_price_term_months_check,
    ADD CONSTRAINT unit_price_term_months_check CHECK (term_months IN (1, 3, 6, 12));

ALTER TABLE lease
    DROP CONSTRAINT lease_term_months_check,
    ADD CONSTRAINT lease_term_months_check CHECK (term_months IN (1, 3, 6, 12));

ALTER TABLE unit
    DROP CONSTRAINT unit_terms_valid,
    ADD CONSTRAINT unit_terms_known CHECK (terms <@ '{1,3,6,12}'::smallint[] AND cardinality(terms) >= 1);

DROP FUNCTION valid_unit_terms(smallint[]);
