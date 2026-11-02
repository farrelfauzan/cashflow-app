-- +goose Up

-- A correction from the API may name the amount the admin wants ("Nominal baru") instead of the
-- difference. The service takes the difference from the current amount inside the transaction
-- and stores it in amount_delta as before; new_amount keeps what the request sent, so a repeat of
-- the same requestId is recognized even though the amount now equals it (T-059).
ALTER TABLE correction ADD COLUMN new_amount bigint CHECK (new_amount >= 0);

-- +goose Down
ALTER TABLE correction DROP COLUMN new_amount;
