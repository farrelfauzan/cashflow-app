-- +goose Up

-- Slice 6 (S6-1): a file a staff member uploads from their phone (a cash receipt photo, a cash
-- deposit photo), straight to object storage (R2 in production) through a presigned PUT. The
-- database keeps only the key. The row stays pending until the scan job (upload:scan) has
-- checked the object's real size and type and scanned it for malware; only a clean upload can be
-- read, through a short-lived presigned GET served as an attachment.
--
-- Where the object lives, all derived from object_key (internal/upload):
--   incoming/<object_key>    what the presigned PUT writes; the scan moves it away before reading
--   files/<object_key>       the copy the scan checks and a clean upload is read from
--   quarantine/<object_key>  where a rejected or infected file is moved
-- The scan reads its own copy, so a second PUT with the same URL can never swap the file after
-- it was scanned.
--
-- status moves once, from pending to one final state: clean, rejected (not an allowed image,
-- its size differs from the declared size or the limit, or it never arrived) or infected. The
-- app role may change only completed_at (set once, when the client says the PUT finished) and
-- the scan's columns; the trigger below holds the same for every role.
CREATE TABLE upload (
    id                        uuid        PRIMARY KEY,
    organization_id           uuid        NOT NULL REFERENCES organization (id),
    purpose                   text        NOT NULL CONSTRAINT upload_purpose
        CHECK (purpose IN ('cash_receipt', 'cash_deposit')),
    -- The tenant a receipt photo is for. A deposit photo belongs to no tenant.
    tenant_id                 uuid,
    -- Made by the server from the organization and fresh randomness, never from client input.
    object_key                text        NOT NULL CONSTRAINT upload_object_key UNIQUE,
    declared_size             bigint      NOT NULL CONSTRAINT upload_declared_size CHECK (declared_size > 0),
    -- The type sniffed from the content by the scan; NULL until then, and for a file that never
    -- arrived.
    content_type              text,
    status                    text        NOT NULL DEFAULT 'pending' CONSTRAINT upload_status
        CHECK (status IN ('pending', 'clean', 'rejected', 'infected')),
    uploaded_by_staff_user_id uuid        NOT NULL,
    created_at                timestamptz NOT NULL,
    completed_at              timestamptz,
    scanned_at                timestamptz,
    UNIQUE (id, organization_id),
    CONSTRAINT upload_tenant_for_receipt CHECK ((purpose = 'cash_receipt') = (tenant_id IS NOT NULL)),
    -- The key sits under the organization's own prefix: uploads/<organization>/<32 hex>.
    -- (A uuid's text is hex digits and hyphens, so it matches itself in the pattern.)
    CONSTRAINT upload_object_key_shape
        CHECK (object_key ~ ('^uploads/' || organization_id::text || '/[0-9a-f]{32}$')),
    -- A scanned upload says when; a pending one has no time. Only a completed upload is scanned.
    CONSTRAINT upload_scanned_at CHECK ((status = 'pending') = (scanned_at IS NULL)),
    CONSTRAINT upload_scanned_completed CHECK (status = 'pending' OR completed_at IS NOT NULL),
    -- A clean file has a type: one of the allowed images.
    CONSTRAINT upload_clean_type CHECK (status <> 'clean' OR content_type IN ('image/jpeg', 'image/png', 'image/webp')),
    FOREIGN KEY (tenant_id, organization_id) REFERENCES tenant (id, organization_id),
    FOREIGN KEY (organization_id, uploaded_by_staff_user_id) REFERENCES membership (organization_id, staff_user_id)
);
-- The reconcile's scan: completed uploads still pending.
CREATE INDEX upload_org_pending ON upload (organization_id, completed_at, id) WHERE status = 'pending';

REVOKE UPDATE, DELETE, TRUNCATE ON upload FROM cashflow_app;
GRANT UPDATE (completed_at, status, content_type, scanned_at) ON upload TO cashflow_app;

-- For every role, the migration role included: a scanned upload is final, completed_at is set
-- once, and what the upload is (its organization, purpose, tenant, key, size, uploader and
-- creation time) never changes. It runs as the caller, on the row it changes.
-- +goose StatementBegin
CREATE FUNCTION reject_upload_change() RETURNS trigger LANGUAGE plpgsql AS $$
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

CREATE TRIGGER upload_forward_only BEFORE UPDATE ON upload
    FOR EACH ROW EXECUTE FUNCTION reject_upload_change();

ALTER TABLE upload ENABLE ROW LEVEL SECURITY;
ALTER TABLE upload FORCE ROW LEVEL SECURITY;
CREATE POLICY upload_org ON upload
    USING (organization_id = app_org_id())
    WITH CHECK (organization_id = app_org_id());

-- +goose Down
DROP TABLE upload;
DROP FUNCTION reject_upload_change();
