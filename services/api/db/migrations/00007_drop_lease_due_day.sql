-- +goose Up

-- Rent is due on the day of the lease's start date (decision 24: each lease bills on the
-- anniversary of its start), so the lease no longer stores a separate due day. Nothing reads
-- the column any more: the app's queries and the API stopped using it in the same change, and
-- no invoices exist yet, so no billed period depends on an old value.
ALTER TABLE lease DROP COLUMN due_day;

-- +goose Down
-- Restores the column for a rollback. The old values are gone, so each lease gets the day of its
-- start date, capped at 28 as the old CHECK required.
ALTER TABLE lease ADD COLUMN due_day smallint;
UPDATE lease SET due_day = LEAST(EXTRACT(DAY FROM start_date)::smallint, 28);
ALTER TABLE lease
    ALTER COLUMN due_day SET NOT NULL,
    ADD CONSTRAINT lease_due_day_check CHECK (due_day BETWEEN 1 AND 28);
