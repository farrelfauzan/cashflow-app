-- +goose Up

-- The lease fields for lease-anniversary billing (decision 24) and the planned end date
-- (decision 25). Slice 2's invoice generation reads them; nothing else does yet.
--
-- entered_on is the Asia/Jakarta day the lease was saved: a lease whose start date is before it
-- was entered late. billed_from ("Mulai tagih dari") is the first period the app invoices, an
-- anniversary of the start date; the application checks that, since the month-end rule is not
-- worth repeating in SQL. planned_end_date ("Rencana keluar") is the tenant's planned last day.
--
-- Existing rows get the day they were created as entered_on and their start date as billed_from,
-- the default for a new lease. No invoice exists yet, so nothing has been billed from any other
-- date.
ALTER TABLE lease
    ADD COLUMN entered_on       date,
    ADD COLUMN billed_from      date,
    ADD COLUMN planned_end_date date;

UPDATE lease
SET entered_on  = (created_at AT TIME ZONE 'Asia/Jakarta')::date,
    billed_from = start_date;

ALTER TABLE lease
    ALTER COLUMN entered_on SET NOT NULL,
    ALTER COLUMN billed_from SET NOT NULL,
    ADD CONSTRAINT lease_billed_from_after_start CHECK (billed_from >= start_date),
    ADD CONSTRAINT lease_planned_end_after_start CHECK (planned_end_date >= start_date),
    ADD CONSTRAINT lease_planned_end_only_active CHECK (status = 'active' OR planned_end_date IS NULL);

-- +goose Down
ALTER TABLE lease
    DROP CONSTRAINT lease_planned_end_only_active,
    DROP CONSTRAINT lease_planned_end_after_start,
    DROP CONSTRAINT lease_billed_from_after_start,
    DROP COLUMN planned_end_date,
    DROP COLUMN billed_from,
    DROP COLUMN entered_on;
