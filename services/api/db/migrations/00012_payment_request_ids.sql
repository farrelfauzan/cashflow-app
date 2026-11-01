-- +goose Up

-- Manual payments and corrections from the API (T-052). A client sends a requestId once per form
-- it opens and again unchanged on a retry, so a double submit records nothing twice. A manual
-- payment keeps it in source_ref, already unique per organization and source; a correction gets
-- its own column. Corrections made by the system (a lease's planned end or move-out) have none.
ALTER TABLE correction ADD COLUMN request_id uuid;
CREATE UNIQUE INDEX correction_request_id_unique ON correction (organization_id, request_id)
    WHERE request_id IS NOT NULL;

-- A payment recorded by hand names the member who recorded it; a gateway payment has none.
ALTER TABLE payment ADD CONSTRAINT payment_manual_recorded_by
    CHECK (source <> 'manual' OR recorded_by_staff_user_id IS NOT NULL);

-- The method follows from the source: by hand it is cash or a transfer the admin checked; QRIS and
-- a virtual account come only from their gateway.
ALTER TABLE payment ADD CONSTRAINT payment_method_matches_source CHECK (
    (source = 'manual' AND method IN ('cash', 'transfer'))
    OR (source = 'qris' AND method = 'qris')
    OR (source = 'va' AND method = 'va')
);

-- +goose Down
ALTER TABLE payment DROP CONSTRAINT payment_method_matches_source;
ALTER TABLE payment DROP CONSTRAINT payment_manual_recorded_by;
DROP INDEX correction_request_id_unique;
ALTER TABLE correction DROP COLUMN request_id;
