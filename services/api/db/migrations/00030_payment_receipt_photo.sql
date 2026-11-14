-- +goose Up

-- The key a link names: an upload with its tenant and purpose.
ALTER TABLE upload ADD CONSTRAINT upload_id_tenant_purpose UNIQUE (id, tenant_id, organization_id, purpose);

-- Slice 6 (S6-2, decision 46.10): the photo of the cash a karyawan collected, attached to the cash
-- payment after it is saved ("foto belum ada" until then). The photo is an upload (00028) for the
-- payment's own tenant, made by the member who attaches it; this row only links the two.
--
-- Append-only: a link is never changed or deleted. A payment's photo is its latest link. A new
-- link is accepted only while every earlier one points at an upload the scan refused (rejected or
-- infected), so the karyawan can retake a bad photo but never swap a good one; the service holds
-- that rule under the payment receipt's row lock. An upload backs at most one payment.
CREATE TABLE payment_receipt_photo (
    upload_id                 uuid        PRIMARY KEY,
    -- The order of a payment's links: its latest is its photo.
    seq                       bigint      GENERATED ALWAYS AS IDENTITY UNIQUE,
    organization_id           uuid        NOT NULL REFERENCES organization (id),
    payment_id                uuid        NOT NULL,
    tenant_id                 uuid        NOT NULL,
    -- Always cash_receipt; part of the key below, so only a receipt photo can be linked.
    upload_purpose            text        NOT NULL DEFAULT 'cash_receipt'
        CONSTRAINT payment_receipt_photo_purpose CHECK (upload_purpose = 'cash_receipt'),
    attached_by_staff_user_id uuid        NOT NULL,
    attached_at               timestamptz NOT NULL,
    -- The payment is a cash payment with a receipt, of this tenant and organization.
    CONSTRAINT payment_receipt_photo_receipt
        FOREIGN KEY (payment_id) REFERENCES payment_receipt (payment_id),
    CONSTRAINT payment_receipt_photo_payment_same_tenant
        FOREIGN KEY (payment_id, tenant_id, organization_id) REFERENCES payment (id, tenant_id, organization_id),
    -- The upload is a receipt photo for the same tenant, in the same organization.
    CONSTRAINT payment_receipt_photo_upload_same_tenant
        FOREIGN KEY (upload_id, tenant_id, organization_id, upload_purpose)
        REFERENCES upload (id, tenant_id, organization_id, purpose),
    FOREIGN KEY (organization_id, attached_by_staff_user_id) REFERENCES membership (organization_id, staff_user_id)
);
-- A payment's links, latest first.
CREATE INDEX payment_receipt_photo_payment ON payment_receipt_photo (organization_id, payment_id, seq DESC);

REVOKE UPDATE, DELETE, TRUNCATE ON payment_receipt_photo FROM cashflow_app;
CREATE TRIGGER payment_receipt_photo_append_only BEFORE UPDATE OR DELETE ON payment_receipt_photo
    FOR EACH ROW EXECUTE FUNCTION reject_change();
CREATE TRIGGER payment_receipt_photo_no_truncate BEFORE TRUNCATE ON payment_receipt_photo
    FOR EACH STATEMENT EXECUTE FUNCTION reject_change();

ALTER TABLE payment_receipt_photo ENABLE ROW LEVEL SECURITY;
ALTER TABLE payment_receipt_photo FORCE ROW LEVEL SECURITY;
CREATE POLICY payment_receipt_photo_org ON payment_receipt_photo
    USING (organization_id = app_org_id())
    WITH CHECK (organization_id = app_org_id());

-- +goose Down
DROP TABLE payment_receipt_photo;
ALTER TABLE upload DROP CONSTRAINT upload_id_tenant_purpose;
