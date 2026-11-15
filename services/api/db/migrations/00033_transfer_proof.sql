-- +goose Up

-- Slice 6 (S6-5, decision 46.8): the tenant's photo of a bank transfer ("Kirim bukti transfer"),
-- sent from the payment page. The tenant has no account: the payment link's token grants it
-- (payment:proof, CLAUDE.md "Not RBAC principals"). The photo streams through our API into the
-- same object storage, pending state and malware scan as the staff photos, so it is an upload of
-- purpose transfer_proof with no staff uploader; transfer_proof records which link sent it, for
-- the admin's check queue (S6-6).

-- An upload of a transfer proof has a tenant and no staff uploader; every other upload keeps a
-- staff uploader.
ALTER TABLE upload ALTER COLUMN uploaded_by_staff_user_id DROP NOT NULL;
ALTER TABLE upload DROP CONSTRAINT upload_purpose;
ALTER TABLE upload ADD CONSTRAINT upload_purpose
    CHECK (purpose IN ('cash_receipt', 'cash_deposit', 'transfer_proof'));
ALTER TABLE upload DROP CONSTRAINT upload_tenant_for_receipt;
ALTER TABLE upload ADD CONSTRAINT upload_tenant_for_receipt
    CHECK ((purpose IN ('cash_receipt', 'transfer_proof')) = (tenant_id IS NOT NULL));
ALTER TABLE upload ADD CONSTRAINT upload_staff_uploader
    CHECK ((purpose = 'transfer_proof') = (uploaded_by_staff_user_id IS NULL));

-- The same rule as before, with the uploader compared NULL-safely now that it may be NULL.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION reject_upload_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.status <> 'pending' THEN
        RAISE EXCEPTION 'upload % is already scanned', OLD.id USING ERRCODE = 'insufficient_privilege';
    END IF;
    IF NEW.id <> OLD.id OR NEW.organization_id <> OLD.organization_id OR NEW.purpose <> OLD.purpose
        OR NEW.tenant_id IS DISTINCT FROM OLD.tenant_id OR NEW.object_key <> OLD.object_key
        OR NEW.declared_size <> OLD.declared_size
        OR NEW.uploaded_by_staff_user_id IS DISTINCT FROM OLD.uploaded_by_staff_user_id
        OR NEW.created_at <> OLD.created_at THEN
        RAISE EXCEPTION 'upload % cannot change what it is', OLD.id USING ERRCODE = 'insufficient_privilege';
    END IF;
    IF OLD.completed_at IS NOT NULL AND NEW.completed_at IS DISTINCT FROM OLD.completed_at THEN
        RAISE EXCEPTION 'upload % is already completed', OLD.id USING ERRCODE = 'insufficient_privilege';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

-- The key a proof names: a link with its tenant and invoice.
ALTER TABLE payment_link ADD CONSTRAINT payment_link_id_tenant_invoice
    UNIQUE (id, tenant_id, invoice_id, organization_id);

CREATE TABLE transfer_proof (
    id              uuid        PRIMARY KEY,
    organization_id uuid        NOT NULL REFERENCES organization (id),
    tenant_id       uuid        NOT NULL,
    -- The link the tenant sent it from, and that link's invoice.
    payment_link_id uuid        NOT NULL,
    invoice_id      uuid        NOT NULL,
    -- The photo: an upload of purpose transfer_proof for this tenant, each behind one proof.
    upload_id       uuid        NOT NULL CONSTRAINT transfer_proof_upload_once UNIQUE,
    upload_purpose  text        NOT NULL DEFAULT 'transfer_proof'
        CONSTRAINT transfer_proof_upload_purpose CHECK (upload_purpose = 'transfer_proof'),
    created_at      timestamptz NOT NULL,
    UNIQUE (id, tenant_id, organization_id),
    FOREIGN KEY (tenant_id, organization_id) REFERENCES tenant (id, organization_id),
    FOREIGN KEY (payment_link_id, tenant_id, invoice_id, organization_id)
        REFERENCES payment_link (id, tenant_id, invoice_id, organization_id),
    FOREIGN KEY (upload_id, tenant_id, organization_id, upload_purpose)
        REFERENCES upload (id, tenant_id, organization_id, purpose)
);
-- The check queue's newest proofs (S6-6) and a tenant's proofs.
CREATE INDEX transfer_proof_org_created ON transfer_proof (organization_id, created_at, id);
CREATE INDEX transfer_proof_tenant ON transfer_proof (organization_id, tenant_id, created_at);

REVOKE UPDATE, DELETE, TRUNCATE ON transfer_proof FROM cashflow_app;

CREATE TRIGGER transfer_proof_append_only BEFORE UPDATE OR DELETE ON transfer_proof
    FOR EACH ROW EXECUTE FUNCTION reject_change();
CREATE TRIGGER transfer_proof_no_truncate BEFORE TRUNCATE ON transfer_proof
    FOR EACH STATEMENT EXECUTE FUNCTION reject_change();

ALTER TABLE transfer_proof ENABLE ROW LEVEL SECURITY;
ALTER TABLE transfer_proof FORCE ROW LEVEL SECURITY;
CREATE POLICY transfer_proof_org ON transfer_proof
    USING (organization_id = app_org_id())
    WITH CHECK (organization_id = app_org_id());

-- +goose Down
DROP TABLE transfer_proof;
ALTER TABLE payment_link DROP CONSTRAINT payment_link_id_tenant_invoice;
-- Down only works while no transfer proof upload exists.
ALTER TABLE upload DROP CONSTRAINT upload_staff_uploader;
ALTER TABLE upload DROP CONSTRAINT upload_tenant_for_receipt;
ALTER TABLE upload ADD CONSTRAINT upload_tenant_for_receipt
    CHECK ((purpose = 'cash_receipt') = (tenant_id IS NOT NULL));
ALTER TABLE upload DROP CONSTRAINT upload_purpose;
ALTER TABLE upload ADD CONSTRAINT upload_purpose CHECK (purpose IN ('cash_receipt', 'cash_deposit'));
ALTER TABLE upload ALTER COLUMN uploaded_by_staff_user_id SET NOT NULL;
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION reject_upload_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.status <> 'pending' THEN
        RAISE EXCEPTION 'upload % is already scanned', OLD.id USING ERRCODE = 'insufficient_privilege';
    END IF;
    IF NEW.id <> OLD.id OR NEW.organization_id <> OLD.organization_id OR NEW.purpose <> OLD.purpose
        OR NEW.tenant_id IS DISTINCT FROM OLD.tenant_id OR NEW.object_key <> OLD.object_key
        OR NEW.declared_size <> OLD.declared_size OR NEW.uploaded_by_staff_user_id <> OLD.uploaded_by_staff_user_id
        OR NEW.created_at <> OLD.created_at THEN
        RAISE EXCEPTION 'upload % cannot change what it is', OLD.id USING ERRCODE = 'insufficient_privilege';
    END IF;
    IF OLD.completed_at IS NOT NULL AND NEW.completed_at IS DISTINCT FROM OLD.completed_at THEN
        RAISE EXCEPTION 'upload % is already completed', OLD.id USING ERRCODE = 'insufficient_privilege';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd
