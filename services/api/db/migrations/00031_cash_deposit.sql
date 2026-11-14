-- +goose Up

-- Slice 6 (S6-3, decision 46.3): "Setor tunai". A karyawan hands the cash they collected to the
-- office: one deposit covers every field cash payment they recorded that no deposit covers yet
-- (field cash: a cash payment with a receipt, recorded through recordCashPayment). The admin
-- counts the money and confirms the amount received; a difference from the payments' total is a
-- "selisih setoran", which the check queue (S6-6) shows until the admin closes it with a note.
-- The tenants' balances never change: their payments were recorded when the cash was taken.
--
-- All three tables are append-only (privileges revoked and a trigger): a deposit, the payments it
-- covers and its confirmation are written once and never changed or deleted.

-- The keys the deposit tables name: a payment with the member who recorded it, and an upload with
-- its purpose.
ALTER TABLE payment ADD CONSTRAINT payment_id_org_recorder UNIQUE (id, organization_id, recorded_by_staff_user_id);
ALTER TABLE upload ADD CONSTRAINT upload_id_org_purpose UNIQUE (id, organization_id, purpose);

CREATE TABLE cash_deposit (
    id                   uuid        PRIMARY KEY,
    organization_id      uuid        NOT NULL REFERENCES organization (id),
    -- The member who hands the cash over; every payment it covers was recorded by them.
    staff_user_id        uuid        NOT NULL,
    -- Made by the client once per form; the same one again records nothing new.
    request_id           uuid        NOT NULL,
    -- What the member says they hand over ("Jumlah yang disetor").
    amount               bigint      NOT NULL CONSTRAINT cash_deposit_amount CHECK (amount >= 0),
    -- The payments it covers, net of their corrections when it was recorded, and how many.
    expected_amount      bigint      NOT NULL,
    payment_count        integer     NOT NULL CONSTRAINT cash_deposit_payment_count CHECK (payment_count > 0),
    note                 text,
    -- An optional photo of the money: a cash_deposit upload by the same member.
    photo_upload_id      uuid        CONSTRAINT cash_deposit_photo_once UNIQUE,
    photo_upload_purpose text        NOT NULL DEFAULT 'cash_deposit'
        CONSTRAINT cash_deposit_photo_purpose CHECK (photo_upload_purpose = 'cash_deposit'),
    created_at           timestamptz NOT NULL,
    CONSTRAINT cash_deposit_request UNIQUE (organization_id, request_id),
    UNIQUE (id, organization_id, staff_user_id),
    FOREIGN KEY (organization_id, staff_user_id) REFERENCES membership (organization_id, staff_user_id),
    FOREIGN KEY (photo_upload_id, organization_id, photo_upload_purpose) REFERENCES upload (id, organization_id, purpose)
);
-- The member's deposits and the organization's, newest first.
CREATE INDEX cash_deposit_staff ON cash_deposit (organization_id, staff_user_id, created_at DESC, id);
CREATE INDEX cash_deposit_org ON cash_deposit (organization_id, created_at DESC, id);

-- The cash payments a deposit covers. A payment is deposited once (the primary key), only by the
-- member who recorded it, and only if it is field cash (it has a receipt).
CREATE TABLE cash_deposit_payment (
    payment_id      uuid NOT NULL PRIMARY KEY,
    organization_id uuid NOT NULL,
    deposit_id      uuid NOT NULL,
    staff_user_id   uuid NOT NULL,
    FOREIGN KEY (deposit_id, organization_id, staff_user_id) REFERENCES cash_deposit (id, organization_id, staff_user_id),
    FOREIGN KEY (payment_id, organization_id, staff_user_id) REFERENCES payment (id, organization_id, recorded_by_staff_user_id),
    FOREIGN KEY (payment_id) REFERENCES payment_receipt (payment_id)
);
CREATE INDEX cash_deposit_payment_deposit ON cash_deposit_payment (organization_id, deposit_id);

-- The admin's count of a deposit: once per deposit, never by the member who deposited it.
CREATE TABLE cash_deposit_confirmation (
    deposit_id                uuid        PRIMARY KEY,
    organization_id           uuid        NOT NULL,
    -- The depositor, to keep them from confirming their own deposit.
    staff_user_id             uuid        NOT NULL,
    received_amount           bigint      NOT NULL CONSTRAINT cash_deposit_confirmation_amount CHECK (received_amount >= 0),
    note                      text,
    confirmed_by_staff_user_id uuid       NOT NULL,
    confirmed_at              timestamptz NOT NULL,
    CONSTRAINT cash_deposit_confirmation_not_self CHECK (confirmed_by_staff_user_id <> staff_user_id),
    FOREIGN KEY (deposit_id, organization_id, staff_user_id) REFERENCES cash_deposit (id, organization_id, staff_user_id),
    FOREIGN KEY (organization_id, confirmed_by_staff_user_id) REFERENCES membership (organization_id, staff_user_id)
);

REVOKE UPDATE, DELETE, TRUNCATE ON cash_deposit, cash_deposit_payment, cash_deposit_confirmation FROM cashflow_app;

CREATE TRIGGER cash_deposit_append_only BEFORE UPDATE OR DELETE ON cash_deposit
    FOR EACH ROW EXECUTE FUNCTION reject_change();
CREATE TRIGGER cash_deposit_no_truncate BEFORE TRUNCATE ON cash_deposit
    FOR EACH STATEMENT EXECUTE FUNCTION reject_change();
CREATE TRIGGER cash_deposit_payment_append_only BEFORE UPDATE OR DELETE ON cash_deposit_payment
    FOR EACH ROW EXECUTE FUNCTION reject_change();
CREATE TRIGGER cash_deposit_payment_no_truncate BEFORE TRUNCATE ON cash_deposit_payment
    FOR EACH STATEMENT EXECUTE FUNCTION reject_change();
CREATE TRIGGER cash_deposit_confirmation_append_only BEFORE UPDATE OR DELETE ON cash_deposit_confirmation
    FOR EACH ROW EXECUTE FUNCTION reject_change();
CREATE TRIGGER cash_deposit_confirmation_no_truncate BEFORE TRUNCATE ON cash_deposit_confirmation
    FOR EACH STATEMENT EXECUTE FUNCTION reject_change();

ALTER TABLE cash_deposit ENABLE ROW LEVEL SECURITY;
ALTER TABLE cash_deposit FORCE ROW LEVEL SECURITY;
CREATE POLICY cash_deposit_org ON cash_deposit
    USING (organization_id = app_org_id())
    WITH CHECK (organization_id = app_org_id());

ALTER TABLE cash_deposit_payment ENABLE ROW LEVEL SECURITY;
ALTER TABLE cash_deposit_payment FORCE ROW LEVEL SECURITY;
CREATE POLICY cash_deposit_payment_org ON cash_deposit_payment
    USING (organization_id = app_org_id())
    WITH CHECK (organization_id = app_org_id());

ALTER TABLE cash_deposit_confirmation ENABLE ROW LEVEL SECURITY;
ALTER TABLE cash_deposit_confirmation FORCE ROW LEVEL SECURITY;
CREATE POLICY cash_deposit_confirmation_org ON cash_deposit_confirmation
    USING (organization_id = app_org_id())
    WITH CHECK (organization_id = app_org_id());

-- +goose Down
DROP TABLE cash_deposit_confirmation;
DROP TABLE cash_deposit_payment;
DROP TABLE cash_deposit;
ALTER TABLE upload DROP CONSTRAINT upload_id_org_purpose;
ALTER TABLE payment DROP CONSTRAINT payment_id_org_recorder;
