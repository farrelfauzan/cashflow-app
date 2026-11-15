-- +goose Up

-- Slice 6 (S6-4, decision 46.6): what a karyawan records after visiting a tenant on their visit
-- list ("Catat hasil kunjungan"). An outcome points at what the field app already recorded: the
-- cash payment (recordCashPayment) for paid, the promise version (makeFieldPromise) for promise;
-- number_changed carries the new number for the admin to apply (the karyawan cannot change a
-- tenant); not_home carries nothing.
--
-- An outcome resolves the tenant's open visit tasks it answers: paid and promise resolve the h7
-- and broken_promises tasks, number_changed the undeliverable one, not_home none (the tenant stays
-- on the list). visit_outcome_task records which, once per task.
--
-- Both tables are append-only (privileges revoked and triggers).

-- The keys the outcome tables name: a visit task with its tenant.
ALTER TABLE visit_task ADD CONSTRAINT visit_task_id_tenant_org UNIQUE (id, tenant_id, organization_id);

CREATE TABLE visit_outcome (
    id                 uuid        PRIMARY KEY,
    organization_id    uuid        NOT NULL REFERENCES organization (id),
    tenant_id          uuid        NOT NULL,
    -- The member who visited and recorded it.
    staff_user_id      uuid        NOT NULL,
    -- Made by the client once per form; the same one again records nothing new.
    request_id         uuid        NOT NULL,
    outcome            text        NOT NULL CONSTRAINT visit_outcome_kind
        CHECK (outcome IN ('paid', 'promise', 'not_home', 'number_changed')),
    -- paid: the field cash payment this member took from the tenant; each backs one outcome.
    payment_id         uuid        CONSTRAINT visit_outcome_payment_once UNIQUE,
    -- promise: the promise version this member made for the tenant; each backs one outcome.
    promise_version_id uuid        CONSTRAINT visit_outcome_promise_once UNIQUE,
    -- number_changed: the tenant's new WhatsApp number, E.164, for the admin to apply.
    new_phone          text        CONSTRAINT visit_outcome_new_phone_e164 CHECK (new_phone ~ '^\+[1-9][0-9]{7,14}$'),
    note               text        CONSTRAINT visit_outcome_note
        CHECK (length(note) BETWEEN 1 AND 500 AND note = btrim(note)),
    created_at         timestamptz NOT NULL,
    CONSTRAINT visit_outcome_request UNIQUE (organization_id, request_id),
    UNIQUE (id, tenant_id, organization_id),
    CONSTRAINT visit_outcome_paid_names_payment CHECK ((outcome = 'paid') = (payment_id IS NOT NULL)),
    CONSTRAINT visit_outcome_promise_names_version CHECK ((outcome = 'promise') = (promise_version_id IS NOT NULL)),
    CONSTRAINT visit_outcome_number_names_phone CHECK ((outcome = 'number_changed') = (new_phone IS NOT NULL)),
    FOREIGN KEY (tenant_id, organization_id) REFERENCES tenant (id, organization_id),
    FOREIGN KEY (organization_id, staff_user_id) REFERENCES membership (organization_id, staff_user_id),
    -- The payment is the tenant's, recorded by this member, and field cash (it has a receipt).
    FOREIGN KEY (payment_id, tenant_id, organization_id) REFERENCES payment (id, tenant_id, organization_id),
    FOREIGN KEY (payment_id, organization_id, staff_user_id) REFERENCES payment (id, organization_id, recorded_by_staff_user_id),
    FOREIGN KEY (payment_id) REFERENCES payment_receipt (payment_id),
    -- The promise version is the tenant's.
    FOREIGN KEY (promise_version_id, tenant_id, organization_id) REFERENCES promise_version (id, tenant_id, organization_id)
);
-- A member's outcomes of a day, for the visit list.
CREATE INDEX visit_outcome_staff_day ON visit_outcome (organization_id, staff_user_id, created_at);

-- The open visit tasks an outcome resolved, each once.
CREATE TABLE visit_outcome_task (
    visit_task_id   uuid NOT NULL PRIMARY KEY,
    organization_id uuid NOT NULL,
    tenant_id       uuid NOT NULL,
    outcome_id      uuid NOT NULL,
    FOREIGN KEY (visit_task_id, tenant_id, organization_id) REFERENCES visit_task (id, tenant_id, organization_id),
    FOREIGN KEY (outcome_id, tenant_id, organization_id) REFERENCES visit_outcome (id, tenant_id, organization_id)
);
CREATE INDEX visit_outcome_task_outcome ON visit_outcome_task (organization_id, outcome_id);

REVOKE UPDATE, DELETE, TRUNCATE ON visit_outcome, visit_outcome_task FROM cashflow_app;

CREATE TRIGGER visit_outcome_append_only BEFORE UPDATE OR DELETE ON visit_outcome
    FOR EACH ROW EXECUTE FUNCTION reject_change();
CREATE TRIGGER visit_outcome_no_truncate BEFORE TRUNCATE ON visit_outcome
    FOR EACH STATEMENT EXECUTE FUNCTION reject_change();
CREATE TRIGGER visit_outcome_task_append_only BEFORE UPDATE OR DELETE ON visit_outcome_task
    FOR EACH ROW EXECUTE FUNCTION reject_change();
CREATE TRIGGER visit_outcome_task_no_truncate BEFORE TRUNCATE ON visit_outcome_task
    FOR EACH STATEMENT EXECUTE FUNCTION reject_change();

ALTER TABLE visit_outcome ENABLE ROW LEVEL SECURITY;
ALTER TABLE visit_outcome FORCE ROW LEVEL SECURITY;
CREATE POLICY visit_outcome_org ON visit_outcome
    USING (organization_id = app_org_id())
    WITH CHECK (organization_id = app_org_id());

ALTER TABLE visit_outcome_task ENABLE ROW LEVEL SECURITY;
ALTER TABLE visit_outcome_task FORCE ROW LEVEL SECURITY;
CREATE POLICY visit_outcome_task_org ON visit_outcome_task
    USING (organization_id = app_org_id())
    WITH CHECK (organization_id = app_org_id());

-- +goose Down
DROP TABLE visit_outcome_task;
DROP TABLE visit_outcome;
ALTER TABLE visit_task DROP CONSTRAINT visit_task_id_tenant_org;
