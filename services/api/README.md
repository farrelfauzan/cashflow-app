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

The launch provider is GOWA, a self-hosted WhatsApp gateway that runs as its own container.
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
  `gowa` (launch), `devlog` (development log) and the test fake in `messagingtest`.
- `internal/phone`, `internal/ratelimit`: phone number parsing and attempt counting.
- `internal/platform/{config,logging,clock,queue}`: process plumbing.
- `internal/platform/pgtest`, `internal/testenv`: integration test setup.

## Rules to remember

- Every table gets row-level security (`ENABLE`, `FORCE` and a policy). The integration test
  `TestEveryTableHasForcedRLS` fails if a migration forgets.
- The pool is private to `internal/platform/store`. Importing `pgxpool` elsewhere fails lint.
- Check permissions with `authz.Service.Authorize` inside the transaction that makes the change.
- A new action goes into `internal/authz/catalog.go` and the role matrix test.
