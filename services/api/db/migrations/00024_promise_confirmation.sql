-- +goose Up

-- Slice 4 (S4-5): the "janji tercatat" message a tenant gets when a promise version is saved, from
-- the payment page or by a member. One row per version, inserted in the transaction that saves
-- the version, so a confirmation can never be lost between the save and the queue: the
-- promise:confirm task sends it, and the sweep (reminders:sweep) re-enqueues a row still pending
-- after a lost enqueue.
--
-- Sending is at most once, like a reminder's: the row is marked sent (or skipped) and committed
-- before the provider is called, so a crash after the commit skips the message instead of sending
-- it twice. Slice 5's message log replaces this. status and sent_at are the only columns that
-- change; nothing is deleted.
CREATE TABLE promise_confirmation (
    promise_version_id uuid        PRIMARY KEY,
    organization_id    uuid        NOT NULL REFERENCES organization (id),
    tenant_id          uuid        NOT NULL,
    status             text        NOT NULL DEFAULT 'pending' CONSTRAINT promise_confirmation_status
        CHECK (status IN ('pending', 'sent', 'skipped')),
    sent_at            timestamptz,
    created_at         timestamptz NOT NULL,
    -- A sent confirmation says when; a pending or skipped one has no time.
    CONSTRAINT promise_confirmation_sent_at CHECK ((status = 'sent') = (sent_at IS NOT NULL)),
    CONSTRAINT promise_confirmation_version_same_tenant
        FOREIGN KEY (promise_version_id, tenant_id, organization_id) REFERENCES promise_version (id, tenant_id, organization_id)
);
CREATE INDEX promise_confirmation_org_status ON promise_confirmation (organization_id, status, created_at, promise_version_id);

REVOKE UPDATE, DELETE, TRUNCATE ON promise_confirmation FROM cashflow_app;
GRANT UPDATE (status, sent_at) ON promise_confirmation TO cashflow_app;

ALTER TABLE promise_confirmation ENABLE ROW LEVEL SECURITY;
ALTER TABLE promise_confirmation FORCE ROW LEVEL SECURITY;
CREATE POLICY promise_confirmation_org ON promise_confirmation
    USING (organization_id = app_org_id())
    WITH CHECK (organization_id = app_org_id());

-- +goose Down
DROP TABLE promise_confirmation;
