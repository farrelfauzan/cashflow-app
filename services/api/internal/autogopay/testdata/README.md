# AutoGoPay fixtures

Responses the contract tests in `client_test.go` replay. Every one was answered with HTTP 200
unless its name says otherwise.

## Scrubbed real recordings (live API, 2026-11-04)

`generate.json`, `status_pending.json`, `cancel.json`, `status_after_cancel.json` and
`status_unknown.json` are real answers. Only the secret or identifying values were replaced, with
values that are clearly synthetic:

- `transaction_id`: `00000000-0000-4000-8000-0000000000a1` (also inside `qr_url`)
- `order_id`: `AUTOGOPAY-1762231189-1`
- `qr_string`: `00020101021226TEST-NOT-A-REAL-QRIS6304ABCD`
- the checkout token in `checkout_url`: `test-checkout-token-not-real`

Every other field and value is exactly as recorded.

## Derived from the public API doc (no real example yet)

No paid or expired QR, refused call or rejected generate has been recorded, so these are built
from the documentation and are not real answers:

- `status_settlement.json`, `status_paid.json`, `status_expire.json`: `status_pending.json` with
  the documented status (`settlement` for the status endpoint, `PAID` for the webhook) and a
  matching message. The real `success` and `message` for these statuses are unknown.
- `status_unknown_value.json`: a status AutoGoPay does not document, to prove it is refused.
- `error_401.json`, `error_403.json`: the documented bodies for an invalid key (HTTP 401) and a
  request from an IP outside the key's whitelist (HTTP 403).
- `generate_rejected.json`: an invented `success:false` answer to generate (HTTP 200).

Replace a derived file with a scrubbed recording once a real one is seen.
