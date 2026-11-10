# Phase 2 autonomy validation

Status on 2026-11-11. All four task reviews are complete. The one whole-branch
fix wave at `f35224e` closed the original uncertain-send and shutdown logging
findings and passed local full tests, race, vet and offline Compose acceptance.
Its scoped re-review reproduced an Important exit-status regression when stdout
has no reader and found a Minor subprocess assertion gap. The draft remains
open for correction. The PR description records CI for its current head.
Issue [#31](https://github.com/suwa68/signal-watch/issues/31), draft
PR [#32](https://github.com/suwa68/signal-watch/pull/32), branch
`feat/phase2-monitoring-mvp`. Product progress and exact commands are in the
[handoff](handoffs/phase2-monitoring-mvp.md).

## Dynamic Orchestration

Parent selected sequential implementation under the applicable Superpowers rule
against parallel implementation subagents. Each task had a separate independent
spec/quality review; fixes resumed the original author and received scoped
independent re-review. No fixed agent count or worker-created reviewer was used.

A read-only Docker/TLS/volume acceptance investigation overlapped durable-state
implementation because it changed no shared files and did not depend on unfinished
state code. Parent handled briefs, contract reconciliation, review dispositions,
GitHub coordination and handoff documentation while authors owned task files.
Models were chosen by task risk: transaction/driver and Docker integration used
the most capable available tier; bounded scheduler/daemon integration used the
standard tier with independent concurrency/lifecycle review.

Observed coordination rework: Task 2 accidentally tracked an ignored scratch
report; a tracking-only follow-up removed it without rewriting history or rerunning
product tests. Usage-limit interruptions were resumed from existing code/logs and
commits; completed Tasks 1/2 were not repeated. These are costs to report, not
evidence of a quantified efficiency gain.

## Non-blocking CI

Actions were started through pushes and read with bounded status snapshots.
Parent used no `gh pr checks --watch` or `gh run watch` commands.

| Observed pending run | Head | Independent work continued |
| --- | --- | --- |
| [42413107894](https://github.com/suwa68/signal-watch/actions/runs/42413107894) | c13bfab | State driver/transaction implementation; acceptance feasibility and execution evidence |
| [42462380592](https://github.com/suwa68/signal-watch/actions/runs/42462380592) | 2840fe5 | Fresh-context daemon implementation; Parent updated the PR with reviewed checkpoint/remaining work |
| [42571122272](https://github.com/suwa68/signal-watch/actions/runs/42571122272) | 8a2fce4 | Parent assembled the tracked review and autonomy handoff while the acceptance fix ran |
| [42571336462](https://github.com/suwa68/signal-watch/actions/runs/42571336462) | b9f0c19 | Fresh whole-branch review compared implemented behavior with the approved evidence tiers |

Every push invalidates earlier-head completion evidence. Some earlier CI runs
passed while independent review still found cancellation defects; CI success was
not substituted for review. Final expected checks must be read against the current
PR head, including the Monitoring acceptance job. Run
[42502790679](https://github.com/suwa68/signal-watch/actions/runs/42502790679)
passed both jobs for `6bce42f` while acceptance-review corrections remained open.
The two later runs in the table passed both jobs as well. Review findings still
govern readiness. The workflow does not configure branch protection.

## Cross-session Continuity

Parent committed reviewed state/scheduler handoff `2840fe5` before dispatching a
Task 3 executor with no inherited conversation. The executor verified clean branch/
base, inferred the exact next task, invariants and validation from tracked artifacts
and actual code, and reported no consequential missing context. It completed real
daemon implementation `12bf8d0`, tests and documentation without reimplementing
accepted state/scheduler tasks. Independent review correction `b7515ca` was then
accepted before packaging.

This establishes an observed **fresh-context execution rehearsal**. It is not an
observed genuinely new Codex session. Same-chat continuation after a usage-limit
interruption also does not prove that stronger claim. Essential decisions, task/
review commits, tests, unresolved work and next steps live in the tracked handoff;
ignored skill workspace files are supplemental rather than the recovery source.

## Autonomous Architecture Review

The [review record](reviews/2026-11-08-phase2-architecture-review.md) preserves the
code-grounded plan review, Parent corrections and material re-review. Implementation
reviews inspected real diffs/contracts and test evidence rather than accepting
author summaries or green CI alone.

| Concrete failure found before downstream acceptance | Correction and independent disposition |
| --- | --- |
| database/sql automatic rollback could race native Close, or ordinary cancellation could become fatal | Single-owner transaction lifetime and cleanup-only SAVEPOINT; actual driver interruption/BUSY/process tests; fix202688a independently approved |
| Fatal Pipeline result canceled only scheduler children, leaving command shutdown unbounded without a signal | Command-owned Runner cancels command with original fatal cause before reporting/join; real Pipeline/runtime barrier regressions; fixb7515ca independently approved |
| Self-review: blocked operational output also blocked the final timeout diagnostic | Bounded best-effort final diagnostic; blocked-writer RED/GREEN; independently assessed with daemon lifecycle |
| Exact-state inspector inferred initialization from seen rows; shutdown assertion could accept a timeout before the signal | Actual SQLite and cancellation-epoch RED/GREEN; independent initialized-set comparison and fresh-start signal/completion barriers in8a2fce4; scoped re-review approved both, no new findings |
| Whole-branch review found missing production uncertain-send acceptance and a missing clean-shutdown record | One fix agent addressed both in `f35224e`. Scoped re-review verified the withheld-response restart and bounded clean record with connected output. |
| Scoped final review reproduced SIGPIPE when final stdout has no reader | Important and open. A silent control exited 0. The new record terminated the probe with a broken pipe. The same review found a Minor missing-marker assertion gap. |

The corrections preserve existing interfaces and avoid an outbox, generic retry
engine, logging queue/framework, public test flags or new product configuration.
No findings quota was imposed; scheduler review returned no findings. Remaining
operator trade-offs include delayed fatal discovery until Pipeline return,
recollection-only retry, log backpressure/diagnostic loss and OS/filesystem limits.

The reviews caught concrete transaction, shutdown and acceptance problems before
human review. A measured reduction in architecture rework remains unverified.
The final scoped review leaves the closed-pipe regression open despite successful
ordinary-output Compose acceptance.

## Parent rulings, in chronological order

These decisions are preserved from the execution ledger, including their costs if wrong:

1. Use a dedicated development Compose project to leave the existing tunnel runtime alone. Cost: separate development caches.
2. Use the approved managed worktree and scoped write escalation; terminate the exact permission-looping helper and rerun with authorized access. Cost if wrong: incorrect workspace authority or unintended filesystem writes; no permissions or socket modes were weakened.
3. Give each SQLite transaction one private uncanceled termination lifetime while retaining caller contexts for acquisition/statements and checking cancellation before commit. Cost if wrong: unsafe cleanup or an unintended commit after cancellation. Actual lifecycle regressions and independent material re-review accepted the correction.
4. Permit a cleanup-only SAVEPOINT before full rollback to cover SQLite's native auto-rollback, with all cleanup failures fatal. Cost if wrong: hidden cleanup failure or uncommitted visibility. Native interruption, absent-key, safe-reuse and cleanup-failure tests plus independent re-review cover it.
5. Add the minimal shared catalog/runner assembly helper and retain the existing query loader as a wrapper. Cost if wrong: query regression or import coupling. Query/CLI/MCP/coexistence and daemon assembly tests plus independent review cover it.
6. Keep the approved scope of the eight whole-review boundaries for delivery/outbox, immediate fatal abort, distributed/filesystem/power-loss, helper-internal crash windows, deferred features, live services/deployment, Windows, and lossless blocked logging. Stronger operator expectations or additional environments would require expanded acceptance. The required uncertain-send scenario and shutdown log were addressed in the fix wave.
7. Treat the closed-stdout SIGPIPE regression as load-bearing for clean exit 0. Surface it at the review gate and require an output path that preserves the exit status with no reader, plus a real subprocess regression. A mistaken clearance could let a normal stop exit nonzero after successful cleanup.
8. Park the Minor subprocess marker assertion for that corrective pass. The separate unit test observes actual join and close before the record. A future subprocess-only cleanup regression might escape the current marker comparison.
