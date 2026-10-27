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
