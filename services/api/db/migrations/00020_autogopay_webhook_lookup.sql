-- +goose Up

-- Slice 3 (S3-6): the AutoGoPay webhook lookup (decision 39, point 2). AutoGoPay calls
-- /webhooks/autogopay/<webhook_id> with no session and no organization, and its body names no
-- account, so no RLS scope can see autogopay_account yet. This function, run as the role that ran
-- this migration, maps a webhook id to the account's organization and nothing else: not the key
-- or its ciphertext. The handler then reads the key inside that organization's scope (WithOrgTx)
-- and checks the signature with it before it trusts anything in the body.
--
-- autogopay_account forces row-level security, which holds the function's owner too, so as
-- payment_link_lookup does (00017_payment_link.sql) the function turns its own setting on for its
-- duration, and one policy lets only the role that ran this migration read accounts while it is
-- on. The app role cannot use the policy whatever it sets. The other SECURITY DEFINER functions
-- (active_organization_ids, payment_link_lookup) never read autogopay_account, and this one reads
-- no other table, so no policy widens what another function can see;
-- TestActiveOrganizationIDsIsTheOnlySecurityDefiner lists all three.
CREATE POLICY autogopay_account_webhook_lookup ON autogopay_account FOR SELECT TO CURRENT_USER
    USING (current_setting('app.autogopay_webhook_lookup', true) = 'on');

-- +goose StatementBegin
CREATE FUNCTION autogopay_webhook_lookup(p_webhook_id text)
    RETURNS TABLE (organization_id uuid)
    LANGUAGE sql STABLE SECURITY DEFINER
    SET search_path = pg_catalog, public
    SET app.autogopay_webhook_lookup = 'on'
AS $$
    SELECT a.organization_id FROM autogopay_account a WHERE a.webhook_id = p_webhook_id
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION autogopay_webhook_lookup(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION autogopay_webhook_lookup(text) TO cashflow_app;

-- +goose Down
DROP FUNCTION autogopay_webhook_lookup(text);
DROP POLICY autogopay_account_webhook_lookup ON autogopay_account;
