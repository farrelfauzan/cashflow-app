-- +goose Up

-- Slice 6 (S6-2, decision 46.5): the receipt a tenant gets the moment a karyawan records their
-- cash ("Terima tunai"). Cash payments only for now. One row per cash payment, inserted in the
-- transaction that records the payment, so a receipt can never be lost between the payment and
-- the queue: the receipt:send task sends it, and the sweep (reminders:sweep) re-enqueues a row
-- still pending after a lost enqueue.
--
-- Sending is at most once, like a promise confirmation's (00024): the row is marked sent (or
-- skipped) and committed before the provider is called, so a crash after the commit loses the
-- message instead of sending it twice. Slice 5's message log takes the send over later; the
-- template keeps its name and parameters.
--
-- status and sent_at are the only columns that change, and only once: a trigger refuses any
-- change to a row that is no longer pending, for every role. Nothing is deleted.
CREATE TABLE payment_receipt (
    payment_id      uuid        PRIMARY KEY,
    organization_id uuid        NOT NULL REFERENCES organization (id),
    tenant_id       uuid        NOT NULL,
    status          text        NOT NULL DEFAULT 'pending' CONSTRAINT payment_receipt_status
        CHECK (status IN ('pending', 'sent', 'skipped')),
    sent_at         timestamptz,
    created_at      timestamptz NOT NULL,
    -- A sent receipt says when; a pending or skipped one has no time.
    CONSTRAINT payment_receipt_sent_at CHECK ((status = 'sent') = (sent_at IS NOT NULL)),
    -- The receipt belongs to the payment's own tenant and organization.
    CONSTRAINT payment_receipt_payment_same_tenant
        FOREIGN KEY (payment_id, tenant_id, organization_id) REFERENCES payment (id, tenant_id, organization_id)
);
CREATE INDEX payment_receipt_org_status ON payment_receipt (organization_id, status, created_at, payment_id);

REVOKE UPDATE, DELETE, TRUNCATE ON payment_receipt FROM cashflow_app;
GRANT UPDATE (status, sent_at) ON payment_receipt TO cashflow_app;

-- +goose StatementBegin
CREATE FUNCTION payment_receipt_forward_only() RETURNS trigger
    LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP <> 'UPDATE' THEN
        RAISE EXCEPTION 'payment_receipt is never deleted' USING ERRCODE = '42501';
    END IF;
    IF OLD.status <> 'pending' THEN
        RAISE EXCEPTION 'payment_receipt % is already %', OLD.payment_id, OLD.status USING ERRCODE = '42501';
    END IF;
    IF NEW.payment_id <> OLD.payment_id OR NEW.organization_id <> OLD.organization_id
        OR NEW.tenant_id <> OLD.tenant_id OR NEW.created_at <> OLD.created_at THEN
        RAISE EXCEPTION 'payment_receipt changes only its status' USING ERRCODE = '42501';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER payment_receipt_forward_only BEFORE UPDATE OR DELETE ON payment_receipt
    FOR EACH ROW EXECUTE FUNCTION payment_receipt_forward_only();
CREATE TRIGGER payment_receipt_no_truncate BEFORE TRUNCATE ON payment_receipt
    FOR EACH STATEMENT EXECUTE FUNCTION reject_change();

ALTER TABLE payment_receipt ENABLE ROW LEVEL SECURITY;
ALTER TABLE payment_receipt FORCE ROW LEVEL SECURITY;
CREATE POLICY payment_receipt_org ON payment_receipt
    USING (organization_id = app_org_id())
    WITH CHECK (organization_id = app_org_id());

-- "Tagihan hari ini" and the karyawan's cash on hand read the cash a member recorded.
CREATE INDEX payment_cash_recorded_by ON payment (organization_id, recorded_by_staff_user_id, received_at)
    WHERE method = 'cash';

-- +goose Down
DROP INDEX payment_cash_recorded_by;
DROP TABLE payment_receipt;
DROP FUNCTION payment_receipt_forward_only();
