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
| `make api-check` | everything CI runs |

## Where things live

- `internal/platform/store`: the database pool, `WithOrgTx` and `WithAuthTx`. Every query
  runs inside one of them.
- `db/migrations`: goose SQL migrations. `db/queries`: the SQL that sqlc turns into Go.
- `internal/auth`: WhatsApp login codes and sessions.
- `internal/authz`: the IAM-style policy engine, built-in roles, guardrails and the action
  catalog, plus loading principals and writing the audit log.
- `internal/member` and `internal/org`: members and their roles; creating organizations.
- `internal/httpapi`: routing, middleware, handlers. `apigen` is generated from the OpenAPI file.
- `internal/messaging`: the WhatsApp provider interface and the message templates. Adapters:
  `gowa`, `twilio` (the Twilio sandbox, development only), `devlog` (development log)
  and the test fake in `messagingtest`.
- `internal/phone`, `internal/ratelimit`: phone number parsing and attempt counting.
- `internal/platform/{config,logging,clock,queue}`: process plumbing.
- `internal/platform/pgtest`, `internal/testenv`: integration test setup.

## Rules to remember

- Every table gets row-level security (`ENABLE`, `FORCE` and a policy). The integration test
  `TestEveryTableHasForcedRLS` fails if a migration forgets.
- The pool is private to `internal/platform/store`. Importing `pgxpool` elsewhere fails lint.
- Check permissions with `authz.Service.Authorize` inside the transaction that makes the change.
- A new action goes into `internal/authz/catalog.go` and the role matrix test.
