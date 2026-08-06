# Restore Five Feature Groups (pre-release)

## Overview

Restore five task groups to the modernized bitbucket-cli, adapted to every convention the
modernization established — six command groups in total, since task group 4 below covers two
separate CLI command groups (`commit` and `branch`) as one unit of work; README/CLAUDE.md count
command groups, not task groups, hence "six" there. Owner decision (2026-08-05): these land
BEFORE the first release. Scope per group (deliberately narrower than upstream —
admin/destructive verbs stay dead):

1. **pipeline** — `get`, `list`, `trigger`, `stop` + `step` subgroup (`get`, `list`, `logs`,
   `report`, `cases`). `trigger`/`stop` gain a y/N confirmation unless `--force`. NO `--tag`
   on trigger (tag package stays dead); NO `--show-logs-command`.
2. **repository** — read-only: `get`, `list` (then `clone` as its own follow-up task). NO
   create/delete/fork/update, NO `get --forks`, list filters limited to `--role`.
3. **workspace** — read-only: `get`, `list`, `members`. NO permission admin.
4. **commit + branch** — read-only: `commit get/list/diff/patch`; `branch list`. NO mutation.
5. **artifact** — `list`, `download`. NO upload/delete, NO `--progress`.

Source for recovered code: pre-trim commit `58a098e` (`git show 58a098e:cmd/...`). Everything
recovered MUST be translated to current conventions — the upstream code predates every phase-1..4
review fix and will not compile as-is (it uses go-git, gildas/go-errors, go-logger, go-request
types, viper, cmd/ paths). internal/pullrequest is the reference translation for every pattern.

## Context (verified against 58a098e and current master 86e921c)

- Conventions: repo CLAUDE.md (read first). History: docs/plans/completed/20260805-umputun-modernization.md,
  /tmp/progress-umputun-modernization.txt.
- Surviving library packages (internal/repository, workspace, commit, branch) kept ONLY their
  core types and the getters pullrequest needs. The trim stripped: all `columns` tables,
  `GetHeaders`/`GetRow` (except Commit's), all collection Tableables (Workspaces, Members,
  Branches, Repositories), and getters `GetRepositories*`, `GetRepositorySlugs`,
  `GetRepositoryAllowedSlugs`, `GetCommits*`, `GetCommitHashes`, `GetCurrentBranch`,
  `common.Selector`. Tasks below re-add what each command needs — this is real work, not just
  command files.
- internal/pipeline and internal/artifact do not exist — full recovery + adaptation.
- HARD BLOCKERS in upstream code (resolved by decisions below — do NOT re-add deps):
  (a) upstream `branch.GetCurrentBranch` and the real `repo clone` are go-git code (dep dropped
  permanently); (b) `cmd/pipeline/trigger.go` imports the deleted `tag` package for `--tag`;
  (c) upstream `artifact download` calls `profile.Download`, which does not exist in the
  stdlib-http client. New-dep temptations that the module-count gate exists to block: go-git,
  progressbar/spinner, golang.org/x/term.

## Development Approach

- One task = one PR, sequential, branch from fresh master, never push master directly.
- PR workflow: branch → implement → validate (go build ./... && go vet ./... && gofmt -l . &&
  go test -race ./... && golangci-lint run && goreleaser check) → push →
  `/opt/homebrew/bin/gh pr create --base master` (no "Test plan") → `gh pr checks --watch`
  FOREGROUND until green → `gh pr merge --squash --delete-branch` → sync master.
- **SHELL SAFETY (mandatory, incidents occurred)**: commit messages via `git commit -F
  <tempfile>` only; `git add <explicit paths>` only — never `-A`/`.`; `git status --short`
  before every commit; never commit .claude/ or docs/plans/ (except docs/plans/completed/).
- Comments: lowercase except godoc, current behavior only, no history/AI mentions.
- Workers on sonnet; plan/review agents on opus (sonnet fallback on 529).

## MANDATORY bug-class fixes for ALL recovered code

The upstream code contains named instances of every bug class fixed in review phases 1–4.
Every task below must apply these while porting — a mechanical translation reintroduces them:

1. **GetRow/GetHeaders/columns coherence**: every restored GetRow uses
   `common.NormalizeColumnKey(header)` + `default: row = append(row, " ")` (reference:
   internal/commit/commit.go). The `columns` table is the single source of truth; row tests
   iterate `columns.Columns()` (not a hand-written list). Known upstream instances: duplicate
   `"creator"` entry in pipeline columns (dedupe); Pipeline/Artifact/Branch/Workspace GetRow
   missing default arm; step columns↔GetRow↔GetHeaders disagree three ways (`name` missing from
   columns; `started on`/`completed on`/`run number`/`max time` in GetRow but not columns;
   `logs-command` synthesized — dropped with the feature).
2. **Time formats**: `common.TableTimeFormat` / `common.JSONTimeFormat` only; zero timestamps
   render the `" "` filler (upstream Step.GetRow prints 0001-01-01T00:00:00Z).
3. **Sort policy**: never call `core.Sort` with an empty sorter — guard on
   `cmd.Flag("sort").Changed` (the pipeline-list pattern) in EVERY restored list command
   (upstream artifact/step/branch list sort unconditionally).
4. **No silent no-op successes**: empty list prints "No <thing> found" on stdout (upstream
   artifact/step list log-only; step's copy says "No comment found" — fix the noun).
5. **Secrets never logged**: trigger must NOT log the TriggerBody payload (contains --variable
   values, routinely tokens; `Variable` even has `Secured`) — log keys only, values via
   redactWithHash-style handling; clone must never log a URL carrying userinfo; reject empty
   keys from `SplitN(v, "=", 2)`.
6. **Error-processing semantics**: multi-arg loops use the FIXED ShouldStopOnError/
   ShouldWarnOnError/ShouldIgnoreErrors matrix + errors.Join (reference:
   internal/pullrequest/create.go tolerateReviewerErrors flow) — never upstream's MultiError.
7. **Completion/allowed-value getters use `profile.GetAllUnbounded`** (never GetAll — the
   --limit truncation fix; precedent: internal/pullrequest/common/getters.go).
8. **Dynamic flags**: `EnumFlagWithFunc` validates by API call during flag parse — acceptable
   ONLY for small enumerations (--branch names, --columns, --sort). Unbounded identifier spaces
   (--pipeline build numbers, --commit hashes) use plain StringVar + RegisterFlagCompletionFunc.
9. **Drop dead upstream code, don't translate it**: empty `Validate()` methods building unused
   MultiError; `Step.MarshalJSON` writes `created_on` for StartedOn while UnmarshalJSON reads
   `started_on` and CreatedOn is never used (fix the round-trip; add a marshal/unmarshal test);
   upstream clone's unused `--password` flag; step's BuildNumber/ShowLogsCommand `json:"-"`
   hack fields.
10. **`stop`/`trigger` outcomes**: no unconditional "stopped successfully" print — report what
    the API returned.

## Technical Details

- **Confirmation helper** (Task 4b): one `common.Confirm(cmd *cobra.Command, prompt string)
  (bool, error)` in internal/common. gh-style `y/N`. Reads `cmd.InOrStdin()` (tests do
  `cmd.SetIn(strings.NewReader("y\n"))`). Non-interactive detection (`os.Stdin.Stat()` &
  ModeCharDevice) applies only when InOrStdin() IS os.Stdin; non-interactive without --force =
  error. `--dry-run` short-circuits BEFORE the prompt. `--force` local bool on trigger and stop.
  Required tests: confirm-yes; confirm-no (zero handler hits); --force skips prompt; --dry-run
  without --force/TTY does not error; non-interactive without --force errors. README notes these
  are the fork's only confirmed commands (deliberate asymmetry vs pr merge/decline).
- **Flat Target** (Task 4a): do NOT recover upstream's Target interface + TypeRegistry + three
  structs + error-string matching (violates stdlib-errors rule; four files for two needs). One
  flat struct: `type Target struct { Type string; RefType, RefName string; Selector *Selector;
  Commit *commit.CommitReference; Source, Destination string; PullRequestID uint64 }` (~40
  lines, json omitempty, one GetDestination()). This also removes the pipeline→pullrequest
  import edge. Recover `internal/common/selector.go` from 58a098e (7 lines).
- **plcommon**: `pipeline` imports `step` (AddCommand), so step cannot import pipeline —
  a small `internal/pipeline/common` getters package breaks the cycle, mirroring
  internal/pullrequest/common. Do not flatten it.
- **GetCurrentBranch** (Task 3, dep-free): read HEAD via the worktree-aware git dir
  (internal/common/git_config.go's resolveWorktreeGitDir) or `exec.Command("git",
  "symbolic-ref", "--short", "HEAD")` — explicit argv, no shell. Tests: normal branch, detached
  HEAD (error), not-a-repo (error). Consumed by pipeline trigger's default-branch behavior.
- **clone** (Task 2b, from-scratch rewrite — upstream's is go-git): single portable file,
  `exec.Command("git", "clone", url, destination)` explicit argv, no shell, Stdout/Stderr wired
  through. Protocol resolution: `--protocol` → profile.CloneProtocol → `git`. SSH key via
  GIT_SSH_COMMAND when `--ssh-key-file`/profile.SshKeyFilename set. CloneProtocol/CloneUser/
  SshKeyFilename become functional — Task 2b updates the README/CLAUDE.md vestigial-fields notes
  (Progress and DefaultProject stay inert; Task 6 owns the final wording). Test: PATH-shim git
  (t.TempDir bin dir) asserting exact argv + protocol precedence + no userinfo in logs.
- **artifact download** (Task 5, new client path — no profile.Download exists): stream the
  response body to the destination file with io.Copy. Explicit design items: Bitbucket
  `/downloads/<name>` 302-redirects to bbuseruploads — TEST (not assume) that Authorization is
  not forwarded cross-host (Go strips it on cross-domain redirect); `filepath.Base` the artifact
  name before joining `--destination` (path traversal); destination default cwd; document
  create/overwrite behavior; multi-arg loop uses the error-tolerance matrix.
- **step logs/report/cases**: `Profile.GetRaw` returns io.Reader — upstream's
  `io.Copy(os.Stdout, r)` ports unchanged (body is buffered by send(); genuine streaming out of
  scope — note tradeoff in PR body). Add the missing `--dry-run` WhatIf check (upstream skipped it).
- **Flag-hiding helpers**: disableUnsupportedFlags/hideUnsupportedFlags are unexported in
  internal/profile — export equivalents to internal/common in the first task that needs them
  (Task 2) rather than duplicating.
- **Fixtures** (recover only what a test references): pipeline.json, pipeline-step.json,
  pipeline-pullrequest.json. The three target fixtures are unnecessary under the flat Target.
  tags.json stays dead (no --tag).

## Progress Tracking

- mark [x] immediately; ➕ discovered tasks; ⚠️ blockers; record PR numbers per task heading.

## Implementation Steps

### Task 1: Restore workspace read-only commands (PR) — PR #21 (merged)

**Files:**
- Create: `internal/workspace/{get,list,members}.go` + tests
- Modify: `internal/workspace/workspace.go`, `internal/workspace/member.go` (columns/GetHeaders/GetRow + Workspaces/Members Tableables re-added), `internal/cmd/root.go`

- [x] re-add Workspace + Member table surface (columns tables, GetHeaders/GetRow per bug-class
      rules 1–2) and collection Tableables; recover/adapt `workspace get`, `workspace list`
- [x] add `workspace members <slug>` subcommand (read-only Member rows via GetMembers)
- [x] register workspace.Command in root.go
- [x] tests: success/API error/dry-run per command; row tests iterating columns.Columns();
      sort-guard test (rule 3); empty-list message test (rule 4)
- [x] full gate green; PR, CI, merge

➕ `get`/`members` take an optional `<workspace-slug-or-id>` and resolve the current workspace
   when omitted, rather than upstream's `get --members`/`--member` flags; `members` is its own
   subcommand per the plan.
➕ added `internal/workspace/flags.go` (`registerListFlags`, a small generic helper): `list` and
   `members`'s flag-registration `init()` blocks were near-identical and tripped `dupl`.
➕ applied rule 7 to the existing `GetWorkspaceAllowedSlugs` completion getter too (not just new
   code): switched it from `profile.GetAll` to `profile.GetAllUnbounded`.
➕ internal test files (`get_test.go`, `list_test.go`, `members_test.go`, `workspace_row_test.go`)
   are `package workspace` (need unexported access) and therefore cannot import
   `internal/testutil` (it imports `internal/workspace`, which would cycle) — added a local
   `helpers_test.go` harness instead, mirroring `internal/user/helpers_test.go`'s existing
   precedent for the same constraint.

### Task 2: Restore repository read-only commands (PR) — PR #22 (merged)

**Files:**
- Create: `internal/repository/{get,list}.go` + tests
- Modify: `internal/repository/repository.go` (columns/GetHeaders/GetRow + Repositories Tableable + GetRepositories/GetRepositorySlugs getters re-added), `internal/common/` (exported flag-hiding helpers), `internal/cmd/root.go`

- [x] re-add Repository table surface + Repositories Tableable + getters (GetRepositorySlugs
      uses GetAllUnbounded — rule 7)
- [x] recover/adapt `repo get` (NO --forks) and `repo list` (flags: --role + standard
      --columns/--sort/--page-length/--limit only; defer upstream's 7 query filters)
- [x] export flag-hiding helpers to internal/common; register repository.Command (alias `repo`)
- [x] tests per rules 1–4 + completion test for repo slugs
- [x] full gate green; PR, CI, merge

➕ exporting the flag-hiding helpers (`common.DisableUnsupportedFlags`/`common.HideUnsupportedFlags`,
   parameterized by command noun and flag names) also let `internal/profile` drop its own
   unexported copies in favor of the shared versions, per the Technical Details note ("rather
   than duplicating") — profile.go now just binds two package vars to the common constructors.
➕ `get`/`list` disable and hide only the root `--repository` flag (not `--workspace`, which
   `list` still uses to pick the workspace), matching upstream's repository-specific
   `disableUnsupportedFlags` rather than profile's broader one.
➕ `Repository.GetRow`'s switch tripped `gocyclo` (18 columns + inline nil/zero checks); split
   into a `getCell` helper plus three one-line helpers (`workspaceName`, `parentFullName`,
   `updatedOnCell`) rather than suppressing the linter.
➕ fixed two upstream comparator bugs while porting the columns table: `has_issues`/`has_wiki`/
   `is_private` used equality (`a.X == b.X`, never reports less-than) instead of a real
   less-than; `parent` had a three-branch nil dance collapsed into one line.
➕ `internal/repository`'s own test files (`package repository`, needing unexported access) can't
   import `internal/testutil` (it imports `internal/repository`, cycle) — added a local
   `helpers_test.go` harness, same constraint and precedent as Task 1's workspace package.

### Task 2b: repo clone — from-scratch exec implementation (PR) — PR #23 (merged)

**Files:**
- Create: `internal/repository/clone.go` + test
- Modify: README.md + CLAUDE.md vestigial-fields notes (clone fields go live)

- [x] implement clone per Technical Details (portable single file, explicit argv, no shell,
      protocol precedence, GIT_SSH_COMMAND, no --password flag, no userinfo in logs — rule 5)
- [x] tests: PATH-shim git asserting exact argv; protocol precedence (--protocol → profile →
      git); ssh-key env; clone failure propagates git's exit error
- [x] update the two doc notes (CloneProtocol/CloneUser/SshKeyFilename functional)
- [x] full gate green; PR, CI, merge

➕ workspace slug for the clone URL is resolved from the fetched `Repository`'s embedded
   `Workspace.Slug`, falling back to splitting `FullName` on `/` when the API response omitted
   the embed — avoids re-deriving workspace resolution logic already owned by
   `GetRepositoryBySlugOrID`.
➕ destination is a second optional positional argument (`clone <slug> [destination]`), not an
   upstream-style `--destination` flag, matching the Technical Details wording exactly.
➕ protocol/ssh-key-file precedence implemented as small pure functions (`resolveProtocol`,
   `resolveSSHKeyFilename`) taking the flag value and profile field directly, rather than reading
   `cmd.Flags().Changed` — simpler to unit-test the precedence matrix in isolation and behaves
   identically since the EnumFlag's zero value is `""` until explicitly set.
➕ `gosec` (G204, subprocess launched with a variable) required a `//nolint:gosec` with an
   explanation on the `exec.CommandContext` call — the repo's zero-exclusion gosec config had no
   prior exec.Command usage to precedent this against; justified inline (explicit argv, no shell,
   no injection vector).

### Task 3: Restore commit + branch read-only commands (PR) — PR #24 (merged)

**Files:**
- Create: `internal/commit/{get,list,diff,patch}.go`, `internal/branch/list.go` + tests
- Modify: `internal/commit/commit.go` (GetCommits/GetLatestCommit/GetCommitByHash/GetCommitHashes re-added), `internal/branch/branch.go` (columns/table surface + Branches Tableable + dep-free GetCurrentBranch), `internal/cmd/root.go`

- [x] re-add commit getters (GetCommitHashes via GetAllUnbounded) and Branch table surface
- [x] implement dep-free `branch.GetCurrentBranch()` per Technical Details (consumed by Task 4b
      trigger — do not reorder past it); tests: normal/detached/not-a-repo
- [x] recover/adapt `commit get/list/diff/patch` (diff/patch via GetRaw) and `branch list`
      (rules 1–4)
- [x] register both Commands in root.go
- [x] tests per rules; diff/patch raw-output tests; row tests iterate columns.Columns()
- [x] full gate green; PR, CI, merge

➕ `commit get` takes an optional `<commit-hash>` and resolves the latest commit when omitted,
   mirroring `repo`/`workspace get`'s "resolve current when omitted" pattern (upstream had a
   dead `if commit == ""` check behind `cobra.ExactArgs(1)`, dropped per rule 9).
➕ `GetLatestCommit` deliberately does NOT touch the local git working directory the way
   upstream's go-git-based version did: it requests a single-item page from BitBucket's commits
   endpoint (newest first) instead, keeping the commit package free of any git dependency beyond
   `GetCurrentBranch`'s own `exec.Command`. `GetCommitByHash` uses the singular `/commit/{hash}`
   endpoint directly rather than upstream's `/commits/{hash}` list-endpoint workaround.
➕ `branch.GetCurrentBranch(ctx) (string, error)` shells out to `git symbolic-ref --short HEAD`
   (explicit argv, no shell — no `nolint:gosec` needed since every argv element is a literal, not
   a variable, so gosec's G204 does not fire here unlike clone.go's).
➕ applied rule 7 to the existing `GetBranchNames` completion getter too (not just new code):
   switched it from `profile.GetAll` (via `GetBranches`) to `profile.GetAllUnbounded`, factoring
   the shared URI-building logic into an unexported `branchesQuery` helper so `GetBranches` (used
   by `branch list`, still bounded) and `GetBranchNames` don't duplicate it.
➕ no `PreRunE: common.DisableUnsupportedFlags` needed on any of these commands: unlike
   `repo`/`workspace get`, none of them take a positional argument that would conflict with the
   root `--repository` flag — commit/branch commands always scope to the current repository.
➕ `internal/commit` and `internal/branch`'s test files (`package commit`/`package branch`, need
   unexported access) CAN import `internal/testutil` directly without a cycle: unlike
   `internal/repository`/`internal/workspace` (which `testutil` itself imports), nothing in
   `testutil`'s own dependency graph imports `commit` or `branch`. No local-cache harness was
   needed for either package — only a thin package-local `setupTest` wrapping
   `testutil.SetupProfile`/`PrimeFixtureCaches` to add each package's own `query`/`columns`/`sort`
   (and commit's `include`/`exclude`) flags.

### Task 4a: Pipeline core — types, get, list (PR) — PR #25 (merged)

**Files:**
- Create: `internal/pipeline/{pipeline,get,list,variable,state,stage,result}.go`, flat `target.go`, `internal/pipeline/common/getters.go`, `internal/common/selector.go` + tests + fixtures (pipeline.json, pipeline-pullrequest.json)
- Modify: `internal/cmd/root.go`

- [x] recover/adapt Pipeline type + state/stage/result + Variable; FLAT Target per Technical
      Details (no registry, no error-string matching); recover selector.go
- [x] plcommon getters package (GetPipelineIDs via GetAllUnbounded — rules 7–8; do not flatten,
      cycle rationale in Technical Details)
- [x] `pipeline get` + `pipeline list` (columns dedupe — duplicate "creator"; rules 1–4)
- [x] register pipeline.Command in root.go
- [x] tests: get/list matrices; Pipeline marshal/unmarshal round-trip; row tests iterate
      columns.Columns()
- [x] full gate green; PR, CI, merge

➕ added `internal/pipeline/pipelines.go` (the `Pipelines` collection `Tableables`) as its own
   file, beyond this task's literal Files list, mirroring `internal/repository/repositories.go` and
   `internal/commit/commits.go`'s established split rather than bundling it into `pipeline.go`.
➕ `PullRequestID` on the flat `Target` is tagged `json:"-"` and handled entirely by
   `Target.MarshalJSON`/`UnmarshalJSON`, which fold it into/out of a nested
   `{"type":"pullrequest","id":...}` object — the one piece of BitBucket's `pullrequest` payload
   this fork keeps, matching the Technical Details' "plain uint64 id ... suffices" call.
➕ `pipeline list`'s `--query` value is read via a small `queryFlagValue(cmd)` helper (reads
   `cmd.Flag("query")` directly) rather than the package-level `listOptions.Query` StringVar
   destination, so `listProcess` behaves identically whether `cmd` is the real `listCmd` or a
   standalone test command carrying its own `--query` flag (mirrors `internal/commit/commits.go`'s
   `addStringQueryFilter` pattern, which has the same reason).
➕ `internal/pipeline`'s test files (`package pipeline`, need unexported access) can import
   `internal/testutil` directly without a cycle, same as `internal/commit`/`internal/branch` (Task
   3's precedent) — `testutil` and its own dependency graph never import `internal/pipeline`.
➕ gosec (G115, uint64→int64 conversion) on `Duration = time.Duration(inner.DurationInSeconds) *
   time.Second` in `Pipeline.UnmarshalJSON` required a `//nolint:gosec` with an inline explanation
   (BitBucket's `duration_in_seconds` is a pipeline runtime in seconds, nowhere near the ~292
   billion year range where the conversion could overflow) — same zero-exclusion gosec config
   precedent as `repository/clone.go`.

### Task 4b: pipeline trigger + stop with confirmation (PR) — PR #26 (merged)

**Files:**
- Create: `internal/pipeline/{trigger,stop}.go`, `internal/common/confirm.go` + tests

- [x] implement `common.Confirm` per Technical Details
- [x] `trigger`: --branch (default: GetCurrentBranch), --commit (StringVar + completion — rule
      8), --pullrequest, --variable KEY=VALUE (reject empty key; NEVER log values — rule 5); NO
      --tag; flat Target payload; confirmation + --force; --dry-run before prompt
- [x] `stop`: confirmation + --force; report actual API outcome (rule 10)
- [x] tests: the five confirmation cases (Technical Details) for BOTH commands + payload
      assertions (variables present, values never in logs) + API error paths
- [x] full gate green; PR, CI, merge

➕ `whatif.go`'s dry-run flag check was factored into a shared unexported `isDryRun(cmd)` helper,
   reused by both `WhatIf` and `Confirm`, so the two never disagree on what counts as dry-run and
   `Confirm`'s dry-run short-circuit doesn't duplicate `WhatIf`'s own "Dry run: ..." message.
➕ trigger's --branch and --commit are both plain string flags read directly off `cmd` at process
   time (via `cmd.Flags().GetString(...)`), each with its own `RegisterFlagCompletionFunc` adapter
   rather than an `EnumFlag`/`EnumFlagWithFunc` bound to the package-level `triggerCmd` instance —
   mirrors `list.go`'s `queryFlagValue` rationale exactly: an `EnumFlag` constructed via
   `NewEnumFlagWithFunc(triggerCmd, ...)` closes over that specific command instance for its
   `AllowedFunc` context, which fights a standalone test command carrying its own flags. Reading
   every trigger flag (branch/commit/pullrequest/variable/force) directly off the passed `cmd`
   keeps `triggerProcess` identical whether `cmd` is the real `triggerCmd` or a test double.
➕ `--pullrequest` builds a minimal `pipeline_pullrequest_target` carrying only the pull request id
   (`Target.PullRequestID`) — BitBucket derives source/destination/commit server-side, so `Source`/
   `Destination` stay unset (omitted by the flat Target's `omitempty` tags) rather than resolving
   the pull request first to fill them in.
➕ non-interactive/dry-run confirmation tests swap the package-level `os.Stdin` for an `os.Pipe`
   read end instead of relying on the ambient test environment's own stdin: empirically, `/dev/null`
   itself reports as a character device in Go's `os.ModeCharDevice` check, so a redirect from it
   would not reliably exercise the non-interactive path; a pipe's read end reports as a named pipe
   (`ModeCharDevice` unset) regardless of environment. A "poison" `io.Reader` that fails the test if
   ever read (for `--force`) and a never-written-to pipe (for `--dry-run`, so a missed short-circuit
   hangs instead of passing silently) prove the prompt is skipped entirely, not merely declined.
➕ `stop` reports the actual outcome via `profile.PostWithResult`'s `Response.StatusText` (e.g.
   "204 No Content") rather than a hardcoded "stopped successfully" string (rule 10).

### Task 4c: pipeline step subgroup (PR) — PR #27 (merged)

**Files:**
- Create: `internal/pipeline/step/{step,get,list,logs,report,cases}.go` + tests + fixture (pipeline-step.json)

- [x] recover/adapt Step type (fix MarshalJSON/UnmarshalJSON round-trip — rule 9; drop
      BuildNumber/ShowLogsCommand hack fields and --show-logs-command/logs-command feature)
- [x] step columns table as single source of truth (fix the three-way disagreement — rule 1);
      --pipeline as StringVar + completion (rule 8)
- [x] `get/list/logs/report/cases` (logs/report/cases via GetRaw io.Copy + add missing --dry-run)
- [x] tests: matrices per command; Step round-trip test; row tests iterate columns.Columns();
      empty-list noun fixed (rule 4)
- [x] full gate green; PR, CI, merge

➕ added `internal/pipeline/step/steps.go` (the `Steps` collection `Tableables`), beyond this
   task's literal Files list, mirroring `internal/pipeline/pipelines.go`'s established split
   (Task 4a's own precedent for the same reason).
➕ added `internal/pipeline/step/flags.go`: `registerPipelineFlag`/`pipelineFlagValue`/
   `stepValidArgs` shared by all five subcommands, avoiding `dupl` across near-identical
   `--pipeline` flag registration and step-UUID completion.
➕ added `internal/pipeline/step/raw.go`: `logs`/`report`/`cases` share a `rawStepOutput` helper
   (differing only in the URL path suffix and a noun used in messages) — their RunE bodies would
   otherwise be byte-for-byte duplicates (`dupl`).
➕ kept shell completion for the step UUID positional argument (`getStepIDs`, via
   `profile.GetAllUnbounded` — rule 7), matching upstream's `GetPipelineStepIDs`: nothing in the
   plan called for dropping it (unlike `--show-logs-command`, which is explicitly dropped).
➕ `PipelineReference` (the step's embedded `pipeline` object) is defined locally in the `step`
   package rather than added to `plcommon`, since nothing else needs to share it.
➕ the round-trip test (`step_marshal_test.go`) checks specific fields survive an
   unmarshal→marshal→unmarshal cycle rather than `assert.JSONEq` against the whole fixture:
   unlike Task 4a's `pipeline.json` (which already matched the `Pipeline` struct field-for-field),
   `pipeline-step.json` carries `trigger`/`build_seconds_used` keys the `Step` type never modeled
   upstream either, so an exact-bytes fixture comparison isn't the right proof here — the round
   trip of `StartedOn`/`CompletedOn`/`Duration`/`MaxTime` is.
➕ `TestListProcessSortFlagChangedSorts` sorts by `"id"` (the column table's `DefaultSorter`), not
   an arbitrary column, mirroring `internal/pipeline/list_test.go`'s own test: `listOptions.SortBy`
   is a package-level `*common.EnumFlag` bound to the real `listCmd`'s own `"sort"` pflag.Value, so
   a standalone test cmd's separate `"sort"` string flag only ever flips
   `cmd.Flag("sort").Changed` to trigger the guard — the actual sort key used is always
   `listOptions.SortBy`'s own value, which is why the test's chosen sort value must be the default.

### Task 5: Restore artifact list + download (PR) — PR #28 (merged)

**Files:**
- Create: `internal/artifact/{artifact,list,download}.go` + tests
- Modify: `internal/cmd/root.go`

- [x] Artifact type + table surface (rules 1–4) + `artifact list` (GetArtifactNames completion
      via GetAllUnbounded)
- [x] `download` per Technical Details (streaming, redirect Authorization test, filepath.Base
      sanitization, destination semantics documented, error-tolerance matrix over multiple args —
      rule 6)
- [x] register artifact.Command in root.go
- [x] tests: list matrices + row tests; download: content written to t.TempDir, redirect
      cross-host auth-stripping, traversal attempt neutralized, API error, multi-arg tolerance matrix
- [x] full gate green; PR, CI, merge

➕ resolved the hard blocker (upstream `artifact download` calls `profile.Download`, which does not
   exist) by adding a genuinely new, narrow client method, `internal/profile/download.go`'s
   `Profile.Download`: a single request whose response body is `io.Copy`'d straight to the
   caller's `io.Writer`, never buffered into a `[]byte` the way `send`/`GetRaw` do — an artifact
   can be arbitrarily large binary content. Deliberately does not go through
   `doRequestWithRetry` (its retry decision depends on having buffered the full body already,
   which is exactly what this method exists to avoid); a failed download just returns an error.
➕ `download <name...>` streams each artifact into a temp file inside `--destination` first and
   only renames it over the final path once the whole download succeeds, so a failed attempt
   never leaves a stray empty/partial file and never corrupts a pre-existing file at that
   destination; the temp file lives in the same directory as the destination specifically so the
   rename is same-filesystem (avoids a cross-device `EXDEV` rename failure).
➕ proved, rather than assumed, that BitBucket's `/downloads/<name>` cross-host redirect strips
   Authorization: `internal/artifact/download_test.go` runs a real two-server httptest redirect
   (initial host `127.0.0.1`, redirect target host `localhost` — different `Hostname()` strings,
   same loopback interface, no extra network config needed) plus a same-host control test proving
   Authorization *is* forwarded when the host doesn't change, so the cross-host result can't be
   passing for the wrong reason.
➕ added `internal/artifact/artifacts.go` (the `Artifacts` collection `Tableables`), beyond this
   task's literal Files list, mirroring `internal/repository/repositories.go`/
   `internal/pipeline/pipelines.go`'s established split.
➕ `internal/artifact`'s test files (`package artifact`, need unexported access) can import
   `internal/testutil` directly without a cycle, same as `internal/commit`/`internal/branch`/
   `internal/pipeline` (Task 3/4a's own precedent) — `testutil`'s dependency graph never imports
   `internal/artifact`.
➕ `download`'s `--destination` is read directly off `cmd` via a small `destinationFlagValue`
   helper rather than bound to a package-level variable, mirroring `list.go`'s/
   `pipeline/list.go`'s own `queryFlagValue` rationale (Task 4a's precedent): keeps
   `downloadProcess` identical whether `cmd` is the real `downloadCmd` or a standalone test
   command carrying its own `--destination` flag.
➕ kept upstream's single `--query` filter on `list` (unlike `repo list`, which deferred its
   7 query filters as its own explicit scope decision in Task 2): artifact's upstream `list` only
   ever had the one simple `q=` passthrough, matching `pipeline list`'s own kept `--query`.

### Task 6: Docs + acceptance (PR) — PR #29 (merged)

**Files:**
- Modify: `README.md`, `CLAUDE.md`; move this plan to `docs/plans/completed/`

- [x] README: Usage sections for all restored groups (accurate to implemented surface); top
      [!IMPORTANT] scope note lists the new surface and the still-dead groups; confirmation
      asymmetry note; Upgrading-from-0.18.x updated (restored vs still-removed)
- [x] CLAUDE.md: layout map + surface list + final vestigial-fields wording (Progress,
      DefaultProject inert; clone fields live)
- [x] verify: binary lists exactly the intended commands; completion works for new groups;
      module count UNCHANGED vs 86e921c (`go list -m all | wc -l` — go-git/progressbar/x/term
      must NOT have appeared); full gate + goreleaser check
- [x] move this plan to docs/plans/completed/ (git add -f) and commit in this PR
- [x] PR, CI, merge

➕ Acceptance verification results (recorded 2026-08-06, master at c5410b4):
   - `bb --help` lists exactly the intended top-level groups: `artifact`, `branch`, `commit`,
     `completion`, `help`, `pipeline`, `profile`, `pullrequest`, `repo` (alias `repository`),
     `user`, `workspace` — no `project`/`issue`/`tag`/`gpg-key`/`ssh-key`/`cache`/`remote`/
     `component`.
   - Every restored subcommand's `--help` surface matches the plan's scope exactly: `workspace
     get/list/members`; `repo get/list` (list: `--role` only, no other filters)/`clone`
     (`<slug> [destination]` positional, no `--destination` flag); `commit get/list/diff/patch`;
     `branch list`; `pipeline get/list/trigger` (no `--tag`)/`stop`, both with `--force` and a
     `y`/`N` prompt (verified via `internal/common/confirm.go`); `pipeline step
     get/list/logs/report/cases`; `artifact list/download` (no upload/delete/progress).
   - `bb __complete` (cobra's dynamic completion protocol, which is what the generated bash-V2/
     zsh/fish/powershell scripts invoke at runtime) returns the correct subcommand set for every
     restored group (`workspace`, `repo`, `commit`, `branch`, `pipeline`, `pipeline step`,
     `artifact`), confirming completion registration for all of them.
   - `go list -m all | wc -l` is 38, unchanged from 86e921c; `git diff 86e921c HEAD -- go.mod
     go.sum` is empty — go.mod/go.sum are byte-identical to pre-restore master, the strongest
     available proof no new dependency (go-git, progressbar/spinner, golang.org/x/term or
     otherwise) was introduced across all six restore tasks.
   - Full gate: `go build ./...`, `go vet ./...`, `gofmt -l .` (empty), `go test -race ./...`
     (all packages green), `golangci-lint run` (v2.12.2, matching CI's pin — 0 issues),
     `goreleaser check` (`.goreleaser.yml` valid) — all green.

## Post-Completion (owner)

- Add HOMEBREW_TAP_TOKEN secret (fine-grained PAT, write access to avitsrimer/homebrew-apps) to
  avitsrimer/bitbucket-cli Actions secrets.
- Run /release-tools:new to cut the first release (suggest v0.19.0); tag push triggers
  release.yml → goreleaser → GitHub release + cask `bb`.
- Verify: brew tap avitsrimer/apps && brew install --cask bb; bb --version shows the tag.
