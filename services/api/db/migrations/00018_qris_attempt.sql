-- +goose Up

-- Slice 3 (S3-5): QRIS attempts, one row per QR code AutoGoPay made for a tenant.

-- An attempt's tenant must be its link's tenant; this key lets qris_attempt say so.
ALTER TABLE payment_link ADD CONSTRAINT payment_link_id_tenant_org_key UNIQUE (id, tenant_id, organization_id);

-- One QRIS code generated through AutoGoPay for a tenant, from the payment page behind one of the
-- tenant's links. The amount is what the tenant chose to pay (whole rupiah, within AutoGoPay's
-- range); transaction_id is AutoGoPay's id for the code and the key every later step (webhook,
-- qris-paid, reconcile) finds the attempt by, so it is unique across organizations.
--
-- The status changes in place (pending, then paid, expired or cancelled): an attempt is not money.
-- The money is the payment row qris-paid inserts (slice 3, S3-6), which is append-only like every
-- money row. So the app role may update only status, closed_at and check_requested_at, and may
-- not delete an attempt.
--
-- A tenant has at most one live QR: one pending attempt at a time, whichever of the tenant's
-- links it came from. AutoGoPay treats repeated generates as spam, so the generate endpoint must
-- close (cancel or expire) the pending attempt before it records a new one; whether it reuses the
-- live QR instead is decided with that endpoint.
--
-- 'paid' is terminal: once the payment is recorded nothing about the attempt changes again, so a
-- late expiry or cancel cannot hide a payment. A closed attempt never goes back to pending. Every
-- other transition is allowed, including expired to paid and cancelled to paid: a QR can be paid
-- in its last second and the webhook or reconcile arrive after it was marked expired, and a
-- cancel can race a payment.
CREATE TABLE qris_attempt (
    id                 uuid        PRIMARY KEY,
    organization_id    uuid        NOT NULL REFERENCES organization (id),
    tenant_id          uuid        NOT NULL,
    payment_link_id    uuid        NOT NULL,
    amount             bigint      NOT NULL CONSTRAINT qris_attempt_amount
        CHECK (amount BETWEEN 1 AND 10000000),
    transaction_id     text        NOT NULL UNIQUE CONSTRAINT qris_attempt_transaction_id
        CHECK (length(transaction_id) BETWEEN 1 AND 100),
    order_id           text        NOT NULL DEFAULT '' CONSTRAINT qris_attempt_order_id
        CHECK (length(order_id) <= 100),
    -- qrimage.MaxQRStringBytes bounds it.
    qr_string          text        NOT NULL CONSTRAINT qris_attempt_qr_string
        CHECK (octet_length(qr_string) BETWEEN 1 AND 1024),
    status             text        NOT NULL CONSTRAINT qris_attempt_status
        CHECK (status IN ('pending', 'paid', 'expired', 'cancelled')),
    expires_at         timestamptz NOT NULL,
    created_at         timestamptz NOT NULL DEFAULT now(),
    -- When the attempt left pending.
    closed_at          timestamptz,
    -- When the tenant tapped "Saya sudah bayar"; the page shows "Sedang dicek" from then on.
    check_requested_at timestamptz,
    CONSTRAINT qris_attempt_closed CHECK ((status = 'pending') = (closed_at IS NULL)),
    -- The attempt's tenant is its link's tenant.
    FOREIGN KEY (payment_link_id, tenant_id, organization_id) REFERENCES payment_link (id, tenant_id, organization_id),
    FOREIGN KEY (tenant_id, organization_id) REFERENCES tenant (id, organization_id)
);
CREATE UNIQUE INDEX qris_attempt_one_pending ON qris_attempt (organization_id, tenant_id) WHERE status = 'pending';
CREATE INDEX qris_attempt_latest ON qris_attempt (organization_id, tenant_id, created_at DESC, id DESC);

REVOKE UPDATE, DELETE, TRUNCATE ON qris_attempt FROM cashflow_app;
GRANT UPDATE (status, closed_at, check_requested_at) ON qris_attempt TO cashflow_app;

-- A paid attempt is final, and a closed attempt never goes back to pending (a status read that
-- lags behind AutoGoPay must not revive a dead QR and take the tenant's one pending slot), for
-- every role. It runs as the caller, on the row it changes.
-- +goose StatementBegin
CREATE FUNCTION reject_paid_qris_attempt_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.status = 'paid' THEN
        RAISE EXCEPTION 'qris_attempt % is paid', OLD.id USING ERRCODE = 'insufficient_privilege';
    END IF;
    IF OLD.status <> 'pending' AND NEW.status = 'pending' THEN
        RAISE EXCEPTION 'qris_attempt % is closed', OLD.id USING ERRCODE = 'insufficient_privilege';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER qris_attempt_paid_final BEFORE UPDATE ON qris_attempt
    FOR EACH ROW EXECUTE FUNCTION reject_paid_qris_attempt_change();

ALTER TABLE qris_attempt ENABLE ROW LEVEL SECURITY;
ALTER TABLE qris_attempt FORCE ROW LEVEL SECURITY;
CREATE POLICY qris_attempt_org ON qris_attempt
    USING (organization_id = app_org_id())
    WITH CHECK (organization_id = app_org_id());

-- +goose Down
DROP TABLE qris_attempt;
DROP FUNCTION reject_paid_qris_attempt_change();
ALTER TABLE payment_link DROP CONSTRAINT payment_link_id_tenant_org_key;
