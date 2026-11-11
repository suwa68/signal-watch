# SignalWatch Phase 2 development startup prompt

Use the following prompt when starting or resuming Phase 2. It uses existing
Superpowers skills and Codex subagents; it installs no custom skill, agent profile,
or framework. The [current plan](../docs/superpowers/plans/2026-11-08-phase2-monitoring-mvp.md)
was approved for implementation by the user's "start phase 2 implementation"
instruction on 2026-11-08. This reusable prompt itself never grants approval.

---

You are starting or resuming SignalWatch Phase 2.

## Objective and scope

Deliver the smallest operational monitoring MVP while validating autonomous
Codex + Superpowers development:

- Long-running Go scheduler and graceful shutdown.
- Independent scheduled collection of configured sources.
- Durable monitoring state that survives container restart.
- Silent initial baseline and existing notification semantics.
- Reuse the existing Telegram delivery pipeline.
- Docker Compose runtime and deterministic acceptance tests.

This scope explicitly lifts deferrals for scheduling, durable monitoring state,
and the Docker monitoring runtime only. Preserve the other AGENTS.md deferrals.
NLP/AI summarization, cross-source or on-demand aggregation, new adapter types,
dashboards, message brokers, distributed workers and speculative infrastructure
remain out of scope. Durable retry/outbox and new item identity semantics are
not approved. Never commit secrets or send real Telegram notifications without
explicit authorization. Do not auto-merge or deploy.

## Start from existing evidence

Read AGENTS.md, applicable Superpowers skills, the current Phase 2 plan and its
linked review/handoff artifacts, relevant repository documentation and actual
implementation/tests. Verify branch/HEAD and reconcile the progress ledger with
Git and the current PR head before choosing the next task. Do not restart planning
or repeat completed work merely because this is a new session.

The current checkpoint is authorized execution of the reviewed plan. Follow the
tracked execution plan/ledger and handoff. A future unapproved architecture
expansion still requires its own applicable approval. Ordinary status questions
do not suspend already authorized work.

## Independent Architecture Review Gate

Before requesting human approval for a non-trivial implementation plan:

1. Obtain an independent review using a fresh-context reviewer, following applicable Superpowers skills.
2. The reviewer must inspect relevant repository code and existing contracts, not only the proposed design document.
3. Review the design against actual implementation risks:
   - State consistency, transactions, and concurrency
   - Failure classification, recovery, and restart behavior
   - Cancellation, timeouts, and resource ownership
   - Compatibility with existing interfaces and behavior
   - Acceptance-test feasibility and environment limitations
   - Scope creep, unnecessary dependencies, and overengineering
4. Classify findings as Blocking, Major, Minor, or Informational. Provide code references, evidence, and recommended corrections.
5. The Parent must address findings and request independent re-review of material corrections when appropriate.
6. Present the final plan with:
   - Review findings and resolutions
   - Remaining risks and trade-offs
   - Unverified assumptions or acceptance tests
   - Explicit decisions requiring human approval

Do not invent findings to satisfy a review quota.

Keep the review proportional to the task. Do not impose a fixed number of agents or require heavyweight review for trivial changes.

The review is not equivalent to human approval. Do not begin implementation before an explicitly required approval gate.

## Review output and regression cases

Blocking means a contradiction or missing decision prevents safe or feasible
execution. Major means a consequential correctness, compatibility or acceptance
gap. Minor means limited impact. Informational records a supported constraint or
trade-off. Classify actual effects; a class may contain zero findings.

For each finding record severity, concrete failure scenario, repository file/line
and contract, evidence versus inference, recommended minimal correction, Parent
disposition, and the test/evidence needed to close it. Record the reviewed commit
and plan revision. Resolving a design gap does not mean its runtime tests passed.
Re-review material changes to concurrency, failure policy or acceptance topology;
link the re-review result rather than declaring the correction independently verified.

Explicitly assess these three regression cases against code and tests:

| Case | Challenge | Evidence expected |
| --- | --- | --- |
| SQLite concurrency and ownership | Are SQL connection/transaction lifetimes bounded? Can two daemon processes send the same unseen item? | StateStore/baseline and Pipeline guards; transaction, pool and lifetime ownership rules; concurrent/process-death test design. |
| Recoverable versus fatal state errors | Can a joined cancellation hide a real storage failure? Does a timeout wrongly stop healthy sources? | Error propagation and continuation tests; typed classification, aggregate precedence and supervisor exit policy. |
| Docker sandbox and acceptance | Can the required commands run with permitted engine access, and do tests exercise the actual production image? | Actual capability results, least necessary permission path, offline fixture topology, and separate proposed/unverified/executed evidence. |

Resolve blocking findings before calling the plan ready for approval. Correct
Major findings autonomously where requirements allow; surface an unresolved Major
as an explicit decision/risk, never silently defer it. Human attention is for
requirements, major choices, accepted risks and the required approval gate.

## Four autonomy goals and evidence

1. **Dynamic Orchestration:** Parent selects delegation, agent count and sequential
   or parallel work from dependencies and risk. Follow applicable Superpowers
   concurrency/review restrictions. Record decisions, actual overlaps and rework;
   no fixed agent count or manufactured parallelism.
2. **Non-blocking CI:** Treat Actions as asynchronous. Never block Parent on watch
   commands. Continue independent review, documentation or handoff work while runs
   are pending. Use bounded status reads and verify expected checks for the current
   PR head SHA; record pending/unverified honestly. Do not change branch protection.
3. **Cross-session Continuity:** At a meaningful reviewed integration boundary,
   commit a concise handoff with decisions, completed tasks/commits, issues/PRs,
   validation, pending checks, unresolved risks and exact next steps. Exercise a
   fresh-context continuation from tracked artifacts. Distinguish a rehearsal from
   an observed new-session continuation.
4. **Autonomous Architecture Review:** Preserve the code-grounded review record,
   Parent corrections and material re-review. Demonstrate pre-implementation failure
   scenarios, existing contracts retained, unnecessary infrastructure rejected,
   and human decisions isolated. Track later architecture rework attributable to
   missed assumptions; do not claim improved efficiency without observed evidence.

## Execution after approval

Follow the approved plan and relevant Superpowers execution skill. Use the
repository's issue-first, feat/ branch, test, independent review and PR workflow.
Do not prescribe a fixed number of implementation or review agents. Keep storage
details out of core models, use existing contracts, and add configuration only for
a demonstrated requirement. Use deterministic local fixtures and httptest.Server;
no tests depend on live websites or Telegram.

Report actual validation results, remaining risks and unverified acceptance.
Finish with a current handoff and PR/check evidence. Do not merge or deploy.
