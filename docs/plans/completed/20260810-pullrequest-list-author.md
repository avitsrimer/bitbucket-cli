# Add --author/--mine cross-repo listing to `bb pullrequest list`

## Overview

- Add `--author <user>` and `--mine` flags to `bb pullrequest list`. When either is set, the
  command lists pull requests **across every repository of the workspace** authored by that
  user, via Bitbucket's `GET /workspaces/{workspace}/pullrequests/{selected_user}` endpoint,
  instead of the repo-scoped `repositories/{ws}/{repo}/pullrequests` endpoint.
- Solves "show my/someone's PRs across the workspace" in one API call — today the only option
  is a shell loop over `bb repo list`, N+1 requests.
- No repository resolution happens in author mode, so the command works from any directory
  (including non-Bitbucket checkouts) as long as the workspace resolves (`--workspace` flag >
  Bitbucket git remote > profile `--default-workspace`).
- Repo-scoped `list` and `get` DEFAULT OUTPUT is unchanged. (Not literally byte-for-byte
  everywhere: the new `repository` column joins the shared column table, so `--columns`/
  `--sort` enums and shell completion gain one value on both commands — deliberate and
  harmless.)

## Context (from discovery)

- Files/components involved:
  - `internal/pullrequest/list.go` — `listCmd`, `listProcess`, `listStates`, `listQueryFilter`
  - `internal/pullrequest/pullrequest.go` — `PullRequest` struct, `columns` slice,
    `GetHeaders` (list defaults: `ID, Title, source, destination, state`), `GetRow`
  - `internal/pullrequest/pullrequest_row_test.go` — GetRow/GetHeaders coverage, including
    `TestPullRequestGetRowCoversEveryColumn` (~line 100), which iterates a HARDCODED
    column-name list (not the unexported `columns` table) and whose fixture builds
    `Destination` with only a `Branch` — both must be updated or the new column ships
    silently uncovered
  - `internal/pullrequest/list_test.go` — existing `setupTest`, `withStateFlag`,
    `withListOptions` helpers (reuse, don't duplicate) and `TestListCmdRegistersLimitFlag`
    (precedent for asserting on the real `listCmd`)
  - `internal/pullrequest/endpoint.go` — `Endpoint.Repository *repository.Repository`
    (carries `full_name`) already deserialized from the payload
  - `internal/workspace/workspace.go:106` — `GetWorkspaceName(ctx, cmd)` resolves workspace
    without touching repository resolution
  - `internal/user/user.go:160` — `GetMe(ctx, cmd)` (cached) for `--mine`
  - `internal/common/identifier.go:27` — `ValidatePathIdentifier` rejects `/`, `\`, and
    dot-segments only — NOT `?`/`#` (see Technical Details)
  - `internal/cmd/root.go:67` — `--repository` is a root PERSISTENT flag, so `init()`-time
    `MarkFlagsMutuallyExclusive` cannot pair it with `--author`/`--mine`; needs a runtime check
  - `skill/bitbucket-cli/SKILL.md` — documents `bb pullrequest list`; must be updated in the
    same PR (sync-guard only catches renamed/removed commands, not flags)
- API contract (verified against `api.bitbucket.org/swagger.json`):
  - `GET /workspaces/{workspace}/pullrequests/{selected_user}` returns `paginated_pullrequests`
    (same item shape as the repo endpoint, so `profile.GetAll[PullRequest]` works as-is)
  - `state` query param, repeatable, `OPEN|MERGED|DECLINED|SUPERSEDED`; defaults to open only
  - supports the standard `q=` filtering/sorting, so `--query`/`--source`/`--destination`
    compose exactly as they do today
  - `selected_user` accepts a username, `{uuid}` (curly braces required), or Atlassian ID.
    CAVEAT: Bitbucket Cloud removed usernames post-GDPR and a workspace member's `nickname`
    (what `bb workspace members` shows) is NOT a username — `--author <nickname>` will
    typically 404, so the error path needs an actionable message (see Task 1)
  - scope: `read:pullrequest`, but the workspace-level path needs workspace-level access — a
    Repository Access Token that works for repo-scoped `list` will 403 here (SKILL.md note)
- Patterns found: `RunE` reads flags direct-off-`cmd` (`common.StringFlagValue`); read-only
  commands gate on plain `common.WhatIf` before the main request; httptest-based tests in
  `internal/pullrequest/action_test.go` style.

## Development Approach

- **testing approach**: Regular (code first, then tests, same task)
- complete each task fully before moving to the next
- make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
  - tests cover both success and error scenarios
- **CRITICAL: all tests must pass before starting next task** — no exceptions
- **CRITICAL: update this plan file when scope changes during implementation**
- run tests after each change
- maintain backward compatibility (repo-scoped `list` default output and requests unchanged)

## Testing Strategy

- **unit tests**: table-driven, `net/http/httptest` for the HTTP-facing paths (matching
  `action_test.go`); no e2e suite exists in this repo.
- HTTP/behavior tests drive `listProcess` with a standalone `*cobra.Command` carrying its own
  flags (the direct-off-`cmd` pattern makes this equivalent to the wired command). EXCEPTION:
  mutual-exclusivity tests cannot go through `listProcess` — cobra enforces flag groups in
  `ValidateFlagGroups` during `Execute`, never in `RunE` — so those assert on the real
  `listCmd` (set flags, call `listCmd.ValidateFlagGroups()`; precedent:
  `TestListCmdRegistersLimitFlag`).
- Test-setup note: `testutil.SetupProfile` registers a `--repository` flag but NOT
  `--workspace` (`internal/testutil/testutil.go:136`), and `GetWorkspaceName` falls through
  git-remote to the profile default — author-mode tests must register/set a `workspace` flag
  themselves. Reuse `list_test.go`'s existing `setupTest`/`withStateFlag` helpers.

## Progress Tracking

- mark completed items with `[x]` immediately when done
- add newly discovered tasks with ➕ prefix
- document issues/blockers with ⚠️ prefix

## Solution Overview

- `--author <user>` (string) and `--mine` (bool) on `listCmd`, mutually exclusive with each
  other and each mutually exclusive with `--commit` (author mode has no commit form).
  Rationale for a separate `--mine` flag instead of an `--author me` sentinel (explicit user
  decision): `--author` stays a verbatim pass-through with no magic values, so a real account
  whose identifier happens to be `me` can never be shadowed; the current-user resolution is
  opt-in via its own flag.
- One unexported helper `authorModeValue(cmd) (author string, mine bool, ok bool)` (or
  equivalent single predicate) in `list.go`, used by BOTH `listProcess` and `GetHeaders`, so
  the two can never drift. Detection must read typed flag values nil-safely
  (`cmd.Flags().GetBool("mine")`, `common.StringFlagValue(cmd, "author") != ""`) — NEVER
  `flag.Value.String() != ""`, since an unset bool flag stringifies as `"false"`, which is
  non-empty and would flip every plain `list`/`get` into author mode.
- `listProcess` gains an author-mode branch, checked FIRST in the `switch` (before the
  `--commit` arm — exclusivity is parse-time only, so a direct `listProcess` call with both
  set must deterministically take the author arm and the runtime guard below errors anyway):
  1. runtime guard: if `cmd.Flags().Changed("repository")` (root persistent flag —
     `MarkFlagsMutuallyExclusive` can't cover it), error out: author mode is workspace-wide
     and an explicitly-passed `--repository` would otherwise be silently ignored;
  2. resolve the workspace with `workspace.GetWorkspaceName(cmd.Context(), cmd)` — no
     `repository.GetRepository` call at all in this branch;
  3. resolve the author: `--mine` → `user.GetMe` → `me.ID.String()` VERBATIM
     (`common.UUID.String()` already returns `"{...}"` — adding braces would double them);
     `--author` → `common.ValidatePathIdentifier("author", value)` then `url.PathEscape`
     (see Technical Details for why both) — no `me` sentinel, explicit values only;
  4. build `/workspaces/<ws>/pullrequests/<author>?state=…&q=…` reusing the existing
     `listStates` and `listQueryFilter` output unchanged;
  5. same `common.WhatIf` gate (message names the workspace and author instead of the
     repository), same `profile.GetAll[PullRequest]` fetch, sort, print;
  6. a 404 from the endpoint gets wrapped with an actionable message: the accepted author
     forms (`{uuid}` or Atlassian ID; usernames are mostly defunct, nicknames don't work)
     and a pointer to `bb workspace members`.
- New `repository` column on the PR column set, rendered from
  `Destination.Repository.FullName` (nil-safe → `common.EmptyCell`). It joins the DEFAULT
  column set only in author mode; repo-scoped `list` and `get` defaults are untouched, but
  `--columns repository` / `--sort repository` work everywhere.
- No shell completion for `--author` (explicit decision, not a gap): the only cheap value
  source, `GetReviewerNicknames`, returns nicknames — which are precisely the values the
  endpoint does NOT accept; completing them would manufacture 404s.

## Technical Details

- **Path construction** (verified, no research needed): `resolveRequestURL`
  (`internal/profile/profile_client.go:633-668`) joins any `uripath` starting with `/` under
  `/2.0`; use `"/workspaces/" + ws + "/pullrequests/" + author` like `user.GetMe`'s `"/user"`.
- **Author path segment — why BOTH validators**: `ValidatePathIdentifier` rejects `/`, `\`,
  and dot-segments (path splicing) but NOT `?`/`#`. `resolveRequestURL` does
  `strings.Split(uripath, "?")` and takes `components[1]` as `RawQuery`, so an unescaped
  `--author 'x?state=MERGED'` would DROP our own `state`/`q` parameters and substitute the
  attacker-controlled query. `url.PathEscape` closes exactly that (`?`→`%3F`, `#`→`%23`).
  For a braced `{uuid}` it changes nothing on the wire (net/url percent-encodes braces during
  serialization regardless). Tests must assert the encoded form via `r.URL.EscapedPath()`/
  `r.RequestURI` — an httptest handler's `r.URL.Path` is already decoded.
- **`--mine` resolution**: `user.GetMe` is cached (`UserCache`), so the extra GET is
  amortized. `common.UUID.String()` (`internal/common/uuid.go:21`) returns `"{" + uuid + "}"`
  — use it verbatim as the path segment. DELIBERATE deviation note: `--mine --dry-run` still
  issues the real `GET /user` before the `WhatIf` gate; this matches today's `listProcess`
  (which resolves the repository first because the WhatIf message names it) and reads are
  safe — record it in a code comment so a reviewer doesn't flag it against CLAUDE.md's
  "checked before any resolution" line.
- **`GetHeaders` author-mode default**: `GetHeaders` calls the shared `list.go` helper
  (nil-safe: a `cmd` without those flags — e.g. `get` — is never author mode). List default
  in author mode: `ID, Title, repository, source, destination, state`.
- **Flag exclusivity**: `listCmd.MarkFlagsMutuallyExclusive("author", "mine")`, plus
  `("commit", "author")` and `("commit", "mine")`. `--repository` interaction is the runtime
  guard above (persistent parent flag, invisible to flag groups at `init()`).
  `--state`/`--query`/`--source`/`--destination` all remain valid in author mode.
- **Read-only command**: keep the plain `common.WhatIf` gate (no `WhatIfPayload` — no write).
- **Sorting**: the new column's `Compare` sorts by `Destination.Repository.FullName`
  (nil-safe, lowercase). Default sorter stays `+id` (changing it would alter repo-scoped
  behavior) even though ids repeat across repos in author mode — SKILL.md recommends
  `--sort repository` / `--sort updated_on` there.

## What Goes Where

- **Implementation Steps** (`[ ]`): code, tests, SKILL.md — all in this repo.
- **Post-Completion** (no checkboxes): manual smoke test against the real Sportscape
  workspace.

## Implementation Steps

### Task 1: Author-mode branch in `pullrequest list`

**Files:**
- Modify: `internal/pullrequest/list.go`
- Create: `internal/pullrequest/list_author_test.go`
- ➕ Modify: `internal/profile/error.go`, `internal/profile/profile_client.go`,
  `internal/profile/profile_client_test.go` (see the scope note below)

- [x] register `--author` (string) and `--mine` (bool) on `listCmd`; mark `author`/`mine`,
      `commit`/`author`, `commit`/`mine` mutually exclusive
- [x] add the shared author-mode helper (nil-safe typed flag reads; unset bool must read as
      false, not `"false" != ""`) used by `listProcess` now and `GetHeaders` in task 2
- [x] add the author-mode branch to `listProcess` as the FIRST switch arm: the
      `--repository`-explicitly-set runtime guard, workspace resolution via
      `workspace.GetWorkspaceName`, author resolution (`--mine` → `user.GetMe` →
      `me.ID.String()` verbatim; `--author` → `ValidatePathIdentifier` + `url.PathEscape`),
      path `/workspaces/<ws>/pullrequests/<author>` with the existing `state`/`q` query
      values, author-specific `WhatIf` message, 404 wrapped with the accepted-forms +
      `bb workspace members` guidance; read both flags direct-off-`cmd`
- [x] write tests: httptest server asserting exact request path (`r.URL.EscapedPath()`) +
      query for `--author` with plain and `{uuid}` forms, repeated `--state`, and
      `--query`/`--source` composition
- [x] write tests: `--mine` resolves via a stubbed `/user` response and hits
      `/workspaces/<ws>/pullrequests/%7B<uuid>%7D` (no doubled braces)
- [x] write tests: query-injection — `--author 'x?state=MERGED'` is escaped so our own
      `state`/`q` params survive (assert on the received `RequestURI`)
- [x] write tests: error cases — `--author` failing `ValidatePathIdentifier` (contains `/`),
      unresolvable workspace, explicit `--repository` + `--mine` runtime rejection, 404
      author with the actionable message
- [x] write tests: mutual-exclusivity via the real `listCmd` + `ValidateFlagGroups()`
      (NOT via `listProcess` — cobra only enforces groups during Execute)
- [x] run tests — must pass before task 2

➕ **Scope note (task 1):** the 404-specific wrap needed a way to *recognize* a 404, which the
client did not expose — `mapErrorResponse` produced either a `*BitBucketError` (no status field)
or a bare `fmt.Errorf("cannot send request: %s", StatusText)`. Added the minimum plumbing for it
in `internal/profile`: a `StatusCode int json:"-"` field on `BitBucketError` (set by
`mapErrorResponse`, not by the payload), an unexported `statusError{StatusCode, StatusText}`
replacing that `fmt.Errorf` with a byte-identical message, and an exported
`profile.IsNotFound(err)` matching either shape. Pinned by
`ProfileSuite.TestIsNotFoundRecognizesBothErrorShapes` (404 with and without a JSON body, plus a
403 and a 500 negative). The pre-existing tests asserting a non-JSON error body is *not* a
`*BitBucketError` still hold — `statusError` is a distinct type.

➕ **Structure note (task 1):** repository resolution used to run before `listProcess`'s `switch`,
so author mode (which must resolve no repository at all) could not simply become another `case`.
The switch was factored into two request builders instead — `listAuthorRequest` and
`listRepositoryRequest`, both returning `(uripath, target string, err error)` — with the shared
`state`/`q` query build extracted into `listQuery`. `target` carries the debug/dry-run subject, so
the repo-scoped lines stay byte-identical (`"repository: <repo>"`) while author mode reports
`"workspace: <ws>, author: <author>"`. Also: the 404 wrap is gated on `--author` specifically, not
on author mode generally — with `--mine` the identifier is resolved by `bb` itself, so a 404 there
is about the workspace and the accepted-author-forms guidance would misdirect.

### Task 2: `repository` column

**Files:**
- Modify: `internal/pullrequest/pullrequest.go`
- Modify: `internal/pullrequest/pullrequest_row_test.go`
- ➕ Modify: `internal/pullrequest/endpoint.go`, `internal/pullrequest/list_test.go` (see the
  scope note below)

- [x] add `repository` to `columns` with a nil-safe `Compare` on
      `Destination.Repository.FullName`
- [x] add the `repository` case to `GetRow` (nil `Destination.Repository` →
      `common.EmptyCell`)
- [x] extend `GetHeaders` to call the task-1 helper; author mode defaults to
      `ID, Title, repository, source, destination, state`; `get` and repo-scoped `list`
      defaults unchanged
- [x] update `TestPullRequestGetRowCoversEveryColumn`: add `"repository"` to its HARDCODED
      column list and populate `Destination.Repository` in its fixture (the guard does not
      read the `columns` table, so skipping this ships the column uncovered)
- [x] write tests: GetRow renders `full_name` and EmptyCell for nil repository; GetHeaders
      defaults with author mode on, off, and on a `cmd` lacking the flags entirely (the
      negative case guards the bool-flag detection bug)
- [x] write tests: `--columns repository` and `--sort repository` work on repo-scoped list
- [x] run tests — must pass before task 3

➕ **Scope note (task 2):** the nil-safe access is an unexported `Endpoint.repositoryFullName()`
in `internal/pullrequest/endpoint.go` rather than two inline nil checks, so the column's `Compare`
and its `GetRow` case cannot drift. `GetRow` renders `common.EmptyCell` for an empty full name too,
not only for a nil `Repository` — a non-nil endpoint repository without `full_name` would otherwise
render a blank cell instead of the shared filler. The `--columns repository`/`--sort repository`
test went into `internal/pullrequest/list_test.go` (internal package) instead of
`pullrequest_row_test.go`: it drives the REAL `listCmd` singleton, which needs that file's
`setRealListFlag` cleanup helper. It also needed a `setRealColumnsFlag` sibling — `--columns` is an
`EnumSliceFlag`, whose `Set` appends rather than replaces and whose bracketed `String()` it will not
accept back, so `setRealListFlag`'s Value/DefValue restore does not work for it.

### Task 3: Documentation (SKILL.md)

**Files:**
- Modify: `skill/bitbucket-cli/SKILL.md`

- [x] document `--author`/`--mine` on the `bb pullrequest list` entry: workspace-wide scope,
      no repository needed (and explicit `--repository` is rejected), accepted author forms
      (`{uuid}` / Atlassian ID; usernames mostly defunct, nicknames from
      `bb workspace members` do NOT work — no `me` sentinel), exclusivity with `--commit`,
      `repository` default column in author mode, `--sort repository`/`updated_on`
      recommendation (default `+id` is meaningless cross-repo), and the workspace-level
      token-scope caveat (a Repository Access Token 403s here)
- [x] run the internal/cmd sync-guard test to confirm nothing else drifted
- [x] run tests — must pass before task 4

➕ **Note (task 3):** the author-mode documentation went in as its own bullet after the
repository-scoped `bb pullrequest list` bullet rather than being folded into it — the repo-scoped
entry stays a short, unchanged description of the default behavior, and author mode gets the room
its caveats (accepted identifier forms, `--repository` rejection, default columns, sort advice,
workspace-level token scope) need. The nickname warning points at `bb workspace members`' `ID`
column as the source of the braced UUID (verified: `internal/workspace/member.go` renders
`member.User.ID.String()` there, which `common.UUID.String()` already brace-wraps).

### Task 4: Verify acceptance criteria

- [x] verify all requirements from Overview are implemented (cross-repo listing, unchanged
      repo-scoped default output, works outside a Bitbucket checkout with `--workspace`)
- [x] run full suite: `make test` (`go test -race ./...`)
- [x] run `make lint` (golangci-lint v2.12.2) and `make cross-build`
- [x] `gofmt -l .` clean

➕ **Verification record (task 4):** all four gates green, no code changes needed, so task 4
produced no commit of its own. Requirement-by-requirement:
- cross-repo listing — `listAuthorRequest` (`internal/pullrequest/list.go:149`) builds
  `/workspaces/<ws>/pullrequests/<escaped-author>` with the shared `listQuery` state/`q` values;
  covered by `TestListProcessAuthorMode` (atlassian id, braced uuid, repeated `--state`,
  `--query`+`--source` composition) and `TestListProcessMineResolvesCurrentUser`.
- unchanged repo-scoped default output — `GetHeaders` (`internal/pullrequest/pullrequest.go:159`)
  only swaps in the `repository`-bearing default set when `authorModeValue` reports author mode;
  `pullrequest get` and plain `list` keep their existing defaults, pinned by the GetHeaders
  default tests plus `TestAuthorModeValue`'s not-registered / registered-but-unset /
  explicitly-false cases.
- works outside a Bitbucket checkout — no `repository.GetRepository` call on the author path, and
  an explicitly-passed `--repository` is rejected; covered by
  `TestListProcessAuthorModeNeedsNoRepository` and `TestListProcessAuthorModeErrors`.
- gates: `make test` (`go test -race ./...`) all packages ok; `make lint` 0 issues on
  golangci-lint 2.12.2 (version confirmed, matching CI); `make cross-build`
  (`GOOS=linux CGO_ENABLED=0` build + vet) clean; `gofmt -l .` empty.
- ⚠️ tooling note: `go`, `gofmt`, and `golangci-lint` are not on the default non-login `PATH` in
  this environment — they live in `/opt/homebrew/bin` and `~/go/bin`, which must be prepended
  before any `make` target will run.

### Task 5: [Final] Wrap up

- [x] update CLAUDE.md's supported-surface description of `bb pullrequest list` if its wording
      needs the new flags
- [x] move this plan to `docs/plans/completed/` and `git add -f` it with the PR

➕ **Note (task 5):** the CLAUDE.md edit extends the `bb pullrequest` entry in the "What this is"
surface enumeration in place (author mode is a property of `pullrequest list`, not a new command
group, so it does not warrant its own paragraph): workspace-wide scope, the
`/workspaces/{ws}/pullrequests/{user}` endpoint, no repository resolution, `--repository`
rejected, and `repository` in the default column set. The Layout entry for
`internal/pullrequest/` needed no change — the author path lives in the same `list.go`.

⚠️ **Working-tree note (task 5):** CLAUDE.md already carried an unrelated uncommitted change
before this plan started — a tool-generated `<!-- jbcontext-instructions-start -->` block appended
at the end of the file, describing the `jbcontext`/`context-explorer` code-discovery workflow. It
has nothing to do with this feature, so it was deliberately NOT included in this commit: only the
surface-description hunk was staged (via `git apply --cached`), leaving the jbcontext block in the
working tree for its author to commit separately.

### Review follow-up

➕ **Note (code review):** a post-implementation review found and fixed, in one follow-up commit:

- *security/correctness:* the workspace segment of the hand-built author-mode path was neither
  validated nor escaped (only the author segment was), so `--workspace 'acme?state=MERGED'`
  retargeted the request and dropped `bb`'s own `state`/`q`, and `acme/../otherws` reached another
  workspace through `JoinPath`'s dot-segment resolution. Now `ValidatePathIdentifier` +
  `url.PathEscape`, like the author segment.
- *correctness:* author mode is now detected via `Flags().Changed("author") || mine` instead of
  `author != ""`, so an explicitly empty `--author ""` fails with "argument author is missing"
  rather than silently listing the current repository's pull requests by every author (and slipping
  past the `--repository` guard).
- *correctness:* the 404 guidance is gated on author mode (not on `--author` being non-empty) and
  names the resolved target (`workspace: X, author: Y`), so a bad workspace is no longer reported as
  a bad `--author`, and `--mine`'s 404 gets guidance too — without the accepted-`--author`-forms
  paragraph, which its user never typed a value for.
- *correctness:* `--mine` rejects a current user carrying no uuid instead of requesting the
  well-formed-but-nonexistent `{00000000-...}`.
- *correctness:* the `id` and `repository` sorters tie-break on each other (`common.Sort` is not
  stable, and ids are only unique per repository in a cross-repository listing).
- *simplification:* `profile.BitBucketError.StatusCode` is gone; `mapErrorResponse` wraps every
  non-2xx error in the single `statusError{StatusCode, err}` carrier (`Error()` verbatim, `Unwrap()`
  to the payload), so `IsNotFound` is one `errors.As`. Also: `GetRow`'s repository case reuses
  `emptyCellIfBlank`, `endpointRepositoryName` became `Endpoint.repositoryName` beside
  `repositoryFullName`, the redundant `Lookup("mine")` guard is gone, and the pre-read `states`
  parameter no longer threads through three call levels.
- *tests:* workspace escaping, empty `--author`, workspace dot-segment/separator rejection,
  `--mine`'s `GetMe` failure and uuid-less user, the `--mine` 404 message, an `--output csv`
  author-mode listing proving the `repository` column reaches the rendered table, the `--commit`
  arm's success path, author-mode-wins-over-`--commit`, `IsNotFound(nil)`/`errors.Join`, and
  byte-identical error messages for both mapped shapes. `TestPullRequestGetRowCoversEveryColumn`
  moved into the package so it iterates `columns.Columns()` instead of a hand-kept name list;
  `TestListCmdAuthorModeRegistration` merged into `TestListCmdRealRegistration`, which now asserts
  the exclusivity error names both flags of the pair; `--mine` tests use a per-test `user.UserCache`
  so `-count=2` passes; the "unresolvable workspace" subtest chdirs to a scratch remote-less
  `.git/config` instead of depending on the developer's own remotes.
- *docs:* README gained the author-mode subsection (plus corrected `read:user`/`read:workspace`
  scope bullets and the `--repository`-rejection note), SKILL.md no longer implies `--sort` exists
  on `pullrequest get`, and CLAUDE.md documents the hand-built-uripath rule, the `WhatIf`-before-
  resolution exception, and the `internal/profile` status-classification surface.

➕ **Note (style/code-smell review):** a later convention pass fixed, in one more commit:

- *convention:* the author-mode 404 message no longer embeds a literal `\n` (guidance follows after
  `; `, so a one-line log never loses it) and no longer uses ` -- ` as an em dash next to flag names.
- *correctness:* `listQuery` returns the states it resolved and both request builders hand them back
  in a `listRequest`, so one invocation reads `--state` exactly once — the mis-registered-flag
  `[WARN]` fired twice before. Flags are still read direct-off-`cmd`: the resolved values travel
  *up* as results, never *down* as pre-read parameters.
  `TestListProcessResolvesStatesOnce` pins the single warning.
- *tests:* `withScratchUserCache`/`loadPullRequestsFixture` moved to `action_test.go` (shared harness
  next to `setupTestNamed`, now that `create_test.go` uses the former and `list_test.go` the latter);
  `withAuthorFlags` → `withAuthorModeFlags` (it also registers `--workspace`);
  `TestListProcessAuthorNotFoundLeavesOtherErrorsAlone` → `...AuthorNon404ErrorsAreNotDecorated`
  (it asserts a 403); four overlapping `internal/profile` suite tests collapsed into the
  `IsNotFound` table plus two byte-identity one-liners inside the tests that already served those
  responses; `columns_coverage_test.go` → `pullrequest_row_internal_test.go` (sibling packages'
  `<type>_row_test.go` naming).
- *quality:* `emptyCellIfBlank` moved from `activity.go` to `pullrequest.go`, beside
  `PullRequest.GetRow`, since it is a package-wide render helper rather than an activity one.
- *docs:* README no longer claims `--sort repository` works on `pullrequest get` (only `--columns`
  does; `--sort` is a `list` flag).

## Post-Completion

**Manual verification:**
- run `bb pullrequest list --mine` and `bb pullrequest list --author '{<uuid>}'` against the
  real Sportscape workspace from (a) a Bitbucket checkout and (b) a non-Bitbucket directory
  with `--workspace` set; confirm the `repository` column renders, `--state all` works, and
  the 404 message reads well for a nickname-shaped `--author` value
- confirm behavior with the workspace-scoped vs repository-scoped token variants actually in
  use (403 path)
