-- +goose Up

-- Slice 2's money rows: invoices with their lines, payments, allocations of payments to
-- invoices, and corrections. Every one of them is append-only (UPDATE, DELETE and TRUNCATE are
-- revoked from cashflow_app, and a trigger rejects them for every role, the owner included). A
-- mistake is fixed by a new correction row with a reason. Balances and credit are derived by the
-- views at the end, never stored (decision 36). Money is integer rupiah.

-- A lease's tenant never changes, so an invoice can carry the tenant and still be held to the
-- lease's tenant by a foreign key on the three columns.
ALTER TABLE lease ADD CONSTRAINT lease_id_tenant_org_unique UNIQUE (id, tenant_id, organization_id);

-- One billing period of a lease. period is the period's first day and the key of FIFO order
-- (oldest period first, then seq); due_date equals period except for a late-entered lease's
-- first invoices. amount is the sum of the invoice's lines, checked at commit.
CREATE TABLE invoice (
    id              uuid        PRIMARY KEY,
    seq             bigint      NOT NULL GENERATED ALWAYS AS IDENTITY UNIQUE,
    organization_id uuid        NOT NULL,
    lease_id        uuid        NOT NULL,
    tenant_id       uuid        NOT NULL,
    period          date        NOT NULL,
    period_end      date        NOT NULL,
    due_date        date        NOT NULL,
    amount          bigint      NOT NULL CHECK (amount > 0),
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (lease_id, period),
    UNIQUE (id, organization_id),
    UNIQUE (id, tenant_id, organization_id),
    CONSTRAINT invoice_period_in_order CHECK (period_end >= period),
    FOREIGN KEY (lease_id, organization_id) REFERENCES lease (id, organization_id),
    FOREIGN KEY (tenant_id, organization_id) REFERENCES tenant (id, organization_id),
    FOREIGN KEY (lease_id, tenant_id, organization_id) REFERENCES lease (id, tenant_id, organization_id)
);
CREATE INDEX invoice_tenant_fifo ON invoice (organization_id, tenant_id, period, seq);

-- What an invoice bills, snapshotted when it is generated: one rent line and one line per extra
-- charge. label is the text shown to people ("Listrik Oktober"), so renaming a charge type
-- never rewrites history.
CREATE TABLE invoice_line (
    id              uuid        PRIMARY KEY,
    seq             bigint      NOT NULL GENERATED ALWAYS AS IDENTITY UNIQUE,
    organization_id uuid        NOT NULL,
    invoice_id      uuid        NOT NULL,
    kind            text        NOT NULL CHECK (kind IN ('rent', 'charge')),
    charge_type_id  uuid,
    label           text        NOT NULL CHECK (length(btrim(label)) > 0),
    amount          bigint      NOT NULL CHECK (amount > 0),
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, organization_id),
    -- A charge line names its charge type; the rent line has none.
    CONSTRAINT invoice_line_charge_type CHECK ((kind = 'charge') = (charge_type_id IS NOT NULL)),
    FOREIGN KEY (invoice_id, organization_id) REFERENCES invoice (id, organization_id),
    FOREIGN KEY (charge_type_id, organization_id) REFERENCES charge_type (id, organization_id)
);
CREATE INDEX invoice_line_invoice ON invoice_line (invoice_id, seq);
-- At most one rent line per invoice, so the rent share of an invoice (commission, clean
-- income) is one row.
CREATE UNIQUE INDEX invoice_line_one_rent ON invoice_line (invoice_id) WHERE kind = 'rent';

-- Money received from a tenant. A manual payment names the member who recorded it; a gateway
-- payment (QRIS, virtual account) has none and carries the gateway's reference in source_ref,
-- unique per organization and source so a webhook or reconcile run twice records it once.
CREATE TABLE payment (
    id                        uuid        PRIMARY KEY,
    seq                       bigint      NOT NULL GENERATED ALWAYS AS IDENTITY UNIQUE,
    organization_id           uuid        NOT NULL,
    tenant_id                 uuid        NOT NULL,
    amount                    bigint      NOT NULL CHECK (amount > 0),
    method                    text        NOT NULL CHECK (method IN ('cash', 'transfer', 'qris', 'va')),
    received_at               timestamptz NOT NULL,
    source                    text        NOT NULL CHECK (source IN ('manual', 'qris', 'va')),
    source_ref                text CHECK (length(btrim(source_ref)) > 0),
    recorded_by_staff_user_id uuid,
    note                      text,
    created_at                timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, organization_id),
    UNIQUE (id, tenant_id, organization_id),
    FOREIGN KEY (tenant_id, organization_id) REFERENCES tenant (id, organization_id),
    FOREIGN KEY (organization_id, recorded_by_staff_user_id) REFERENCES membership (organization_id, staff_user_id)
);
CREATE UNIQUE INDEX payment_source_ref_unique ON payment (organization_id, source, source_ref)
    WHERE source_ref IS NOT NULL;
CREATE INDEX payment_tenant_fifo ON payment (organization_id, tenant_id, received_at, seq);

-- Part of a payment applied to an invoice of the same tenant. Signed: a negative row takes
-- money back from an invoice (it was cancelled, or the payment was corrected down). The two
-- foreign keys on tenant_id keep a payment from ever paying another tenant's invoice.
CREATE TABLE payment_allocation (
    id              uuid        PRIMARY KEY,
    seq             bigint      NOT NULL GENERATED ALWAYS AS IDENTITY UNIQUE,
    organization_id uuid        NOT NULL,
    payment_id      uuid        NOT NULL,
    invoice_id      uuid        NOT NULL,
    tenant_id       uuid        NOT NULL,
    amount          bigint      NOT NULL CHECK (amount <> 0),
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, organization_id),
    FOREIGN KEY (payment_id, tenant_id, organization_id) REFERENCES payment (id, tenant_id, organization_id),
    FOREIGN KEY (invoice_id, tenant_id, organization_id) REFERENCES invoice (id, tenant_id, organization_id)
);
CREATE INDEX payment_allocation_tenant ON payment_allocation (organization_id, tenant_id, seq);
CREATE INDEX payment_allocation_payment ON payment_allocation (payment_id);
CREATE INDEX payment_allocation_invoice ON payment_allocation (invoice_id);

-- A signed change to what an invoice bills or what a payment received, with a reason. An
-- invoice corrected to zero is cancelled. Listed in the Supervisor report.
CREATE TABLE correction (
    id                       uuid        PRIMARY KEY,
    seq                      bigint      NOT NULL GENERATED ALWAYS AS IDENTITY UNIQUE,
    organization_id          uuid        NOT NULL,
    invoice_id               uuid,
    payment_id               uuid,
    amount_delta             bigint      NOT NULL CHECK (amount_delta <> 0),
    reason                   text        NOT NULL CHECK (length(btrim(reason)) > 0),
    created_by_staff_user_id uuid        NOT NULL,
    created_at               timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, organization_id),
    CONSTRAINT correction_one_target CHECK (num_nonnulls(invoice_id, payment_id) = 1),
    FOREIGN KEY (invoice_id, organization_id) REFERENCES invoice (id, organization_id),
    FOREIGN KEY (payment_id, organization_id) REFERENCES payment (id, organization_id),
    FOREIGN KEY (organization_id, created_by_staff_user_id) REFERENCES membership (organization_id, staff_user_id)
);
CREATE INDEX correction_invoice ON correction (invoice_id) WHERE invoice_id IS NOT NULL;
CREATE INDEX correction_payment ON correction (payment_id) WHERE payment_id IS NOT NULL;
CREATE INDEX correction_org_created ON correction (organization_id, created_at DESC, seq DESC);

-- Append-only, for every role ------------------------------------------------------------

REVOKE UPDATE, DELETE, TRUNCATE ON invoice, invoice_line, payment, payment_allocation, correction FROM cashflow_app;

CREATE TRIGGER invoice_append_only BEFORE UPDATE OR DELETE ON invoice
    FOR EACH ROW EXECUTE FUNCTION reject_change();
CREATE TRIGGER invoice_no_truncate BEFORE TRUNCATE ON invoice
    FOR EACH STATEMENT EXECUTE FUNCTION reject_change();
CREATE TRIGGER invoice_line_append_only BEFORE UPDATE OR DELETE ON invoice_line
    FOR EACH ROW EXECUTE FUNCTION reject_change();
CREATE TRIGGER invoice_line_no_truncate BEFORE TRUNCATE ON invoice_line
    FOR EACH STATEMENT EXECUTE FUNCTION reject_change();
CREATE TRIGGER payment_append_only BEFORE UPDATE OR DELETE ON payment
    FOR EACH ROW EXECUTE FUNCTION reject_change();
CREATE TRIGGER payment_no_truncate BEFORE TRUNCATE ON payment
    FOR EACH STATEMENT EXECUTE FUNCTION reject_change();
CREATE TRIGGER payment_allocation_append_only BEFORE UPDATE OR DELETE ON payment_allocation
    FOR EACH ROW EXECUTE FUNCTION reject_change();
CREATE TRIGGER payment_allocation_no_truncate BEFORE TRUNCATE ON payment_allocation
    FOR EACH STATEMENT EXECUTE FUNCTION reject_change();
CREATE TRIGGER correction_append_only BEFORE UPDATE OR DELETE ON correction
    FOR EACH ROW EXECUTE FUNCTION reject_change();
CREATE TRIGGER correction_no_truncate BEFORE TRUNCATE ON correction
    FOR EACH STATEMENT EXECUTE FUNCTION reject_change();

-- Invoice lines add up -------------------------------------------------------------------

-- At commit, an invoice has at least one line and its lines add up to its amount. It fires for
-- the invoice row and for each line, deferred, so the invoice is inserted first and its lines
-- after it in the same transaction. It runs as the caller, inside the caller's organization scope;
-- an invoice the scope cannot see fails closed.
-- +goose StatementBegin
CREATE FUNCTION check_invoice_lines() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    target  uuid;
    billed  bigint;
    total   bigint;
    n_lines bigint;
BEGIN
    IF TG_TABLE_NAME = 'invoice' THEN
        target := NEW.id;
    ELSE
        target := NEW.invoice_id;
    END IF;
    SELECT amount INTO billed FROM invoice WHERE id = target;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'invoice % is not visible to check its lines', target
            USING ERRCODE = 'check_violation';
    END IF;
    SELECT count(*), coalesce(sum(amount), 0) INTO n_lines, total FROM invoice_line WHERE invoice_id = target;
    IF n_lines = 0 THEN
        RAISE EXCEPTION 'invoice % has no lines', target
            USING ERRCODE = 'check_violation', CONSTRAINT = 'invoice_lines_add_up';
    END IF;
    IF total <> billed THEN
        RAISE EXCEPTION 'the lines of invoice % add up to %, not its amount %', target, total, billed
            USING ERRCODE = 'check_violation', CONSTRAINT = 'invoice_lines_add_up';
    END IF;
    RETURN NULL;
END;
$$;
-- +goose StatementEnd

CREATE CONSTRAINT TRIGGER invoice_lines_add_up AFTER INSERT ON invoice
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION check_invoice_lines();
CREATE CONSTRAINT TRIGGER invoice_line_adds_up AFTER INSERT ON invoice_line
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION check_invoice_lines();

-- Corrections never go below zero ----------------------------------------------------------

-- A correction may not make an invoice's billed amount, or a payment's net amount, negative.
-- Zero is allowed: an invoice corrected to zero is cancelled. It runs as the caller, inside
-- the caller's organization scope. The tenant's row is locked first
-- (FOR NO KEY UPDATE, the same lock allocation takes), so two corrections of the same invoice
-- or payment cannot both pass against the same old total.
-- +goose StatementBegin
CREATE FUNCTION check_correction_not_negative() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    owner_tenant uuid;
    base         bigint;
    corrected    bigint;
BEGIN
    IF NEW.invoice_id IS NOT NULL THEN
        SELECT tenant_id, amount INTO owner_tenant, base FROM invoice WHERE id = NEW.invoice_id;
    ELSE
        SELECT tenant_id, amount INTO owner_tenant, base FROM payment WHERE id = NEW.payment_id;
    END IF;
    IF NOT FOUND THEN
        -- Nothing this scope can see: the foreign keys, the one-target check or row-level
        -- security refuse the row, each with its own error.
        RETURN NEW;
    END IF;
    PERFORM 1 FROM tenant WHERE id = owner_tenant FOR NO KEY UPDATE;
    IF NEW.invoice_id IS NOT NULL THEN
        SELECT coalesce(sum(amount_delta), 0) INTO corrected FROM correction WHERE invoice_id = NEW.invoice_id;
    ELSE
        SELECT coalesce(sum(amount_delta), 0) INTO corrected FROM correction WHERE payment_id = NEW.payment_id;
    END IF;
    IF base + corrected + NEW.amount_delta < 0 THEN
        RAISE EXCEPTION 'the correction would make the amount negative (% + % + %)', base, corrected, NEW.amount_delta
            USING ERRCODE = 'check_violation', CONSTRAINT = 'correction_not_negative';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER correction_not_negative BEFORE INSERT ON correction
    FOR EACH ROW EXECUTE FUNCTION check_correction_not_negative();

-- Row-level security ---------------------------------------------------------------------

ALTER TABLE invoice ENABLE ROW LEVEL SECURITY;
ALTER TABLE invoice FORCE ROW LEVEL SECURITY;
CREATE POLICY invoice_org ON invoice USING (organization_id = app_org_id());

ALTER TABLE invoice_line ENABLE ROW LEVEL SECURITY;
ALTER TABLE invoice_line FORCE ROW LEVEL SECURITY;
CREATE POLICY invoice_line_org ON invoice_line USING (organization_id = app_org_id());

ALTER TABLE payment ENABLE ROW LEVEL SECURITY;
ALTER TABLE payment FORCE ROW LEVEL SECURITY;
CREATE POLICY payment_org ON payment USING (organization_id = app_org_id());

ALTER TABLE payment_allocation ENABLE ROW LEVEL SECURITY;
ALTER TABLE payment_allocation FORCE ROW LEVEL SECURITY;
CREATE POLICY payment_allocation_org ON payment_allocation USING (organization_id = app_org_id());

ALTER TABLE correction ENABLE ROW LEVEL SECURITY;
ALTER TABLE correction FORCE ROW LEVEL SECURITY;
CREATE POLICY correction_org ON correction USING (organization_id = app_org_id());

-- Derived balances (decision 36) ---------------------------------------------------------

-- security_invoker makes the views read the tables as the caller, so row-level security
-- applies through them. Without it a view reads as its owner, and the owner of these views
-- (the migration role) is not held to the organization scope.

-- One row per invoice: what it bills after corrections, what is paid, and what is left.
-- cancelled: corrected to zero. paid: nothing left. partial: something paid. open: nothing paid.
CREATE VIEW invoice_balance WITH (security_invoker = true) AS
SELECT b.invoice_id, b.organization_id, b.tenant_id, b.lease_id, b.period, b.period_end, b.due_date,
       b.seq, b.amount, b.corrected,
       (b.amount + b.corrected)::bigint          AS billed,
       b.paid,
       (b.amount + b.corrected - b.paid)::bigint AS outstanding,
       CASE
           WHEN b.amount + b.corrected = 0 THEN 'cancelled'
           WHEN b.paid >= b.amount + b.corrected THEN 'paid'
           WHEN b.paid > 0 THEN 'partial'
           ELSE 'open'
       END::text                                 AS status
FROM (
    SELECT i.id AS invoice_id, i.organization_id, i.tenant_id, i.lease_id, i.period, i.period_end,
           i.due_date, i.seq, i.amount,
           (SELECT coalesce(sum(c.amount_delta), 0) FROM correction c WHERE c.invoice_id = i.id)::bigint AS corrected,
           (SELECT coalesce(sum(a.amount), 0) FROM payment_allocation a WHERE a.invoice_id = i.id)::bigint AS paid
    FROM invoice i
) b;

-- One row per tenant, also a tenant with no invoice or payment: the sum of what its invoices
-- still owe, the earliest due date among them, and its credit (what it paid after corrections,
-- less what is allocated). How old that due date is depends on today in Asia/Jakarta, so the
-- application computes the age with its clock.
CREATE VIEW tenant_balance WITH (security_invoker = true) AS
SELECT t.id              AS tenant_id,
       t.organization_id,
       (SELECT coalesce(sum(b.outstanding), 0) FROM invoice_balance b
        WHERE b.tenant_id = t.id AND b.outstanding > 0)::bigint AS outstanding,
       (SELECT min(b.due_date) FROM invoice_balance b
        WHERE b.tenant_id = t.id AND b.outstanding > 0)::date    AS oldest_unpaid_due_date,
       ((SELECT coalesce(sum(p.amount), 0) FROM payment p WHERE p.tenant_id = t.id)
        + (SELECT coalesce(sum(c.amount_delta), 0) FROM correction c
           JOIN payment p ON p.id = c.payment_id WHERE p.tenant_id = t.id)
        - (SELECT coalesce(sum(a.amount), 0) FROM payment_allocation a WHERE a.tenant_id = t.id))::bigint AS credit
FROM tenant t;

REVOKE ALL ON invoice_balance, tenant_balance FROM cashflow_app;
GRANT SELECT ON invoice_balance, tenant_balance TO cashflow_app;

-- The organization list for jobs ---------------------------------------------------------

-- Scheduled jobs run one task per organization, so they need the list of organization ids
-- before any organization scope exists. This is the one sanctioned cross-organization read: it
-- returns ids only. There is no "active" flag on organization yet, so every organization is
-- active; once one exists, this function filters on it.
--
-- organization forces row-level security, which holds the table's owner too unless it is a
-- superuser (it is in tests, it should not be in production). So the function sets
-- app.list_org_ids for its own duration, and one policy lets only the role that ran this
-- migration (the function's owner) read every organization while that setting is on. The app
-- role cannot use the policy whatever it sets. Any role can turn the setting on for its own
-- session, so a second SECURITY DEFINER function owned by the migration role that reads
-- organization would see every one: TestActiveOrganizationIDsIsTheOnlySecurityDefiner fails
-- until such a function is reviewed against this policy.
CREATE POLICY organization_list_ids ON organization FOR SELECT TO CURRENT_USER
    USING (current_setting('app.list_org_ids', true) = 'on');

-- +goose StatementBegin
CREATE FUNCTION active_organization_ids() RETURNS SETOF uuid
    LANGUAGE sql STABLE SECURITY DEFINER
    SET search_path = pg_catalog, public
    SET app.list_org_ids = 'on'
AS $$
    SELECT id FROM organization ORDER BY id
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION active_organization_ids() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION active_organization_ids() TO cashflow_app;

-- +goose Down
DROP FUNCTION active_organization_ids();
DROP POLICY organization_list_ids ON organization;
DROP VIEW tenant_balance;
DROP VIEW invoice_balance;
DROP TABLE correction;
DROP TABLE payment_allocation;
DROP TABLE payment;
DROP TABLE invoice_line;
DROP TABLE invoice;
DROP FUNCTION check_correction_not_negative();
DROP FUNCTION check_invoice_lines();
ALTER TABLE lease DROP CONSTRAINT lease_id_tenant_org_unique;
