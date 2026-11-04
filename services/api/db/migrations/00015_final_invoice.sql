-- +goose Up

-- The final invoice at move-out (T-067): metered usage recorded after the last regular invoice of
-- a lease is billed on one invoice made when the lease ends, which the move-out-with-arrears rule
-- then treats like any other open invoice. A regular invoice bills one period of the lease's term;
-- a final one bills only metered amounts no regular invoice billed, and its period is the lease's
-- last day, so it belongs to the month the tenant left.
--
-- invoice stays append-only: adding a column with a default rewrites no row and fires no trigger,
-- and every invoice made so far is regular.
ALTER TABLE invoice ADD COLUMN kind text NOT NULL DEFAULT 'regular'
    CONSTRAINT invoice_kind CHECK (kind IN ('regular', 'final'));
-- A final invoice covers one day, the lease's last.
ALTER TABLE invoice ADD CONSTRAINT invoice_final_one_day CHECK (kind <> 'final' OR period = period_end);

-- One regular invoice per period of a lease, and at most one final invoice per lease. The final
-- invoice's period (the end date) may be the start of a regular period, so the old unique key on
-- (lease_id, period) becomes one per kind. Generation inserts with ON CONFLICT on each, so a rerun
-- or a retried move-out creates nothing twice.
ALTER TABLE invoice DROP CONSTRAINT invoice_lease_id_period_key;
CREATE UNIQUE INDEX invoice_regular_period ON invoice (lease_id, period) WHERE kind = 'regular';
CREATE UNIQUE INDEX invoice_one_final ON invoice (lease_id) WHERE kind = 'final';

-- +goose Down
-- Fails while a final invoice shares its period with a regular invoice of the same lease: invoice
-- rows are never deleted, so such a database cannot go back to one invoice per period.
DROP INDEX invoice_one_final;
DROP INDEX invoice_regular_period;
ALTER TABLE invoice ADD CONSTRAINT invoice_lease_id_period_key UNIQUE (lease_id, period);
ALTER TABLE invoice DROP CONSTRAINT invoice_final_one_day;
ALTER TABLE invoice DROP COLUMN kind;
