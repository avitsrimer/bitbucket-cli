# Dep slimming (tablewriter, go-core out) + `pr create --reviewer none` (+ README --draft gap)

## Overview

Four work items, one PR each (OWNER REQUEST 2026-08-07):
1. Remove `kataras/tablewriter` — replace with a local renderer, byte-identical output.
2. + 3. Remove `gildas/go-core` in two stages: mechanical generics/env sweep, then the
   JSON-marshaling type port.
4. `pr create --reviewer none` (suppress default-reviewer resolution) + close the README
   gap that made `--draft` look unsupported.

`gopkg.in/yaml.v3` stays AS IS (owner decision; umputun's own repos all use it).

## Context (from discovery — master 5cbbf79; verified by plan review against module-cache sources)

- `kataras/tablewriter`: ONE import site, `internal/profile/profile.go` (:20, `printTable`
  :640-657 calling only `NewWriter`/`SetHeader`/`SetAutoWrapText(false)`/`Append`/`Render`).
  ⚠️ There is NO existing byte-level table oracle — every table assertion in the repo is a
  `strings.Contains(stdout, "+--")`; the truncation tests never touch tablewriter. Goldens
  must be CAPTURED FIRST (Task 1).
- `gildas/go-core`: 95 usages / 35 files. Mechanical: `core.Sort` (37 — via
  `columns.SortBy` LESS-functions; `Columns[T].Compare` is `func(a,b T) bool` with **129**
  comparator definitions that must NOT be touched), `core.Map` (21), `core.Filter` (6),
  `core.Contains` (1), `core.GetEnvAsString` (5), `core.GetEnvAsDuration` (1). Types:
  `core.URL`/`core.Time`/`core.Timestamp` confined to 4 files (`internal/common/link.go`,
  `internal/profile/profile.go`, `internal/profile/token.go`,
  `internal/pullrequest/task/task.go`). `core.TypeCarrier`/`Named`/`Identifiable` appear
  ONLY in doc comments (8 comment sites, zero compile-time uses) — nothing to port.
  `internal/common/{cache,column,filter_validargs}.go` use only mechanical symbols
  (Task 2, not Task 3). Sole test importing go-core:
  `internal/pullrequest/activity_sort_test.go` (`core.Sort` ×5).
- go-core source: `$(go env GOMODCACHE)/github.com/gildas/go-core@v0.6.4` — read it, do
  not guess.
- `--draft` on `pr create` EXISTS since PR #8 (create.go:63/:28/:98), ships in v0.1.0+.
  SKILL.md:121 documents it; README does NOT (the gap).
- Reviewer resolution (create.go:104-118): `values[0] == "default"` or empty →
  `resolveCreateDefaultReviewers` (explicit `default` hard-fails on lookup error;
  implicit follows the tolerance matrix, PR #63); otherwise `resolveExplicitReviewers`
  (which would treat a literal `none` as a nickname). ⚠️ `createProcess` reads the
  package-level `createOptions.Reviewers` (legacy pattern). `reviewerCompletionFunc`
  (`pullrequest.go:298`) returns member nicknames ONLY (no `default`/`all` offered
  today) and is SHARED with `pr update`'s `--add-reviewer`/`--remove-reviewer` — do not
  add `none` to the shared function. `WhatIfPayload` runs after reviewer resolution
  (create.go:118), so `--dry-run` symmetry is preserved by `none` skipping resolution
  in both modes — FR-6 invariant intact (state this in the PR body).

## Pinned technical decisions (from plan review — binding, do not re-derive)

**Sort shim (C1):** do NOT change `Columns[T].Compare` or any of the 129 comparators.
Add `common.Sort[S ~[]T, T any](items S, less func(T, T) bool)` reproducing go-core's
adapter body exactly: `slices.SortFunc(items, func(a,b T) int { if less(a,b) { return -1 }
else if less(b,a) { return 1 }; return 0 })`. Never the naive `-1/1` adapter — returning
1 for equal elements violates SortFunc's contract (undefined order under pdqsort).
Stability: at the pinned x/exp version, `x/exp/slices.SortFunc` forwards to stdlib
`slices.SortFunc`, so this shim is bit-identical to go-core including tie order. All 37
`core.Sort` sites become an identifier swap.

**Nil-preserving Map/Filter (C2):** go-core's Map/Filter append into a nil slice — empty
input yields NIL, and two call sites make that wire-visible:
`create.go:166` (`Reviewers` has `omitempty`: non-nil-empty would emit `"reviewers": []`
in the POST and dry-run echo) and `profile/list.go:60` (`-o json` with zero profiles:
`null` vs `[]`). `common.Map`/`common.Filter` MUST be `var result []R; for … append` —
never `make([]R, 0, n)`. Prefer a local `Filter` over `slices.DeleteFunc` (mutates,
non-nil). Table-driven tests assert `Map(nil)==nil` AND `Map([]T{})==nil` (same for
Filter).

**Env helpers (M2):** `GetEnvAsString(n, def)` ≡ `if v := os.Getenv(n); v != "" { return v }; return def`.
`GetEnvAsDuration`: use plain `time.ParseDuration` — go-core additionally accepts
ISO-8601 (`PT30S`) via a 60-line regex parser; that capability is DELIBERATELY DROPPED
(CLAUDE.md documents "a Go duration string"); record the narrowing in the PR body and
CLAUDE.md's cache paragraph.

**tablewriter default behaviors the local renderer must reproduce (C4)** — the risk is
in NewWriter's defaults, not the called options:
1. rows use ALIGN_DEFAULT: cells matching `^-*\d*\.?\d*$` (after TrimSpace) are
   RIGHT-aligned (every id/build-number/count column today), others left-aligned
2. headers are centered with the extra space of an odd gap going RIGHT (the upstream
   `Pad` does `Ceil(gap/2)` on integer division — a no-op; replicate `gap/2` left,
   remainder right)
3. headers pass through `Title()`: `_`→space, `.`→space (only when not digit-adjacent —
   check upstream `isNumOrSpace` guard), TrimSpace, empty→`" "`, then ToUpper
4. SetHeader runs before SetAutoWrapText(false), so headers are parsed with
   autoWrap/reflow at maxWidth 30 (unreachable today — all headers short; replicate or
   document as N/A with a guard test on the longest real header)
5. cell = `" " + padded + " "`; separator `|`; rule lines `+` then `-`×(width+2);
   header separator line on; row separator lines off; top+bottom borders on; trailing
   `\n` after every line including the last
6. multi-line cells: split on `\n`, row height = max lines, missing lines padded with
   literal `"  "` then padded right to width
7. rule-line column count = max column index seen across headers AND rows (a ragged
   long row widens borders past the header)
8. widths measured with `runewidth.StringWidth` after ANSI stripping

**core type port scope (m2/m3/m4, M7):** port ONLY the used surface —
- `Time`: `MarshalJSON` = `time.Time(t).UTC().Format(time.RFC3339)` (second precision,
  sub-second dropped); `UnmarshalJSON` maps `""` to zero time; the conversions.
- `Timestamp`: `MarshalJSON` emits a BARE integer of `UnixNano()/1e6` (do not "clean up"
  to UnixMilli — differs at the zero time); `UnmarshalJSON` strips quotes then ParseInt,
  so it accepts `12345` AND `"12345"` — a plain int64 unmarshal would break the quoted
  form.
- `URL`: `AsURL`, `MarshalJSON` (zero URL marshals as `""`, not omitted — relevant to
  `common/link.go:32` embedding by value), `UnmarshalJSON` (empty string → leave zero,
  return nil), the `(*URL)(*url.URL)` conversion.
- `Task.MarshalJSON` (`task/task.go:136-154`) overrides created_on/updated_on/
  resolved_on to RFC3339-UTC via a surrogate struct — DELIBERATE design (recorded in
  docs/plans/progress-restore-feature-groups.txt:241, NOT in CLAUDE.md — don't expect to
  find it there); port the shape verbatim, do not "fix" it.

**`--reviewer none` semantics (M4/M5):** matched CASE-SENSITIVELY (mirroring today's
`values[0] != "default"` comparison). `none` anywhere in the list with
`len(values) > 1` is an ERROR ("cannot combine reviewer \"none\" with other reviewers"),
checked BEFORE the `default` branch and before `expandAllReviewers` — so
`--reviewer alice --reviewer none`, `--reviewer none,alice`, `--reviewer none --reviewer
all` all yield the same message. Exactly `["none"]` → skip resolution entirely, send NO
reviewers key (nil slice + omitempty). As part of this task, switch the reviewer read
from the package-level `createOptions.Reviewers` to
`cmd.Flags().GetStringSlice("reviewer")` (CLAUDE.md direct-off-cmd rule; also what makes
the new tests drivable). Completion: create-only wrapper offering `none`, `default`,
`all` ahead of nicknames; the shared `reviewerCompletionFunc` (pr update) stays
untouched; update its :285-297 doc comment.

## Binding rules

1. CLAUDE.md conventions; golangci-lint v2.12.2.
2. Workflow non-negotiables (as all prior cycles): throwaway worktree per task,
   explicit-path add, commit -F tempfile, full gate `go build ./... && go vet ./... &&
   gofmt -l .` (empty) `&& go test -race ./... && golangci-lint run && goreleaser check
   && make cross-build` (cross-build included — CI runs it), PR → `gh pr checks --watch`
   foreground → squash-merge → sync master; never touch the main checkout's tree
   (CLAUDE.md pull conflict → STOP and report); classifier denials are never retried.
3. Tasks 1-3 are behavior-preserving: goldens captured BEFORE the change; existing tests
   pass unchanged except `activity_sort_test.go`'s `core.Sort` import (swap to
   `common.Sort`). Task 4 is a behavior change: regression tests proven to fail on
   pre-fix master (throwaway worktree).
4. SKILL.md updates ride in the same PR as any surface change (Task 4).

## Implementation Steps

### Task 1: replace kataras/tablewriter with a local renderer (one PR)

**Files:**
- Create: `internal/profile/table.go`, `internal/profile/table_golden_test.go`
  (declared `package profile` — internal, to reach the unexported `printTable`)
- Modify: `internal/profile/profile.go` (printTable), `go.mod`/`go.sum`

- [x] FIRST: in a pre-change worktree at master, capture `printTable` stdout goldens for
      a fixed matrix: numeric-only column; mixed numeric/text column; empty cell
      (`common.EmptyCell`); CJK/emoji cell; a cell containing `\n`; a ragged row (fewer
      AND more cells than headers); headers containing `_` and `.`; zero rows with
      headers; an 80-col-truncated free-text cell. Commit the goldens with this PR
- [x] implement the renderer in `internal/profile/table.go` reproducing the eight pinned
      default behaviors above (numeric right-align, odd-gap-right header centering,
      Title() header rewriting, padding/borders/multi-line/ragged-row rules,
      runewidth-based widths)
- [x] golden tests pass against the captured bytes; existing `strings.Contains` table
      tests pass unchanged
- [x] drop `kataras/tablewriter` from go.mod; `go mod tidy`; verify it left the graph
- [x] full gate + PR flow

### Task 2: go-core mechanical sweep (one PR)

**Files:**
- Create: `internal/common/generics.go` (+ `generics_test.go`), env helpers in
  `internal/common` (file per local convention)
- Modify: 33 files using the mechanical symbols (35 importers minus `link.go` and
  `token.go`, which are type-only; `profile/profile.go` and `pullrequest/task/task.go`
  appear in BOTH Task 2 and Task 3), incl.
  `internal/common/{cache,column,filter_validargs}.go` and
  `internal/pullrequest/activity_sort_test.go` (the one test importing go-core)
- Modify (doc comments only): ten test files name `core.Sort`/`core.GetEnvAsString` in
  prose without importing go-core — reword in this PR (comments must describe current
  behavior): `internal/commit/list_test.go:70`, `internal/pipeline/list_test.go:57`,
  `internal/pipeline/step/list_test.go:55`, `internal/repository/list_test.go:90`,
  `internal/workspace/members_test.go:58`, `internal/workspace/list_test.go:53`,
  `internal/branch/list_test.go:59`, `internal/artifact/list_test.go:53`,
  `internal/common/config_test.go:47`, `internal/profile/profiles_test.go:80`

- [x] `common.Sort` shim per the pinned decision (exact go-core adapter body; comparator
      table and all 129 Compare funcs untouched); swap all 37 `core.Sort` sites
- [x] nil-preserving `common.Map`/`common.Filter` per the pinned decision (+ the
      nil/empty assertions); swap 21 Map + 6 Filter sites; `core.Contains` (1) →
      `slices.Contains`
- [x] env helpers per the pinned decision (plain `time.ParseDuration`; ISO-8601 support
      deliberately dropped — PR body + CLAUDE.md cache paragraph note)
- [x] existing tests pass unchanged (except the activity_sort_test import swap); unit
      tests for Sort/Map/Filter/env helpers
- [x] go-core REMAINS in go.mod after this task (types still used)
- [x] full gate + PR flow

### Task 3: port go-core's marshaling types and drop the dependency (one PR)

**Files:**
- Create: `internal/common/coretypes.go` (+ `coretypes_test.go`)
- Modify: `internal/common/link.go`, `internal/profile/profile.go`,
  `internal/profile/token.go`, `internal/pullrequest/task/task.go`; the 8 doc-comment
  sites naming core.TypeCarrier/Named/Identifiable (`internal/commit/commit.go:105`,
  `internal/pipeline/pipeline.go:104`, `internal/pipeline/step/step.go:143`,
  `internal/repository/repository.go:124`,
  `internal/pullrequest/pullrequest_reference.go:21`, `internal/workspace/workspace.go:49`,
  `internal/user/user.go:68,75` — reword the comments, port nothing); `go.mod`/`go.sum`;
  CLAUDE.md dependency paragraph

- [x] FIRST: capture golden marshal bytes in a pre-change worktree — NO existing fixture
      covers these types (verified): `Profile.MarshalJSON` with and without apiRoot;
      `Link.MarshalJSON` for both the ssh/GitRef and HTTP branches; `Task.MarshalJSON`
      with and without resolved_on; `Token` JSON with expires_on as bare number AND
      quoted string. Pin them as fixtures in `coretypes_test.go` (round-trip both ways)
- [x] port `common.URL`/`common.Time`/`common.Timestamp` per the pinned scope (only the
      used surface; Timestamp quoted-and-bare acceptance; URL zero-value `""` marshal;
      Time RFC3339-UTC second precision)
- [x] swap the 4 embed files; port `Task.MarshalJSON`'s surrogate shape verbatim
      (deliberate design — see pinned decisions)
- [x] reword the 8 doc comments; drop `gildas/go-core` from go.mod; `go mod tidy`;
      record the actual `go list -m all | wc -l` (x/exp leaves and drags others —
      expect ~31, but record, don't assert)
- [x] update CLAUDE.md's dependency paragraph ("go-core is the one dependency kept" is
      now false — describe the local types in internal/common) and the cache-duration
      sentence if not already done in Task 2
- [x] full gate + PR flow

### Task 4: `pr create --reviewer none` + README `--draft` gap (one PR)

**Files:**
- Modify: `internal/pullrequest/create.go` (+ create tests),
  `internal/pullrequest/pullrequest.go` (completion doc comment + create-only wrapper)
- Modify: `README.md`, `skill/bitbucket-cli/SKILL.md`

- [x] implement `none` per the pinned semantics (any-position error when combined;
      exactly `["none"]` → nil reviewers, zero resolution requests; checked before the
      `default` branch and `expandAllReviewers`)
- [x] switch the reviewer read to `cmd.Flags().GetStringSlice("reviewer")` (drop the
      field from the package-level binding if nothing else reads it)
- [x] create-only completion wrapper offering `none`/`default`/`all` before nicknames;
      shared `reviewerCompletionFunc` untouched; its :285-297 doc comment updated
- [x] tests (httptest, request-recording): `--reviewer none` → payload has NO reviewers
      key and zero effective-default-reviewers requests; `--reviewer none --reviewer
      alice` and `--reviewer none,alice` and `--reviewer none --reviewer all` all error
      with the pinned message before any request; plain create still resolves defaults;
      `--reviewer default` unchanged; dry-run echo shows no reviewers key
- [x] prove the none-sentinel test fails on pre-fix master (throwaway worktree)
- [x] README: document `--reviewer none` AND add `--draft` to the create section (exists
      since the fork's start; SKILL.md already documents it); note the FR-6 symmetry in
      the PR body (dry-run and real runs skip resolution identically under `none`)
- [x] SKILL.md: add `none` to the create grammar + one line on when to use it; verify
      the `--draft` mention is still accurate
- [x] full gate + PR flow

### Task 5: scoped review pass

- [x] one review pass over the accumulated diff (base = master before Task 1): Go
      correctness with emphasis on the byte-compatibility claims (renderer goldens, wire
      fixtures, sort tie order) + doc accuracy; findings logged to the progress file
      (12 findings; fixer PR #73)
- [x] fixer PR(s) if warranted; critical re-check until clean
      (critical re-check: 5 majors + 2 minors; fixer PR #74)
- [x] tick this plan's boxes, move to `docs/plans/completed/` (`git add -f`); log the
      outcome; update roadmap memory (final dep/module counts, where the ported types
      live)
      (closing verification: 1 doc major — reviewer-sentinel semantics — fixed in this PR)

## Post-Completion

- Next release picks these up (suggest v0.3.0 — minor: new `--reviewer none`; dep
  removals invisible). Owner's call when to cut.
