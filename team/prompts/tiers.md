
# Task tiers and sub-agents (orchestrator, backend, frontend)

The backend and frontend developers each have four sub-agents, named `<role>-<tier>`, for example
`backend-hard` or `frontend-light`. The developer spawns them with the Agent tool and
`subagent_type`. Their model and thinking level are fixed in their definitions; do not override
them per call.

| Tier | Model · thinking | Use it when |
|---|---|---|
| `hard` | Opus 5.5 · high | The approach is not obvious, or a mistake would be costly or hard to see in review. |
| `medium-critical` | Opus 5.5 · medium | The pattern exists, but the blast radius is high. |
| `medium` | Sonnet 5.5 · high | A standard piece of work that follows an existing example. |
| `light` | Sonnet 5.5 · medium (Haiku if configured) | Small, mechanical, or low stakes. |

## Choosing the tier

Choose by **difficulty** first, then raise it by **how much a mistake would cost**. When in doubt
between two tiers, take the higher one for anything on the critical list, and the lower one for
everything else.

**Critical** means it touches money (allocation, payments, payroll, corrections), authorization
(`authz`, roles, the catalog), tenant isolation (RLS, `organization_id`, scopes), a migration, a
webhook or signature check, sessions and login, tenant payment pages (`/t/[token]`), or the
`api/openapi.yaml` contract.

Backend examples:
- `hard`: FIFO allocation and overpayment credit, payroll math, the QRIS webhook and reconcile
  flow, a new authz condition, locking and concurrency, a design that spans several packages.
- `medium-critical`: a new endpoint or table that copies the member slice's pattern but touches
  money or authz; a migration with RLS and append-only rules from the template; an RBAC change in
  the role matrix.
- `medium`: a standard query, service and handler for a non-money entity (property, unit, tenant)
  following the member example; table tests for defined behavior.
- `light`: docs and phase-doc updates, error message wording, renames, lint fixes, a test for
  behavior that already exists, a README step.

Frontend examples:
- `hard`: the scaffold and its tooling, the Orval mutator and query-key rules, the session and
  `proxy.ts` gate, the tenant payment page flow, anything with tricky state or accessibility.
- `medium-critical`: a screen with money mutations, a screen driven by `/orgs/{orgId}/me`
  capabilities, anything that reads or forwards the session cookie or the tenant token.
- `medium`: a screen from an existing board using generated hooks and existing primitives; a
  Playwright spec at both sizes.
- `light`: Bahasa Indonesia copy, spacing and styling fixes, a formatter test, renames, docs.

## Who decides

- The orchestrator suggests a tier in each task brief (`Tier:`) with one line of reasoning. It
  reads the docs and sees the whole backlog, but not the code.
- The developer owns the final choice. Raise the tier when the code turns out harder or more
  critical than the brief thought, and say why in your report. Lowering it is fine when the
  reason is clear.
- A task can have parts at different tiers. Split it: the design and the critical core at the
  higher tier, the standard parts and the tests at a lower one.

## How the developer uses a sub-agent

You stay accountable for everything that goes into a PR.

1. Give the sub-agent a brief that stands alone: the goal, the files and docs to read (task
   brief, phase doc section, the worked example to copy), what is out of scope, the acceptance
   check (the commands that must pass), and the invariants that apply. It has no memory of this
   conversation.
2. Run sub-agents in parallel only when they touch different files. Two sub-agents editing the
   same file will overwrite each other.
3. **Read the diff of everything a sub-agent returns** before you build on it or open a PR:
   against the task, the rules and the invariants, and run the checks yourself (`make api-check`,
   or lint, type check and Playwright). For `medium-critical` and `hard` work, read it as
   if you were the reviewer. Never pass a sub-agent's "done" on without checking.
4. A sub-agent that reports a problem or a question has hit the edge of its brief. Do not just
   retry it at the same tier; settle the question, or move the work up a tier.
5. Sub-agents write code and tests in your working tree only. They do not open PRs, use `gh`,
   push, or message the team. Opening PRs, replying to reviews and every `team-` message stay
   with you.
