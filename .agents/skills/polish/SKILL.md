---
name: polish
description: "Polish a completed implementation through delegated review, independent validation, and fixes for the user's requested number of rounds, defaulting to three. Use when the user asks to polish an implementation, run iterative review-and-fix passes, or have delegates review completed work and address valid findings. If requested alongside implementation, run after implementation is complete."
---

# Polish

Use the number of review rounds the user specifies. If the user does not specify a count, default to three rounds. Complete exactly that many rounds. The parent agent owns validation and fixes; delegates return findings. Keep the work within the original requirements.

## Establish the review scope

Identify the original requirements and the complete implementation change set from the session, branch, or user-supplied baseline. Include relevant committed, staged, unstaged, and untracked changes while excluding unrelated user work. Resolve an ambiguous baseline before delegating.

Capture the state about to be reviewed before each round. Use immutable commits or temporary snapshots and diffs so later scopes remain precise even when fixes are uncommitted. Creating commits is unnecessary.

## Run the selected number of rounds

Perform these steps in order for each round, completing fixes and verification before starting the next.

### 1. Delegate the review

Start fresh review delegates for each round using the environment's delegation rules. Choose complementary scopes appropriate to the changes. Usually split functional correctness and requirements coverage from code quality, separation of concerns, duplication, and abstraction bloat. Add specialist coverage only when the implementation warrants it.

Give each delegate:

- The original requirements, applicable repository instructions, and its assigned review scope.
- The exact before/after states or diff, including relevant new files, and available verification results.
- A request for actionable findings with file and line references, concrete impact, supporting evidence, and a suggested correction. An explicit no-findings result is valid.
- Instructions to review and report only, leave files unchanged, and start no further delegates.

Apply the round's scope:

- **Round 1:** Review the full implementation comprehensively, including affected behavior, integration points, failure paths, tests, and maintainability. Read surrounding code as needed to judge the change.
- **Each subsequent round:** Review only changes made after the preceding round's snapshot, including fixes and their tests.

For subsequent rounds, give delegates the prior findings and their dispositions. They may inspect surrounding code to understand a change and identify regressions caused by it. Findings must arise from the new diff or demonstrate that a claimed fix remains incomplete. Keep unrelated existing code and settled findings outside the review unless new evidence from the incremental changes invalidates the earlier decision.

If a round produces no changes, continue through the remaining rounds with the empty delta explicit. Have delegates confirm that there is no additional change to review, without reopening the full implementation.

### 2. Validate the findings

Treat every finding as a claim to investigate. Inspect the relevant code and requirements, reproduce the problem or run a focused check where useful, and assess whether the proposed correction is warranted. A code smell alone does not justify a refactor.

Record each finding as accepted, rejected, or unresolved, with a brief evidence-based reason. Push back on incorrect assumptions, speculative concerns, unnecessary abstractions, and changes beyond the original requirements. Merge duplicate findings before fixing them. Keep unresolved claims visible rather than treating them as verified defects.

### 3. Fix and verify

Implement the smallest sufficient fix for each accepted finding. Add or update tests when they meaningfully verify the behavior, then run checks appropriate to the changes and any required repository checks. Resolve failures caused by the fixes before moving to the next round.

Retain the round's reviewed snapshot and finding dispositions so the next round can isolate the additional changes. Fixes made after the final selected round receive normal verification; they do not start an additional delegated review round.

## Report the outcome

Summarize the scope and result of each round, accepted findings and fixes, material rejected findings and reasons, verification performed, and any unresolved issues. State how many rounds actually completed. If delegation is unavailable or a round is blocked, report the limitation without claiming that self-review completed the delegated workflow.
