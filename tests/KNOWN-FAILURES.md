# Known test failures — the baseline a phase gate diffs against

`TMPDIR=/tmp go test ./...` in this repo is not green. The failures below are
declared, with provenance, so a per-phase gate can **diff against this list**
instead of re-deriving each one by hand. Anything not listed here is a
regression.

This is the core half of W0.3 / audit **B3** — the `KNOWN-FAILURES.md` contract
`daemon/tests/KNOWN-FAILURES.md` established and `nb/tests/KNOWN-FAILURES.md`
carries for tend. It landed in nb only, so the Phase 4 gate's "diffed against
each repo's declared baseline" had nothing to diff against here; the Phase 4
final holistic review (F3) established the provenance below, and this file is
where it stops being re-derived.

## Why this file exists

A phase that adds code to a package inherits that package's gate. When the gate
is already red for reasons the phase did not cause, every reviewer after the
first re-derives the same provenance — export the base, run the test there,
confirm it fails identically — and the one who does not re-derive it waves the
failure through instead. Both outcomes are worse than writing the answer down
once.

This is recommendation #4 of the satellite-PoC retrospective: capture the
known-fail set at plan start, so per-phase gates diff against a declaration
rather than re-proving pre-existence.

## The contract

- A failing test may be listed here **only** with: its full name, the commit or
  condition that makes it fail, the date, and why it is not the current phase's.
- An entry is a debt, not a waiver. Nothing here is expected to stay.
- **An unlisted failure fails the gate.** This file cannot hide a new failure —
  it can only pre-declare an old one, in writing, with its cause.
- A listed test that starts PASSING is also a signal: move it to *Resolved* with
  the fixing commit rather than deleting the entry. The record of what was once
  broken is the part a future reviewer needs.

## How the provenance below was established

No `git worktree add`, no `git stash`, nothing written under any repo's `.git`
(the B8 rule, and job 147's technique):

```sh
mkdir -p /tmp/p4close-base/core
git -C core archive main | tar -x -C /tmp/p4close-base/core
# a scratch go.work in /tmp/p4close-base whose `use` block points at ./core and
# at this worktree's other 26 module dirs, so the exported core builds against
# the SAME siblings the working copy does
cd /tmp/p4close-base && TMPDIR=/tmp go test ./core/pkg/plan/ ./core/pkg/workspace/
```

The comparison base is core `main` **`ee6147f`** (2026-08-12,
*fix(worktreeregistry): adopt create-only so a sweep cannot blank a live entry*).
Every entry below fails **identically** there.

## Open

### `pkg/plan` — `TestResolveTargetOwnerQualifiedPlanDir`

Two subtests: `legacy_standalone-repo_container_qualifies_by_the_owner_workspace`
and `deleted_container_still_derives_the_plan_dir_through_the_owner`.

```
target_test.go: Not equal:
  expected: ".../002/.grove/notebooks/nb/notespaces/alpha-repo/plans/alpha-view"
  actual  : ".../003/alpha-repo/.grove-worktrees/alpha-view/.notebook/plans/alpha-view"
```

| | |
|---|---|
| **Failing since** | not identified. Red at `main` `ee6147f` (2026-08-12) and at every Phase 4 head. |
| **Cause** | the assertion expects the owner-qualified plan dir to render through the recorded **notespace** layout (`<notebook>/notespaces/<ns>/plans/<plan>`); resolution returns the repository-local `.notebook/plans/<plan>` instead. A layout expectation the notespace cutover moved out from under it, not a resolution defect the test caught. |
| **Not P4** | fails identically at `main`, which contains no Phase 4 commit. Phase 4's core work is `pkg/notespace` and `pkg/workspace`'s resolver; it adds no caller and no change in `pkg/plan`. |
| **Owner** | plan targeting. Whoever fixes it must decide which of the two paths is the intended answer — that is a product question, not a test-repair one. |

### `pkg/plan` — `TestResolvePlanBindingsOwnerQualified`

```
target_test.go:163: Not equal:
  expected: (plan.BindingHealth) "valid"
  actual  : (plan.BindingHealth) "binding mismatch"
  Messages: alpha binding: {... Reason:same-named registry entries belong to other workspaces ...}
```

| | |
|---|---|
| **Failing since** | not identified. Red at `main` `ee6147f` and at every Phase 4 head. |
| **Cause** | the same layout expectation as above, read through the binding table: the key the fixture builds under `.../notespaces/alpha-repo/plans/view` does not match the entry resolution produces, so the binding reports `binding mismatch` rather than `valid`. |
| **Not P4** | fails identically at `main`. Same reasoning as above. |
| **Owner** | plan targeting, with the entry above. They are one defect seen twice. |

### `pkg/workspace` — `TestPrepare`

Three subtests: `single_repo_workspace_creation`, `ecosystem_worktree_with_repos`,
`prepare_with_branch_already_checked_out_in_worktree`.

```
prepare_test.go:102: "" does not contain "feature/test"
prepare_test.go:152: Received unexpected error
prepare_test.go:474: Should return existing worktree path when branch is already checked out
```

| | |
|---|---|
| **Failing since** | **before this plan started.** Recorded as a pre-existing baseline failure by the P1 W1.1/W1.2 job against core `a99fdc4`, and again by the P2 machine-B guide job — each time reproduced with the job's own changes stashed. |
| **Cause** | not identified. `Prepare` does not produce the worktree the assertions read back. |
| **Not P4** | fails identically at `main`; predates Phase 1, let alone Phase 4. This is audit **B2**'s "fix or declare `TestPrepare`", now declared. |
| **Owner** | workspace preparation. W0.2 / B2 remains open as a fix. |

## Resolved

*(nothing yet)*
