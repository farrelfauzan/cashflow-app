-- +goose Up
CREATE TABLE organization (
    id           uuid PRIMARY KEY,
    name         text        NOT NULL,
    display_name text        NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);

-- Row-level security. A session sees only the organization named in app.org_id, which
-- store.WithOrgTx sets for the length of one transaction. Every later table that holds
-- organization data repeats these three statements with organization_id in the policy.
-- If app.org_id is not set the policy raises an error instead of returning rows.
ALTER TABLE organization ENABLE ROW LEVEL SECURITY;
ALTER TABLE organization FORCE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON organization
    USING (id = current_setting('app.org_id')::uuid);

-- +goose Down
DROP TABLE organization;
