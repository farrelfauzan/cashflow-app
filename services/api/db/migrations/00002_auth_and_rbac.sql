-- +goose Up

-- Scope helpers for row-level security. Each returns NULL when its setting is unset, so a
-- policy that compares against it matches nothing instead of raising. The store sets them per
-- transaction: WithOrgTx sets app.org_id, WithAuthTx sets app.scope = 'auth' and, once the
-- staff user is known, app.user_id.
-- +goose StatementBegin
CREATE FUNCTION app_org_id() RETURNS uuid LANGUAGE sql STABLE AS $$
    SELECT NULLIF(current_setting('app.org_id', true), '')::uuid
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION app_user_id() RETURNS uuid LANGUAGE sql STABLE AS $$
    SELECT NULLIF(current_setting('app.user_id', true), '')::uuid
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION app_in_auth_scope() RETURNS boolean LANGUAGE sql STABLE AS $$
    SELECT coalesce(current_setting('app.scope', true), '') = 'auth'
$$;
-- +goose StatementEnd

-- Rejects UPDATE, DELETE and TRUNCATE on append-only tables. Corrections are new rows.
-- +goose StatementBegin
CREATE FUNCTION reject_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION '% is append-only', TG_TABLE_NAME USING ERRCODE = 'insufficient_privilege';
END;
$$;
-- +goose StatementEnd

-- The organization policy now uses the NULL-safe helper, because organizations are also read
-- from the auth scope (a staff user listing the organizations they belong to).
DROP POLICY org_isolation ON organization;
CREATE POLICY organization_org ON organization
    USING (id = app_org_id());

-- A staff user is one person across every organization they work for. The phone is the login.
CREATE TABLE staff_user (
    id         uuid PRIMARY KEY,
    phone      text        NOT NULL UNIQUE,
    name       text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT staff_user_phone_e164 CHECK (phone ~ '^\+[1-9][0-9]{7,14}$')
);

-- A membership is a staff user inside one organization.
CREATE TABLE membership (
    id              uuid PRIMARY KEY,
    organization_id uuid        NOT NULL REFERENCES organization (id),
    staff_user_id   uuid        NOT NULL REFERENCES staff_user (id),
    status          text        NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, staff_user_id),
    UNIQUE (id, organization_id)
);

-- Custom roles: a policy document per organization. Built-in roles live in code.
CREATE TABLE role (
    id              uuid PRIMARY KEY,
    organization_id uuid        NOT NULL REFERENCES organization (id),
    name            text        NOT NULL,
    policy          jsonb       NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, name),
    UNIQUE (id, organization_id)
);

-- Exactly one of builtin_role or role_id is set. The composite foreign keys keep an assignment
-- inside the same organization as its membership and its role.
CREATE TABLE role_assignment (
    id              uuid PRIMARY KEY,
    organization_id uuid        NOT NULL,
    membership_id   uuid        NOT NULL,
    builtin_role    text,
    role_id         uuid,
    created_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT role_assignment_one_role CHECK ((builtin_role IS NULL) <> (role_id IS NULL)),
    CONSTRAINT role_assignment_builtin CHECK (builtin_role IN ('supervisor', 'admin', 'karyawan')),
    FOREIGN KEY (membership_id, organization_id) REFERENCES membership (id, organization_id) ON DELETE CASCADE,
    FOREIGN KEY (role_id, organization_id) REFERENCES role (id, organization_id)
);
CREATE UNIQUE INDEX role_assignment_builtin_unique
    ON role_assignment (membership_id, builtin_role) WHERE builtin_role IS NOT NULL;
CREATE UNIQUE INDEX role_assignment_custom_unique
    ON role_assignment (membership_id, role_id) WHERE role_id IS NOT NULL;

-- One-time login codes. Only the HMAC of the code is stored.
CREATE TABLE otp_challenge (
    id          uuid PRIMARY KEY,
    phone       text        NOT NULL,
    code_hash   bytea       NOT NULL,
    expires_at  timestamptz NOT NULL,
    attempts    int         NOT NULL DEFAULT 0,
    consumed_at timestamptz,
    created_at  timestamptz NOT NULL
);
CREATE INDEX otp_challenge_open ON otp_challenge (phone, created_at DESC) WHERE consumed_at IS NULL;

-- Staff sessions. Only the SHA-256 of the cookie token is stored.
CREATE TABLE staff_session (
    id            uuid PRIMARY KEY,
    token_hash    bytea       NOT NULL UNIQUE,
    staff_user_id uuid        NOT NULL REFERENCES staff_user (id),
    created_at    timestamptz NOT NULL,
    last_seen_at  timestamptz NOT NULL,
    expires_at    timestamptz NOT NULL,
    revoked_at    timestamptz
);

-- Every denied permission check and every allowed sensitive action. Append-only. seq gives a
-- strict order even when several rows share a timestamp.
CREATE TABLE audit_log (
    id                  uuid PRIMARY KEY,
    seq                 bigint      GENERATED ALWAYS AS IDENTITY UNIQUE,
    organization_id     uuid        NOT NULL REFERENCES organization (id),
    actor_staff_user_id uuid        REFERENCES staff_user (id),
    action              text        NOT NULL,
    resource            text        NOT NULL,
    decision            text        NOT NULL CHECK (decision IN ('allow', 'deny')),
    reason              text        NOT NULL DEFAULT '',
    detail              jsonb       NOT NULL DEFAULT '{}',
    created_at          timestamptz NOT NULL
);
CREATE INDEX audit_log_org_created ON audit_log (organization_id, created_at DESC);
REVOKE UPDATE, DELETE, TRUNCATE ON audit_log FROM cashflow_app;
CREATE TRIGGER audit_log_append_only BEFORE UPDATE OR DELETE ON audit_log
    FOR EACH ROW EXECUTE FUNCTION reject_change();
CREATE TRIGGER audit_log_no_truncate BEFORE TRUNCATE ON audit_log
    FOR EACH STATEMENT EXECUTE FUNCTION reject_change();

-- Row-level security ---------------------------------------------------------------------

ALTER TABLE staff_user ENABLE ROW LEVEL SECURITY;
ALTER TABLE staff_user FORCE ROW LEVEL SECURITY;
-- Login and session handling see every staff user.
CREATE POLICY staff_user_auth ON staff_user
    USING (app_in_auth_scope());
-- An organization sees only its own members, read-only.
CREATE POLICY staff_user_org_member ON staff_user FOR SELECT
    USING (EXISTS (
        SELECT 1 FROM membership m
        WHERE m.staff_user_id = staff_user.id AND m.organization_id = app_org_id()
    ));

ALTER TABLE membership ENABLE ROW LEVEL SECURITY;
ALTER TABLE membership FORCE ROW LEVEL SECURITY;
CREATE POLICY membership_org ON membership
    USING (organization_id = app_org_id());
-- After login a staff user may list their own memberships, and nothing else.
CREATE POLICY membership_auth_self ON membership FOR SELECT
    USING (app_in_auth_scope() AND staff_user_id = app_user_id());

-- The same staff user may read the organizations they belong to.
CREATE POLICY organization_auth_member ON organization FOR SELECT
    USING (app_in_auth_scope() AND EXISTS (
        SELECT 1 FROM membership m
        WHERE m.organization_id = organization.id
          AND m.staff_user_id = app_user_id()
          AND m.status = 'active'
    ));

ALTER TABLE role ENABLE ROW LEVEL SECURITY;
ALTER TABLE role FORCE ROW LEVEL SECURITY;
CREATE POLICY role_org ON role USING (organization_id = app_org_id());

ALTER TABLE role_assignment ENABLE ROW LEVEL SECURITY;
ALTER TABLE role_assignment FORCE ROW LEVEL SECURITY;
CREATE POLICY role_assignment_org ON role_assignment USING (organization_id = app_org_id());

ALTER TABLE otp_challenge ENABLE ROW LEVEL SECURITY;
ALTER TABLE otp_challenge FORCE ROW LEVEL SECURITY;
CREATE POLICY otp_challenge_auth ON otp_challenge USING (app_in_auth_scope());

ALTER TABLE staff_session ENABLE ROW LEVEL SECURITY;
ALTER TABLE staff_session FORCE ROW LEVEL SECURITY;
CREATE POLICY staff_session_auth ON staff_session USING (app_in_auth_scope());

ALTER TABLE audit_log ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_log FORCE ROW LEVEL SECURITY;
CREATE POLICY audit_log_org ON audit_log USING (organization_id = app_org_id());

-- +goose Down
DROP TABLE audit_log;
DROP TABLE staff_session;
DROP TABLE otp_challenge;
DROP TABLE role_assignment;
DROP TABLE role;
DROP POLICY organization_auth_member ON organization;
DROP TABLE membership;
DROP TABLE staff_user;
DROP POLICY organization_org ON organization;
CREATE POLICY org_isolation ON organization
    USING (id = current_setting('app.org_id')::uuid);
DROP FUNCTION reject_change();
DROP FUNCTION app_in_auth_scope();
DROP FUNCTION app_user_id();
DROP FUNCTION app_org_id();
