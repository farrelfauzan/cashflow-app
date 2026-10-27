-- +goose Up

-- A person who rents. One tenant can hold several leases, in one property or several. The
-- phone is their WhatsApp number in E.164 and identifies them inside the organization.
CREATE TABLE tenant (
    id              uuid PRIMARY KEY,
    organization_id uuid        NOT NULL REFERENCES organization (id),
    name            text        NOT NULL,
    phone           text        NOT NULL,
    note            text        NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, phone),
    UNIQUE (id, organization_id),
    CONSTRAINT tenant_phone_e164 CHECK (phone ~ '^\+[1-9][0-9]{7,14}$')
);

-- A tenant renting a unit. Rent and deposit are terms of the lease in rupiah, not money
-- movements, so they can be edited; every edit is audited and a rent change applies from the
-- next invoice. end_date is the last day, set by move-out. The collector is the staff member
-- who collects from this tenant; the RBAC "self" condition for karyawan uses it. The composite
-- foreign keys keep the unit, the tenant and the collector in the lease's organization.
CREATE TABLE lease (
    id                      uuid        PRIMARY KEY,
    organization_id         uuid        NOT NULL,
    unit_id                 uuid        NOT NULL,
    tenant_id               uuid        NOT NULL,
    start_date              date        NOT NULL,
    end_date                date,
    rent_amount             bigint      NOT NULL CHECK (rent_amount > 0),
    due_day                 smallint    NOT NULL CHECK (due_day BETWEEN 1 AND 28),
    deposit_amount          bigint      NOT NULL DEFAULT 0 CHECK (deposit_amount >= 0),
    status                  text        NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'ended')),
    collector_staff_user_id uuid,
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, organization_id),
    CONSTRAINT lease_ended_has_end_date CHECK ((status = 'ended') = (end_date IS NOT NULL)),
    CONSTRAINT lease_dates_in_order CHECK (end_date >= start_date),
    FOREIGN KEY (unit_id, organization_id) REFERENCES unit (id, organization_id),
    FOREIGN KEY (tenant_id, organization_id) REFERENCES tenant (id, organization_id),
    FOREIGN KEY (organization_id, collector_staff_user_id) REFERENCES membership (organization_id, staff_user_id)
);
-- One active lease per unit.
CREATE UNIQUE INDEX lease_one_active_per_unit ON lease (unit_id) WHERE status = 'active';
CREATE INDEX lease_tenant ON lease (organization_id, tenant_id);
CREATE INDEX lease_collector ON lease (organization_id, collector_staff_user_id) WHERE status = 'active';

ALTER TABLE tenant ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenant FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_org ON tenant USING (organization_id = app_org_id());

ALTER TABLE lease ENABLE ROW LEVEL SECURITY;
ALTER TABLE lease FORCE ROW LEVEL SECURITY;
CREATE POLICY lease_org ON lease USING (organization_id = app_org_id());

-- +goose Down
DROP TABLE lease;
DROP TABLE tenant;
