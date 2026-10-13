# Decode merge-queue pipeline targets fully

## Overview
- Bitbucket runs merge-queue pipelines with a `pipeline_merge_queue_ref_target` target carrying `ref_name` (the
  queue ref, e.g. `bitbucket-merge-queue-62`), `target_branch` (the real destination, e.g. `master`) and
  `pullrequest_ids`. The fork's flattened `Target` has no `target_branch`/`pullrequest_ids`, so those are
  dropped on decode and `GetDestination()` — which feeds `pipeline list`/`get`'s `branch` column and the
  branch sort — reports the queue ref instead of the real branch; `-o json|yaml` loses both fields.
- Ports upstream 56c3ab4 (gildas #129), which added a separate `MergeQueueReferenceTarget` type to upstream's
  target registry; here the deliberate flattened-struct design is kept (decisions.md D5).
- One PR against `master`: `fix(pipeline): decode merge queue targets and report their target branch`.

## Context (from discovery)
- `internal/pipeline/target.go:15-40` — flattened `Target` + doc comment explaining the design; `GetDestination`.
- `internal/pipeline/pipeline.go:77` (branch sort) and `:130` (`branch` column) consume `GetDestination`.
- `internal/pipeline/pipeline_marshal_test.go` — `loadTestData` reads repo-root `testdata/`; existing
  round-trip test (`pipeline.json`), PR target test (`pipeline-pullrequest.json`), `TestTargetGetDestination`.
- Upstream fixture (git show 56c3ab4:testdata/pipeline-target-merge-queue-ref.json) gives the wire shape.
- CLAUDE.md: every `testdata/` file must be referenced by a test.

## Development Approach
- **testing approach**: TDD — failing tests first, record the failure in the progress file, then implement.
- complete each task fully before the next; **every task includes tests; all tests pass before the next task**
- **update this plan when scope changes**; keep every existing target shape's JSON byte-identical.

## Testing Strategy
- unit tests in `internal/pipeline` (package pipeline, plain `testing` + testify assert as the file already does).

## Progress Tracking
- mark completed items `[x]` immediately; ➕ for new tasks; ⚠️ for blockers.

## Solution Overview
- Add to `Target`: `TargetBranch string \`json:"target_branch,omitempty"\`` and
  `PullRequestIDs []uint64 \`json:"pullrequest_ids,omitempty"\``.
- `GetDestination` precedence: `TargetBranch` > `RefName` > `Destination` (so a merge-queue target reports
  `master`; reference/PR/commit targets are unchanged since they never carry `target_branch`).
- Update the `Target` and `GetDestination` doc comments to name the fourth shape and say why a merge-queue
  target reports `target_branch` (the real destination; `ref_name` is the queue ref) — current behaviour only.
- Note: with `omitempty`, an empty `pullrequest_ids: []` from the server renders as an absent key under `-o json`;
  acceptable, mention it in the doc comment.

## Technical Details
- New fixture `testdata/pipeline-merge-queue.json`: a full pipeline object (copy `testdata/pipeline.json`'s
  envelope) whose `target` is the upstream merge-queue payload (`type pipeline_merge_queue_ref_target`,
  selector `merge-queues`/`mergequeue-main`, `ref_type branch`, `ref_name bitbucket-merge-queue-62`,
  `target_branch master`, `pullrequest_ids [62]`, commit hash `abcdef1234567890abcdef1234567890abcdef12`).

## What Goes Where
- Implementation Steps: code, fixture, tests, docs.
- Post-Completion: none.

## Implementation Steps

### Task 1: Merge-queue target fields and destination

**Files:**
- Modify: `internal/pipeline/target.go`
- Modify: `internal/pipeline/pipeline_marshal_test.go`
- Modify: `internal/pipeline/pipeline_row_test.go`
- Create: `testdata/pipeline-merge-queue.json`

- [x] create the fixture and a test `TestPipelineUnmarshalMergeQueueTarget` asserting Type, Selector
  type/pattern, RefType, RefName, TargetBranch == "master", PullRequestIDs == []uint64{62}, Commit hash, and
  `GetDestination() == "master"`; plus a JSON round-trip of that fixture (`assert.JSONEq`) so
  `target_branch`/`pullrequest_ids` survive `-o json`
- [x] add a `TestTargetGetDestination` row: merge-queue target (RefName + TargetBranch) uses TargetBranch
- [x] add a `GetRow` test in `pipeline_row_test.go` (next to `TestPipelineGetRowCoversEveryColumn`) that a
  merge-queue pipeline's `branch` column renders `master`, and a sort test that it sorts by `master`
  (pipeline.go:77), not by `bitbucket-merge-queue-62`
- [x] extend `TestTargetMarshalReferenceTarget` / `TestTargetMarshalPullRequestTarget` to assert the
  `target_branch` and `pullrequest_ids` keys are ABSENT (guards the trigger payload, trigger.go:19,95)
- [x] run the tests and record in this plan (a note under the box) that they FAIL (compile error or wrong value)
  before the fix
  - note: confirmed FAIL before the fix (build failed): `pipeline_marshal_test.go:118:37: p.Target.TargetBranch
    undefined (type Target has no field or method TargetBranch)`; `:196:88: unknown field TargetBranch in struct
    literal of type Target`; `pipeline_row_test.go:59:114`/`:73:72`: same unknown-field error
- [x] add the two fields, change `GetDestination` precedence, update doc comments
- [x] run `go test -race ./internal/pipeline/...` — must pass; existing round-trip tests unchanged

### Task 2: Verify acceptance criteria
- [x] existing round-trip tests pass unchanged
- [x] run `go build ./...`, `go vet ./...`, `gofmt -l .` (empty), `go test -race ./...`, `golangci-lint run`
  (v2.12.2), `make cross-build`

### Task 3: [Final] Update documentation
- [x] confirm `skill/bitbucket-cli/SKILL.md` / `CLAUDE.md` carry no pipeline-branch wording made wrong (plan review
  found none: SKILL.md:274 covers trigger flags only) — expected outcome: no doc changes
  - note: re-checked SKILL.md, CLAUDE.md and README.md against `git diff origin/master...HEAD`; no wording
    about the pipeline branch column, `GetDestination` or pipeline targets is affected — no doc changes
- [x] move this plan to `docs/plans/completed/` (`git add -f` — docs/plans/* is gitignored)

## Post-Completion
- none.

## Review
- commits: `4e72bac` fix(pipeline): decode merge queue targets and report their target branch;
  `e57862d` test(pipeline): tighten merge queue target coverage.
- TDD failure evidence (before the fix, build failed): `pipeline_marshal_test.go:118:37: p.Target.TargetBranch
  undefined (type Target has no field or method TargetBranch)`; `:196:88: unknown field TargetBranch in struct
  literal of type Target`; `pipeline_row_test.go:59:114`/`:73:72`: same unknown-field error.
- phase 1 (comprehensive review): 12 findings, fixed in `e57862d`. Declined: yaml tags / a YAML round-trip test
  (a separate repo-wide change), a README sentence, and the duplicate `GetRow`/sort tests (kept per plan).
- phase 1 critical re-check: clean.
- phase 2 (smells): clean.
- phase 3 (codex): skipped — codex not installed.
- phase 4: satisfied by the phase 1 critical re-check on unchanged HEAD.
- ⚠️ `-o yaml` renders `Target`'s zero values (the struct has no yaml tags) — pre-existing, out of scope.
