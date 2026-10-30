# CLAUDE.md

Multi-tenant SaaS for Indonesian rental-property owners (kos, kontrakan, ruko, plus
optional other business units). It replaces manual billing and bookkeeping (notebooks,
Excel). Core principle: **"Supervisor melihat, sistem bekerja"**. The owner-level
Supervisor never types data. Admin, karyawan and penyewa enter it, and the system
summarizes, reminds and reports.

## Project status (read first)

- **Backend: slice 1 is nearly done; the web app is starting.** `services/api` has slice 0
  (scaffold, RLS, CI) and slice 1's backend: WhatsApp OTP login, sessions, the policy engine,
  built-in roles, members, `cmd/admin` to create organizations, properties, units, roles
  limited to one property, tenants, leases with move-out, and the tenant CSV import with its
  template. Its README says how to run it. Not built yet: invoices and everything after them
  (phase B onward), including the extra charges designed for slice 2. The VPS provider and
  staging are planned for the last phase (F, slice 12, decision 31); until then checkpoints are
  shown on a local machine.
- **Web:** the scaffold plan is approved (decision 20) and the frontend is building `apps/web`.
  Create or change the scaffold only as part of that work. The designs for the login,
  organization picker and members screens are still awaiting approval.
- **WhatsApp sending works through GOWA.** `internal/messaging/gowa` sends login codes today.
  In development `WHATSAPP_PROVIDER=devlog` writes messages to the API log instead, and only
  development may use it. Inbound messages (the GOWA webhook), the message log and send pacing
  are slice 5.
- **Plan and records** live in `docs/` (start at `docs/README.md`): the roadmap, how a slice is
  built, the decision log, the architecture, and one doc per phase. When a slice finishes,
  update its phase doc and the status table; when a decision is made, add it to
  `docs/plan/decisions.md`. The next phase to build is B (money in), after the rest of slice 1.
- **Pull requests**: write the description from `.github/pull_request_template.md`, titled
  `[<phase><slice>] <what works now>` (guide and example in `docs/plan/pull-requests.md`). When
  opening a PR, use this template instead of a generic summary/changes/how-to-test body. No code
  blocks, no test output, no file-by-file list. Reviews follow
  `docs/plan/reviewing-pull-requests.md`: labelled inline comments (`blocking:`, `should:`,
  `question:`, `nit:`) plus one summary comment from `.github/review_template.md`, with the
  matching verdict.
- **Rules and skills**: Go style, patterns, security and testing rules are in
  `.claude/rules/golang-rules.md`. Go project layout is the `golang` skill
  (`.claude/skills/golang`). See "Go conventions" for what is specific to this project.
  Web rules (`apps/web`: server/client split, API client, `proxy.ts`, UI) are in
  `.claude/rules/nextjs-rules.md`.
- **Design is under revision.** `design/` (once added) is the reference for structure,
  copy and flows, but flows may still change. Confirm a flow before building it.
- **WhatsApp**: one platform-wide number for all organizations (decided). Launch on GOWA
  (decided), because the WhatsApp Cloud API needs a registered company (PT) we do not have yet.
  Everything stays designed so a Cloud API adapter can replace GOWA later behind
  `MessagingProvider` without touching callers. See "Messaging".
- Ask before deviating from any decision marked **Decided** below.

## Decisions

| Area | Status | Choice |
|---|---|---|
| QRIS gateway | **Decided** | AutoGoPay. Never Midtrans. |
| Money | **Decided** | Integer rupiah (`int64` in Go, `bigint` in Postgres). Never floats. |
| Timezone | **Decided** | Asia/Jakarta for every date rule (H-3, H0, payroll day, ...). |
| Penyewa auth | **Decided** | No accounts. Signed per-invoice token links. |
| Staff auth | **Decided** | OTP over WhatsApp (admin, karyawan, Supervisor). |
| Authorization | **Decided** | RBAC in the style of AWS IAM: `resource:verb` permissions, policy documents with Allow/Deny statements, default deny, explicit Deny wins. Details in "Authorization (RBAC)". |
| Multi-tenancy | **Decided** | `organization_id` on every business row, enforced in app layer AND Postgres RLS. |
| Provider interfaces | **Decided** | `MessagingProvider` (adapters: `gowa` at launch, `devlog` for development, a Cloud API adapter later), `BankTransferProvider`, and an AutoGoPay client, each with fakes. `DisbursementProvider` is deferred (see Disbursement). |
| Web / API / data / infra | **Decided** | Next.js 16 App Router PWA (React 19, TypeScript strict) + Tailwind 4 (one responsive app), TanStack Query, shadcn/ui, Go API and worker (one module, two binaries), PostgreSQL, Redis for the queue, Docker Compose on a VPS. |
| Data layer | **Decided** | sqlc over pgx v5 with goose SQL migrations, no ORM. ent was compared and rejected: money rules are easier to review and safer as plain SQL, and the team is new to Go. |
| Libraries and tooling | Tentative | chi, pgx v5, goose, asynq on Redis (replaces BullMQ), OpenAPI contract with oapi-codegen for Go and Orval for the web (TypeScript types, request functions and TanStack Query hooks through one axios mutator; replaces `packages/shared` and zod), S3-compatible storage (R2), Caddy, GitHub Actions with GHCR. Queue library is revisited after slice 3 (River, Postgres-backed, is the alternative). |
| WhatsApp account | **Decided** | ONE platform-owned number sends for every organization. No per-org WA numbers or tokens. At launch it is a WhatsApp Business app number linked to GOWA; later it can move to a WhatsApp Business Account on the Cloud API. |
| WhatsApp provider | **Decided** | Launch: GOWA (self-hosted `aldinokemal/go-whatsapp-web-multidevice`, pinned image), because the Cloud API needs a registered company (PT) and there is none yet. Later: WhatsApp Cloud API, integrated directly, once a PT exists. Both sit behind `MessagingProvider`; templates have names and ordered parameters and buttons carry both a payload and a link, so the switch is a new adapter, not a rewrite. |
| Disbursement | Deferred | No `DisbursementProvider` for now: payroll is paid by hand and recorded in the app, as in the design. Revisit (Flip for Business or Xendit) only if the owner wants the app to send money. Confirm by week 14 of the project plan. |
| Bank transfer | Open | Default: static open-amount VA per tenant. Second adapter: bank-mutation polling (e.g. Moota). Chosen per organization. |

## Stack and architecture

- **Repo**: `apps/web` (Next.js, pnpm), `services/api` (Go module: `cmd/api`, `cmd/worker`,
  `cmd/migrate`, `internal/<domain>` per build slice, `db/migrations` for goose, `db/queries`
  for sqlc), `api/openapi.yaml`, `infra/` (compose files, Caddyfile, backup scripts).
- **Contract**: `api/openapi.yaml` is the shared source of types. Go request and response types
  are generated from it into `internal/httpapi/apigen` (`make api-gen`, models only; handlers
  are hand-written chi handlers) and CI fails on stale output. In development the API serves
  Swagger UI for it at `/docs`; staging and production do not. TypeScript types will come from
  the same file. Update the spec first when an endpoint changes. The Go API validates all
  input, rejects unknown JSON fields, and answers errors as `{"error":{"code","message","field"}}`
  with Indonesian messages; clients branch on `code`. Zod in the browser is only for friendly
  form messages.
- **Lists** (decision 28): every list that can grow takes `page` (from 1) and `pageSize`
  (default 20, max 100) and answers `{items, page, pageSize, total}`, in a fixed order with the
  id breaking ties. Search (`q`), filters and `sort` run on the server, each from a whitelist
  (an unknown value is a 400); `total` counts only what the caller may see. Small bounded lists
  (charge types, collector candidates) stay plain arrays.
- **Routing**: Caddy on one domain. `/api/*` and `/webhooks/*` go to the Go API, everything
  else to Next.js, so there is no CORS. `Referrer-Policy: no-referrer` on `/t/*`.
- **Data access**: sqlc over pgx v5, no ORM. Every query runs inside one of two scopes, set
  per transaction with `set_config(..., true)`:
  - `WithOrgTx` sets `app.org_id`: one organization's data and its members' names.
  - `WithAuthTx` sets `app.scope = 'auth'` and, once known, `app.user_id`: staff users, OTP
    challenges and sessions, plus the signed-in user's own memberships. No organization data.
    Only the auth, member and org packages call it.
  RLS policies read the scope through `app_org_id()`, `app_user_id()` and `app_in_auth_scope()`,
  which return NULL when unset, so a missing scope matches no rows. The app role does not own
  the tables and has no BYPASSRLS. Migrations run as a separate owner role.
- **Queue**: asynq on Redis with `appendonly yes` and `maxmemory-policy noeviction`. The
  scheduler runs in Asia/Jakarta. A task ID only blocks duplicates while the first task is
  pending, so correctness comes from database unique keys and idempotent handlers. Reconcile
  jobs cover a lost enqueue.
- **Staff login**: a 6-digit code over WhatsApp, valid 5 minutes and 5 tries, stored as an HMAC
  keyed by `AUTH_SECRET`. A new code cancels the previous one. Per phone, Redis allows 3 code
  requests and 10 verify attempts per 15 minutes. A code request answers the same whether or
  not the number is registered.
- **Sessions** live in PostgreSQL (`staff_session`, SHA-256 of the token). The `cf_session`
  cookie lasts 30 days and is HttpOnly, SameSite=Lax and Secure outside development.
  State-changing requests from a browser must come from an origin in `ALLOWED_ORIGINS`.
- **Time**: embed tzdata, store UTC `timestamptz`, compute date rules in Asia/Jakarta with an
  injected clock.
- **Infra**: Docker Compose on a VPS. Only Caddy publishes ports. The worker runs the API
  image with a different command. Staging is a separate small VPS. Backups are a nightly dump
  plus WAL archiving to R2, with a monthly restore drill.
- **GOWA** is its own container on the internal network, never routed by Caddy. Its
  `storages` volume holds the linked WhatsApp session: back it up, because losing it means
  scanning the QR code again. Run it with `APP_BASIC_AUTH` set, `MCP_ENABLED=false`, and a pinned
  image tag; its dashboard (used to scan the QR code) is reached only through an SSH tunnel.
  Staging uses its own number, never production's.
- **Go style**: follow `.claude/rules/golang-rules.md` (see "Go conventions"). Project rule on
  top of it: no goroutines inside request handlers, use the queue.

## Go conventions

Rules live in `.claude/rules/golang-rules.md` and layout in the `golang` skill. This section
records only what is specific to this project. Where a default there conflicts with a decision
in this file, this file wins.

**Overrides of rule and skill defaults**
- **Data layer**: sqlc over pgx v5, no ORM. The skill's default of ent (and ogent for generated
  REST) is not used: we want SQL-first queries, RLS through `WithOrgTx`, and a hand-written
  OpenAPI contract as the source of truth.
- **sqlc guardrails**:
  - CI runs `sqlc diff`, so generated code can never be stale.
  - No `UPDATE` or `DELETE` queries on payment, allocation, correction or invoice line tables.
    A database trigger also rejects them, so a hand-written query cannot bypass the rule.
  - `depguard` forbids importing the pgx pool outside the store package, so every query goes
    through `WithOrgTx`.
  - An integration test connects as the app role, sets organization A and expects zero rows
    from organization B in every business table. Write it in slice 1.
- **Mocks**: provider interfaces (`MessagingProvider`, `BankTransferProvider`, the AutoGoPay
  client) get hand-written fakes plus contract tests, not `mockgen` mocks. testify is fine for
  assertions.

**Layout (the skill's checklist, applied)**
- The Go module lives in `services/api`. Its module path matches the repository URL, lowercase
  with hyphens, and is currently `github.com/farrelfauzan/cashflow-app/services/api` (change
  it with `go mod edit -module` if the repository lives elsewhere). No `pkg/`, since nothing
  is a public library. `main`
  packages in `cmd/` stay thin: load config, wire dependencies, call `Run()`.
- **Architecture**: modular monolith, one package per domain under `internal/`, thin handlers,
  domain logic independent of HTTP and the queue. Not full clean or hexagonal layers; the
  provider interfaces are the only ports. Add `depguard` import rules once the packages settle.
- **Dependency injection**: constructors take interfaces and are wired by hand in `main()`. No
  DI library and no `init()` wiring.
- Repo root has the `Makefile` (build, test, lint, run for both apps) and `.gitignore`.
  `.golangci.yml` lives in `services/api`.
- The architecture and DI choices are Tentative. The skill asks the developer to choose, so
  confirm both in slice 0.

**12-factor**: config from environment variables, validated at startup so a bad value stops
the process. Logs go to stdout as JSON. Processes are stateless (sessions live in PostgreSQL).
Shutdown is graceful. Migrations run as the one-off command `cmd/migrate`.

**Style points that matter here**
- Accept interfaces, return structs. Define interfaces where they are used, 1 to 3 methods.
- `context.Context` first on anything that does I/O. Wrap errors with `%w`, use sentinel errors
  for expected cases (`ErrNotFound`), never discard an error, never panic for flow control.
- Never send internal error text to a client or back to a webhook caller. Log the detail on the
  server and return a code with a generic message.

**Security**
- Set `ReadTimeout`, `WriteTimeout` and `IdleTimeout` on `http.Server`. Wrap every request body,
  webhooks included, in `http.MaxBytesReader`. HSTS and the other security headers are set in
  Caddy.
- Use `crypto/rand` for tenant link tokens, OTP codes and anything secret. Compare HMACs with
  `hmac.Equal` and other secrets with `crypto/subtle`.
- Parameterized queries only. Dynamic filters go through sqlc optional arguments, never string
  concatenation.
- Rate limiting uses Redis counters (OTP, WhatsApp sends per organization). `x/time/rate` is for
  in-process limits.
- CI runs `govulncheck ./...`, `go vet`, `golangci-lint` and `go test -race`. Dependency changes
  are reviewed in PRs.

**Observability**: `slog` JSON to stdout. Metrics at `/metrics` on an internal port that Caddy
does not route. OpenTelemetry tracing and a Prometheus scrape stack are added when needed, not
at launch. `pprof` in development only.

## UX rules

- All user-facing copy is **Bahasa Indonesia**. Code, identifiers and comments are English.
- Many users are 40+: body text ≥16px on iPad and ≥14px on phone, high contrast, touch
  targets ≥44px, **one primary action per screen**.
- Targets: iPad 1180×820 (Supervisor and admin), phone 390×844 (penyewa and karyawan).
- Palette: ink `#1E2A2B`, cream `#F5F3EE`, accent `#1D6B63`, warning `#B4530F`.
  Font: Plus Jakarta Sans.
- Rupiah format: `Rp1.250.000` (dot thousands separator, no decimals).

## Roles

- **Supervisor**: reads everything. Approves payroll and large expenses (iPad or WA button).
- **Admin**: manages tenants and units, checks manual payments, makes corrections, records payroll payments.
- **Karyawan**: visit list, records cash collected and visit outcomes.
- **Penyewa**: WhatsApp plus the payment page only. No account.
- **Pemegang porsi**: receives the monthly WA report only.

These are the built-in roles. "Authorization (RBAC)" below defines what each may do.

## Authorization (RBAC)

AWS IAM style. Authentication says who the member is (WhatsApp OTP). Authorization says what
they may do inside one organization. RLS separates organizations, RBAC separates what members
of the same organization can do. Both must pass.

- **Permissions** are `resource:verb` strings: `invoice:read`, `payment:record`,
  `correction:create`, `payroll:approve`, `expense:approve`, `settings:update`, `member:manage`.
  `*` is a wildcard (`invoice:*`, `*:read`).
- **Policies** are documents of statements: `Effect` (`Allow` or `Deny`), `Action` (permissions),
  optional `Resource` (scope such as `property/<id>`) and optional `Condition`.
- **Roles** are named sets of policies. A **membership** (a staff user inside one organization)
  holds one or more roles, optionally scoped to properties. The same phone can have different
  roles in different organizations.
- **Evaluation**: default deny. An explicit `Deny` beats any `Allow`. Otherwise any matching
  `Allow` wins. The evaluator is a pure function with table tests.
- **Conditions** use a short list of keys at launch: `"property": "assigned"` (the record's
  property is one the member is assigned to), `"self"` (the record is owned by or assigned to
  the member), `"actor"` (the member already acted on the record) and `"amountAtMost"` (rupiah).
  When the request lacks the data a condition needs, an Allow does not grant and a Deny still
  denies, so missing data always fails closed.
- **Guardrails** are Deny statements applied to every principal before any role, like an AWS
  SCP. No role can override them.
- **Built-in roles** are managed policies defined in code and versioned with releases:
  Supervisor, Admin, Karyawan. Custom roles per organization are stored as policy documents in
  the database. The data model supports them from day one, the editor screen is not designed yet.
- **Intent of the built-in roles** (initial, confirm against the screens):
  - Supervisor: read everything, plus `payroll:approve` and `expense:approve`. No other writes.
  - Admin: manage property, unit (its terms and prices included), tenant and lease (extra
    charges included); `charge:record`, `payment:record`, `payment:check`, `correction:create`,
    `payroll:check`, `payroll:record`, `expense:create`, members. Cannot approve. May grant the
    Admin and Karyawan roles (`role:assign` on `role/admin` and `role/karyawan`), never
    Supervisor. Supervisors are set when the organization is created.
  - Karyawan: `visit:*`, `cash:record` and `charge:record` (meter amounts) on assigned
    tenants, and read their own payslip and commission.
- **Separation of duties** (guardrail): whoever checked a payroll run or created an expense
  cannot approve it (`"actor": true` denies).
- **No self-granted access** (guardrail): nobody runs `member:manage` on their own membership.
- **Changing roles** needs `member:manage` on the member and `role:assign` on every role that is
  added or removed.
- **Not RBAC principals**: penyewa and pemegang porsi have no accounts. A penyewa link token
  grants only `invoice:read`, `payment:attempt` and `promise:create` on that one invoice or
  lease. Pemegang porsi only receive the WA report.
- **Enforcement**: the service layer calls `authz.Service.Authorize(ctx, tx, principal, request)`
  inside the transaction that makes the change, for every API handler and every queued job, not
  only in middleware or the UI. `Check` is the same without a transaction, for checks that must
  run before one opens. A WA button resolves the organization from its payload, then the member
  by phone inside that organization, then runs the same check. Unknown actions are denied.
- **Audit**: every denied check, and every allowed sensitive action (money, approvals,
  corrections, role changes), goes to `audit_log` with actor, action, resource and organization.
  Denials are written in their own transaction so they survive the caller's rollback; allowed
  actions are written in the caller's transaction so they commit only with the change.
  `audit_log` is append-only (privileges revoked and a trigger) and ordered by `seq`.
- **The catalog** in `internal/authz/catalog.go` lists every action. A policy that names an
  action outside it fails validation, so typos cannot silently grant nothing. Adding an action
  means updating the catalog and the role matrix test.
- Code lives in `internal/authz`. `GET /api/orgs/{orgId}/me` returns each action as allow,
  conditional or deny for the UI; the server still checks every request.

## Domain model (minimum)

organization, property, unit (with the terms it offers), unit_price (append-only price history
per unit and term), tenant, lease (with its term), charge_type, lease_charge, metered_charge,
invoice, invoice_line, payment, payment_allocation,
payment_promise (+ promise_version), qris_attempt, va_account, cash_deposit, correction,
employee, payroll_run, payslip, payroll_payment (manual record), share_holder, share_allocation (versioned
per asset), message_log, audit_log, settings, staff_user, membership, role, role_assignment
(a built-in role key or a custom role id), otp_challenge, staff_session.

## Invariants (never break these)

- **Tenant isolation**: no cross-tenant query anywhere, including jobs and webhooks.
  Payment webhooks resolve the organization from the account or key before touching data.
  WhatsApp is one shared number, so inbound WA messages resolve the org from the reply
  context (message id → `message_log`) or the button payload, never from the phone number
  alone: one phone can be a tenant or staff member in several orgs. If it's still
  ambiguous, ask the sender to choose.
  Per-org secrets (payment keys) are encrypted at rest.
- **Authorization**: default deny. Every handler and job checks permission through `authz`,
  and denials and sensitive actions are audited. See "Authorization (RBAC)".
- **FIFO allocation**: every payment pays the open invoice with the oldest period first. One
  payment can split across invoices via `payment_allocation`. Overpayment becomes credit and
  auto-applies to the next invoice.
- **No minimum partial payment, no interest, no late fees.**
- **Corrections are append-only** with a mandatory reason, and show up in the Supervisor report.
  Never UPDATE or DELETE money rows to fix them.
- **Promise edits create a new version**. Keep the full history.
- **Idempotency**: QRIS tasks keyed by `transaction_id` and backed by a database UNIQUE
  constraint. Payroll payment records keyed by payslip id.
- Refunds of double payments only with admin approval. The default is credit.
- **Unit prices are append-only**: a price change adds a row with its effective date; rows are
  never updated or deleted.
- **Invoice lines are append-only**, inserted with their invoice, and add up to the invoice
  amount. A wrong line is fixed with a correction on the invoice, never by editing the line.

## Billing and reminders

- A lease is billed every 1, 3, 6 or 12 months (its term, decision 32), or every N months for
  a unit's custom term (decision 33): besides the four standard terms a unit may offer one term
  of 2 to 60 months other than 3, 6 or 12, which the admin types, and it behaves exactly like the others. A unit lists
  the terms it offers, each with its own price typed by the owner, not months times the monthly
  rent; by default a ruko offers 12 months and a kos, kontrakan or other unit 1, and at least
  one term stays on.
  A lease picks one offered term when it is created and keeps it; another term is a new lease.
- Each lease bills on the anniversary of its own start date, in advance, one period per term: a
  monthly lease starting 15 Oct has periods 15 Oct to 14 Nov (due 15 Oct), 15 Nov to 14 Dec (due
  15 Nov), and so on; a 3-month one has 15 Oct to 14 Jan, then 15 Jan to 14 Apr. The first
  period is full, with no proration. A start on the 29th to 31st is due on the last day of
  shorter months, always counted from the start date (31 Jan, 28 Feb, 31 Mar).
- Prices are an append-only history per unit and term (amount, effective-from date, who, when),
  never edited or deleted. A period's rent is the price in force on the day its invoice is
  made, so a new price reaches a monthly tenant at the next month's invoice and a yearly tenant
  at the next yearly one; there is no price lock per lease. A price takes effect today or later
  (Asia/Jakarta) and after the day the unit's newest invoice was made; on the same effective date
  the latest recorded row wins.
- A lease starting within the lead time is invoiced as soon as it is saved (the daily run is the
  safety net); a lease booked further ahead is invoiced by the daily run, like any other. The
  start date can change only until the lease's first invoice exists. An invoice belongs to the
  month of its period, which for a regular invoice is the month of its due date (dashboard
  filters, "Tagihan November").
- An invoice not fully paid by its due date is debt (tunggakan) from H+1. Debt stacks: what a
  partial payment leaves stays open, FIFO pays the oldest period first, and the tenant always
  sees one total to pay, while each invoice stays its own row.
- A lease entered late (its start date is before the day it is saved, as in the tenant import)
  is invoiced for every period since its start date, by default; its past periods are due on
  the day it is entered and its current period 7 days after. Started 15 Sep and entered 28 Oct
  gives 15 Sep to 14 Oct, due 28 Oct, and 15 Oct to 14 Nov, due 4 Nov, then 15 Nov as usual. The
  admin can skip periods the owner collected by hand with "Mulai tagih dari" (a period start of
  the lease's term; also an import column). A wrong invoice is fixed by a correction, never
  deleted.
- When a tenant says they will not continue, the admin sets the lease's planned end date
  ("Rencana keluar", the tenant's last day). No invoice is generated for a period that starts
  after it, and one already generated is cancelled by a correction with the reason "sewa tidak
  diperpanjang"; its reminders stop, while older debt and its reminders stay. For a longer term
  the rule applies per period of the term. Money already paid on a cancelled invoice goes back
  through FIFO and the rest becomes credit. Setting the date
  checks `correction:create` as well as `lease:manage` when it cancels or restores an invoice.
  A move-out cancels, the same way, any invoice for a period that starts after its end date,
  before the arrears rule applies. A period the tenant started is billed in full, with no
  proration and no refund unless an admin approves one.
- Invoices per period. Each invoice has lines: the rent plus the lease's extra charges (listrik,
  air, Wi-Fi, parkir, kebersihan): fixed monthly amounts set on the lease, billed per period as
  the amount times the term's months, and metered amounts recorded per month and billed once,
  on the next invoice generated after they are recorded. Lines are snapshotted when the invoice
  is generated and never recomputed; the invoice total is the one amount FIFO allocation pays.
  A missing meter amount never delays the invoice.
- A metered amount can be edited until an invoice line bills it; then it is locked (the service
  refuses and a trigger rejects the update), and fixes are corrections on the invoice. Metered
  usage recorded after the last regular invoice goes on one final invoice at move-out (at most
  one per lease), which the move-out-with-arrears rule treats like any other open invoice.
- WA: H-3 (invoice + pay link), H0, H+3. At H+7 the tenant joins the karyawan visit list.
  Reminders stop the moment the invoice is fully paid.
- Rollover message: "Tagihan Desember Rp X + sisa November Rp Y = Rp Z" with one pay link.
  Messages show an invoice's line breakdown as one template parameter.
- `payment_promise` belongs to the tenant or lease, not to one invoice. Fields:
  promised_date, promised_amount (null = all outstanding at that time),
  kind = `specific_date | with_next_invoice`, source = `tenant_page | karyawan | admin`, note.
  The tenant sets it freely with no approval (admin and karyawan are notified).
  Status: `active → kept | partially_kept | broken`.
- While a promise is active: suppress H+3/H+7 for the covered arrears and send one
  reminder at H-1 of promised_date. Broken promise: send a link to make a new promise,
  no penalty. After N broken promises (org setting, default 2) the tenant joins the visit list.
- Dashboard and the Supervisor's weekly WA report: total arrears per tenant and the age
  of the oldest unpaid invoice.
- Owner reports show gross income and clean income. Gross income is all money collected (rent
  plus extra charges). Clean income is the rent (the rent lines of the invoices) minus the costs
  the owner records: maintenance, payroll and other expenses. Extra charges never count toward
  clean income. It is shown per property and unit where a cost belongs to one, and both figures
  drill down to the underlying rows (invoice lines, expenses, payroll payments). Extra charges
  are not a separate report category. Open until the report design (slices 9 and 10): whether
  rent counts as billed or collected (it matters more for rent paid months or years ahead), and
  how shared costs are attributed to units.

## Payments

### QRIS (AutoGoPay)
Docs: https://autogopay.site/docs/autogopay-api.md. Base `https://v1-gateway.autogopay.site`,
`Authorization: Bearer <API key>`. Endpoints: `POST /qris/generate {amount}` (Rp1 to
Rp10.000.000, valid 15 minutes), `/qris/status`, `/qris/cancel`, `/transactions`.
Webhook `transaction.received`, header `X-Signature` = HMAC-SHA256(raw body, API key), so
the API needs the raw body. Statuses: pending, settlement, expire, cancel.

The WA message links to OUR page. The QR is created only when the tenant opens it
(the page is Next.js, the `/api` and `/webhooks` routes are the Go API, same domain):
- `GET /t/:token` payment page (summary; pay full / partial / make a promise)
- `POST /api/t/:token/qris {amount}` creates a qris_attempt and cancels any previous live one
- `GET /api/t/:token/qris.png` server-rendered PNG with amount, unit and expiry; 410 if expired or paid
- `GET /api/t/:token/status` polled by the page
- `POST /webhooks/autogopay` verifies the signature, then enqueues
- Task `qris-paid` (asynq TaskID = transaction_id; the payment insert also has a UNIQUE key).
  Task `qris-reconcile` every 30 min + nightly checks pending attempts against `/qris/status`.
- Invoices above Rp10 juta must use bank transfer.

### Bank transfer
`BankTransferProvider`. Default: static open-amount VA per tenant (auto-matched, so
partials need no manual check). Stub adapter: bank-mutation polling, where unmatched
transfers go to the admin "Antrean cek" queue.

## Negative cases (each must be handled AND tested)

- **A**: partial payment, promise by date, rollover to next month, broken promise, overpayment → credit.
- **B**: QR expired → "Buat QR baru", invoice unchanged. Paid but status stuck → "sedang dicek",
  reconcile resolves it. Double payment → credit. Transfer without code or wrong amount →
  admin queue. Karyawan cash short → tenant gets a WA receipt the moment cash is recorded,
  so the gap shows in the admin queue. AutoGoPay down → fall back to bank transfer.
- **C**: WA undeliverable → dashboard flag + karyawan visit task to get a new number.
  Forwarded pay link → per-invoice link shows name + unit. Disputed amount → correction
  with reason. Move-out with arrears → org setting (deduct deposit first, or keep billing).

## Payroll

Day X (setting): payslip = base + collection commission + overtime/bonus − kasbon.
Commission on full payment or per instalment (setting), counted on rent only: extra charges
earn no commission, and a partly paid invoice counts its rent share in proportion to its lines.
Flow: admin checks → Supervisor approves (iPad or WA button) → admin pays by hand (cash or own
bank transfer) and records each payment (date, amount, optional proof photo; any difference from
the payslip stays visible) → WA payslip to the employee. The app never sends money. Recording is idempotent
per payslip id.

Deferred: automatic transfer through a `DisbursementProvider` (prepaid deposit balance check,
idempotency key = payslip id, bank account name inquiry, per-transfer webhook, "Kirim ulang
yang gagal" retrying only the failed ones). Not built unless the owner asks.

## Messaging

`MessagingProvider` interface (`internal/messaging`). Callers send a template name, its
parameters and optional buttons; the adapter turns that into what its provider accepts.
Every message goes to `message_log` with its status and `organization_id`.

**Adapters**
- `gowa` (launch): a self-hosted GOWA server drives the platform number, linked like WhatsApp
  Web. The API calls `POST /send/message` with basic auth (`GOWA_URL`, `GOWA_USERNAME`,
  `GOWA_PASSWORD`, optional `GOWA_DEVICE_ID`). GOWA's error codes map to `ErrRateLimited`,
  `ErrUndeliverable` and `ErrNotConnected`; the last one means an operator must re-link the
  phone, so it is logged as an error and shown to users as a 503.
- `devlog` (development only): renders the message and writes it to the log.
- Cloud API (later, once a PT exists): templates registered with Meta under the same names,
  parameters in the same order, native reply buttons using the payload.

**Templates** live in `internal/messaging/templates.go`: a name, ordered parameters and the
Bahasa Indonesia text. `messaging.Render` fills them and fails on an unknown template, a
missing or extra parameter, or a button without a link, so mistakes never reach a chat.
Every template names the organization or property in its body (tenants see the platform's
display name, not their landlord's). The login code is the one platform message and names
none.

**Buttons** carry an opaque `Payload` (maps back to org + invoice, lease or payroll run) and a
`URL` to our page that does the same thing. GOWA has no native buttons, so it sends the links:
"Setujui" (Supervisor), "Bayar" / "Ubah janji" (tenant) open our pages.

**Inbound** (slice 5): GOWA posts events to `/webhooks/gowa`, signed with HMAC-SHA256 of the
raw body in `X-Hub-Signature-256` (`WHATSAPP_WEBHOOK_SECRET`). Verify before parsing. Resolve
the org from the reply context (quoted message id → `message_log`) or the code in the link,
never from the phone number alone.

**One shared, unofficial number.** GOWA uses the WhatsApp Web protocol, which WhatsApp does not
support for businesses, so the number can be restricted or banned, and that would stop every
organization's messages at once:
- Pace sends per number through the queue, spread reminders over the day, and dedupe per
  recipient. Rate-limit and dedupe per org too, so one org's volume cannot burn the number.
- Message only tenants and staff who expect it, personalize every message, and stop when
  someone asks.
- Keep a standby number linked to GOWA as a second device for failover. Switching changes the
  sender tenants see, so announce it.
- Watch for `ErrNotConnected` and `ErrRateLimited` and alert an operator.

## Per-organization settings

Display name used in WA templates (org or property name); commission amount and when it's paid; payroll dates; primary + backup
payment checker; broken-promise threshold; move-out-with-arrears rule; payment rules
(minimum amount off, fees off, tenant sets promise date on) as editable defaults.

## Build order (vertical slices, each with tests)

1. orgs, auth + RBAC (policy engine, built-in roles), properties/units/tenants/leases
2. invoices + manual payments + FIFO allocation + corrections + dashboard
3. payment page + AutoGoPay QRIS + webhook + reconcile
4. partial payments + promises + reminder scheduler
5. WhatsApp messaging (GOWA: message log, inbound webhook, send pacing, standby number)
6. admin check queue + karyawan app
7. payroll (manual payment recording)
8. settings/onboarding
9. distribution (monthly pemegang porsi report; shares per asset and versioned; only
   executes shares already agreed in writing)

## Testing rules

- Never call real payment or WA APIs in tests. Use adapters with fakes and contract tests. The
  GOWA adapter is tested against an `httptest` server that checks the request GOWA expects and
  replays its real responses and error codes.
- Unit tests use fakes and run on every push. Integration tests carry `//go:build integration`,
  use testcontainers for PostgreSQL and Redis and an injected clock set to Asia/Jakarta, and run
  in CI. Tenant isolation, webhook idempotency, reconcile and cash-book tests are integration tests.
- Table-driven tests with named cases, `t.Parallel()` where cases are independent, `t.Helper()`
  and `t.Cleanup()` in helpers, `package foo_test` for black-box tests of exported APIs, and
  fixtures in `testdata/`.
- CI sets a minimum coverage threshold, aimed at business logic and not at generated code
  (sqlc, oapi-codegen).
- Playwright covers the web app at 1180×820 and 390×844.
- Seed realistic Indonesian sample data (names, addresses, kos/kontrakan/ruko, rupiah amounts).
- Must-have tests: FIFO across invoices, overpayment credit, webhook signature + idempotency,
  reconcile job, promise lifecycle + reminder suppression, tenant isolation,
  payslip math and approval before a payment is recorded, and the authorization matrix
  (default deny, every built-in role against every permission, separation of duties).
