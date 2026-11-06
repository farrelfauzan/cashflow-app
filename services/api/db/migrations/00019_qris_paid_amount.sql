-- +goose Up

-- Slice 3 (S3-5b): what a paid QRIS attempt was paid.
--
-- paid_amount is the amount recorded as the payment when the attempt turned paid. A dynamic QRIS
-- code fixes its amount, so it is normally the attempt's amount; when AutoGoPay reports another
-- amount, the payment is recorded at the paid amount and amount_mismatch flags the attempt for
-- the admin (decision 39, point 5) until the check queue (Antrean cek, slice 6) shows it. The flag
-- is computed by the database, so it can never disagree with the two amounts.
ALTER TABLE qris_attempt
    ADD COLUMN paid_amount bigint CONSTRAINT qris_attempt_paid_amount CHECK (paid_amount > 0),
    ADD COLUMN amount_mismatch boolean NOT NULL
        GENERATED ALWAYS AS (paid_amount IS NOT NULL AND paid_amount <> amount) STORED,
    ADD CONSTRAINT qris_attempt_paid_has_amount CHECK ((status = 'paid') = (paid_amount IS NOT NULL));

-- The app role sets paid_amount when it marks an attempt paid; the paid-is-final trigger keeps it
-- from changing afterwards.
GRANT UPDATE (paid_amount) ON qris_attempt TO cashflow_app;

-- The attempts reconcile asks AutoGoPay about: pending ones, oldest first.
CREATE INDEX qris_attempt_pending ON qris_attempt (organization_id, created_at, id) WHERE status = 'pending';

-- +goose Down
DROP INDEX qris_attempt_pending;
REVOKE UPDATE (paid_amount) ON qris_attempt FROM cashflow_app;
ALTER TABLE qris_attempt
    DROP CONSTRAINT qris_attempt_paid_has_amount,
    DROP COLUMN amount_mismatch,
    DROP COLUMN paid_amount;
