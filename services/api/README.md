# Cashflow API

The Go backend: an HTTP API (`cmd/api`), a job worker with a cron scheduler (`cmd/worker`), a
migration runner (`cmd/migrate`) and the operator tool (`cmd/admin`). Project rules are in the
repository's `CLAUDE.md`; the plan, architecture and phase records are in
[docs/](../../docs/README.md).

## Run it locally

From the repository root:

```bash
make dev-up     # PostgreSQL and Redis in Docker
make migrate    # create the tables as the owner role
make api-run    # API on :8080
make worker-run # in a second terminal
```

Check it: `curl localhost:8080/healthz` and `curl localhost:8080/readyz`.

### Create an organization and sign in

There is no onboarding screen yet, so create an organization with the operator tool. At least
one member must be an admin. It prints the organization id.

```bash
make admin ARGS='create-org -name "Griya Sejahtera" -display-name "Griya Sejahtera" -member "0812-1000-0001|Bu Ani|supervisor" -member "0812-1000-0002|Pak Budi|admin"'
```

Then sign in as Pak Budi. With `WHATSAPP_PROVIDER=devlog` (the default) no WhatsApp message
is sent: the API log prints a "development only" line with the code in `params`.

```bash
curl -i -H 'Content-Type: application/json' -d '{"phone":"0812-1000-0002"}' localhost:8080/api/auth/otp/request
curl -i -c jar -H 'Content-Type: application/json' -d '{"phone":"0812-1000-0002","code":"123456"}' localhost:8080/api/auth/otp/verify
curl -b jar localhost:8080/api/me
curl -b jar localhost:8080/api/orgs/<organization id>/members
```

The endpoints are described in `api/openapi.yaml`.

Inviting a member (`POST /api/orgs/<organization id>/members`) sends them a WhatsApp invitation
with a link to the web login page, built from `PUBLIC_WEB_ORIGIN` (development defaults to
`http://localhost:3000`; staging and production must set an https origin). With devlog the
invitation shows up in the API log like the login code. A failed or rate-limited send leaves the
member created with `invitation.state` `not_sent`; `POST .../members/<member id>/invitation`
sends it again.

### Demo data (development only)

To get a realistic database without typing it in, seed three demo organizations (Griya
Sejahtera, Kost Putri Anggrek and Kos Bulanan Sejahtera) with their staff, properties, units,
tenants and leases. From `services/api`, with `DATABASE_URL` pointing at your local database
(the app role, as for `create-org`):

```bash
APP_ENV=development go run ./cmd/admin seed-demo
```

It refuses to run unless `APP_ENV` is explicitly `development` (unset does not count), before it
touches the database. It is safe to run again: it matches every record (organizations, members,
properties, units, tenants, leases) and creates only what is missing; it never changes or
deletes existing data. When it finishes it prints the login list to stdout: phone, name,
organization, roles, property limit and status of every demo member. Sign in with any active
one's phone as above. One member, Mas Agus Setiawan in Griya Sejahtera, is disabled on purpose
and cannot sign in: he stays the collector on Eko Prasetyo's lease, so the members screen and
the tenant list show "1 penyewa belum punya penagih". That needs a database seeded from empty
(`make dev-reset`, `make dev-up`, `make migrate`, then `seed-demo`; the reset also removes the
GOWA WhatsApp link): a rerun leaves an existing lease as it is, so on a database seeded before
Agus was added, Eko's lease keeps no collector and the flag does not show.

Seeded leases are entered 40 days before today and bill from the period that ran 70 days ago,
so they have only a few invoices, some overdue. `seed-demo` creates no invoice; run
`APP_ENV=development go run ./cmd/admin generate-invoices` right after it (same development-only
rule). It generates the invoices of the three demo organizations only, never of any other
organization, and records two payments in each: one demo tenant pays everything they owe
(transfer), one pays about half of their oldest invoice (cash), and a third pays nothing and stays
overdue. It is safe to run again: the payments carry fixed request ids, so a repeat records
nothing new, and a later day only bills the periods that have come due since. Like the seed, it
needs a database seeded from empty: a lease seeded earlier keeps its old dates and would bill
every period since its start.

Kos Bulanan Sejahtera is the volume organization, for trying pagination: 27 members (4 of them
disabled, 2 still collecting on running leases), 25 properties, 95 units (30 in Kos Sejahtera
Utama) and 45 tenants, some with a booked lease (starting in June or July 2027), some moved out
and some with no lease. Sign in as Pak Dimas Prakoso (0812-0009-0027, Admin of the whole
organization) or as Mbak Lestari Anjani (0812-0009-0028, Admin of Kontrakan Seturan only).

### The secrets key (`SECRETS_KEY`)

Per-organization secrets, such as an organization's AutoGoPay API key, are stored encrypted
(AES-256-GCM, `internal/secret`) under a master key from the environment. The API, the worker and
`cmd/admin` all refuse to start without a valid one. `SECRETS_KEY` is `<id>:<base64 of 32 random
bytes>`; the id (1 to 32 characters of `a-z`, `0-9` and `-`) is stored next to every ciphertext,
so the database says which key made it.

- `.env.example` carries a development-only key (`dev-only:...`); `make env` adds it to an older
  `.env` that lacks one. It is public: never use it outside your machine.
- For any other environment generate one and keep it in that environment's secret store, never
  in git: `echo "k1:$(openssl rand -base64 32)"`. Losing it means every stored key must be
  entered again.
- **Rotating**: move the current value to `SECRETS_KEY_PREVIOUS` (comma-separated if there are
  several), set a new `SECRETS_KEY` with a new id, and restart the API and the worker. New and
  re-entered secrets use the new key; rows made under an old key still decrypt through
  `SECRETS_KEY_PREVIOUS`. Nothing re-encrypts existing rows yet, so keep an old key in
  `SECRETS_KEY_PREVIOUS` until every row made with it has been set again (for AutoGoPay, with
  `set-autogopay-key` below).

### Set an organization's AutoGoPay API key

QRIS money goes to the landlord's own AutoGoPay account, so each organization has its own API
key. Until a settings screen exists, the platform operator stores it with `cmd/admin`, in any
environment. The key is read as one line from stdin, never from a flag or an environment
variable, and is never printed:

```bash
make admin ARGS='set-autogopay-key -org <organization id>' < autogopay-key.txt
```

It prints the organization id and the organization's webhook path,
`/webhooks/autogopay/<webhook id>`, to register at AutoGoPay (with the public origin in front).
Setting the key again replaces it and keeps the same path.

The landlord (or the operator, signed in to their AutoGoPay dashboard) pastes the full URL,
`https://<public origin>/webhooks/autogopay/<webhook id>`, as the callback URL. AutoGoPay checks
it on save with a signed `verification.challenge` call, which the API answers with
`{"success": true}`; the dashboard refuses the URL if it cannot reach it. From then on a paid QR
arrives as a signed `transaction.received` callback: the API checks `X-Signature` (hex
HMAC-SHA256 of the raw body with that organization's key) before reading anything, and queues
`qris:paid`, which the worker records once per transaction id. A wrong id or signature gets the
same `401 {"success": false}`. If a callback is lost, reconcile records the payment within 30
minutes. Each setting is in the organization's
audit log as `settings:update` on `autogopay_account/<organization id>`.

The API and the worker call AutoGoPay at `AUTOGOPAY_BASE_URL`, by default its gateway
(`https://v1-gateway.autogopay.site`); outside development it must be https. The development
`.env` (from `.env.example`, and added by `make env` to an older one) points it at the local
stand-in, `http://127.0.0.1:8091` (`make fake-autogopay`), so nothing local reaches AutoGoPay. Tests never call it:
they use the fake in `internal/autogopay/autogopaytest`. In development an organization without a
key answers the payment page's "Bayar QRIS" with `qris_unavailable` (bank transfer), and nothing
reaches AutoGoPay. With a real key, every QR made locally is a real QR on that account: AutoGoPay
can block an account that makes QRs in a loop, so make them by hand only.

A payment page request that waits on AutoGoPay holds a database connection for up to about 20
seconds. At most `QRIS_MAX_CONCURRENT` (default 10) do so at once, below the pool's
`DATABASE_MAX_CONNS` (default 20); one more answers `503 qris_overloaded` with `Retry-After: 5`
at once (nothing reached AutoGoPay, so the page may try again), so a hanging AutoGoPay never stops
the page and its bank transfer details from loading.

### The AutoGoPay stand-in (development only)

Locally and in the web's end-to-end tests nothing calls the real AutoGoPay. `make fake-autogopay`
runs `cmd/fakeautogopay` on `127.0.0.1:8091` (`FAKE_AUTOGOPAY_ADDR` to change it): it answers
AutoGoPay's `/qris/generate`, `/qris/status` and `/qris/cancel` with the real response shapes,
accepts any key, and keeps its QRs in memory (they expire after 15 minutes, and a restart forgets
them). The development `.env` already points the API and the worker at it
(`AUTOGOPAY_BASE_URL`, above), so both see the same QRs. It refuses to start unless `APP_ENV=development`.

`seed-demo` gives Griya Sejahtera a made-up key (`demoseed.DemoAutoGoPayKey`) that only the
stand-in accepts, so its payment page offers QRIS; Kost Putri Anggrek has none and shows bank
transfer only. A key already set, a real one included, is never replaced.

Tests act like a tenant or an outage through its control routes (no authentication, so keep it on
loopback):

- `GET /_control/transactions`: every QR, newest first (`transactionId`, `amount`, `status`,
  `createdAt`, `expiresAt`).
- `POST /_control/pay` and `POST /_control/expire` with `{"transactionId"?}`: the QR is paid or
  expires now; without an id, the newest pending QR of any key, so tests that run in parallel
  should name the id.
- `POST /_control/fail-next` with `{"method": "generate" | "status" | "cancel", "amount"?}`: the
  next call of that method answers HTTP 503; with an amount, only a generate for it.
- `POST /_control/reset`: forget every QR and queued failure.

### Set an organization's bank account

The tenant payment page shows the organization's bank account for a manual transfer (decision
38). Until a settings screen exists, the platform operator sets it with `cmd/admin`, in any
environment. The values are not secret:

```bash
make admin ARGS='set-bank-account -org <organization id> -bank BCA -number "123 456 7890" -holder "Suhwendi"'
```

The account number is 5 to 34 digits, spaces or `-`. It prints what it stored; setting it again
replaces it. Each setting is in the organization's audit log as `settings:update` on
`organization_bank_account/<organization id>`.

### Set an organization's broken-promise threshold

A tenant whose broken payment promises reach the organization's threshold (1 to 10, default 2;
decision 41) gets a karyawan visit task when the worker's daily `promises:close` run (00:30
Asia/Jakarta) closes a promise as broken. Until the settings screen exists, the platform operator
sets it with `cmd/admin`, in any environment:

```bash
make admin ARGS='set-broken-promise-threshold -org <organization id> -n 3'
```

It applies from the next promise that closes broken and is in the organization's audit log as
`settings:update` on `organization/<organization id>`.

### Login codes for end-to-end tests (development only)

With `WHATSAPP_PROVIDER=devlog`, set `DEVLOG_FILE=/path/to/devlog.jsonl` and the API also appends
every message to that file as one JSON line: `time`, `to` (E.164), `template`, `params` (in the
template's order) and the rendered `text`. A login code is the last line whose `to` is the phone
and whose `template` is `login_code`; the code is `params[0]`. The file is created with mode 0600.
Config refuses `DEVLOG_FILE` outside development or with any other provider.

### Try the API in Swagger

In development the API serves Swagger UI at http://localhost:8080/docs, from the same origin, so
the login cookie works in "Try it out". The page reloads `api/openapi.yaml` from disk on every
visit.

1. `POST /auth/otp/request` with `{"phone": "0812-1000-0002"}`.
2. Copy the code from the API log (the "development only" line).
3. `POST /auth/otp/verify` with the phone and code. The browser keeps the session cookie, and
   the response lists your organizations with their ids.
4. Call `GET /orgs/{orgId}/me` and `GET /orgs/{orgId}/members` with that id.
5. `POST /auth/logout` when done.

### Send real WhatsApp messages with GOWA

GOWA is a self-hosted WhatsApp gateway that runs as its own container. Which provider the pilot
and launch use is open (decision 35); WhatsApp restricted the number we linked to GOWA.
Use a spare number for development, never the production one.

1. `make wa-up` starts GOWA on http://localhost:3001 (login `cashflow` / `gowa-dev-password`).
2. Open it and link the phone: in WhatsApp, Settings › Linked devices › Link a device, then scan
   the QR code.
3. In `services/api/.env` set `WHATSAPP_PROVIDER=gowa` and restart `make api-run`.

Login codes now arrive on WhatsApp. If the phone is not linked, a code request answers 503
`whatsapp_unavailable` and the API log says why. `make wa-down` and `make dev-down` keep the
linked session in the `gowadata` volume; `make dev-reset` deletes it, which means scanning again.

### Prove a real login code through GOWA (runbook)

Do this by hand with your own phone; no test does it. Use a spare number, never the production
one (staging has its own number too).

1. Run GOWA with a pinned image tag, never `latest`. `infra/compose.dev.yaml` already does:
   `aldinokemal2104/go-whatsapp-web-multidevice:v9.6.0`, `APP_BASIC_AUTH` set, `MCP_ENABLED=false`,
   `gowadata:/app/storages` mounted, port bound as `127.0.0.1:3001:3000`. `make wa-up` starts it.
   On a server copy that service, change the password and tag deliberately, keep the loopback bind.
2. Link the number. Open http://localhost:3001 (basic auth from `APP_BASIC_AUTH`), then in
   WhatsApp on the phone: Settings, Linked devices, Link a device, scan the QR code. On a server
   reach the dashboard only through a tunnel, never a public port:
   `ssh -L 3001:127.0.0.1:3001 user@server`, then open http://localhost:3001.
   Back up the `gowadata` volume: losing it means scanning again.
3. In `services/api/.env` set `WHATSAPP_PROVIDER=gowa`, `GOWA_URL` (`http://localhost:3001`),
   `GOWA_USERNAME` and `GOWA_PASSWORD` (the `APP_BASIC_AUTH` pair); `GOWA_DEVICE_ID` stays
   empty with one linked device. Config refuses to start with gowa and any of the first three
   missing. Restart `make api-run`.
4. Register your phone as a staff member (it must be a number WhatsApp can reach):
   `make admin ARGS='create-org -name "Uji GOWA" -display-name "Uji GOWA" -member "<your phone>|<your name>|admin"'`
   (a supervisor-only org is rejected: at least one member must be an admin).
5. Request a code from the web login page (`make web-dev`, http://localhost:3000/login), or:
   `curl -i -H 'Content-Type: application/json' -d '{"phone":"<your phone>"}' localhost:8080/api/auth/otp/request`
6. Check:
   - The message arrives within seconds: "Kode masuk Anda: *123456*", valid 5 minutes.
   - Enter the code on the login page (or `otp/verify` as above); you are signed in.
   - The API log has `login code sent` with a `staff_user_id` and never the code itself.
   - Not linked (or session lost): the request answers 503 `whatsapp_unavailable` and the log
     has `whatsapp unavailable` naming `the WhatsApp session is not connected`. Re-link in step 2.
   - Wrong `GOWA_USERNAME` or `GOWA_PASSWORD`: the request answers 500 `internal`, and the log has
     `request failed` with "the server rejected our credentials; check GOWA_USERNAME and GOWA_PASSWORD".
   - Limits, not failures: 3 code requests per phone per 15 minutes, 5 tries per code.
     An unregistered phone gets the same answer as a registered one, but no message.
7. Stop: end `make api-run` (and the web), then `make wa-down`. This keeps the link. To unlink,
   remove the device in WhatsApp (Linked devices), or call GOWA's `GET /app/logout` with basic
   auth. Keep the `gowadata` volume to stay linked, or delete it with `make dev-reset`
   (which also wipes the database).

### Send WhatsApp through Twilio (Twilio sandbox, development only)

A Twilio sandbox for WhatsApp sends from a number Twilio owns, with no phone of our own to link.
It is for development only: staging and production refuse `WHATSAPP_PROVIDER=twilio` until a
verified sender with approved templates exists. The sandbox number is on the console's sandbox
page and can differ per account, so copy it from there.

1. In the Twilio console go to Messaging, Try it out, Send a WhatsApp message. Note the sandbox
   number and the join keyword shown there.
2. From your phone, send `join <keyword>` to the sandbox number on WhatsApp and wait for
   Twilio's confirmation reply.
3. Create the login code template (Twilio wants one, see below): Messaging, Content Template
   Builder, Create new. Name it `login_code`, language Indonesian, content type Text. Body:
   `Kode masuk Anda: *{{1}}*`, a blank line, then `Berlaku 5 menit. Jangan berikan kode ini
   kepada siapa pun, termasuk orang yang mengaku petugas.` Give `123456` as the sample for
   `{{1}}`. Save it; there is no need to submit it for WhatsApp approval. Copy its Content SID,
   which starts with HX.
4. In `services/api/.env` set `WHATSAPP_PROVIDER=twilio`, `TWILIO_ACCOUNT_SID` and
   `TWILIO_AUTH_TOKEN` (from the console's Account Info; never commit them),
   `TWILIO_WHATSAPP_FROM` (the sandbox number from step 1, as `+` and digits), and
   `TWILIO_CONTENT_SIDS=login_code=<the HX value from step 3>`. Config refuses to start with
   any of the first three missing, a malformed Account SID, a sender that is not E.164, a
   template name it does not know or a Content SID that is not HX and 32 hex characters.
5. Restart `make api-run`.
6. From your phone, send any message to the sandbox number (this opens the 24-hour window, see
   below), then request a login code for a member whose phone joined (steps 4 and 5 of the GOWA
   runbook above). The code arrives as a WhatsApp message from the sandbox number.

Content templates: `TWILIO_CONTENT_SIDS` maps our template names to Twilio Content templates,
as `name=HX...` pairs separated by commas. A template listed there is sent as its Content SID,
with the values numbered as the template lists its parameters (`login_code`: `{{1}}` is the
code; `member_invite`: `{{1}}` name, `{{2}}` role, `{{3}}` organization, `{{4}}` inviter, `{{5}}`
the login link). Any template not listed is sent as plain text, which Twilio may refuse with
`code 21654`. The Content template's text should match `internal/messaging/templates.go`.

A Twilio trial account cannot create Content templates (the console asks to upgrade first), so
step 3 needs an upgraded account. Twilio's sample template on the Try it out page is no
substitute: it is fixed text with no `{{1}}` ("Reminder: Your event starts tonight at 7 PM ...
Test message from Twilio."), so mapping `login_code` to it delivers that text and the login code
never appears (tried 2026-11-01). On a trial, use `WHATSAPP_PROVIDER=devlog` (the code is in the
API log); to get real login codes over WhatsApp, upgrade Twilio and create `login_code` as in
step 3.

What the sandbox can do: send to phones that joined it, within 24 hours of the phone's last
message to the sandbox number. What it cannot do: reach a phone that has not joined, or send
outside that 24-hour window. An unapproved Content template does not lift the window; only a
WhatsApp-approved template on a verified sender of our own does, which is not set up.

When a rule bites, Twilio does one of two things. It may refuse the message at once (for
example `code 21654`, "ContentSid Required", for plain text); the code request then answers 503
`whatsapp_unavailable` and the log names the code. Or it accepts the message and fails it a
moment later; the API cannot see that yet (status callbacks come with slice 5), so it answers
that the code was sent, and nothing arrives. Either way, check `TWILIO_CONTENT_SIDS`, then fix
it from the phone: send any message to the sandbox number (or `join <keyword>` again if the
phone left the sandbox), then request a new code within 24 hours.

Wrong credentials or a sender Twilio does not accept fail at once, like an unlinked GOWA phone:
a code request answers 503 `whatsapp_unavailable` and the log has `whatsapp unavailable` naming
`the WhatsApp session is not connected`; a member invitation answers 503 "WhatsApp gagal
mengirim". Any other refusal from Twilio, and Twilio failing on its side, answer the same 503
instead of a 500. The log carries only Twilio's HTTP status and error code (for example
`code 20003` for a wrong Account SID or Auth Token, `code 63007` for a sender without
WhatsApp), never the phone number or the message; look the code up at
https://www.twilio.com/docs/api/errors.

### Telegram bot in development (polling)

Telegram is a demo channel only (decision 42), like the Twilio sandbox: it shows the flows end to
end in development and staging with test accounts, and the API and worker refuse any `TELEGRAM_*`
setting in production. Staging will receive the bot's updates on a webhook; on a developer machine `make telegram-poll` runs `cmd/telegrampoll`,
which asks Telegram for updates by long polling instead, so no public address or tunnel is
needed. It refuses to start unless `APP_ENV=development`, a bot token is set and `DATABASE_URL`
reaches the database (it links and unlinks chats as the app role).

1. In Telegram, open @BotFather, send `/newbot` and follow it. Use a development bot of your own,
   never the staging bot. BotFather answers with the bot's username and its token.
2. In `services/api/.env` set `TELEGRAM_BOT_TOKEN` (the token) and `TELEGRAM_BOT_USERNAME` (the
   username without the @). Never commit the token and never paste it in a chat, an issue or a
   pull request; if it leaks, revoke it in BotFather (`/revoke`) and set the new one.
3. Run `make telegram-poll`. It checks the token with `getMe` and logs the bot's username and id,
   then hands each update to the same handler the webhook uses (`internal/telegramlink`): on a
   tenant's payment page, "Hubungkan Telegram" opens the bot with a one-time code (valid 10
   minutes), and pressing Start links that chat to the tenant; `/stop` unlinks it. Each update is
   logged by its id, kind, command and outcome, never the chat, the code or the text. Run the
   API with the same `TELEGRAM_*` settings plus `TELEGRAM_MESSAGES=linked` so the page offers the
   button. Ctrl-C stops it.

Set `TELEGRAM_MESSAGES=linked` for the worker too, so a linked tenant's messages reach that chat.
The router (`internal/messaging/router`) then sends each reminder and promise
confirmation for a tenant with a live link to their chat, as plain text with the pay link as a
button; other tenants and invitations still go to `WHATSAPP_PROVIDER`. Telegram
messages are not held by the WhatsApp pacing guard (decision 41.6), because they do not use the
shared number. When Telegram says the chat cannot be reached (the bot was blocked), the link ends
and the tenant's next message goes to WhatsApp; nothing falls back within one send. The quickest to
see: link a test tenant, then make a promise on their payment page; the "janji tercatat"
confirmation arrives in the chat, and the worker logs "message sent" with `"channel":"telegram"`.

Test staff link the same way, for their login codes (TG-4). Signed in, `POST /api/me/telegram`
("Hubungkan Telegram" on the account) answers the bot's deep link with a one-time code (10
minutes, used once, at most 5 per staff member per 15 minutes); pressing Start links that chat to
the staff user (`internal/auth`'s `TelegramStaff`, in the auth scope), and the bot answers
"Terhubung. Kode masuk untuk <nama> akan dikirim ke sini." `GET /api/me/telegram` says whether it
is linked (`available` is false without a bot or with `TELEGRAM_MESSAGES` off, like the
payment page's button), `DELETE` unlinks, and `/stop` in the chat unlinks
its tenants and staff alike. A newer link replaces the older one and the older chat is told. With
`TELEGRAM_MESSAGES=linked` the API sends a linked staff member's login code to their chat;
`whatsappOnly: true` on `POST /api/auth/otp/request` ("Kirim lewat WhatsApp") sends it on
WhatsApp instead, under the same limit of 3 per number per 15 minutes. The 202 is the same for
every number: its `telegramAvailable` only says whether this environment sends codes to linked
chats (a bot and `TELEGRAM_MESSAGES=linked`), never whether this number is linked or registered.

With `TELEGRAM_MESSAGES=linked`, linked staff also get staff notices (TG-5, `internal/staffnotice`):
when a tenant makes or edits a promise on the payment page (not one a member saves), when a
promise closes as broken, and when a tenant joins the visit list (H+7, or too many broken
promises). A notice goes to the collectors of the tenant's active leases and to every active
member allowed `payment:record` on the tenant through one of them (Admins of that property or of
the whole organization; not a Supervisor unless they collect), and only to those with a live
Telegram link: never on WhatsApp, which comes for staff in slice 5. The notice is recorded with
its event and sent once by the worker's `staff:notice` task between 08:00 and 21:59 (a promise
broken at 00:30 is told at 08:00, by `reminders:sweep`), and is skipped when it no longer applies
(the promise was edited again, the visit task was resolved). With the setting off nothing is
recorded. The worker logs "staff notice sent" with the number of recipients, those delivered and
those not linked.

If it stops with "getUpdates is blocked", the bot has a webhook set (Telegram allows either a
webhook or polling, not both) or another poller is running with the same token. Delete the
webhook with the Bot API's `deleteWebhook` method, or stop the other poller. A token Telegram
refuses (wrong, or revoked in BotFather) stops it with "telegram refused the bot token". Tests
never call Telegram: `internal/messaging/telegram` is tested against `httptest` servers that
replay Bot API answers.

## Everyday commands

| Command | What it does |
|---|---|
| `make api-test` | unit tests |
| `make api-test-integration` | tests that start PostgreSQL and Redis in Docker |
| `make api-lint` | `go vet` and `golangci-lint` |
| `make sqlc` | regenerate `internal/platform/store/dbgen` after editing `db/queries` or `db/migrations` |
| `make api-gen` | regenerate `internal/httpapi/apigen` after editing `api/openapi.yaml` |
| `make admin ARGS='...'` | run the operator tool |
| `make wa-up` / `make wa-down` | start or stop the GOWA WhatsApp gateway |
| `make telegram-poll` | long-poll the development Telegram bot (development only) |
| `make api-check` | everything CI runs |

## Where things live

- `internal/platform/store`: the database pool, `WithOrgTx` and `WithAuthTx`. Every query
  runs inside one of them.
- `db/migrations`: goose SQL migrations. `db/queries`: the SQL that sqlc turns into Go.
- `internal/auth`: WhatsApp login codes and sessions, and staff members' Telegram links for login
  codes in demo environments (`TelegramStaff`).
- `internal/authz`: the IAM-style policy engine, built-in roles, guardrails and the action
  catalog, plus loading principals and writing the audit log.
- `internal/member` and `internal/org`: members and their roles; creating organizations.
- `internal/httpapi`: routing, middleware, handlers. `apigen` is generated from the OpenAPI file.
- `internal/messaging`: the WhatsApp provider interface and the message templates. Adapters:
  `gowa`, `twilio` (the Twilio sandbox, development only), `devlog` (development log)
  and the test fake in `messagingtest`. `telegram` is the Telegram Bot API client (send to a
  chat, `getMe`, `getUpdates`, the development poll loop); it sends to a chat id, so it is not a
  WhatsApp provider. `router` is the provider in front of WhatsApp when `TELEGRAM_MESSAGES=linked`:
  it sends a linked tenant's messages, and a linked staff member's login codes, to their Telegram
  chat, and a `TelegramOnly` message (staff notices) there or nowhere.
- `internal/staffnotice`: staff notices on Telegram in demo environments (TG-5): recording them
  with their event, choosing the recipients, sending; `noticetask` is the `staff:notice` task.
- `internal/secret`: AES-256-GCM encryption of per-organization secrets under `SECRETS_KEY`, with
  key rotation, and `secret.Value`, which prints as `[redacted]`.
- `internal/autogopay`: each organization's AutoGoPay account (encrypted API key, webhook
  id) and the QRIS client (generate, status, cancel), with its fake in `autogopaytest`.
- `internal/qris`: a tenant's QRIS attempts behind a payment link: making a QR (reusing the live
  one, capped per tenant), "Saya sudah bayar", the status the payment page polls, the QR image,
  recording a paid QR once per transaction id, and the reconcile that asks AutoGoPay about pending
  attempts. `qristask` runs reconcile from the worker every 30 minutes.
- `internal/promise`: tenants' payment promises: making and editing them, closing one as kept
  when a payment keeps it, and closing the rest once their day has ended (kept, partially kept,
  broken), with the broken_promises visit task. `promisetask` runs the close from the worker at
  00:30 Asia/Jakarta.
- `internal/reminder`: the daily reminder decisions (H-3, H0, H+3, a promise's H-1, a broken
  promise's notice, and the h7 visit task at H+7), recorded once each as `reminder` rows, one per
  tenant and run, then sent at most once with a fresh pay link (`reminder:send`), like each
  promise version's "janji tercatat" confirmation (`promise:confirm`). `remindertask` runs
  `reminders:decide` from the worker at 08:00 Asia/Jakarta, one task per organization and day, and
  the sweep that re-enqueues anything still pending.
- `internal/phone`, `internal/ratelimit`: phone number parsing and attempt counting.
- `internal/platform/{config,logging,clock,queue}`: process plumbing.
- `internal/platform/pgtest`, `internal/testenv`: integration test setup.

## Rules to remember

- Every table gets row-level security (`ENABLE`, `FORCE` and a policy). The integration test
  `TestEveryTableHasForcedRLS` fails if a migration forgets.
- The pool is private to `internal/platform/store`. Importing `pgxpool` elsewhere fails lint.
- Check permissions with `authz.Service.Authorize` inside the transaction that makes the change.
- A new action goes into `internal/authz/catalog.go` and the role matrix test.
