-- +goose Up

-- TG-5 (decision 42, demo only): a staff notice tells the staff who look after a tenant that
-- something happened to them: the tenant made or edited a promise on the payment page
-- (promise_made, one per promise version), a promise closed as broken (promise_broken, one per
-- promise), or a visit task opened (visit_task, one per task). It goes only to members with a
-- live Telegram link, and only with TELEGRAM_MESSAGES=linked; with the setting off nothing is
-- recorded.
--
-- A notice is inserted in the transaction that makes its event (the version save, the broken
-- close, the visit task open), so it cannot be lost between the event and the queue: the
-- staff:notice task sends it, and the sweep (reminders:sweep) re-enqueues a notice still pending
-- after a lost enqueue. Its recipients are decided when it is sent, not stored.
--
-- Sending is at most once, like a promise confirmation's (00024): the row is marked sent (or
-- skipped) and committed before any message goes out. status and sent_at are the only columns
-- that change; nothing is deleted.

-- The composite key a staff notice's foreign key names, so a notice and its visit task share
-- one tenant, as promise_version and payment_promise already allow.
ALTER TABLE visit_task ADD CONSTRAINT visit_task_id_tenant UNIQUE (id, tenant_id, organization_id);

CREATE TABLE staff_notice (
    id                 uuid        PRIMARY KEY,
    organization_id    uuid        NOT NULL REFERENCES organization (id),
    tenant_id          uuid        NOT NULL,
    kind               text        NOT NULL CONSTRAINT staff_notice_kind
        CHECK (kind IN ('promise_made', 'promise_broken', 'visit_task')),
    -- Exactly one source, the one its kind names; each is noticed at most once.
    promise_version_id uuid        CONSTRAINT staff_notice_promise_version UNIQUE,
    promise_id         uuid        CONSTRAINT staff_notice_promise UNIQUE,
    visit_task_id      uuid        CONSTRAINT staff_notice_visit_task UNIQUE,
    status             text        NOT NULL DEFAULT 'pending' CONSTRAINT staff_notice_status
        CHECK (status IN ('pending', 'sent', 'skipped')),
    sent_at            timestamptz,
    created_at         timestamptz NOT NULL,
    CONSTRAINT staff_notice_source CHECK (
        (kind = 'promise_made') = (promise_version_id IS NOT NULL)
        AND (kind = 'promise_broken') = (promise_id IS NOT NULL)
        AND (kind = 'visit_task') = (visit_task_id IS NOT NULL)
    ),
    -- A sent notice says when; a pending or skipped one has no time.
    CONSTRAINT staff_notice_sent_at CHECK ((status = 'sent') = (sent_at IS NOT NULL)),
    FOREIGN KEY (tenant_id, organization_id) REFERENCES tenant (id, organization_id),
    CONSTRAINT staff_notice_version_same_tenant
        FOREIGN KEY (promise_version_id, tenant_id, organization_id) REFERENCES promise_version (id, tenant_id, organization_id),
    CONSTRAINT staff_notice_promise_same_tenant
        FOREIGN KEY (promise_id, tenant_id, organization_id) REFERENCES payment_promise (id, tenant_id, organization_id),
    CONSTRAINT staff_notice_visit_task_same_tenant
        FOREIGN KEY (visit_task_id, tenant_id, organization_id) REFERENCES visit_task (id, tenant_id, organization_id)
);
CREATE INDEX staff_notice_org_status ON staff_notice (organization_id, status, created_at, id);

REVOKE UPDATE, DELETE, TRUNCATE ON staff_notice FROM cashflow_app;
GRANT UPDATE (status, sent_at) ON staff_notice TO cashflow_app;

ALTER TABLE staff_notice ENABLE ROW LEVEL SECURITY;
ALTER TABLE staff_notice FORCE ROW LEVEL SECURITY;
CREATE POLICY staff_notice_org ON staff_notice
    USING (organization_id = app_org_id())
    WITH CHECK (organization_id = app_org_id());

-- +goose Down
DROP TABLE staff_notice;
ALTER TABLE visit_task DROP CONSTRAINT visit_task_id_tenant;
