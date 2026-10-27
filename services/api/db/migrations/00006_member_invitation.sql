-- +goose Up

-- The WhatsApp invitation of a member. Both columns stay NULL for members who were never
-- invited (founders created with the organization). invitation_state is 'not_sent' from the
-- moment the invite commits until a send reaches WhatsApp, so a crash between the two leaves an
-- honest "not sent". invitation_last_sent_at is the last send that reached WhatsApp; a later
-- failed resend keeps it. membership is written in the organization scope only.
ALTER TABLE membership
    ADD COLUMN invitation_state        text CHECK (invitation_state IN ('sent', 'not_sent')),
    ADD COLUMN invitation_last_sent_at timestamptz,
    ADD CONSTRAINT membership_invitation_sent_has_time
        CHECK (invitation_state IS DISTINCT FROM 'sent' OR invitation_last_sent_at IS NOT NULL),
    ADD CONSTRAINT membership_invitation_time_needs_state
        CHECK (invitation_last_sent_at IS NULL OR invitation_state IS NOT NULL);

-- The person's first successful sign-in, in any organization. Set once by the login, in the
-- auth scope; organizations read it through the staff_user_org_member policy.
ALTER TABLE staff_user ADD COLUMN first_sign_in_at timestamptz;

-- +goose Down
ALTER TABLE staff_user DROP COLUMN first_sign_in_at;
ALTER TABLE membership
    DROP CONSTRAINT membership_invitation_time_needs_state,
    DROP CONSTRAINT membership_invitation_sent_has_time,
    DROP COLUMN invitation_last_sent_at,
    DROP COLUMN invitation_state;
