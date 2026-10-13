# Bound pull-request id shell completion

## Overview
- Pressing Tab after `bb pullrequest get|update` enumerates every pull request ever opened
  (`state=ALL` through `profile.GetAllUnbounded`), i.e. dozens of API pages per keypress on a repo with 2,300+
  PRs. Every other PR-id completion (approve/decline/merge/... , diff, patch, commits, activities,
  merge-status, comment/task subcommands) also pages through all OPEN PRs unbounded, and the comment/task
  completion falls back to unbounded ALL when nothing is open.
- Ports the intent of upstream 6dc12c3 (gildas #127) with a different mechanism (decisions.md D6): upstream
  synthesizes `1..latest`; this plan keeps offering real ids but caps each completion at ONE bounded request.
- Must NOT change `GetAllUnbounded` / `GetPullRequestIDsFromRepositoryWithState` semantics for the id-resolution
  caller `GetPullRequestIDFromArgs` (`internal/pullrequest/pullrequest.go:305-318`), which needs the full open
  list to detect "too many open pullrequests" regardless of `--limit` (see getters.go:29-37 doc comment).
- One PR against `master`: `fix(pullrequest): bound pull request id shell completion`.

## Context (from discovery)
- `internal/pullrequest/common/getters.go` — `GetPullRequestIDsWithState`, `GetPullRequestIDsFromRepositoryWithState`
  (GetAllUnbounded), `GetPullRequestIDs` (OPEN then ALL fallback), `PullRequestIDValidArgs`.
- PR-id ValidArgsFunctions: `get.go:34` (ALL), `update.go:73` (ALL), `commits.go:34`, `activities.go:66`,
  `action.go:53` `openPullRequestIDsCompletion` (approve/unapprove/decline/request-changes/... and
  `merge.go:40` via `mergeValidArgs`), `patch.go:33`, `merge_status.go:38`, `diff.go:39` (all OPEN);
  `comment/*` and `task/*` via `prcommon.PullRequestIDValidArgs` (incl. `comment.go:405`, `task.go:180`,
  `comment/delete.go:27`, `task/delete.go:27`).
- `internal/profile/profile_client.go:200-265` — `GetAll` / `GetAllUnbounded` / `getAll` (NOT modified);
  `profile.PaginatedResources[T]`, `Profile.Get`.
- Existing tests: `internal/pullrequest/common/getters_test.go`, `internal/pullrequest/comment/completion_test.go`,
  `internal/pullrequest/task/completion_test.go`, `internal/pullrequest/action_test.go`,
  `internal/profile/profile_client_test.go` (httptest patterns).

## Development Approach
- **testing approach**: TDD — failing tests first, record the failure in the progress file, then implement.
- complete each task fully before the next; **every task includes tests; all tests pass before the next task**
- **update this plan when scope changes**; minimal change, no behaviour change outside completion.

## Testing Strategy
- unit tests with `net/http/httptest` fake Bitbucket APIs (see existing completion tests for profile/repo setup);
  testify suites stay suite-style where already established; use `suite.NoError` (not `Require`) inside handlers.

## Progress Tracking
- mark completed items `[x]` immediately; ➕ for new tasks; ⚠️ for blockers.

## Solution Overview
- No new pagination helper: completion must never paginate, so it makes ONE plain `profile.Get` and never follows
  `next` (one-request-per-state by construction; `getAll`, GetAll and GetAllUnbounded are untouched). A generic
  `GetAllWithLimit` was rejected in plan review: `ResolvePageLengthAndLimit` only lowers pageLength to the limit,
  so `--page-length 10` / a profile `defaultPageLength: 10` would turn it into 5 requests.
- `internal/pullrequest/common`: add `const completionLimit = 50` (the pullrequests endpoint's maximum pagelen;
  Bitbucket's default page is smaller, so pagelen is always sent explicitly) and
  `CompletePullRequestIDsWithState(ctx, cmd, state) ([]string, error)`: resolves the repository like
  GetPullRequestIDsWithState, then `currentProfile.Get(ctx, repo.GetPath("pullrequests?state=<state>&sort=-updated_on&pagelen=50"), &page)`
  with `page profile.PaginatedResources[PullRequestID]`, maps + sorts ids as today (most recently active PRs).
  Add `CompletePullRequestIDs(ctx, cmd)`: bounded OPEN, falling back to bounded ALL only when no PR is open.
- Rewire every PR-id ValidArgsFunction listed in Context: OPEN ones -> `CompletePullRequestIDsWithState(...,"OPEN")`;
  `get`/`update` -> `CompletePullRequestIDsWithState(...,"ALL")` (keeps today's ALL semantics, one request);
  `PullRequestIDValidArgs` -> `CompletePullRequestIDs` (keeps its OPEN-then-ALL shape, max 2 requests).
- Cleanup: after rewiring, `GetPullRequestIDsWithState` and `GetPullRequestIDs` have no callers — delete both.
  `GetPullRequestIDsFromRepositoryWithState` (GetAllUnbounded) stays, unchanged, for `GetPullRequestIDFromArgs`.
- Typed-out ids that completion does not offer still work (completion never validates args) — note in PR.

## Technical Details
- Request shape asserted by tests: path `/repositories/<ws>/<repo>/pullrequests`, query `state=OPEN|ALL`,
  `sort=-updated_on`, `pagelen=50`, even with `--page-length 10` / `--limit 1` set on cmd; the fake API always
  returns a `next` link, and the test asserts completion never follows it (exactly one request per state).
- `GetPullRequestIDFromArgs` regression test: fake API with 2 pages of OPEN PRs -> it still follows `next`
  and reports "too many open pullrequests" listing ids from both pages, even with `--limit 1` set on cmd.
- `sort=-updated_on` and `state=ALL` on this endpoint are UNVERIFIED against live docs (`state=ALL` is already
  relied on by today's code); verify on the built binary in Task 3 if a usable profile exists, else note it.

## What Goes Where
- Implementation Steps: code, tests, docs.
- Post-Completion: none.

## Implementation Steps

### Task 1: Bounded completion getters

**Files:**
- Modify: `internal/pullrequest/common/getters.go`
- Modify: `internal/pullrequest/common/getters_test.go`

- [x] write httptest tests: `CompletePullRequestIDsWithState` makes exactly one request with the asserted query
  and never follows `next`; `CompletePullRequestIDs` falls back to bounded ALL only when OPEN is empty; errors
  propagate; `--page-length 10`/`--limit 1` on cmd do not change the request; run and record in this plan
  that they FAIL first (undefined)
  - FAILED first: `getters_test.go:173:16: undefined: CompletePullRequestIDsWithState` /
    `getters_test.go:223:16: undefined: CompletePullRequestIDs` -> `[build failed]`; all pass after implementing
    (TestCompletePullRequestIDsWithState x5, TestCompletePullRequestIDs x3, TestCompletePullRequestIDsAPIError x2)
- [x] implement `completionLimit`, `CompletePullRequestIDsWithState`, `CompletePullRequestIDs`
  - ➕ the `profile.Get` error is wrapped (`cannot get <state> pullrequests: %w`) — wrapcheck requires it
- [x] add the `GetPullRequestIDFromArgs`-style regression test (2 OPEN pages, `--limit 1`) proving the unbounded
  resolution path still pages through everything (place it where `GetPullRequestIDFromArgs` lives if that avoids an
  import cycle)
  - `internal/pullrequest/limit_test.go` `TestGetPullRequestIDFromArgsPagesThroughAllOpen`; guards unchanged
    behaviour so it passes on current code; mutation check (resolution swapped to the bounded getter) made it fail:
    `error = "too many open pullrequests, specify one: 1, 2", want ... "1, 2, 3"` / `requested pages = [""]`
- [x] run `go test -race ./internal/pullrequest/...` — must pass

### Task 2: Route every PR-id completion through the bounded getters

**Files:**
- Modify: `internal/pullrequest/get.go`, `update.go`, `commits.go`, `activities.go`, `action.go`, `patch.go`,
  `merge_status.go`, `diff.go`
- Modify: `internal/pullrequest/common/getters.go` (`PullRequestIDValidArgs`; delete helpers left without callers)
- Modify: relevant tests (`update_test.go`, `action_test.go`, `comment/completion_test.go`,
  `task/completion_test.go`, new `get`/`completion` test as needed)

- [x] write a test that `getValidArgs` (get) and `updateValidArgs` against a 3-page fake API issue exactly one
  bounded `state=ALL` request — record in this plan that it FAILS on the old code (it pages through all 3)
  - `internal/pullrequest/completion_test.go` `TestPullRequestIDCompletionIsBounded` (table of all 9 PR-id
    ValidArgsFunctions, `--page-length 10 --limit 1` on cmd). FAILED on old wiring: `get: ids = [3 42 5 7], want
    [42 7] (first page only)` / `got 3 requests ["state=ALL&pagelen=10" "page=2&pagelen=10&state=ALL"
    "page=3&pagelen=10&state=ALL"], want exactly 1 bounded request` (same for update; OPEN for the other 7)
- [x] write/extend a test for one OPEN completion (e.g. `openPullRequestIDsCompletion`) and for
  `PullRequestIDValidArgs` asserting bounded requests
  - OPEN completions are rows of `TestPullRequestIDCompletionIsBounded`; `PullRequestIDValidArgs` ->
    `common/getters_test.go` `TestPullRequestIDValidArgsIsBounded`. FAILED on old wiring: `got 2 requests
    ["state=OPEN&pagelen=10" "page=2&pagelen=10&state=OPEN"], want one bounded request per state [OPEN]` /
    `got 4 requests [... OPEN ... OPEN page=2 ... ALL ... ALL page=2], want one bounded request per state [OPEN ALL]`
- [x] rewire every ValidArgsFunction listed in Context; delete `GetPullRequestIDsWithState` and
  `GetPullRequestIDs`; grep afterwards — only `GetPullRequestIDFromArgs` remains on the unbounded path
  - grep: the sole caller of `GetPullRequestIDsFromRepositoryWithState` is `pullrequest.go:307`
    (`GetPullRequestIDFromArgs`); no reference to the deleted helpers remains in any `.go` file (their 4 old
    tests in `common/getters_test.go` were deleted; the Complete* tests cover the same intent)
- [x] update existing completion tests whose fake handlers assumed the old query shape (keep their intent)
  - reviewed `update_test.go`, `action_test.go`, `comment/completion_test.go`, `task/completion_test.go`: none
    depends on the old query shape (single-page handlers, only `state` asserted) — all pass unmodified
- [x] run `go test -race ./...` — must pass

### Task 3: Verify acceptance criteria
- [x] verify every PR-id ValidArgsFunction is bounded (list them in this plan with the getter each uses)
  - `get.go:34` `getValidArgs` -> `CompletePullRequestIDsWithState(ALL)`
  - `update.go:73` `updateValidArgs` -> `CompletePullRequestIDsWithState(ALL)`
  - `commits.go:34` `commitsValidArgs` -> `CompletePullRequestIDsWithState(OPEN)`
  - `activities.go:66` `activitiesValidArgs` -> `CompletePullRequestIDsWithState(OPEN)`
  - `action.go:53` `openPullRequestIDsCompletion` (approve/unapprove/decline/request-changes/
    remove-request-changes via `newActionCommand`; `merge.go:40` `mergeValidArgs` delegates to it)
    -> `CompletePullRequestIDsWithState(OPEN)`
  - `patch.go:33` `validPatchArgs` -> `CompletePullRequestIDsWithState(OPEN)`
  - `merge_status.go:38` `mergeStatusValidArgs` -> `CompletePullRequestIDsWithState(OPEN)`
  - `diff.go:39` `validDiffArgs` -> `CompletePullRequestIDsWithState(OPEN)`
  - `common/getters.go` `PullRequestIDValidArgs` -> `CompletePullRequestIDs` (bounded OPEN, bounded ALL only if
    none open); used directly by `comment/list.go`, `comment/create.go`, `task/list.go`, `task/create.go`, and
    for arg 0 by `comment/delete.go:27`, `task/delete.go:27`, `comment/comment.go:405`
    (`pullRequestAndCommentIDValidArgs`: get/update/reopen/resolve), `task/task.go:180`
    (`pullRequestAndTaskIDValidArgs`: get/update)
  - grep: `GetPullRequestIDsFromRepositoryWithState` (GetAllUnbounded) has one caller, `pullrequest.go:307`
    (`GetPullRequestIDFromArgs`); no completion reaches it
- [x] if a working bb profile exists, run one real completion on the built binary with `--debug`
  (`./bb __complete pullrequest get ''` inside a Bitbucket checkout) and record the request + that it returned ids;
  otherwise note UNVERIFIED-live in this plan
  - ⚠️ `--debug` emits nothing under `__complete` (either position): `initializeLogger` runs from
    `cobra.OnInitialize`, which fires for the `__complete` command before the target command's flags are parsed,
    so the request URL could not be read from debug output (pre-existing, not touched by this plan). Verified the
    request's effect instead, read-only, on the built binary:
  - `sportpursuit/aws-nest-microservices` (520 PRs total): `__complete pullrequest get ''` exit 0,
    `ShellCompDirectiveNoFileComp`, exactly 50 ids in ~2.4s; they equal the top 50 of an independent
    `pullrequest list --state all -o json` (520 PRs, ~24s) ranked by `updated_on`, incl. old PR 95 -> Bitbucket
    accepts and honours `state=ALL`, `sort=-updated_on`, `pagelen=50`. One request is by construction (single
    `profile.Get`, `next` never read; unit tests assert it); the 50-entry cap with 520 PRs and the 10x latency
    gap are consistent with it
  - OPEN: `__complete pullrequest approve ''` in `sportpursuit/airflow` -> exit 0, 50 ids (capped);
    in `sportpursuit/odoo` -> 31 32 34; in aws-nest-microservices (0 open) -> empty, and
    `__complete pullrequest comment list ''` there fell back to the same 50 ALL ids
- [x] run `go build ./...`, `go vet ./...`, `gofmt -l .` (empty), `go test -race ./...`, `golangci-lint run`
  (v2.12.2), `make cross-build`
  - all green: build/vet OK, gofmt empty, all packages `ok`, golangci-lint v2.12.2 `0 issues.`, cross-build OK

### Task 4: [Final] Update documentation
- [x] grep `CLAUDE.md` and `skill/bitbucket-cli/SKILL.md` for completion / GetAllUnbounded wording; update any
  statement now wrong (CLAUDE.md's Layout line for `/common (prcommon)` lists the shared getters — keep it accurate)
  - CLAUDE.md Layout: `GetPullRequestIDs` (deleted) replaced by `CompletePullRequestIDs`/
    `CompletePullRequestIDsWithState` (bounded, single-request completion) alongside `PullRequestIDValidArgs`
  - ➕ README.md `--limit` paragraph: added that PR-id completion makes one request and offers at most the 50 most
    recently updated PRs, and that an id it does not offer can still be typed
  - SKILL.md says nothing about PR-id completion — left untouched
- [x] move this plan to `docs/plans/completed/` (`git add -f` — docs/plans/* is gitignored)

## Post-Completion
- none.

## Review
- Outcome: every PR-id shell completion is bounded to ONE request per state (`pagelen=50`, `sort=-updated_on`,
  `next` never followed); `get`/`update` use bounded ALL, the other 7 PR-id completions bounded OPEN,
  comment/task (`PullRequestIDValidArgs`) bounded OPEN then bounded ALL only when nothing is open.
  `GetPullRequestIDsFromRepositoryWithState` (GetAllUnbounded) is unchanged and used only by
  `GetPullRequestIDFromArgs`; `GetPullRequestIDsWithState`/`GetPullRequestIDs` deleted.
- Commits: `1382758` feat(pullrequest): add bounded pull request id completion getters; `f5ec3ed`
  fix(pullrequest): bound pull request id shell completion; plus the docs/plan commit.
- Live check (read-only, built binary): `get` completion returned 50 ids in ~2.4s on a 520-PR repo, matching the
  top 50 by `updated_on` (Bitbucket honours `state=ALL`, `sort=-updated_on`, `pagelen=50`); OPEN and the
  comment/task ALL fallback behaved as specified.
- ⚠️ `--debug` emits nothing under `__complete` (logger initialised before flags are parsed) — pre-existing, out
  of scope; request shape is asserted by unit tests instead.
- Gates: build, vet, gofmt, `go test -race ./...`, golangci-lint v2.12.2 (0 issues), cross-build — all green.
