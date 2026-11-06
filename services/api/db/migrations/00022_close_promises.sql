-- +goose Up

-- Slice 4 (S4-3): closing payment promises, the karyawan visit list and the broken-promise
-- threshold (decision 41.3 and 41.4).

-- Which of the tenant's payments came after a version: the tenant's highest payment.seq when the
-- version was made (0 when they had none), read under the tenant's allocation lock, which every
-- payment insert also holds. The payments since the version are the tenant's payments with a
-- higher seq. A timestamp cannot say it: payment.created_at is the database's now() and
-- promise_version.created_at is the application's clock.
--
-- NULL on versions made before this migration. Those count the tenant's payments with
-- created_at at or after the version's created_at instead (nothing is deployed yet, so there are
-- none outside development). Adding a column is DDL, which the append-only trigger does not see.
ALTER TABLE promise_version ADD COLUMN payment_seq_mark bigint
    CONSTRAINT promise_version_payment_seq_mark CHECK (payment_seq_mark >= 0);

-- How many broken promises put a tenant on the karyawan visit list (decision 41.4, CLAUDE.md
-- "Per-organization settings"). Set with cmd/admin until the settings screen (W28). The app role
-- already updates organization through its default privileges, inside its own scope.
ALTER TABLE organization ADD COLUMN broken_promise_threshold smallint NOT NULL DEFAULT 2
    CONSTRAINT organization_broken_promise_threshold CHECK (broken_promise_threshold BETWEEN 1 AND 10);

-- A tenant a karyawan should visit, and why: h7 (an invoice unpaid 7 days after its due date),
-- broken_promises (the tenant broke as many promises as the organization's threshold) or
-- undeliverable (WhatsApp could not reach them; a visit gets a new number). Slice 4 opens
-- broken_promises tasks; h7 and undeliverable come later.
--
-- A tenant has at most one open task per reason, so opening one that is already open does
-- nothing (INSERT ... ON CONFLICT DO NOTHING on visit_task_one_open). Resolving a task sets
-- resolved_at once; nothing else changes and nothing is deleted, so the list keeps its history.
CREATE TABLE visit_task (
    id              uuid        PRIMARY KEY,
    organization_id uuid        NOT NULL REFERENCES organization (id),
    tenant_id       uuid        NOT NULL,
    reason          text        NOT NULL CONSTRAINT visit_task_reason
        CHECK (reason IN ('h7', 'broken_promises', 'undeliverable')),
    created_at      timestamptz NOT NULL DEFAULT now(),
    resolved_at     timestamptz,
    FOREIGN KEY (tenant_id, organization_id) REFERENCES tenant (id, organization_id)
);
CREATE UNIQUE INDEX visit_task_one_open ON visit_task (organization_id, tenant_id, reason) WHERE resolved_at IS NULL;
CREATE INDEX visit_task_org_open ON visit_task (organization_id, created_at, id) WHERE resolved_at IS NULL;

REVOKE UPDATE, DELETE, TRUNCATE ON visit_task FROM cashflow_app;
GRANT UPDATE (resolved_at) ON visit_task TO cashflow_app;

-- A resolved task is final for every role: reopening one would collide with a newer open task of
-- the same reason. It runs as the caller, on the row it changes.
-- +goose StatementBegin
CREATE FUNCTION reject_resolved_visit_task_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.resolved_at IS NOT NULL THEN
        RAISE EXCEPTION 'visit_task % is resolved', OLD.id USING ERRCODE = 'insufficient_privilege';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER visit_task_resolved_final BEFORE UPDATE ON visit_task
    FOR EACH ROW EXECUTE FUNCTION reject_resolved_visit_task_change();

ALTER TABLE visit_task ENABLE ROW LEVEL SECURITY;
ALTER TABLE visit_task FORCE ROW LEVEL SECURITY;
CREATE POLICY visit_task_org ON visit_task
    USING (organization_id = app_org_id())
    WITH CHECK (organization_id = app_org_id());

-- +goose Down
DROP TABLE visit_task;
DROP FUNCTION reject_resolved_visit_task_change();
ALTER TABLE organization DROP COLUMN broken_promise_threshold;
ALTER TABLE promise_version DROP COLUMN payment_seq_mark;
