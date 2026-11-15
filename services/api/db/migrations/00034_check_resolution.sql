-- +goose Up

-- Slice 6 (S6-6): Antrean cek, the admin's check queue. The queue is not a table: it is read
-- from its sources (a tenant's transfer proof, a deposit whose counted cash differs from its
-- payments, an infected upload, a QRIS payment AutoGoPay reported at another amount, a new phone
-- number a karyawan brought back from a visit). An item leaves the queue when it has a
-- check_resolution: matched (a proof became a transfer payment), applied (the new number is now
-- the tenant's) or dismissed (with a note: not rent, the deposit gap closed, handled).
--
-- Each kind has its own column, a foreign key to its source within the same organization, and is
-- answered once (UNIQUE). The table is append-only (privileges revoked and triggers).

-- The keys the resolutions name: each source row with its organization.
ALTER TABLE transfer_proof ADD CONSTRAINT transfer_proof_id_org UNIQUE (id, organization_id);
ALTER TABLE cash_deposit_confirmation ADD CONSTRAINT cash_deposit_confirmation_id_org UNIQUE (deposit_id, organization_id);
ALTER TABLE qris_attempt ADD CONSTRAINT qris_attempt_id_org UNIQUE (id, organization_id);
ALTER TABLE visit_outcome ADD CONSTRAINT visit_outcome_id_org UNIQUE (id, organization_id);

CREATE TABLE check_resolution (
    id                uuid        PRIMARY KEY,
    organization_id   uuid        NOT NULL REFERENCES organization (id),
    kind              text        NOT NULL CONSTRAINT check_resolution_kind
        CHECK (kind IN ('transfer_proof', 'deposit_gap', 'upload_infected', 'qris_amount_mismatch', 'phone_changed')),
    transfer_proof_id uuid        CONSTRAINT check_resolution_proof_once UNIQUE,
    cash_deposit_id   uuid        CONSTRAINT check_resolution_deposit_once UNIQUE,
    upload_id         uuid        CONSTRAINT check_resolution_upload_once UNIQUE,
    qris_attempt_id   uuid        CONSTRAINT check_resolution_qris_once UNIQUE,
    visit_outcome_id  uuid        CONSTRAINT check_resolution_outcome_once UNIQUE,
    outcome           text        NOT NULL CONSTRAINT check_resolution_outcome
        CHECK (outcome IN ('matched', 'applied', 'dismissed')),
    -- matched: the transfer payment the proof became.
    payment_id        uuid        CONSTRAINT check_resolution_payment_once UNIQUE,
    note              text        CONSTRAINT check_resolution_note
        CHECK (length(note) BETWEEN 1 AND 500 AND note = btrim(note)),
    staff_user_id     uuid        NOT NULL,
    created_at        timestamptz NOT NULL,
    -- Exactly the column of its kind names the item.
    CONSTRAINT check_resolution_names_its_item CHECK (
        (kind = 'transfer_proof') = (transfer_proof_id IS NOT NULL)
        AND (kind = 'deposit_gap') = (cash_deposit_id IS NOT NULL)
        AND (kind = 'upload_infected') = (upload_id IS NOT NULL)
        AND (kind = 'qris_amount_mismatch') = (qris_attempt_id IS NOT NULL)
        AND (kind = 'phone_changed') = (visit_outcome_id IS NOT NULL)),
    -- Only a proof is matched, and a matched proof names its payment; only a number is applied;
    -- a dismissal says why.
    CONSTRAINT check_resolution_matched CHECK ((outcome = 'matched') = (payment_id IS NOT NULL)),
    CONSTRAINT check_resolution_matched_proof CHECK (outcome <> 'matched' OR kind = 'transfer_proof'),
    CONSTRAINT check_resolution_applied_phone CHECK (outcome <> 'applied' OR kind = 'phone_changed'),
    CONSTRAINT check_resolution_dismissed_note CHECK (outcome <> 'dismissed' OR note IS NOT NULL),
    FOREIGN KEY (transfer_proof_id, organization_id) REFERENCES transfer_proof (id, organization_id),
    FOREIGN KEY (cash_deposit_id, organization_id) REFERENCES cash_deposit_confirmation (deposit_id, organization_id),
    FOREIGN KEY (upload_id, organization_id) REFERENCES upload (id, organization_id),
    FOREIGN KEY (qris_attempt_id, organization_id) REFERENCES qris_attempt (id, organization_id),
    FOREIGN KEY (visit_outcome_id, organization_id) REFERENCES visit_outcome (id, organization_id),
    FOREIGN KEY (payment_id, organization_id) REFERENCES payment (id, organization_id),
    FOREIGN KEY (organization_id, staff_user_id) REFERENCES membership (organization_id, staff_user_id)
);
CREATE INDEX check_resolution_org_created ON check_resolution (organization_id, created_at);

REVOKE UPDATE, DELETE, TRUNCATE ON check_resolution FROM cashflow_app;

CREATE TRIGGER check_resolution_append_only BEFORE UPDATE OR DELETE ON check_resolution
    FOR EACH ROW EXECUTE FUNCTION reject_change();
CREATE TRIGGER check_resolution_no_truncate BEFORE TRUNCATE ON check_resolution
    FOR EACH STATEMENT EXECUTE FUNCTION reject_change();

-- The queue: every item of every kind still waiting, with when it arrived. security_invoker
-- makes the view read its tables as the caller, so row-level security applies through it.
--   transfer_proof        a tenant's proof whose photo the scan has not refused (an infected
--                         one is an upload_infected item instead)
--   deposit_gap           a confirmed deposit whose counted cash differs from its payments
--   upload_infected       any upload the scan found infected
--   qris_amount_mismatch  a QRIS attempt AutoGoPay reported at another amount, from the audit
--                         rows the platform writes for it (decision 39.5: payment:record, allowed,
--                         no staff actor), so also one whose payment was already recorded; a
--                         member's audited check on the same resource (answering the item) never
--                         puts an attempt on the queue
--   phone_changed         a new number a karyawan brought back from a visit (decision 46.6)
CREATE VIEW check_queue_item WITH (security_invoker = true) AS
SELECT 'transfer_proof'::text AS kind, tp.organization_id, tp.id AS item_id, tp.created_at
FROM transfer_proof tp
JOIN upload u ON u.organization_id = tp.organization_id AND u.id = tp.upload_id
WHERE u.status IN ('pending', 'clean')
  AND NOT EXISTS (SELECT 1 FROM check_resolution r
                  WHERE r.organization_id = tp.organization_id AND r.transfer_proof_id = tp.id)
UNION ALL
SELECT 'deposit_gap', c.organization_id, c.deposit_id, c.confirmed_at
FROM cash_deposit_confirmation c
JOIN cash_deposit d ON d.organization_id = c.organization_id AND d.id = c.deposit_id
WHERE c.received_amount <> d.expected_amount
  AND NOT EXISTS (SELECT 1 FROM check_resolution r
                  WHERE r.organization_id = c.organization_id AND r.cash_deposit_id = c.deposit_id)
UNION ALL
SELECT 'upload_infected', u.organization_id, u.id, coalesce(u.scanned_at, u.created_at)
FROM upload u
WHERE u.status = 'infected'
  AND NOT EXISTS (SELECT 1 FROM check_resolution r
                  WHERE r.organization_id = u.organization_id AND r.upload_id = u.id)
UNION ALL
SELECT 'qris_amount_mismatch', a.organization_id, a.id, min(l.created_at)
FROM qris_attempt a
JOIN audit_log l ON l.organization_id = a.organization_id
                AND l.resource = 'qris_amount_mismatch/' || a.id::text
                AND l.action = 'payment:record' AND l.decision = 'allow' AND l.actor_staff_user_id IS NULL
WHERE NOT EXISTS (SELECT 1 FROM check_resolution r
                  WHERE r.organization_id = a.organization_id AND r.qris_attempt_id = a.id)
GROUP BY a.organization_id, a.id
UNION ALL
SELECT 'phone_changed', o.organization_id, o.id, o.created_at
FROM visit_outcome o
WHERE o.outcome = 'number_changed'
  AND NOT EXISTS (SELECT 1 FROM check_resolution r
                  WHERE r.organization_id = o.organization_id AND r.visit_outcome_id = o.id);
GRANT SELECT ON check_queue_item TO cashflow_app;
-- The QRIS branch finds the platform's own mismatch rows by resource; audit_log only grows, so
-- this partial index keeps the join to the platform's few payment:record rows.
CREATE INDEX audit_log_platform_payment_record ON audit_log (organization_id, resource)
    WHERE action = 'payment:record' AND decision = 'allow' AND actor_staff_user_id IS NULL;

ALTER TABLE check_resolution ENABLE ROW LEVEL SECURITY;
ALTER TABLE check_resolution FORCE ROW LEVEL SECURITY;
CREATE POLICY check_resolution_org ON check_resolution
    USING (organization_id = app_org_id())
    WITH CHECK (organization_id = app_org_id());

-- +goose Down
DROP INDEX audit_log_platform_payment_record;
DROP VIEW check_queue_item;
DROP TABLE check_resolution;
ALTER TABLE visit_outcome DROP CONSTRAINT visit_outcome_id_org;
ALTER TABLE qris_attempt DROP CONSTRAINT qris_attempt_id_org;
ALTER TABLE cash_deposit_confirmation DROP CONSTRAINT cash_deposit_confirmation_id_org;
ALTER TABLE transfer_proof DROP CONSTRAINT transfer_proof_id_org;
