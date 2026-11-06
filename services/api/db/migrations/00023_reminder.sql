-- +goose Up

-- Slice 4 (S4-4): what the daily reminders:decide run (08:00 Asia/Jakarta, decision 41.6) decided
-- to tell each tenant. It records decisions only; sending them is S4-5, which reads the pending
-- rows and sets their status.

-- One message to one tenant, decided on run_day (the Asia/Jakarta calendar day of the run). It
-- carries everything decided for the tenant in that run: invoice stages (reminder_invoice), the
-- H-1 of a promise version (promise_version_id) and the notice of a broken promise
-- (broken_promise_id). A row carries at least one of them, checked at commit by
-- reminder_carries_something below.
--
-- The H-1 of a version and the notice of a broken promise are each recorded once (the UNIQUE
-- constraints; NULLs never collide). Sending (S4-5) may set status and sent_at; nothing else
-- changes and nothing is deleted.
CREATE TABLE reminder (
    id                 uuid        PRIMARY KEY,
    organization_id    uuid        NOT NULL REFERENCES organization (id),
    tenant_id          uuid        NOT NULL,
    run_day            date        NOT NULL,
    promise_version_id uuid        CONSTRAINT reminder_promise_version_once UNIQUE,
    broken_promise_id  uuid        CONSTRAINT reminder_broken_promise_once UNIQUE,
    status             text        NOT NULL DEFAULT 'pending' CONSTRAINT reminder_status
        CHECK (status IN ('pending', 'sent', 'skipped')),
    sent_at            timestamptz,
    created_at         timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, tenant_id, organization_id),
    FOREIGN KEY (tenant_id, organization_id) REFERENCES tenant (id, organization_id),
    -- The promise version and the broken promise are the tenant's own.
    CONSTRAINT reminder_promise_version_same_tenant
        FOREIGN KEY (promise_version_id, tenant_id, organization_id) REFERENCES promise_version (id, tenant_id, organization_id),
    CONSTRAINT reminder_broken_promise_same_tenant
        FOREIGN KEY (broken_promise_id, tenant_id, organization_id) REFERENCES payment_promise (id, tenant_id, organization_id)
);
CREATE INDEX reminder_org_status ON reminder (organization_id, status, created_at, id);
CREATE INDEX reminder_tenant ON reminder (organization_id, tenant_id, run_day);

REVOKE UPDATE, DELETE, TRUNCATE ON reminder FROM cashflow_app;
GRANT UPDATE (status, sent_at) ON reminder TO cashflow_app;

-- One invoice stage a reminder carries: h_minus_3 (3 days before the due date), h0 (the due date)
-- or h_plus_3 (3 days after it). The primary key is the "never twice" rule: an invoice's stage is
-- recorded once, ever. Append-only for every role, like promise_covered_invoice.
CREATE TABLE reminder_invoice (
    organization_id uuid NOT NULL,
    reminder_id     uuid NOT NULL,
    invoice_id      uuid NOT NULL,
    tenant_id       uuid NOT NULL,
    stage           text NOT NULL CONSTRAINT reminder_invoice_stage
        CHECK (stage IN ('h_minus_3', 'h0', 'h_plus_3')),
    PRIMARY KEY (invoice_id, stage),
    -- The reminder and the invoice are the same tenant's.
    CONSTRAINT reminder_invoice_reminder_same_tenant
        FOREIGN KEY (reminder_id, tenant_id, organization_id) REFERENCES reminder (id, tenant_id, organization_id),
    CONSTRAINT reminder_invoice_same_tenant
        FOREIGN KEY (invoice_id, tenant_id, organization_id) REFERENCES invoice (id, tenant_id, organization_id)
);
CREATE INDEX reminder_invoice_reminder ON reminder_invoice (organization_id, reminder_id);
CREATE INDEX reminder_invoice_tenant ON reminder_invoice (organization_id, tenant_id);

REVOKE UPDATE, DELETE, TRUNCATE ON reminder_invoice FROM cashflow_app;

CREATE TRIGGER reminder_invoice_append_only BEFORE UPDATE OR DELETE ON reminder_invoice
    FOR EACH ROW EXECUTE FUNCTION reject_change();
CREATE TRIGGER reminder_invoice_no_truncate BEFORE TRUNCATE ON reminder_invoice
    FOR EACH STATEMENT EXECUTE FUNCTION reject_change();

-- A reminder carries something ---------------------------------------------------------------

-- At commit, a reminder has an H-1, a broken notice or at least one invoice stage, so an empty
-- message can never be sent. Deferred, so the reminder is inserted first and its stages after it
-- in the same transaction. It runs as the caller, inside the caller's organization scope; a
-- reminder the scope cannot see fails closed.
-- +goose StatementBegin
CREATE FUNCTION check_reminder_carries_something() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.promise_version_id IS NOT NULL OR NEW.broken_promise_id IS NOT NULL THEN
        RETURN NULL;
    END IF;
    PERFORM 1 FROM reminder_invoice WHERE reminder_id = NEW.id;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'reminder % carries nothing', NEW.id
            USING ERRCODE = 'check_violation', CONSTRAINT = 'reminder_carries_something';
    END IF;
    RETURN NULL;
END;
$$;
-- +goose StatementEnd

CREATE CONSTRAINT TRIGGER reminder_carries_something AFTER INSERT ON reminder
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION check_reminder_carries_something();

-- Row-level security ---------------------------------------------------------------------

ALTER TABLE reminder ENABLE ROW LEVEL SECURITY;
ALTER TABLE reminder FORCE ROW LEVEL SECURITY;
CREATE POLICY reminder_org ON reminder
    USING (organization_id = app_org_id())
    WITH CHECK (organization_id = app_org_id());

ALTER TABLE reminder_invoice ENABLE ROW LEVEL SECURITY;
ALTER TABLE reminder_invoice FORCE ROW LEVEL SECURITY;
CREATE POLICY reminder_invoice_org ON reminder_invoice
    USING (organization_id = app_org_id())
    WITH CHECK (organization_id = app_org_id());

-- +goose Down
DROP TABLE reminder_invoice;
DROP TABLE reminder;
DROP FUNCTION check_reminder_carries_something();
