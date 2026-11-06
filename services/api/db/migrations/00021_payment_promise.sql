-- +goose Up

-- Slice 4 (S4-2): payment promises ("Janji pelunasan", decision 41).

-- A tenant's promise to pay what they owe by a date. It belongs to the tenant, not to one invoice
-- (CLAUDE.md "Billing and reminders"). Its terms live in promise_version: making a promise while
-- one is active adds a version to it (an edit), so the full history is kept.
--
-- The status changes in place, once: active, then kept, partially_kept or broken when the
-- promise is closed (slice 4, S4-3). A promise is not money, so the app role may update only
-- status and closed_at, and may not delete a promise. A tenant has at most one active promise.
CREATE TABLE payment_promise (
    id              uuid        PRIMARY KEY,
    organization_id uuid        NOT NULL REFERENCES organization (id),
    tenant_id       uuid        NOT NULL,
    -- Always NULL for now: a promise covers everything the tenant owes over all their leases.
    -- Kept for a later promise limited to one lease.
    lease_id        uuid,
    status          text        NOT NULL CONSTRAINT payment_promise_status
        CHECK (status IN ('active', 'kept', 'partially_kept', 'broken')),
    created_at      timestamptz NOT NULL DEFAULT now(),
    -- When the promise left active.
    closed_at       timestamptz,
    CONSTRAINT payment_promise_closed CHECK ((status = 'active') = (closed_at IS NULL)),
    UNIQUE (id, tenant_id, organization_id),
    FOREIGN KEY (tenant_id, organization_id) REFERENCES tenant (id, organization_id),
    FOREIGN KEY (lease_id, tenant_id, organization_id) REFERENCES lease (id, tenant_id, organization_id)
);
CREATE UNIQUE INDEX payment_promise_one_active ON payment_promise (organization_id, tenant_id) WHERE status = 'active';
CREATE INDEX payment_promise_tenant_latest ON payment_promise (organization_id, tenant_id, created_at DESC, id DESC);

REVOKE UPDATE, DELETE, TRUNCATE ON payment_promise FROM cashflow_app;
GRANT UPDATE (status, closed_at) ON payment_promise TO cashflow_app;

-- A closed promise is final for every role: closing decides kept, partially kept or broken once,
-- and the broken count (the visit list threshold) must not change after the fact. It runs as the
-- caller, on the row it changes.
-- +goose StatementBegin
CREATE FUNCTION reject_closed_promise_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.status <> 'active' THEN
        RAISE EXCEPTION 'payment_promise % is closed', OLD.id USING ERRCODE = 'insufficient_privilege';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER payment_promise_closed_final BEFORE UPDATE ON payment_promise
    FOR EACH ROW EXECUTE FUNCTION reject_closed_promise_change();

-- One version of a promise's terms, numbered from 1. Append-only: an edit is the next version.
--
-- promised_date is the day the tenant promised to pay by (Asia/Jakarta), tomorrow to 60 days
-- ahead when made; for with_next_invoice the server set it to the next invoice's due date.
-- promised_amount is what the tenant promised to pay by then; NULL means everything the promise
-- covers. paying_now is what the tenant said they pay right after saving ("Simpan janji, lalu
-- bayar Rp X"), so it is not part of the promised rest. outstanding_at is what the tenant owed
-- when the version was made. tenant_id repeats the promise's tenant so the foreign keys can hold
-- the payment link to the same tenant.
--
-- A tenant makes one from the payment page through a link (payment_link_id, no staff member, no
-- note); a member makes one from the app (created_by_staff_user_id, optional note).
CREATE TABLE promise_version (
    id                       uuid        PRIMARY KEY,
    organization_id          uuid        NOT NULL,
    promise_id               uuid        NOT NULL,
    tenant_id                uuid        NOT NULL,
    version                  integer     NOT NULL CONSTRAINT promise_version_number CHECK (version >= 1),
    kind                     text        NOT NULL CONSTRAINT promise_version_kind
        CHECK (kind IN ('specific_date', 'with_next_invoice')),
    promised_date            date        NOT NULL,
    promised_amount          bigint      CONSTRAINT promise_version_promised_amount CHECK (promised_amount > 0),
    paying_now               bigint      NOT NULL DEFAULT 0 CONSTRAINT promise_version_paying_now CHECK (paying_now >= 0),
    outstanding_at           bigint      NOT NULL CONSTRAINT promise_version_outstanding_at CHECK (outstanding_at >= 0),
    source                   text        NOT NULL CONSTRAINT promise_version_source
        CHECK (source IN ('tenant_page', 'karyawan', 'admin')),
    note                     text        CONSTRAINT promise_version_note
        CHECK (length(note) BETWEEN 1 AND 500 AND note = btrim(note)),
    created_by_staff_user_id uuid,
    payment_link_id          uuid,
    created_at               timestamptz NOT NULL DEFAULT now(),
    UNIQUE (promise_id, version),
    UNIQUE (id, tenant_id, organization_id),
    -- Something is left to promise after what is paid now, and the promised amount fits in it.
    CONSTRAINT promise_version_something_promised CHECK (paying_now < outstanding_at),
    CONSTRAINT promise_version_amount_fits CHECK (promised_amount <= outstanding_at - paying_now),
    -- The tenant page names its link and no member; a member names themselves and no link.
    CONSTRAINT promise_version_link_from_page CHECK ((source = 'tenant_page') = (payment_link_id IS NOT NULL)),
    CONSTRAINT promise_version_staff_from_app CHECK ((source = 'tenant_page') = (created_by_staff_user_id IS NULL)),
    CONSTRAINT promise_version_note_from_staff CHECK (source <> 'tenant_page' OR note IS NULL),
    CONSTRAINT promise_version_promise_same_tenant
        FOREIGN KEY (promise_id, tenant_id, organization_id) REFERENCES payment_promise (id, tenant_id, organization_id),
    CONSTRAINT promise_version_link_same_tenant
        FOREIGN KEY (payment_link_id, tenant_id, organization_id) REFERENCES payment_link (id, tenant_id, organization_id),
    FOREIGN KEY (organization_id, created_by_staff_user_id) REFERENCES membership (organization_id, staff_user_id)
);

-- The invoices a version covers: the tenant's invoices still owing something when it was made
-- (decision 41.3). Closing the promise (S4-3) decides kept or broken from them, and reminders
-- (S4-4) skip H+3 and H+7 for them while the promise is active. Append-only.
CREATE TABLE promise_covered_invoice (
    organization_id    uuid NOT NULL,
    promise_version_id uuid NOT NULL,
    invoice_id         uuid NOT NULL,
    tenant_id          uuid NOT NULL,
    PRIMARY KEY (promise_version_id, invoice_id),
    -- The invoice is the promise's tenant's.
    CONSTRAINT promise_covered_invoice_version_same_tenant
        FOREIGN KEY (promise_version_id, tenant_id, organization_id) REFERENCES promise_version (id, tenant_id, organization_id),
    CONSTRAINT promise_covered_invoice_same_tenant
        FOREIGN KEY (invoice_id, tenant_id, organization_id) REFERENCES invoice (id, tenant_id, organization_id)
);
CREATE INDEX promise_covered_invoice_invoice ON promise_covered_invoice (organization_id, invoice_id);

-- Append-only, for every role, like the money rows (00011_money.sql).
REVOKE UPDATE, DELETE, TRUNCATE ON promise_version, promise_covered_invoice FROM cashflow_app;

CREATE TRIGGER promise_version_append_only BEFORE UPDATE OR DELETE ON promise_version
    FOR EACH ROW EXECUTE FUNCTION reject_change();
CREATE TRIGGER promise_version_no_truncate BEFORE TRUNCATE ON promise_version
    FOR EACH STATEMENT EXECUTE FUNCTION reject_change();
CREATE TRIGGER promise_covered_invoice_append_only BEFORE UPDATE OR DELETE ON promise_covered_invoice
    FOR EACH ROW EXECUTE FUNCTION reject_change();
CREATE TRIGGER promise_covered_invoice_no_truncate BEFORE TRUNCATE ON promise_covered_invoice
    FOR EACH STATEMENT EXECUTE FUNCTION reject_change();

-- Row-level security ---------------------------------------------------------------------

ALTER TABLE payment_promise ENABLE ROW LEVEL SECURITY;
ALTER TABLE payment_promise FORCE ROW LEVEL SECURITY;
CREATE POLICY payment_promise_org ON payment_promise
    USING (organization_id = app_org_id())
    WITH CHECK (organization_id = app_org_id());

ALTER TABLE promise_version ENABLE ROW LEVEL SECURITY;
ALTER TABLE promise_version FORCE ROW LEVEL SECURITY;
CREATE POLICY promise_version_org ON promise_version
    USING (organization_id = app_org_id())
    WITH CHECK (organization_id = app_org_id());

ALTER TABLE promise_covered_invoice ENABLE ROW LEVEL SECURITY;
ALTER TABLE promise_covered_invoice FORCE ROW LEVEL SECURITY;
CREATE POLICY promise_covered_invoice_org ON promise_covered_invoice
    USING (organization_id = app_org_id())
    WITH CHECK (organization_id = app_org_id());

-- +goose Down
DROP TABLE promise_covered_invoice;
DROP TABLE promise_version;
DROP TABLE payment_promise;
DROP FUNCTION reject_closed_promise_change();
