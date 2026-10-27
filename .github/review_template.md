<!--
Review summary template. Copy it into the review's summary comment.
Guide, checklist and example: docs/plan/reviewing-pull-requests.md

Inline comments start with a label: blocking: / should: / question: / nit:
Each says the problem, a case where it goes wrong, and the fix (suggestion block of 5 lines at most).

Verdict → GitHub event:
- Approve: nothing blocking, nothing unanswered → APPROVE
- Approve with comments: only should: and nit: left → APPROVE
- Questions first: a question decides whether something blocks → COMMENT
- Request changes: at least one blocking: → REQUEST_CHANGES

Delete any empty section, and these comments.
-->

## Verdict: <Approve | Approve with comments | Questions first | Request changes>

<One or two sentences: what is good and what stands in the way of merging.>

### Tasks

<!-- The tasks the PR claims, each checked against the phase doc. -->

- ✅ **1. <task>**: matches the plan
- ⚠️ **2. <task>**: <what differs from the plan, or what is missing> (blocking 1)

### Rules

<!-- Only the rules this PR touches, including ones the author did not list. -->

- ✅ **<rule>**: <how you checked it, a few words>
- ❌ **<rule>**: <what breaks it> (blocking 2)

### Blocking

1. `<path:line>`: <problem>. <Case where it goes wrong.> <Fix.>

### Should fix

1. `<path:line>`: <problem and fix, one line>. <"Here" or "follow-up is fine".>

### Questions

1. `<path:line>`: <question>

### Missing tests

- <behavior or negative case with no test that proves it>

<!-- Nits stay inline. Mention only how many: "3 nits inline, none blocking." -->
