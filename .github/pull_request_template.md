<!--
Title: [<phase><slice>] <what works now>, e.g. "[B2] FIFO allocation and overpayment credit".
Work outside a slice: [fix], [infra], [docs], [chore].

Keep it to about one screen (guide and example: docs/plan/pull-requests.md):
- Outcomes, not code. No code blocks, no test output, no file-by-file list.
- One to three bullets per task. If a task needs more, the PR is probably too big.
- Delete any section or line that does not apply, including these comments.
-->

## Summary

**Phase · slice:** <B · slice 2>, tasks <1–2> of <7> ([plan](<link to the slice in the phase doc>))
**Why:** <one or two sentences: what this unblocks or which problem it removes>
**Depends on:** <#PR, or "nothing">  ·  **Closes:** <issue, or delete>

## Tasks in this PR

<!-- Use the task list from the phase doc. Tick what this PR finishes; leave the rest unticked
so the reviewer sees where the slice stands. -->

- [x] **1. <task name>**
  - <what now exists or works, in one line>
  - <a non-obvious choice, and why, in one line>
- [x] **2. <task name>**
  - <...>
- [ ] 3. <task name> (next PR)

## Start review here

1. `<path>`: <what to look for here>
2. `<path>`: <...>

Skip: generated code (`dbgen/`, `apigen/`), wiring in `cmd/`.

## Rules touched

<!-- Keep only the lines this PR touches, each with a few words on how it holds. -->

- [ ] **Tenant isolation:** new tables have `organization_id` and forced RLS
- [ ] **Money is append-only:** no UPDATE or DELETE, trigger and revoked privileges in place
- [ ] **Authorization:** checks inside the transaction; new actions in the catalog and role matrix
- [ ] **Idempotency:** guarded by a UNIQUE key, reruns are harmless
- [ ] **Dates:** computed in Asia/Jakarta with the injected clock
- [ ] **API contract:** `api/openapi.yaml` changed, types regenerated
- [ ] **Migration:** <adds / changes tables; any backfill or lock risk>
- [ ] **User-facing copy:** Bahasa Indonesia, design board followed
- [ ] **Decision:** recorded in `docs/plan/decisions.md`

## What the tests prove

<!-- One line per behavior, in plain words. The test file in brackets. No output, no counts. -->

- <behavior that is now guaranteed> (`<test file>`)
- <negative case that is handled> (`<test file>`)

## Try it

<!-- Only when there is something to see. At most three steps. Screens: one screenshot at
1180×820 and one at 390×844. -->

1. <step>

## Not in this PR

- <what is left for later, and where it is tracked>

## Risks

- <what could go wrong in production, and what limits it>
