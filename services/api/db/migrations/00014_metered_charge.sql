-- +goose Up

-- Metered extras (T-063): listrik, air and the like, a different amount every month, recorded per
-- lease, charge type and usage month and billed once, on the next invoice generated after it.

-- The metered types a lease pays for ("Listrik dicatat dari meteran tiap bulan"). It carries no
-- amount: it only says which readings to expect, so Penagihan can flag a month with no amount
-- yet. Like lease_charge it is a lease term, not a money row: the set is replaced as a whole
-- (every change audited through lease:manage), so there is no append-only trigger. The service
-- checks that the charge type is metered.
CREATE TABLE lease_metered_type (
    id              uuid        PRIMARY KEY,
    organization_id uuid        NOT NULL,
    lease_id        uuid        NOT NULL,
    charge_type_id  uuid        NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (lease_id, charge_type_id),
    FOREIGN KEY (lease_id, organization_id) REFERENCES lease (id, organization_id),
    FOREIGN KEY (charge_type_id, organization_id) REFERENCES charge_type (id, organization_id)
);
CREATE INDEX lease_metered_type_org_lease ON lease_metered_type (organization_id, lease_id);
CREATE INDEX lease_metered_type_charge_type ON lease_metered_type (charge_type_id);

-- One month's metered amount of a lease and charge type, in rupiah. usage_month is the first
-- day of the month the usage belongs to ("Listrik Oktober"). The meter numbers are optional and
-- only shown; amount is what is billed, typed by the member (the app keeps no tariff). It may be
-- edited until an invoice line bills it; from then on it is locked (the trigger below), and a fix
-- is a correction on that invoice. It is never deleted: the app role has no DELETE on it.
CREATE TABLE metered_charge (
    id                        uuid        PRIMARY KEY,
    organization_id           uuid        NOT NULL,
    lease_id                  uuid        NOT NULL,
    charge_type_id            uuid        NOT NULL,
    usage_month               date        NOT NULL CHECK (extract(day FROM usage_month) = 1),
    amount                    bigint      NOT NULL CHECK (amount > 0),
    meter_start               bigint      CHECK (meter_start >= 0),
    meter_end                 bigint      CHECK (meter_end >= 0),
    recorded_by_staff_user_id uuid        NOT NULL,
    created_at                timestamptz NOT NULL DEFAULT now(),
    updated_at                timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, organization_id),
    UNIQUE (lease_id, charge_type_id, usage_month),
    CONSTRAINT metered_charge_meter_order CHECK (meter_start IS NULL OR meter_end IS NULL OR meter_end >= meter_start),
    FOREIGN KEY (lease_id, organization_id) REFERENCES lease (id, organization_id),
    FOREIGN KEY (charge_type_id, organization_id) REFERENCES charge_type (id, organization_id),
    FOREIGN KEY (organization_id, recorded_by_staff_user_id) REFERENCES membership (organization_id, staff_user_id)
);
CREATE INDEX metered_charge_org_lease ON metered_charge (organization_id, lease_id, usage_month);
CREATE INDEX metered_charge_charge_type ON metered_charge (charge_type_id);

REVOKE DELETE, TRUNCATE ON metered_charge FROM cashflow_app;

-- The line that bills a metered amount points at it. The unique index bills each amount exactly
-- once, so a rerun of generation can never bill it twice. invoice_line stays append-only; this
-- only adds a column.
ALTER TABLE invoice_line ADD COLUMN metered_charge_id uuid;
ALTER TABLE invoice_line ADD CONSTRAINT invoice_line_metered_charge_fk
    FOREIGN KEY (metered_charge_id, organization_id) REFERENCES metered_charge (id, organization_id);
ALTER TABLE invoice_line ADD CONSTRAINT invoice_line_metered_is_charge
    CHECK (metered_charge_id IS NULL OR kind = 'charge');
CREATE UNIQUE INDEX invoice_line_metered_charge_once ON invoice_line (metered_charge_id)
    WHERE metered_charge_id IS NOT NULL;

-- A billed amount is locked, for every role: an update or a delete of a metered_charge that an
-- invoice line references is rejected. It runs as the caller, inside the caller's organization
-- scope, which also sees the line (both rows carry the same organization). The service refuses
-- first with its own error; this is the backstop for hand-written SQL.
-- +goose StatementBegin
CREATE FUNCTION reject_billed_metered_charge() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF EXISTS (SELECT 1 FROM invoice_line WHERE metered_charge_id = OLD.id) THEN
        RAISE EXCEPTION 'metered_charge % is billed', OLD.id USING ERRCODE = 'insufficient_privilege';
    END IF;
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER metered_charge_billed_locked BEFORE UPDATE OR DELETE ON metered_charge
    FOR EACH ROW EXECUTE FUNCTION reject_billed_metered_charge();

-- Row-level security ---------------------------------------------------------------------

ALTER TABLE lease_metered_type ENABLE ROW LEVEL SECURITY;
ALTER TABLE lease_metered_type FORCE ROW LEVEL SECURITY;
CREATE POLICY lease_metered_type_org ON lease_metered_type USING (organization_id = app_org_id());

ALTER TABLE metered_charge ENABLE ROW LEVEL SECURITY;
ALTER TABLE metered_charge FORCE ROW LEVEL SECURITY;
CREATE POLICY metered_charge_org ON metered_charge USING (organization_id = app_org_id());

-- +goose Down
DROP INDEX invoice_line_metered_charge_once;
ALTER TABLE invoice_line DROP CONSTRAINT invoice_line_metered_is_charge;
ALTER TABLE invoice_line DROP CONSTRAINT invoice_line_metered_charge_fk;
ALTER TABLE invoice_line DROP COLUMN metered_charge_id;
DROP TABLE metered_charge;
DROP FUNCTION reject_billed_metered_charge();
DROP TABLE lease_metered_type;
