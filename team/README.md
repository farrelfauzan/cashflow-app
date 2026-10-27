# The team

Four Claude Code agents in one tmux session, for building the cashflow app.

| Agent | Job |
|---|---|
| **orchestrator** (Sonnet 5.5) | Checks your requests against `CLAUDE.md` and `docs/`, delegates, keeps the status, summarizes for you. Talk to this one. |
| **backend** | Go API in `services/api`, owns `api/openapi.yaml`. Opens PRs. |
| **frontend** | Next.js in `apps/web`, follows the design and plan artifacts, generates the API client with Orval. Opens PRs. |
| **reviewer** | Reviews every PR on GitHub as `farrelfauzan`, then your turn. |

## Run it

```sh
team/start.sh            # start and attach (re-attaches if already running)
team/start.sh --detach   # start in the background
team/stop.sh             # stop; messages, status and digests are kept
```

Window `team`: orchestrator on the left, backend / frontend / reviewer stacked on the right (the
mouse works). Window `clock` is the hourly timer (`Ctrl-b n` to switch). Detach with `Ctrl-b d`.

Give work to the orchestrator in its pane. It will validate, ask you if something conflicts with
the docs, and delegate.

## Sub-agent tiers

Backend and frontend each have four sub-agents, defined at launch from `config.env`
(`team/bin/team-agents-json` builds them). Effort is Claude Code's thinking level.

| Tier | Model · effort | For |
|---|---|---|
| `<role>-hard` | Opus 5.5 · high | Unclear approach, or a costly mistake: money correctness, concurrency, authz design, scaffold, session and tenant-page flows |
| `<role>-medium-critical` | Opus 5.5 · medium | Existing pattern, high blast radius: money, authz, RLS, migrations, the API contract |
| `<role>-medium` | Sonnet 5.5 · high | Standard slice or screen that follows an example |
| `<role>-light` | Sonnet 5.5 · medium | Docs, copy, renames, lint fixes, tests for existing behavior |

The orchestrator suggests a tier in every task brief; the developer decides, can split a task
across tiers, and reviews every sub-agent's diff before it goes in a PR. Sub-agents cannot spawn
more sub-agents, open PRs, or message the team. The full rules, with examples, are in
`prompts/tiers.md`. For Haiku on the light tier: `MODEL_TIER_LIGHT=claude-haiku-5-5 team/start.sh`
(Haiku takes no effort setting). Backend and frontend defaults to Opus 5.5 themselves, as in
`config.env`.

## How the agents talk

Every agent has `team-tell`, `team-report`, `team-ask` and `team-inbox` on its `PATH`.

- **Any agent can message any other directly.** The reviewer tells the author what to fix, the
  frontend tells the backend about an API gap, authors tell the reviewer a PR is ready. A message
  is a file in `state/inbox/<agent>/`, plus a one-line nudge typed into that agent's pane.
- **The orchestrator hears milestones, not conversations.** Agents send it one-line reports (PR
  opened, verdict, blocked, done) and `team-ask` for anything that needs you. It keeps
  `state/STATUS.md`, tells you immediately when something needs you or a PR is ready for your
  review, and writes a digest every hour (`state/digests/`). Peer chatter is indexed in
  `state/messages.log` and never forwarded to you.

## The PR flow

1. Developer opens a PR (template in `.github/pull_request_template.md`) and tells the reviewer.
2. Reviewer posts one review (template in `.github/review_template.md`) and tells the author.
3. Author fixes, tells the reviewer, reviewer re-checks the changes.
4. Reviewer approves, the orchestrator tells you it is ready, you review and merge. Agents never
   merge and never push to `main`.

Until the repository exists on GitHub there are no PRs; the orchestrator will tell you when the
first reviewable work is ready so you can initialize it.

## Things to know

- **Same GitHub account everywhere.** `gh` is signed in as `farrelfauzan`, so developers and the
  reviewer are the same user, and GitHub refuses to approve or request changes on your own PR.
  The reviewer falls back to a `COMMENT` review with the verdict as its first line, and asks you
  once. A separate GitHub identity for the developers fixes this.
- **Approval steps are waived for PRs and reviews.** Your global rules for `github-pr-create` and
  `github-pr-review` say to get approval for each one; the team's flow assumes you do not want
  that. The prompts say so (`prompts/shared.md`, "Git and GitHub"). Delete that paragraph to
  bring the approval step back.
- **Enable branch protection on `main`** (require a PR and a review) once the repo exists. The
  deny rules in `settings/` block `gh pr merge` and force pushes, but they are a guardrail, not a
  lock.
- **Typing in the orchestrator pane.** Nudges are typed into the input box. If you are halfway
  through a sentence when one arrives, it lands in your text. Finish or clear your line first
  when you see activity; nudges are held back while a permission dialog is open.
- **Permissions.** Agents run in `acceptEdits` mode with the shell commands in
  `settings/<agent>.json` allowed, so they can work while you are away. A command outside the list
  waits for you in that pane. Change the lists or `PERMISSION_MODE` in `config.env` as you like.
- **Frontend scaffold.** The web app is not scaffolded and `CLAUDE.md` forbids it until agreed, so
  the frontend's first job is a scaffold proposal that the orchestrator brings to you.
- **Orval replaces openapi-typescript** in the frontend's instructions, as you asked. The first
  frontend PR updates `nextjs-rules.md`, `CLAUDE.md` and `docs/plan/decisions.md` to match.

## Files

```
team/
  start.sh stop.sh config.env     run it, models, intervals, GitHub account
  bin/                            team-tell team-inbox team-report team-ask team-clock team-launch
                                  team-agents-json
  prompts/                        shared.md, tiers.md + one file per agent (the roles);
                                  sub/ holds the sub-agents' prompts
  settings/                       per-agent shell permissions
  state/                          (ignored) inboxes, STATUS.md, tasks, digests, reviews, worktrees
```

Change an agent's behavior by editing its prompt in `prompts/` and restarting it
(`team/stop.sh && team/start.sh`).
