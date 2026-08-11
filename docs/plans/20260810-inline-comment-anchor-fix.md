# Inline Comment Anchor Fix

## Overview

`bb pullrequest comment create/update --line N` anchors inline comments to the wrong line in
Bitbucket's PR view. Root cause: `--line` is bound to `FileAnchor.From`, and the Bitbucket Cloud
API defines `inline.from` as the anchor line in the **old** version of the file. Users compute
line numbers against the file at the PR's head (the **new** side), which the API expects in
`inline.to`. The comment therefore lands offset by the net insertions/deletions above that point
in the file.

Verified against the official API schema (`api.bitbucket.org/swagger.json`, `comment`
definition, fetched 2026-08-10):

- `inline.from` — anchor line in the **old** version of the file (multi-line: ending line, old side)
- `inline.to` — anchor line in the **new** version of the file (multi-line: ending line, new side)
- `inline.start_from` / `inline.start_to` — starting line of a multi-line comment (old/new side);
  both are present in the schema with explicit descriptions
- NOT yet verified against a live API (Task 0 spike): whether Bitbucket drops `to` when `from`
  is also sent (community-post claim only), whether out-of-hunk anchor lines are rejected or
  accepted, whether a `start_to`+`to` range comment actually renders as a range in the UI, and
  whether the diff endpoint honors a `path=` query filter

This plan, delivered as **three PRs** (one change per PR, per CLAUDE.md):

- **PR 1 — the bug fix**: rebind `--line` to the new-file side (`to`), keep `--from` as the
  explicit old-side anchor, remove `--to` (its documented "range end" semantics never existed in
  the API), fix all docs (SKILL.md, README.md)
- **PR 2 — line validation**: preflight validation of anchor line(s) against the PR's own raw
  diff (the exact diff Bitbucket renders), strictness decided by the Task 0 spike
- **PR 3 — multi-line ranges**: `--line 1030-1040` → `start_to: 1030, to: 1040`;
  `--from 990-1000` → `start_from: 990, from: 1000` — gated on the spike confirming range
  comments actually work end-to-end

Breaking CLI changes in PR 1 (deliberate — the old semantics were the bug): `--line` switches
sides, `--to` is removed. `--line`/`--from` become string flags in PR 3 (or PR 1 if trivially
cheap to do once — decide at implementation; the flag *meaning* changes only once, in PR 1).

## Context (from discovery)

- `internal/pullrequest/comment/comment.go` — `commentEditOptions`, `payload()` (`:49`, doc
  comment `:45-48` names `--to`), `registerCommentEditFlags()` (`:103`; `--line` bound to
  `options.From` at `:107`; `line`/`from` mutual exclusion already exists at `:112`, only the
  `line`/`to` pair at `:113` goes), `validateFileAnchor()` (`:138`, doc comment `:132-137`,
  diffstat GET, path-only check)
- `internal/pullrequest/comment/create.go:54-63` — deliberately skips
  `prcommon.ExistsPullRequest` when an anchor exists because validateFileAnchor's diffstat GET
  404s identically for a nonexistent PR; PR 2 transfers this invariant to the diff endpoint and
  must update the doc comment + add a test
- `internal/pullrequest/comment/update.go:59` — second preflight call site
- `internal/common/file_anchor.go` — `FileAnchor{From, To, Path}`; `String()` feeds `comment
  list`'s "file" column via `GetRow` (`comment.go:265-270`) AND the `file` column sorter
  (`comment.go:215-220`), so rendering changes also change sort order
- `internal/common/flag_value.go:10-19` — `common.StringFlagValue` tolerates an unregistered
  flag (raw `cmd.Flags().GetString` errors); required because the process-test harness cmd
  doesn't carry the comment flags
- `internal/pullrequest/comment/helpers_test.go:19-28` (`setupTest`) +
  `internal/testutil/testutil.go:133-145` — harness cmd registers only
  profile/repository/output/dry-run/pending/paging flags; anchor is currently expressed by
  mutating the package-level options struct (`create_process_test.go:169-170`,
  `update_process_test.go:164`), which this rework removes. `setupTest`'s cmd must register
  `--line`/`--from` locally (do not widen shared `testutil.SetupProfile`)
- tests that break or pin old semantics (enumerated): `create_process_test.go:165-201` (asserts
  `From:10 To:12`), `create_process_test.go:263-284` + `update_process_test.go:174` (pin the
  literal `"cannot specify from/to without a file"`), `update_process_test.go:161-180`
  (`updateOptions.To = 5`), `comment_file_test.go:89-102` (FR-11 help-text test)
- `internal/pullrequest/diff.go:73` — `profile.GetRaw(ctx, uripath)` fetches the raw unified
  diff (`pullrequests/{id}/diff`, served `text/plain`; the 302 to the diff host is followed —
  `httpClient` is a default `http.Client`, `profile_client.go:60`). Note: `GetRaw` buffers the
  whole response; a big PR diff can be large — spike checks whether `path=` narrows it
- `testdata/activity-comment.json:51-57` — read-side `inline` carries `context_lines`,
  `outdated`, `to`, `from`, `path`; read comments CAN carry both `from` and `to`, so `String()`
  must render sensibly for that shape
- docs: `skill/bitbucket-cli/SKILL.md:205-212` and `:223-226` (incl. the "validated against the
  diffstat" sentence); `README.md:1050-1053` (`--line 404` canonical example),
  `README.md:1056-1060` (diffstat validation paragraph), `README.md:1369+` ("Upgrading from
  upstream 0.18.x" — where breaking flag changes are documented)
- repo convention: new flag-reading code reads directly off `cmd.Flags()`, not package-level
  bound variables

## Worker execution constraints

Non-negotiable rules for whoever executes this plan (agent or human):

- work ONLY inside this linked worktree; never touch the main checkout at
  `~/Code/my/go/bitbucket-cli` (it carries another branch's in-progress state)
- commit messages: write to a temp file and `git commit -F <file>` — NEVER `git commit -m` with
  prose (backticks/`$()`/quotes in a `-m` string have executed before and destroyed user files)
- stage files explicitly by path — `git add -A` / `git add .` are FORBIDDEN; run
  `git status --short` before every commit and verify nothing unexpected is staged
- never stage or commit `.claude/`, `.mcp.json`, or any credentials/config files
- one branch per PR boundary, cut from `master`; conventional commits; no AI/Claude/Anthropic
  mentions in commits or PR descriptions (repo rule)

## Development Approach

- **testing approach**: TDD (tests first) — for each task, write the failing tests for the
  correct semantics, then implement until green
- three PR boundaries (marked per task); each PR independently green
- complete each task fully before moving to the next
- make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
  - tests are not optional - they are a required part of the checklist
  - table-driven with subtests; httptest for HTTP-facing paths (repo convention)
  - tests cover both success and error scenarios
- **CRITICAL: all tests must pass before starting next task** - no exceptions
- **CRITICAL: update this plan file when scope changes during implementation**
- run `go test -race ./...` and `golangci-lint run` after each task
- backward compatibility is deliberately broken for `--line`/`--to` (see Overview); config file
  compatibility is untouched

## Testing Strategy

- **unit tests**: required for every task (see Development Approach above)
- no e2e/UI test suite in this project; `go test -race ./...` + `golangci-lint run` are the gates
- payload JSON pinned in marshal tests so a field rename can never silently reappear
- diff fixtures for the hunk parser live as `const` strings inside the test file —
  `internal/pullrequest/comment/` has no `testdata/` dir and CLAUDE.md forbids orphaned fixtures
- new `file_anchor` tests: `package common_test` (FileAnchor is exported; also keeps the
  testutil import-cycle rule irrelevant)

## Progress Tracking

- mark completed items with `[x]` immediately when done
- add newly discovered tasks with ➕ prefix
- document issues/blockers with ⚠️ prefix
- update plan if implementation deviates from original scope
- keep plan in sync with actual work done

## Solution Overview

- **flag surface** (create + update, shared via `registerCommentEditFlags`):
  - `--line <n>` (PR 3: `<n | start-end>`) — anchor on the **new** file side (`to`, plus
    `start_to` for a range). This is what "comment on line N" means everywhere else.
  - `--from <n>` (PR 3: `<n | start-end>`) — anchor on the **old** file side (`from`, plus
    `start_from`), the escape hatch for commenting on deleted lines.
  - `--to` — removed. Its documented meaning (range end paired with `--from`) never matched the
    API; keeping it as an alias of `--line` would silently flip its side for existing scripts —
    a hard "unknown flag" error is safer.
  - `--line`/`--from` mutual exclusion already exists and stays (spike decides whether a
    legitimate `from`+`to` pair anchor exists; if it does, revisit in PR 3).
  - either still requires `--file`; `--file` alone (file-level comment) stays valid.
- **error wording**: the `--line`/`--from`-without-`--file` error message is decided ONCE at
  implementation start (it appears in `comment.go:77` and two test assertions) — e.g.
  `"cannot specify --line/--from without --file"`.
- **line validation** (PR 2; preflight, before `WhatIfPayload` so `--dry-run` fails
  identically): when a line anchor is present, GET the PR's raw diff via `profile.GetRaw`
  (with `path=<file>` if the spike confirms the filter works), parse its hunks for
  `anchor.Path`, and check every anchored line against its side's hunk coverage.
  - file absent from the diff → hard error (same strictness as today's diffstat check)
  - line outside every hunk → strictness per spike: hard error only if the API/UI actually
    rejects or mis-renders such anchors; otherwise `[WARN]` with the valid ranges and proceed
    (a false rejection is worse than the current permissive behavior)
  - the diff GET replaces the diffstat GET when a line anchor exists (path check comes free);
    plain `--file`-only keeps the existing diffstat check unchanged

## Technical Details

- `common.FileAnchor` gains `StartFrom uint64 \`json:"start_from,omitempty"\`` and
  `StartTo uint64 \`json:"start_to,omitempty"\`` (PR 3 for writes; harmless on reads earlier).
  `String()`: prefer the new side when `To` is set — `path:1040`, range `path:1030-1040` —
  else old side with a space-free marker: `path:990(old)`, `path:990-1000(old)`. Space-free
  keeps the `path:line` token copy-pasteable. Read comments carrying both sides render the new
  side. Exact format pinned by tests.
- range parser (PR 3): `parseLineRange(flagName, value string) (start, end uint64, err error)`
  — `start == 0` means single-line; rejects `S > E`, zero/negative, malformed; local to the
  comment package (single consumer, YAGNI)
- flag reads: `payload(cmd)` reads `--line`/`--from` via `common.StringFlagValue` (tolerates
  the flag being absent on a test-constructed cmd); untouched flags keep their existing struct
  bindings
- unified diff hunk parser (PR 2): local to the comment package; walks `diff --git` sections,
  matches `anchor.Path` against `--- a/<path>` / `+++ b/<path>` (incl. `/dev/null` for
  adds/deletes, renames, paths containing spaces), collects hunk coverage per side. Hunk header
  grammar: `@@ -oldStart[,oldLen] +newStart[,newLen] @@[ trailing section text]` — counts are
  OPTIONAL (absent ⇒ length 1) and trailing text after the closing `@@` is tolerated. Sections
  with no hunks (binary files — `Binary files ... differ` / `GIT binary patch` — and mode-only
  changes) yield empty coverage, not a parse error. A structurally malformed hunk header inside
  a matched file section is still a hard error.

## What Goes Where

- **Implementation Steps** (`[ ]` checkboxes): spike, code, tests, SKILL.md/README.md/CLAUDE.md
- **Post-Completion** (no checkboxes): release/version-bump note, reply to the bug reporter

## Implementation Steps

### Task 0: API behavior spike (gates PR 2 strictness and PR 3 entirely)

**Files:**
- Modify: this plan (record findings in Context as ➕ notes)

- [ ] against a scratch PR in a test repo, POST via `bb`/curl and record the response `inline`
      object for each: (a) `{path, to}` — sanity check the rebind renders on the right line in
      the UI; (b) `{path, from, to}` — is `to` dropped? (c) `{path, start_to, to}` — does a
      range comment render as a range? (d) an out-of-hunk but in-file `to` — accepted?
      rendered where?
- [ ] check whether `GET pullrequests/{id}/diff?path=<file>` filters the diff
- [ ] record findings here; set PR 2's out-of-hunk strictness (error vs `[WARN]`) and PR 3's
      go/no-go accordingly — if ranges don't work end-to-end, drop PR 3 (YAGNI) and keep
      `--line`/`--from` plain int flags

---
**PR 1 — fix `--line` side + remove `--to` + docs (Tasks 1-3)**

### Task 1: Rebind --line to the new side, remove --to

**Files:**
- Modify: `internal/pullrequest/comment/comment.go`
- Modify: `internal/pullrequest/comment/comment_file_test.go`
- Modify: `internal/pullrequest/comment/helpers_test.go` (register `--line`/`--from` on
  `setupTest`'s cmd — local to the package, don't widen `testutil.SetupProfile`)
- Modify: `internal/pullrequest/comment/create_process_test.go`, `update_process_test.go`
- Modify: `internal/pullrequest/comment/comment_marshal_test.go` (payload JSON pins)

- [x] decide the without-`--file` error wording once (see Solution Overview) — settled on
      `"cannot specify --line/--from without --file"`
- [x] write tests first (via `newIsolatedCommentEditCmd` + process-level via `setupTest`):
      `--line 1040` → payload `{"inline":{"path":...,"to":1040}}`; `--from 1000` →
      `{"from":1000}`; `--to` is an unknown flag; `--line`+`--from` rejected; `--line` without
      `--file` rejected with the new wording; zero/negative rejected
- [x] rework `registerCommentEditFlags`: `--line` binds to the new-side value, help text states
      old-side vs new-side semantics; delete `--to` and the `line`/`to` mutual exclusion
      (`line`/`from` exclusion already exists — keep)
- [x] rework `payload(cmd)` to read `--line`/`--from` via `common.StringFlagValue` and drop
      `From`/`To` from `commentEditOptions`; update the doc comment at `comment.go:45-48`
      (names `--to`)
- [x] fix the enumerated breaking tests: `create_process_test.go:165-201`, `:263-284`,
      `update_process_test.go:161-180`, `:174`, `comment_file_test.go:89-102` (FR-11 test —
      replace with one pinning the new old/new-side help text). `comment_marshal_test.go` needed
      no change — it pins `Comment`'s `created_on`/`updated_on` marshaling only, not the anchor.
- [x] run tests - must pass before task 2

### Task 2: FileAnchor.String() renders sides correctly

**Files:**
- Modify: `internal/common/file_anchor.go`
- Create: `internal/common/file_anchor_test.go` (`package common_test`)

- [x] write tests first: `String()` cases — new-side single (`path:1040`), old-side single
      (`path:990(old)`), both sides set (read comment — new side wins), path-only; JSON
      marshal keeps `from`/`to` names + omitempty
- [x] implement; check `comment list` "file" column output AND its sorter
      (`comment.go:215-220`, `:265-270`) plus any pinned fixtures — update where the new
      format is intentional (sorter/GetRow call `.String()` generically; no format-specific
      pinned fixtures found for the anchor string)
- [x] run tests - must pass before task 3

### Task 3: Documentation sync (ships in PR 1)

**Files:**
- Modify: `skill/bitbucket-cli/SKILL.md`
- Modify: `README.md`
- Modify: `CLAUDE.md` (grep for stale `--line`/`--from`/`--to` comment-flag references; update
  the "What this is" summary if it describes the old flag model)

- [x] rewrite SKILL.md `:205-212` and `:223-226`: old-side vs new-side semantics, `--to`
      removal (leave validation/range wording to PR 2/3 edits)
- [x] README.md: fix the `--line 404` example (`:1050-1053`), the validation paragraph
      (`:1056-1060`) as far as PR 1 changes it, and add the breaking change (`--line` side
      flip, `--to` removal) to the Upgrading section (`:1369+`)
- [x] verify the SKILL.md sync-guard test in `internal/cmd` passes
- [x] full gate: `go test -race ./...`, `golangci-lint run` — **PR 1 boundary**

---
**PR 2 — validate anchor lines against the PR's own diff (Task 4)**

### Task 4: Diff-based line validation preflight

**Files:**
- Create: `internal/pullrequest/comment/anchor_diff.go`
- Create: `internal/pullrequest/comment/anchor_diff_test.go` (diff fixtures as `const` strings)
- Modify: `internal/pullrequest/comment/comment.go`, `create.go` (doc comment `:54-56`),
  `update.go`
- Modify: `internal/pullrequest/comment/create_process_test.go`, `update_process_test.go`
- Modify: `skill/bitbucket-cli/SKILL.md`, `README.md` (validation behavior paragraphs)

- [ ] write hunk-parser tests first: single hunk; multiple hunks; multiple files; rename; pure
      add (`/dev/null`); pure delete; optional-count headers (`@@ -5 +5 @@`); trailing section
      text after `@@`; binary file section; mode-only change (no hunks); path with a space;
      no-newline marker; malformed header inside a matched section → error
- [ ] write validation tests (httptest serving a fixture diff): new-side line inside hunk
      passes; outside every hunk → per spike (error listing valid ranges, or `[WARN]` +
      proceed); old-side line checked against old-side coverage; file absent from diff errors;
      **line anchor + nonexistent PR still errors with no write sent** (the transferred
      `ExistsPullRequest` invariant); `--file`-only (no line) still uses the diffstat path
- [ ] implement the hunk parser per the grammar in Technical Details
- [ ] extend the preflight: line anchor present → GET raw diff via `profile.GetRaw` (with
      `path=` if spike confirmed), validate path + lines from it (no diffstat call); no line
      anchor → keep `validateFileAnchor` as is; update the `create.go:54-56` doc comment to
      describe the diff-endpoint 404 invariant (current behavior only, no history)
- [ ] confirm ordering: validation before `common.WhatIfPayload` in create and update
- [ ] update SKILL.md/README.md validation wording; full gate — **PR 2 boundary**

---
**PR 3 — multi-line range syntax (Task 5; only if Task 0 confirmed ranges work)**

### Task 5: Range syntax on --line/--from

**Files:**
- Modify: `internal/common/file_anchor.go` + `file_anchor_test.go` (StartFrom/StartTo, range
  rendering in `String()`)
- Create: `internal/pullrequest/comment/line_range.go` + `line_range_test.go`
- Modify: `internal/pullrequest/comment/comment.go` (+ flag help), `comment_marshal_test.go`
- Modify: `internal/pullrequest/comment/anchor_diff.go`/`_test.go` (validate both endpoints)
- Modify: `skill/bitbucket-cli/SKILL.md`, `README.md`

- [ ] write `parseLineRange` tests first (table-driven: `N`, `S-E`, `0`, `-5`, `10-`, `20-10`,
      `abc` → errors naming the flag)
- [ ] implement `parseLineRange`
- [ ] write payload tests: `--line 1030-1040` → `{"start_to":1030,"to":1040}`;
      `--from 990-1000` → `{"start_from":990,"from":1000}`; marshal pins `start_from`/`start_to`
- [ ] add `StartFrom`/`StartTo` to `FileAnchor` + range rendering (`path:1030-1040`,
      `path:990-1000(old)`) with tests
- [ ] wire ranges through `payload(cmd)`; both range endpoints validated by the PR 2 preflight
- [ ] update SKILL.md + README.md range docs; full gate — **PR 3 boundary**

### Task 6: Verify acceptance criteria

- [ ] all Overview items implemented (or PR 3 explicitly dropped per spike, recorded with ⚠️)
- [ ] edge cases: file-level comment (no line) unchanged; `--dry-run` echoes the corrected
      payload and fails identically for bad targets; update path behaves identically to create
- [ ] run full suite: `go test -race ./...`
- [ ] run `golangci-lint run`, `go vet ./...`, `gofmt -l .` — all clean
- [ ] `make cross-build` still green

### Task 7: [Final] Close out

- [ ] move this plan to `docs/plans/completed/` and `git add -f` it with the final PR

## Post-Completion

**Manual verification:**
- post an inline comment on a real PR (ideally the reporter's repro: a branch with a
  `Merged main into <branch>` commit) with `--line` computed against the head blob; confirm it
  lands on the intended line in the Bitbucket UI
- if PR 3 shipped: try a multi-line `--line S-E` comment and an old-side `--from` comment in
  the UI

**External:**
- breaking CLI change (`--line` side flip, `--to` removed): next release should be a minor bump
  (0.4.0) with the change called out in release notes
- reply to the original bug report: root cause was field mapping, not diff-base drift; the
  requested line validation and preview (via `--dry-run` payload echo) are covered
