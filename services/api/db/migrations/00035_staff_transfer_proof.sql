-- +goose Up

-- Decision 49 (T-102): a member may send a tenant's transfer proof, when the tenant gave the
-- photo to the karyawan on site instead of sending it from the payment page. It is the same
-- proof (an upload of purpose transfer_proof, the same scan, the same Antrean cek item), sent
-- through RBAC (payment:proof) instead of a payment link. The upload then has a staff uploader,
-- and the proof records who sent it and names no link or invoice: FIFO decides what the admin's
-- match pays.

-- A transfer proof's upload is the tenant's (no staff uploader) or a member's; every other upload
-- keeps a staff uploader.
ALTER TABLE upload DROP CONSTRAINT upload_staff_uploader;
ALTER TABLE upload ADD CONSTRAINT upload_staff_uploader
    CHECK (purpose = 'transfer_proof' OR uploaded_by_staff_user_id IS NOT NULL);

ALTER TABLE transfer_proof ALTER COLUMN payment_link_id DROP NOT NULL;
ALTER TABLE transfer_proof ALTER COLUMN invoice_id DROP NOT NULL;
ALTER TABLE transfer_proof ADD COLUMN sent_by_staff_user_id uuid REFERENCES staff_user (id);
-- Sent from a link (with its invoice) or by a member, never both, never neither.
ALTER TABLE transfer_proof ADD CONSTRAINT transfer_proof_link_invoice
    CHECK ((payment_link_id IS NULL) = (invoice_id IS NULL));
ALTER TABLE transfer_proof ADD CONSTRAINT transfer_proof_one_sender
    CHECK ((payment_link_id IS NULL) <> (sent_by_staff_user_id IS NULL));

-- The proof's sender is its upload's uploader: none for a link's proof, the member for a staff
-- proof. A foreign key cannot say "none", so a trigger checks it on insert (the table is
-- append-only, so there is nothing else to check).
-- +goose StatementBegin
CREATE FUNCTION check_transfer_proof_sender() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    uploader uuid;
BEGIN
    SELECT u.uploaded_by_staff_user_id INTO uploader
    FROM upload u
    WHERE u.organization_id = NEW.organization_id AND u.id = NEW.upload_id;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'transfer proof % names no upload of its organization', NEW.id
            USING ERRCODE = 'foreign_key_violation';
    END IF;
    IF uploader IS DISTINCT FROM NEW.sent_by_staff_user_id THEN
        RAISE EXCEPTION 'transfer proof % was not sent by its upload''s uploader', NEW.id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER transfer_proof_sender BEFORE INSERT ON transfer_proof
    FOR EACH ROW EXECUTE FUNCTION check_transfer_proof_sender();

-- +goose Down
-- Down only works while no member has sent a proof.
DROP TRIGGER transfer_proof_sender ON transfer_proof;
DROP FUNCTION check_transfer_proof_sender();
ALTER TABLE transfer_proof DROP CONSTRAINT transfer_proof_one_sender;
ALTER TABLE transfer_proof DROP CONSTRAINT transfer_proof_link_invoice;
ALTER TABLE transfer_proof DROP COLUMN sent_by_staff_user_id;
ALTER TABLE transfer_proof ALTER COLUMN invoice_id SET NOT NULL;
ALTER TABLE transfer_proof ALTER COLUMN payment_link_id SET NOT NULL;
ALTER TABLE upload DROP CONSTRAINT upload_staff_uploader;
ALTER TABLE upload ADD CONSTRAINT upload_staff_uploader
    CHECK ((purpose = 'transfer_proof') = (uploaded_by_staff_user_id IS NULL));
