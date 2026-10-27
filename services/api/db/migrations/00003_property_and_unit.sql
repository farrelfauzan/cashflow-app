-- +goose Up

-- Sorts labels the way people count: "Kamar 2" before "Kamar 11".
CREATE COLLATION natural_sort (provider = icu, locale = 'und-u-kn-true');

-- A building or site: a kos, a kontrakan, a ruko or another business unit.
CREATE TABLE property (
    id              uuid PRIMARY KEY,
    organization_id uuid        NOT NULL REFERENCES organization (id),
    name            text        NOT NULL,
    kind            text        NOT NULL CHECK (kind IN ('kos', 'kontrakan', 'ruko', 'other')),
    address         text        NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, organization_id)
);
-- The tenant import finds a property by its name, so names are unique per organization,
-- ignoring case.
CREATE UNIQUE INDEX property_name_unique ON property (organization_id, lower(name));

-- A rentable unit inside a property: a room, a house, a shop. The composite foreign key keeps
-- it in its property's organization. default_rent is the usual monthly rent in rupiah, a
-- suggestion for new leases; each lease carries its own rent.
CREATE TABLE unit (
    id              uuid PRIMARY KEY,
    organization_id uuid        NOT NULL,
    property_id     uuid        NOT NULL,
    label           text        NOT NULL,
    default_rent    bigint      CHECK (default_rent > 0),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, organization_id),
    FOREIGN KEY (property_id, organization_id) REFERENCES property (id, organization_id)
);
-- A label names one unit inside its property, ignoring case. Two properties may both have a
-- "Kamar 1".
CREATE UNIQUE INDEX unit_label_unique ON unit (property_id, lower(label));

-- A role assignment can be limited to one property. NULL means the whole organization.
ALTER TABLE role_assignment ADD COLUMN property_id uuid;
ALTER TABLE role_assignment ADD CONSTRAINT role_assignment_property_fkey
    FOREIGN KEY (property_id, organization_id) REFERENCES property (id, organization_id);

-- The same role may now be held once per property, and once for the whole organization.
DROP INDEX role_assignment_builtin_unique;
DROP INDEX role_assignment_custom_unique;
CREATE UNIQUE INDEX role_assignment_builtin_unique
    ON role_assignment (membership_id, builtin_role, property_id) NULLS NOT DISTINCT
    WHERE builtin_role IS NOT NULL;
CREATE UNIQUE INDEX role_assignment_custom_unique
    ON role_assignment (membership_id, role_id, property_id) NULLS NOT DISTINCT
    WHERE role_id IS NOT NULL;

ALTER TABLE property ENABLE ROW LEVEL SECURITY;
ALTER TABLE property FORCE ROW LEVEL SECURITY;
CREATE POLICY property_org ON property USING (organization_id = app_org_id());

ALTER TABLE unit ENABLE ROW LEVEL SECURITY;
ALTER TABLE unit FORCE ROW LEVEL SECURITY;
CREATE POLICY unit_org ON unit USING (organization_id = app_org_id());

-- +goose Down
DROP INDEX role_assignment_custom_unique;
DROP INDEX role_assignment_builtin_unique;
DELETE FROM role_assignment WHERE property_id IS NOT NULL;
CREATE UNIQUE INDEX role_assignment_builtin_unique
    ON role_assignment (membership_id, builtin_role) WHERE builtin_role IS NOT NULL;
CREATE UNIQUE INDEX role_assignment_custom_unique
    ON role_assignment (membership_id, role_id) WHERE role_id IS NOT NULL;
ALTER TABLE role_assignment DROP COLUMN property_id;
DROP TABLE unit;
DROP TABLE property;
DROP COLLATION natural_sort;
