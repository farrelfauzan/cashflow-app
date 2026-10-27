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
run builds the app and starts it on port 3000, or reuses a server already running there.
Reports land in `playwright-report/`, screenshots in `test-results/`.

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
