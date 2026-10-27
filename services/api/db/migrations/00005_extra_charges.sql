-- +goose Up

-- A kind of extra charge on top of rent: listrik, air, Wi-Fi, parkir, kebersihan. Organization
-- wide, so a name means the same thing in every property. Metered types (a different amount
-- every month, such as listrik by the meter) are billed from recorded usage in slice 2 and can
-- never be a fixed lease charge; the service enforces that.
CREATE TABLE charge_type (
    id              uuid PRIMARY KEY,
    organization_id uuid        NOT NULL REFERENCES organization (id),
    name            text        NOT NULL,
    metered         boolean     NOT NULL DEFAULT false,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, organization_id)
);
-- Names are unique per organization, ignoring case, like property names.
CREATE UNIQUE INDEX charge_type_name_unique ON charge_type (organization_id, lower(name));

-- A fixed monthly extra on a lease, in rupiah. Like rent it is a lease term, not a money
-- movement: it may be changed or removed (every change is audited through lease:manage) and a
-- change applies from the next invoice, which snapshots it into an invoice line. So there is no
-- append-only trigger here. The composite foreign keys keep the lease and the charge type in the
-- row's organization.
CREATE TABLE lease_charge (
    id              uuid        PRIMARY KEY,
    organization_id uuid        NOT NULL,
    lease_id        uuid        NOT NULL,
    charge_type_id  uuid        NOT NULL,
    amount          bigint      NOT NULL CHECK (amount > 0),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (lease_id, charge_type_id),
    FOREIGN KEY (lease_id, organization_id) REFERENCES lease (id, organization_id),
    FOREIGN KEY (charge_type_id, organization_id) REFERENCES charge_type (id, organization_id)
);
CREATE INDEX lease_charge_org_lease ON lease_charge (organization_id, lease_id);
CREATE INDEX lease_charge_charge_type ON lease_charge (charge_type_id);

ALTER TABLE charge_type ENABLE ROW LEVEL SECURITY;
ALTER TABLE charge_type FORCE ROW LEVEL SECURITY;
CREATE POLICY charge_type_org ON charge_type USING (organization_id = app_org_id());

ALTER TABLE lease_charge ENABLE ROW LEVEL SECURITY;
ALTER TABLE lease_charge FORCE ROW LEVEL SECURITY;
CREATE POLICY lease_charge_org ON lease_charge USING (organization_id = app_org_id());

-- +goose Down
DROP TABLE lease_charge;
DROP TABLE charge_type;
