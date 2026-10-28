# Web app (`apps/web`)

The Next.js app for staff (iPad and phone) and, later, the tenant payment pages. It talks only
to the Go API in `services/api`. Rules: `CLAUDE.md` and `.claude/rules/nextjs-rules.md`.

Stack: Next.js 16 (App Router, React 19), TypeScript strict, Tailwind 4, shadcn/ui (new-york),
Vitest, Playwright. pnpm only: use `pnpm`, `pnpm exec` and `pnpm dlx`, never npm, npx, yarn or
bun. The pnpm version is pinned in `package.json` (`packageManager`), Node in `.nvmrc`.

## Run it

From the repository root:

```sh
make web-install   # pnpm install --frozen-lockfile
make web-dev       # http://localhost:3000
make web-check     # lint, type check, unit tests, build: what CI runs
make web-e2e       # Playwright at iPad (1180x820) and phone (390x844) sizes
```

Or inside `apps/web`: `pnpm dev`, `pnpm lint`, `pnpm format`, `pnpm typecheck`, `pnpm test`,
`pnpm build`, `pnpm e2e`.

Playwright needs Chromium once per machine: `pnpm exec playwright install chromium`. The e2e
run needs the Go API; see "End-to-end tests" below.

## End-to-end tests

Playwright runs against the real Go API (with its fakes; nothing is sent to WhatsApp), at iPad
(1180x820) and phone (390x844) sizes. A `setup` project signs a seed-demo staff member in once
per run: it requests a login code through the app's `/api` rewrite, reads the code from the
API's `DEVLOG_FILE`, verifies it, and saves the `cf_session` cookie to `e2e/.auth/`
(git-ignored), which the specs reuse. A saved session the API still accepts is reused on the
next run, because the API allows only 3 code requests per phone per 15 minutes.

Records a test leaves behind (invited members) use a run id set in `playwright.config.ts`, so
the specs can run again on the same database. The members specs send six WhatsApp invitations
per run, and the API allows 20 per organization per hour: after three runs in an hour, wait, or
start from a fresh database (`make dev-reset`, then migrate and seed again).

1. Start PostgreSQL and Redis, migrate, and seed (from the repository root): `make dev-up`,
   `make migrate`, then `make admin ARGS=seed-demo` (needs `APP_ENV=development` in
   `services/api/.env`, as in the example).
2. Add `DEVLOG_FILE=/tmp/cashflow-devlog.jsonl` to `services/api/.env` (with
   `WHATSAPP_PROVIDER=devlog`) and run `make api-run`. `ALLOWED_ORIGINS` must include the
   app's origin, `http://localhost:3000` by default.
3. From `apps/web`: `DEVLOG_FILE=/tmp/cashflow-devlog.jsonl pnpm e2e`.

The run builds the app with the `/api` rewrite and starts it. Settings, all optional except
`DEVLOG_FILE`:

- `E2E_PORT`: the app's port (default 3000). Add its origin to the API's `ALLOWED_ORIGINS`.
- `API_DEV_ORIGIN`: the API (default `http://localhost:8080`); `API_INTERNAL_URL` defaults to
  it. Prefer `http://127.0.0.1:<port>` when the API listens on IPv4 only.
- `E2E_REUSE_SERVER=1`: test a server already running on `E2E_PORT` instead of failing. It
  must proxy `/api` to the API (`pnpm dev` does; a production build only when it was built
  with `API_DEV_ORIGIN`).

A login failure names the cause (`bad_origin`, `rate_limited`, no code in the file). If the
rate limit is hit, wait 15 minutes or flush the dev Redis. Reports land in
`playwright-report/`, screenshots in `test-results/`. CI runs the same in the `e2e` job of
`.github/workflows/web.yml`.

## Where things go

- `src/app/`: routes. `page.tsx` and `layout.tsx` only compose.
- `src/components/server/<feature>/`: server components.
- `src/components/client/<feature>/`: interactive components (`'use client'`).
- `src/components/ui/`: shadcn primitives, added with `pnpm dlx shadcn@<version> add <name>`,
  then `pnpm format`. Buttons keep a 44px minimum height at every size.
- `src/lib/format/`: `formatRupiah`, `parseRupiahInput`, `formatDate`, `formatDateTime`.
  Unit tests run in `America/New_York` on purpose, so Jakarta time is never assumed.
- `e2e/`: Playwright specs.

Colors, radius and the font are defined once in `src/app/globals.css`; components use the
theme tokens (`bg-background`, `text-muted-foreground`, `bg-warning`, ...), never raw colors.
