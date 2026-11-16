-- +goose Up

-- Follow-up of decision 50 (T-104): the karyawan uploads the cash photo before recording the
-- cash, so a photo taken for cash that was never recorded backs no payment. The daily
-- upload:cleanup job deletes the stored file of such a cash_receipt upload once it is 30 days
-- old. The upload row stays (a scanned upload never changes, 00028); this table records that its
-- file is gone, so the job never lists it again and no payment can link it afterwards.
--
-- Append-only: an upload's file is deleted once. The job writes the row and deletes the file in
-- the same transaction, holding the upload's row lock; linking a photo to a payment takes the
-- same lock and refuses an upload listed here.
CREATE TABLE upload_deletion (
    upload_id       uuid        PRIMARY KEY,
    organization_id uuid        NOT NULL REFERENCES organization (id),
    deleted_at      timestamptz NOT NULL,
    FOREIGN KEY (upload_id, organization_id) REFERENCES upload (id, organization_id)
);

-- The job's candidates: clean receipt photos, oldest first.
CREATE INDEX upload_cash_receipt_clean ON upload (organization_id, created_at, id)
    WHERE purpose = 'cash_receipt' AND status = 'clean';

REVOKE UPDATE, DELETE, TRUNCATE ON upload_deletion FROM cashflow_app;
CREATE TRIGGER upload_deletion_append_only BEFORE UPDATE OR DELETE ON upload_deletion
    FOR EACH ROW EXECUTE FUNCTION reject_change();
CREATE TRIGGER upload_deletion_no_truncate BEFORE TRUNCATE ON upload_deletion
    FOR EACH STATEMENT EXECUTE FUNCTION reject_change();

ALTER TABLE upload_deletion ENABLE ROW LEVEL SECURITY;
ALTER TABLE upload_deletion FORCE ROW LEVEL SECURITY;
CREATE POLICY upload_deletion_org ON upload_deletion
    USING (organization_id = app_org_id())
    WITH CHECK (organization_id = app_org_id());

-- +goose Down
DROP TABLE upload_deletion;
DROP INDEX upload_cash_receipt_clean;
