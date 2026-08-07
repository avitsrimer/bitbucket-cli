# Field-report fixes FR-5..FR-14 (cycle A, last fixes before first release)

## Overview

Fix the ten owner-triaged findings from the round-2/round-3 agent field reports
(`docs/plans/field-report-findings.md`, FR-5..FR-14). These are the release blockers: a
hard decode failure that blinds `pr activities`, a `--dry-run` that validates nothing,
secret exposure in `profile list/get` table output, missing `--comment-file`/stdin,
missing PR participants, an inconsistent PR-id convention (owner decision: positional
everywhere, hard break), a non-repeatable `pr list --state` and missing branch filters,
no `profile.DefaultRepository`, and pipeline-step name resolution + positional
convention. Every "REQUIRED FIX" and "OWNER DECISION" line in the findings file is
binding.

## Context (from discovery — verified against origin/master c8349fb)

- `internal/pullrequest/activity.go` — `Validate` (:248) called from `UnmarshalJSON`
  (:282-295) rejects unknown activity types. Activities are decoded via
  `profile.GetAll[Activity]` → `PaginatedResources[T]` (one `json.Unmarshal` per page,
  `internal/profile/profile_client.go:80,224`), so a per-element error aborts the whole
  page (FR-5).
- `internal/common/whatif.go` — `common.WhatIf` short-circuits before ANY resolution;
  dry-run output is input-independent (FR-6). Full call-site enumeration is in Task 4.
- `internal/profile/profile.go` — `redactWithHash` (:31); AccessToken out of default
  columns (:207) but `GetRow` emits the RAW token for `--columns accesstoken`
  (:249-254), while the `GetHeaders` doc comment (~:205-215) FALSELY claims it is
  masked; `MarshalJSON` (~:701) and `forDisplay` (~:822) docs record a DELIBERATE
  decision that json/yaml show real secrets (FR-7).
- `internal/pullrequest/comment/comment.go` — `--comment` is `MarkFlagRequired` (:93);
  `--line` and `--from` both bind `&options.From` and `--line`'s usage string (:85) is
  `--from`'s text, so only the usage string changes (FR-8/FR-11-help).
- `internal/pullrequest/pullrequest.go` — no `Participants` field; `internal/user` has
  a `Participant` type with `Role`, `Approved`, `State`, `ParticipatedOn` (FR-9).
- `internal/pullrequest/pullrequest.go:17,19` imports the `comment` and `task`
  subpackages, so those packages CANNOT import `internal/pullrequest` — the PR-id
  resolver `GetPullRequestIDFromArgs` (`pullrequest.go:208`) is unreachable from them
  without moving it (FR-10).
- `internal/pullrequest/list.go:31-38` — `--state` ALREADY EXISTS as a single-valued
  `common.NewEnumFlag("all", "declined", "merged", "+open", "superseded")`, default
  `open`, `MarkFlagsMutuallyExclusive("commit","state")`, emitted as `?state=` at
  :53-56. FR-11's work is conversion to repeatable, not addition (FR-11).
- `internal/workspace/workspace.go:103` `GetWorkspaceName` — the slug-chain pattern to
  mirror; `internal/repository/repository.go:234` `GetRepositoryName` — the repository
  chain ends at git remote with no profile fallback. `internal/remote/remote.go:36-67`
  ALREADY rejects non-`bitbucket.org` remotes and both chains already treat that as
  fall-through (FR-12).
- `internal/pipeline/step/{get,raw,flags}.go` — `args[0]` goes straight into the
  request path, no name→UUID resolution; `--pipeline` is a required flag (FR-13/14).
- `internal/common/identifier.go` `ValidatePathIdentifier` (landed in PR #36) is the
  standing guard for every user-supplied value that reaches `repo.GetPath` — all new
  positionals must go through it.

## Binding rule sets (read before ANY task)

1. `docs/plans/field-report-findings.md` — the spec. Each task below names its FR
   section; the worker MUST read that section in full before coding.
2. `docs/plans/completed/20260806-restore-feature-groups.md` "MANDATORY bug-class fixes"
   — GetRow/columns coherence, time formats, sort-guard, no silent no-op successes,
   secrets never logged, error-tolerance matrix, GetAllUnbounded for getters,
   static-vs-dynamic enum flags, drop dead code, honest trigger/stop outcomes.
3. `CLAUDE.md` — conventions: lowercase current-behavior-only comments, no AI mentions
   anywhere, stdlib errors, lgr logging, table-driven tests, RunE reads flags off `cmd`
   (package-level option bindings are legacy — new flag reads go direct-off-`cmd`),
   conventional commits.
4. Architectural rule (handoff): **read paths are permissive, write paths are strict** —
   but permissive means tolerating unrecognized VARIANT/TYPE VALUES only; malformed
   JSON, wrong shapes, and missing required identity fields still error.
5. The owner's real token is narrowly scoped (no `read:workspace`) — never "fix"
   anything by requiring broader scope.

## Development Approach

- **testing approach**: Regular (code first, then tests) — but every regression test for
  a behavior change MUST be proven to fail on pre-fix code via a throwaway `git
  worktree` on the previous master commit before the task is declared done. This
  applies to EVERY task that changes behavior (Tasks 1-9), not just the ones that call
  it out.
- one task = one PR against master. Full PR flow per task (see Workflow below).
- complete each task fully before moving to the next; tasks are ordered so the
  positional-id convention (Task 3) lands before the code that rewrites the same RunEs
  (Tasks 4, 5) and before the step positionals (Task 9).
- **CRITICAL: every task MUST include new/updated tests** — success and error scenarios,
  table-driven for new code.
- **CRITICAL: all tests must pass before starting next task.**
- **CRITICAL: update this plan file when scope changes during implementation.**

## Workflow (per task — non-negotiable, incidents happened)

- branch from FRESH master (`git fetch origin master && git checkout -b <branch> origin/master`).
- stage with `git add <explicit paths>` only — never `-A`/`.`; `git status --short`
  before every commit; never commit `.claude/`, `.mcp.json`, or `docs/plans/*` (only
  `docs/plans/completed/` is tracked).
- commit messages via `git commit -F <tempfile>` ONLY (backtick in `-m` once truncated
  the owner's ~/.bashrc). Conventional commit format.
- full gate before push: `go build ./... && go vet ./... && gofmt -l . (empty) &&
  go test -race ./... && golangci-lint run && goreleaser check`.
- push, `/opt/homebrew/bin/gh pr create --base master` (no "Test plan" section),
  `gh pr checks --watch` in the FOREGROUND, squash-merge, sync master, delete branch.
- breaking-change notes go in the PR BODY (release notes are generated from PRs at tag
  time; the root `CHANGELOG.md` is deliberately a stub delegating to GitHub Releases —
  do not grow it).

## Testing Strategy

- unit tests required for every task (table-driven, `net/http/httptest` for HTTP paths,
  request-recording handlers where the assertion is "zero requests to X" or "no write
  request issued").
- regression tests proven to fail on pre-fix code (throwaway worktree technique) for
  every behavior change.
- no e2e framework in this repo; the built binary is smoke-checked in Task 10.

## Progress Tracking

- mark completed items with `[x]` immediately when done
- add newly discovered tasks with ➕ prefix
- document issues/blockers with ⚠️ prefix
- append one line per task to `docs/plans/progress-field-report-fr5-fr14.txt`
  (timestamp, task, PR number, gate result, notable discoveries)

## Solution Overview

Ten PRs in dependency order, then a review loop. FR-5 first (only finding that makes a
command unusable). FR-10 (Task 3) before FR-6/FR-8 so the comment/task RunEs are
rewritten once against their final positional signatures, and before FR-13/14 which
reuse its convention. FR-13+FR-14 land together (same files). Docs/acceptance last.
Each PR is self-contained and leaves master green.

## Implementation Steps

### Task 1: FR-5 — permissive read-path activity decoding + bounded audit of strict read decoders

**Spec**: FR-5 section of `docs/plans/field-report-findings.md`. CRITICAL.

**Files:**
- Modify: `internal/pullrequest/activity.go`, `internal/pullrequest/activities.go`
- Modify: `internal/pullrequest/activity_marshal_test.go`, `activity_row_test.go`,
  `activities_test.go` (as needed)
- Modify: any other read-path decoder the bounded audit converts
- Create/Modify: fixture-driven tests per changed decoder

**Pinned design (do not deviate):**
- `Activity.UnmarshalJSON` STOPS erroring on an unrecognized variant; it records the raw
  variant/type kind on the struct in an UNEXPORTED field (Activity has a custom
  `MarshalJSON` and round-trip tests — the marker must not appear in any output format;
  the existing marshal round-trip tests must pass unchanged) and returns success.
  Malformed JSON still errors.
- `activities.go`'s RunE (`activitiesProcess`, around :103) filters unknown-kind entries
  out before sorting/printing and emits ONE `[WARN]` per DISTINCT unknown kind, deduped
  via a local `map[string]struct{}` inside the RunE — no package-level state (`-race`).
- **Do NOT change `PaginatedResources[T]` or `profile.GetAll` decode semantics** — the
  tolerance lives in the element type and the consuming RunE only.

- [x] enumerate Bitbucket's actual activity types from the API docs (approval, approval
      removal, changes_requested and its removal, update variants) — do not guess;
      record the list in a code comment on the model
- [x] add `changes_requested` (and other documented types) to the Activity model with
      columns/GetRow coverage per the GetRow-coherence rule
- [x] implement the pinned unknown-kind skip-with-warn design above
- [x] bounded audit of strict read-path decoders: check `Repository.UnmarshalJSON`
      (`repository.go:516-522`), `Pipeline.UnmarshalJSON` (`pipeline.go:193`),
      `Comment.Validate` (`comment.go:240`, currently uncalled), pipeline `Target`, and
      any other `Validate()` reachable from `UnmarshalJSON` on a GET path. For EACH
      site record a disposition in the PR body: tolerate-unknown-variant / keep-strict
      (structural or identity validation) / dead-code-delete. Permissiveness applies
      ONLY to unrecognized variant/type values — do not delete genuine structural
      validation
- [x] tests: fixture feed containing changes_requested + a synthetic unknown type
      decodes, renders all known entries, warns once per unknown type; per-decoder
      tests for any other converted site
- [x] prove the changes_requested fixture test fails on pre-fix master (worktree)
- [x] full gate + PR flow

### Task 2: FR-9 — `pr get` participants (+ FR-11's `--line` help fix)

**Spec**: FR-9 section + the help-string half of FR-11. MAJOR.

**Files:**
- Modify: `internal/pullrequest/pullrequest.go`
- Modify: `internal/pullrequest/pullrequest_row_test.go`, `pullrequest_marshal_test.go`
- Modify: `internal/pullrequest/comment/comment.go` (:85 — `--line` usage string only;
  `--line`/`--from` deliberately share the same bound variable, leave that alone)

- [x] add `Participants` field to the PullRequest struct (reuse `internal/user`'s
      `Participant` type), json/yaml round-trip included
- [x] add a `participants` column (GetRow/columns/GetHeaders coherent). The table cell
      renders a compact per-participant `nickname:state` summary (approval state must
      be readable), checked against the 80-rune cell-truncation behavior; json/yaml
      carry the full objects. Keep it OUT of the default column set (available via
      `--columns participants`) unless implementation shows it fits — note the decision
      in the PR body
- [x] fix the `--line` help string to describe `--line`
- [x] tests: row test iterating `columns.Columns()`, unmarshal test with a real-shaped
      participants payload asserting approval state per reviewer is reachable in json
      output
- [x] prove the participants unmarshal test fails on pre-fix master (worktree)
- [x] full gate + PR flow

### Task 3: FR-10 — positional PR id everywhere (hard break)

**Spec**: FR-10 section. MAJOR. OWNER DECISION: positional everywhere, remove
`--pullrequest` entirely from comment/task subcommands; no users yet, no deprecation
period.

**Pinned semantics (decided once, here):**
- comment/task subcommands take the PR id as a REQUIRED first positional
  (`cobra.ExactArgs`/`cobra.RangeArgs` as appropriate). No single-open-PR auto-resolve
  for them: subcommands that also take their own id become `<pr-id> <comment-id|task-id>`
  and an optional first positional would make one-arg invocations ambiguous.
- top-level `pr` commands (get/approve/merge/...) keep their EXISTING positional
  behavior unchanged — this task touches only comment/task.
- import cycle: `internal/pullrequest` imports `comment`/`task`
  (`pullrequest.go:17,19`), so those packages must not reach for
  `internal/pullrequest`. Under the pinned required-positional semantics no code move
  is needed: read `args[0]` directly (guarded by `ValidatePathIdentifier`) and complete
  via the EXISTING `prcommon.GetPullRequestIDs`
  (`internal/pullrequest/common/getters.go:58`). Move code into prcommon only if
  implementation shows an actual shared need — do not refactor without a consumer.

**Files:**
- Modify: `internal/pullrequest/common/` (resolver/completion home)
- Modify: `internal/pullrequest/pullrequest.go` (if the resolver moves)
- Modify: `internal/pullrequest/comment/*.go` (all subcommands: create, get, list,
  update, delete, resolve, reopen)
- Modify: `internal/pullrequest/task/*.go` (create, get, list, update, delete)
- Modify: README.md (usage examples)
- Modify: all affected tests

- [x] every comment/task subcommand takes the PR id as its REQUIRED first positional;
      the `--pullrequest` flag, its registrations, and any `MarkFlagRequired` are
      deleted
- [x] `common.ValidatePathIdentifier` guards the new positional before it reaches
      `GetPath`; tests cover `""`/`.`/`..`
- [x] Use strings, help text, examples updated; `ValidArgsFunction` completes PR ids
      for arg 0 (getter via GetAllUnbounded from prcommon) and the comment/task id for
      arg 1 where applicable
- [x] README updated; breaking-change note in the PR body
- [x] tests: positional id lands in the request path; missing-arg and too-many-args
      errors are clear; completion offers PR ids for arg 0; `""`/`.`/`..` rejected
- [x] prove at least one converted subcommand's positional test fails on pre-fix master
      (worktree)
- [x] full gate + PR flow

### Task 4: FR-6 — full-preflight `--dry-run` for every mutating command

**Spec**: FR-6 section. MAJOR. OWNER DECISION: full preflight, no writes — resolve
PR/repo/reviewers via GETs, validate inputs, reject an empty body, and echo the
RESOLVED JSON payload + target URL. Only the write is skipped.

**Scope (explicit — the WhatIf grep was run at plan time against c8349fb; Task 3 lands
first and rewrites the comment/task RunEs, so the file/symbol names below are
authoritative, not the line offsets):**
- IN (Bitbucket write commands): `pullrequest create.go:112`, `update.go:130`,
  `merge.go:76`, `action.go:80` (approve/unapprove/decline/request-changes/
  remove-request-changes via the shared action helper); `comment create.go:46`,
  `update.go:59`, `reopen.go:58`, `resolve.go:58`; `pullrequest/common/delete.go:27`
  (comment delete + task delete); `task create.go:84`, `update.go:87`;
  `pipeline trigger.go:114`, `stop.go:45`.
- OUT (unchanged): every read-only command's WhatIf short-circuit (pr list/get/diff/...,
  pipeline list/get/step, repo, workspace, user, branch, commit, artifact list);
  `artifact download` (local write, WhatIf-per-item loop stays); profile
  create/update/delete/use/authorize (local config mutations; create/update's
  pre-WhatIf secret prompt/stdin ordering at `profile/create.go:113-122` and
  `profile/update.go:130-144` is DELIBERATE and documented — do not reorder);
  `common.Confirm`'s dry-run short-circuit contract (`confirm.go:18`) stays intact for
  trigger/stop, with WhatIf still reporting once.
- `--file` on comment create/update is a DIFF ANCHOR PATH inside the PR, not a local
  file (`comment.go:84`, consumed into `common.FileAnchor`). Do NOT `os.Stat` it.
  Preflight validates it against the PR's diffstat file list (a GET — exactly what full
  preflight means). Local-file stat semantics apply only to FR-8's `--comment-file`
  (Task 5).

**Files:**
- Modify: `internal/common/whatif.go` (or a preflight helper beside it)
- Modify: the IN-scope RunEs listed above
- Create/Modify: tests per converted command

- [x] move the WhatIf gate AFTER argument/target resolution in each IN-scope RunE:
      resolution GETs run, the write is skipped; dry-run output echoes the resolved
      JSON payload and target URL — input-dependent, no fabricated success lines
- [x] a nonexistent PR id and an empty body each FAIL under `--dry-run` with the same
      error they would produce for real; an unknown `--file` anchor path fails the
      diffstat preflight (deliberately STRICTER than what the API might accept — say so
      in the PR body)
- [x] trigger/stop: preflight resolves the pipeline/target; `Confirm` dry-run semantics
      unchanged
- [x] tests (httptest): dry-run issues the resolution GETs but ZERO write requests
      (request-recording handler asserts method+path); bad inputs error; payload echo
      matches resolved values; read-only commands' behavior unchanged
- [x] prove the "PR 999999 fails under dry-run" test fails on pre-fix master (worktree)
- [x] full gate + PR flow

### Task 5: FR-8 — `--comment-file` / stdin for comment and PR-description bodies

**Spec**: FR-8 section. MAJOR (ergonomics; the shell-quoting hazard is proven).

**Files:**
- Modify: `internal/pullrequest/comment/comment.go`, `create.go`, `update.go`
- Modify: `internal/pullrequest/create.go`, `update.go` (`--description-file`)
- Create/Modify: tests for each flag pair

- [x] `--comment-file <path>` and `--comment-file -` (stdin via `cmd.InOrStdin()`) on
      pr comment create/update; replace `MarkFlagRequired("comment")` (comment.go:93)
      with `MarkFlagsOneRequired("comment", "comment-file")` +
      `MarkFlagsMutuallyExclusive("comment", "comment-file")` (mirror on update with
      its own required-ness rules)
- [x] equivalent `--description-file` on pr create/update, same semantics (mutually
      exclusive with `--description`; keep create/update's existing required-ness)
- [x] file read errors surface with the path in the error; empty file/stdin body is
      rejected (consistent with FR-6's empty-body rule)
- [x] completion: file flags complete as filenames (`MarkFlagFilename`); `-` documented
      in help text
- [x] tests: body-from-file lands in the request payload VERBATIM (markdown containing
      backticks and `$()` — the exact hazard class); stdin variant; `--comment-file`
      alone succeeds (no missing-required error); mutual-exclusion error; neither-flag
      error is clear; empty-body rejection
- [x] prove the body-from-file test fails on pre-fix master (worktree)
- [x] full gate + PR flow

### Task 6: FR-12 — `profile.DefaultRepository` + documented precedence

**Spec**: FR-12 section. MAJOR (owner-added).

**Files:**
- Modify: `internal/profile/profile.go` (field + Update merge + columns if appropriate)
- Modify: `internal/profile/create.go`, `update.go` (`--default-repository` flag)
- Modify: `internal/repository/repository.go` (`GetRepositoryName`, :234)
- Modify: README.md (precedence documentation), CLAUDE.md (the "Profile carries five
  fields" paragraph becomes stale — update it in THIS task, not just Task 10)
- Create/Modify: tests per precedence rung

- [x] add `DefaultRepository` to Profile (persisted like DefaultWorkspace; yaml/json
      tags consistent with siblings; Update merge handles it)
- [x] `--default-repository` on profile create/update (plain string flag — unbounded
      identifier space, no dynamic EnumFlag validation per FR-1's rule; completion via
      getter optional)
- [x] extend `GetRepositoryName`'s chain: `--repository` flag > bitbucket git remote >
      profile.DefaultRepository. NOTE: Bitbucket-only remote detection ALREADY exists
      (`internal/remote/remote.go:36-67` rejects non-bitbucket.org URLs and the chain
      already falls through) — do NOT add a second detection layer; verify with a
      regression test instead
- [x] when all rungs fail, the error names all three ways to supply the value (flag,
      bitbucket remote, profile default)
- [x] README documents the precedence for both workspace and repository
- [x] tests: each rung in isolation; a GitHub-only-remote checkout falls through to the
      profile default; both defaults set + no flags = no "argument missing" errors;
      error-message content test
- [x] prove the profile-default-fallback test fails on pre-fix master (worktree)
- [x] full gate + PR flow

### Task 7: FR-7 — finish token masking in `profile list`/`get` display output

**Spec**: FR-7 section. MAJOR. Partially done: AccessToken is out of default columns and
`redactWithHash` exists. STILL BROKEN: `GetRow` emits the RAW token for `--columns
accesstoken` (profile.go:249-254) while the `GetHeaders` doc comment (~:205-215)
FALSELY claims it is masked. A fix IS needed — there is no "close with no PR" branch.

**Pinned framing:** the repo already carries a DOCUMENTED decision (`MarshalJSON` ~:701,
`forDisplay` ~:822) that `profile get/list -o json/yaml` intentionally show real
secrets (the scripting path for retrieving a stored token). Default direction: UPHOLD
that decision — mask table/csv/tsv display output, keep json/yaml complete — and make
every doc comment truthful. If the worker instead reverses it (mask everywhere +
explicit reveal opt-in), that is a deliberate reversal that must update BOTH doc
comments and be justified in the PR body. Either way, no output path may leak
undocumented.

**Files:**
- Modify: `internal/profile/profile.go` (GetRow mask; GetHeaders/MarshalJSON/forDisplay
  doc comments)
- Modify: profile row/marshal tests

- [x] mask every secret-bearing column in table/csv/tsv GetRow output (AccessToken,
      ClientSecret, Password — audit the columns table for all of them) using the
      redactWithHash style
- [x] fix the false `GetHeaders` doc comment; update MarshalJSON/forDisplay comments to
      match whatever is decided so all three tell the truth
- [x] automated tests (not manual checks) covering `profile list` AND `profile get` in
      table, json, yaml with a live-looking token: explicit `--columns accesstoken`
      renders the masked value; json/yaml behavior matches the documented decision
- [x] prove the masked-column test fails on pre-fix master (worktree)
- [x] full gate + PR flow

### Task 8: FR-11 remainder — repeatable `pr list --state` + source/destination branch filters

**Spec**: FR-11 section. MINOR. NOTE: `--state` already exists (single-valued EnumFlag,
default `open`, values all/declined/merged/open/superseded, mutually exclusive with
`--commit`, emitted as `?state=` — list.go:31-38,53-56). The work is conversion, not
addition. Registering a second `--state` flag panics at init.

**Files:**
- Modify: `internal/pullrequest/list.go`
- Modify: `internal/pullrequest/list_test.go`

- [x] convert `--state` to a repeatable static EnumSliceFlag
      (OPEN/MERGED/DECLINED/SUPERSEDED), emitting one `state=` query param per value
      (Bitbucket's repeatable-param form). Decide and document the fate of the legacy
      `all` value (it is not a Bitbucket API state — e.g. keep it as sugar for all four,
      or drop it with a breaking note in the PR body). Default behavior (`open`)
      UNCHANGED; `--commit`/`--state` exclusivity preserved. Kept `all` as sugar for all
      four states via `common.NewEnumSliceFlagWithAllAllowed` (README already documents
      `bb pr list --state all`)
- [x] add `--source` / `--destination` branch filters via the existing `q=` query
      support, with proper quoting/escaping of branch names
- [x] filters compose (states + branches together) and compose with any existing
      `--query`; new flag reads happen direct-off-`cmd` in `listProcess` (do not extend
      the legacy package-level `listOptions` binding for the new flags). `--state` was
      also switched to direct-off-`cmd` reading (type-asserted off
      `cmd.Flags().Lookup("state")`) since `EnumSliceFlag.String()`'s bracketed
      representation does not round-trip through `cmd.Flags().GetStringSlice`
- [x] tests: each filter alone asserts the emitted request params; multiple `--state`
      values; combined filters; invalid state rejected at parse time; branch name
      needing escaping; default still `open`
- [x] prove the repeatable-state test fails on pre-fix master (worktree)
- [x] full gate + PR flow

### Task 9: FR-13 + FR-14 — step name→UUID resolution + fully positional `pipeline step`

**Spec**: FR-13 + FR-14 sections. MAJOR + consistency. Lands AFTER Task 3 (same
positional convention). One PR.

**Files:**
- Modify: `internal/pipeline/step/flags.go` (shared resolver beside pipelineFlagValue;
  completion)
- Modify: `internal/pipeline/step/get.go`, `raw.go`, `logs.go`, `report.go`,
  `cases.go`, `list.go`
- Modify: README.md
- Modify: all step tests

- [x] shared resolver (one function, used by get/logs/report/cases):
      `common.ParseUUID` success → passthrough (NO list request); otherwise
      GetAllUnbounded the pipeline's steps and match on NAME (case-insensitive,
      trimmed): exactly one → its UUID; zero → error naming the value AND listing
      available step names; 2+ → error listing the ambiguous candidates with their
      UUIDs, telling the user to pass a UUID
- [x] positional convention (FR-14): `step list <pipeline>`;
      `step get|logs|report|cases <pipeline> <step>`; the `--pipeline` flag and its
      `MarkFlagRequired` removed entirely
- [x] `common.ValidatePathIdentifier` guards both positionals before they reach
      `GetPath`; tests cover `""`/`.`/`..`
- [x] completion: arg 0 completes pipeline ids (plcommon.GetPipelineIDs); arg 1
      completes step NAMES first then UUIDs, reading the pipeline from args[0]
- [x] Use strings/help/README updated — `<pipeline-step-uuid-or-name>` is now true;
      breaking-change note in the PR body
- [x] tests: uuid passthrough asserts NO extra list request; name match asserts the
      resolved UUID lands in the request path; unknown-name error lists available
      names; duplicate-name ambiguity error; both positionals resolve into the right
      path; completion of both args; missing-args errors; `""`/`.`/`..` rejected
- [x] prove the name-resolution test fails on pre-fix master (worktree)
- [x] full gate + PR flow

### Task 10: Docs + acceptance

**Files:**
- Modify: README.md, CLAUDE.md (surface changes from FR-6/8/10/11/13/14 not already
  covered by per-task doc updates)
- Move: this plan to `docs/plans/completed/`

- [x] README/CLAUDE.md reflect the final command surface (positional ids,
      --comment-file/--description-file, full-preflight dry-run, defaults precedence,
      repeatable --state, step positionals) — verify against the BUILT binary's
      `--help` output, not from memory
- [x] full gate on master; module count unchanged: `go list -m all | wc -l` reports 38
      (needs module cache/network)
- [x] smoke: build `./bb` and exercise `--help` of every changed command group
- [x] verify all FR-5..FR-14 items in `field-report-findings.md` are addressed; map
      each "REQUIRED FIX" line to a landed PR in the progress log
- [x] roadmap memory (handled by orchestrator)
- [x] move this plan to `docs/plans/completed/` and commit it with the final PR
      (`git add -f`)

### Task 11: Review phases over the accumulated cycle-A diff

Run the same review loop the two completed plans used: comprehensive review of the
accumulated diff (master since c8349fb) → fixer PR(s) → critical re-check, LOOPING until
the critical re-check is clean. That loop caught defects in every prior round, including
defects introduced by fixes — do not skip or single-pass it.

- [x] comprehensive review (correctness/security, tests, simplification, docs) over the
      full cycle-A diff; findings recorded to the progress log — comprehensive + smells
      review, findings recorded to the progress log
- [x] fixer PR(s) for all confirmed findings, full gate each — fixer PRs #47, #49-#55
      (comprehensive review) and #56 (smells review), full gate each
- [x] critical re-check over the post-fix diff; loop fixer→re-check until clean —
      comprehensive + 8 critical re-check iterations, loop clean
- [x] final gate green on master; progress log records the loop outcome — final gate
      clean, zero critical/major findings; external codex review skipped (tool not
      installed)

## Post-Completion

*Owner-only; do NOT do these:*

- add `HOMEBREW_TAP_TOKEN` secret (fine-grained PAT, write access to
  `avitsrimer/homebrew-apps`) to Actions secrets
- cut the release via `/release-tools:new` (suggest v0.19.0) AFTER cycle B
  (`bb install-skill`) also lands
