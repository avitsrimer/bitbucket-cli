# Pull request draft state: `--ready`/`--draft` on update, `draft` on get/list

## Overview

`bb pullrequest create --draft` opens a pull request as a draft, but nothing in `bb` can take it
out of draft again (or put it back), and nothing can read the draft state: `bb pullrequest get
<id> -o json` has no `draft` key and `--columns draft` is not offered. A fully scripted
"open as draft → add reviewers → promote" flow currently breaks out to the web UI for the last
step, and a caller who just ran `create --draft` cannot even confirm the flag took effect.

This plan:

- adds `--ready` (clear draft) and `--draft` (set draft) to `bb pullrequest update`, mutually
  exclusive with each other, combinable with every other `update` flag in the same PUT;
- adds a `Draft` field to the `PullRequest` model so `get`/`list -o json|yaml` carry `draft`,
  `--columns draft` (and `--sort draft`) work on `get` and `list`, and `get`'s default table
  shows it;
- documents both halves in README.md and the embedded `skill/bitbucket-cli/SKILL.md`.

## Context (from discovery)

**Root cause of the read gap (verified, not assumed).** The Bitbucket Cloud OpenAPI document
(`https://dac-static.atlassian.com/cloud/bitbucket/swagger.v3.json`) declares `draft` as a
`boolean` property of the `pullrequest` schema, and `PUT
/repositories/{workspace}/{repo_slug}/pullrequests/{pull_request_id}` takes that same
`pullrequest` schema as its request body. So:

- the API **does** return `draft`; `bb` drops it because `internal/pullrequest/pullrequest.go:27-48`'s
  `PullRequest` struct has no `Draft` field, and `encoding/json` silently discards unknown keys.
  The fix is a mapping fix, not an API workaround.
- `draft` **is** accepted alongside `title`/`description`/`reviewers`/... in one PUT: the body is
  the whole pull request object. `--ready` can therefore be combined with `--title`,
  `--add-reviewer`, etc. in a single invocation. No separate request is needed.

**Files/components involved:**

- `internal/pullrequest/pullrequest.go` — `PullRequest` struct (:27-48), `columns` table (:58-~140,
  `common.Columns[PullRequest]` with `Name`/`Compare`), `GetHeaders` (:171-180, `get` appends
  `description` to the defaults), `GetRow` (:185-231, `switch common.NormalizeColumnKey(header)`).
- `internal/pullrequest/update.go` — `updateOptions` (:30-38), flag registration in `init()`
  (:40-56), `updateProcess` (:71-164; GET → mutate in-memory `PullRequest` → strip server-owned
  fields → `common.WhatIfPayload` → PUT), `applySimpleFieldUpdates` (:168-192; one
  `cmd.Flag(name).Changed` block per simple field, returns `updateWanted`).
- `internal/pullrequest/create.go:20-28` — `PullRequestCreator` already has
  `Draft bool json:"draft,omitempty"`; `--draft` is bound to `createOptions.Draft` (:52).
- `internal/pullrequest/update_test.go` — `registerUpdateFlags` (:51-68) mirrors `init()`'s flags
  onto a standalone `*cobra.Command`; `TestApplySimpleFieldUpdates` (:104), the
  `TestUpdateProcess*` httptest-driven tests (:417+) decode the PUT body into a `PullRequest`.
- `internal/pullrequest/description_file_test.go:11-43` — the established pattern for testing a
  real `MarkFlagsMutuallyExclusive` registration on an isolated throwaway command through
  `cmd.Execute()` (the exclusivity check runs in cobra's execute path, not on a direct `RunE` call).
- `internal/pullrequest/pullrequest_row_internal_test.go` — `TestPullRequestGetRowCoversEveryColumn`
  iterates `columns.Columns()` and fails if any declared column falls through to `GetRow`'s
  default `common.EmptyCell` arm; `pullrequest_row_test.go` has `TestPullRequestGetHeadersDefault`,
  `TestPullRequestGetHeadersGetIncludesDescription`, `TestPullRequestGetRow`.
- `testdata/pullrequest.json`, `testdata/pullrequests.json`, `testdata/pullrequest-with-participants.json`,
  `testdata/pullrequest-no-dest-repo.json` — existing fixtures, none carries `draft`.
- `README.md:798-806` (`create --draft`), `README.md:837-866` (`update` section);
  `skill/bitbucket-cli/SKILL.md:147` (`create` synopsis with `--draft`), `:167-176` (`update`
  synopsis). `internal/cmd/skill_sync_test.go` guards command paths only, not flags — SKILL.md
  must be updated by hand in this change.

**Patterns to follow:**

- New flag-reading code reads directly off `cmd` (`cmd.Flag("ready").Changed`), never off a
  package-level binding (CLAUDE.md "Go conventions"). `--ready`/`--draft` are pure boolean
  presence flags, so `Changed` is the whole signal; no `updateOptions` field is needed.
- Mutual exclusion via cobra's `MarkFlagsMutuallyExclusive`, registered in a small
  `registerDraftStateFlags(cmd *cobra.Command)` helper (mirroring `registerDescriptionFileFlag`)
  so the test-side `registerUpdateFlags` and the isolated-command exclusivity test exercise the
  real registration rather than a copy.
- Booleans render in table cells with `strconv.FormatBool` (see `internal/repository/repository.go:166`,
  `internal/pullrequest/comment/comment.go:276`).
- Stdlib errors, lowercase in-code comments describing current behavior only, conventional
  commits, no AI mentions, tests table-driven with `httptest`.
- **All test data uses placeholders** (`acme/widgets`, ids like `42`, `{00000000-0000-0000-0000-000000000001}`
  style UUIDs); never real workspace/repo/user identifiers.

**Planning-mode assumptions** (the request arrived from an autonomous peer session with the
instruction not to wait for answers, so the interactive planning questions were answered with
these defaults):

- Goal: feature development as specified above; scope exactly the two halves requested.
- Testing approach: **Regular** (code, then tests in the same task), matching the repo's
  existing style.
- `draft` joins `pullrequest get`'s default table columns (a single-row table has room, and
  "include `draft` in `pullrequest get` output" was the explicit ask); it does **not** join
  `list`'s default set (kept narrow like `description`/`participants`), but is reachable via
  `--columns draft` / `--sort draft` on both commands, and always present in `-o json|yaml`.
- `--ready`/`--draft` behave like `--close-source-branch`: passing the flag marks the update as
  wanted even if the PR is already in that state (the PUT is a harmless no-op server-side). No
  client-side "already ready" short-circuit.
- `PullRequest.Draft` is tagged `json:"draft"` **without** `omitempty`: `--ready` must serialize
  `"draft": false`, and `-o json` output should always show the key. Since `updateProcess` PUTs
  the GET'd object back, an update that does not touch draft state echoes the current value
  unchanged — a no-op for the server.
- Closed/merged/declined pull requests: Bitbucket refuses to mutate them ("Only open pull
  requests can be mutated"); the existing PUT error path surfaces that. No extra client check.

## Development Approach

- **testing approach**: Regular (code first, then tests, within each task)
- complete each task fully before moving to the next
- make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
  - tests are not optional - they are a required part of the checklist
  - write unit tests for new functions/methods and modified functions/methods
  - cover both success and error scenarios
- **CRITICAL: all tests must pass before starting next task** (`go test -race ./...`)
- **CRITICAL: update this plan file when scope changes during implementation**
- run `gofmt -l .`, `go vet ./...`, `golangci-lint run` after each task; the repo's lint config is
  strict (`default: none`, ~45 linters) and CI runs golangci-lint **v2.12.2**, which is the
  locally installed version
- maintain backward compatibility: existing flags, defaults for `list`, and JSON output keys are
  only ever extended, never renamed or removed

## Testing Strategy

- **unit tests**: required for every task (see Development Approach above)
- **e2e tests**: none in this project (CLI with `httptest`-driven command tests); no e2e work
- **manual verification** against a real Bitbucket workspace is a Post-Completion item — it needs
  the operator's own credentials and a throwaway pull request, and is not something the
  implementation session runs on its own

## Progress Tracking

- mark completed items with `[x]` immediately when done
- add newly discovered tasks with ➕ prefix
- document issues/blockers with ⚠️ prefix
- update plan if implementation deviates from original scope
- keep plan in sync with actual work done

## Solution Overview

1. **Model**: add `Draft bool \`json:"draft"\`` to `PullRequest` (next to `CloseSourceBranch`).
   This alone fixes `-o json|yaml` for `get`, `list`, and `update`'s printed result.
2. **Columns**: add a `draft` entry to the `columns` table (compare `false < true`, tie-break on
   id) and a `case "draft"` arm in `GetRow` rendering `strconv.FormatBool(pullrequest.Draft)`;
   append `draft` to `GetHeaders`' `get` defaults (after `description`, keeping the existing
   order otherwise).
3. **Update flags**: `registerDraftStateFlags(cmd)` registers `--ready` ("mark the pullrequest
   ready for review, clearing its draft status") and `--draft` ("mark the pullrequest as a
   draft") as `Bool` flags and calls `cmd.MarkFlagsMutuallyExclusive("ready", "draft")`.
   `applySimpleFieldUpdates` gains two `Changed` blocks: `--ready` → `pullrequest.Draft = false`,
   `--draft` → `pullrequest.Draft = true`, each setting `updateWanted = true`. Everything else in
   `updateProcess` (GET-first, server-owned field stripping, `WhatIfPayload` gate, PUT) is
   unchanged, so `--ready` composes with `--title`/`--add-reviewer`/... automatically and
   `--dry-run` shows the resolved payload including `"draft": false`.
4. **Docs**: README `update` section and SKILL.md `update` synopsis gain the two flags and the
   "can be combined with other update flags in one call" note; README/SKILL.md `get`/`list`
   column notes mention `draft`.

## Technical Details

- `PullRequest.Draft bool \`json:"draft"\`` — no `omitempty`. Fixtures: add `"draft": true` to one
  existing fixture pull request (in `testdata/pullrequests.json`, so a list-decoding test can
  assert both a `true` and an absent→`false` entry) rather than adding a new fixture file (every
  `testdata/` file must be read by a test; extending one that already is keeps that invariant
  trivially).
- Column entry:
  ```go
  {Name: "draft", DefaultSorter: false, Compare: func(a, b PullRequest) bool {
      if a.Draft != b.Draft {
          return !a.Draft && b.Draft
      }
      return a.ID < b.ID
  }},
  ```
- `GetRow`: `case "draft": row = append(row, strconv.FormatBool(pullrequest.Draft))`.
- `GetHeaders`: `get` defaults become `ID, Title, source, destination, state, description, draft`.
  Check `TestPullRequestGetHeadersGetIncludesDescription` and any `get` golden/table test and
  update expectations accordingly.
- `update.go`:
  ```go
  // registerDraftStateFlags registers the mutually exclusive --ready/--draft pair on cmd.
  func registerDraftStateFlags(cmd *cobra.Command) {
      cmd.Flags().Bool("ready", false, "Mark the pullrequest as ready for review (clears its draft status). Mutually exclusive with --draft; combinable with every other update flag in the same request")
      cmd.Flags().Bool("draft", false, "Mark the pullrequest as a draft. Mutually exclusive with --ready; combinable with every other update flag in the same request")
      cmd.MarkFlagsMutuallyExclusive("ready", "draft")
  }
  ```
  called from `init()`; and in `applySimpleFieldUpdates`:
  ```go
  if cmd.Flag("ready").Changed {
      pullrequest.Draft = false
      updateWanted = true
  }
  if cmd.Flag("draft").Changed {
      pullrequest.Draft = true
      updateWanted = true
  }
  ```
  (`cmd.Flag(name)` returns nil for an unregistered flag and `.Changed` would panic — the test
  helper `registerUpdateFlags` must therefore call `registerDraftStateFlags(cmd)` too, exactly as
  production `init()` does.)
- Processing flow for `bb pullrequest update 42 --ready --add-reviewer X`: validate id → GET PR
  → `applySimpleFieldUpdates` sets `Draft=false` → reviewer resolution as today → strip
  server-owned fields → `WhatIfPayload` (dry-run echo shows `"draft": false`) → single PUT.

## What Goes Where

- **Implementation Steps** (`[ ]` checkboxes): code, tests, docs in this repo.
- **Post-Completion** (no checkboxes): manual verification against a live Bitbucket workspace,
  release/tag.

## Implementation Steps

### Task 1: Add `Draft` to the `PullRequest` model and surface it as a column

**Files:**
- Modify: `internal/pullrequest/pullrequest.go`
- Modify: `internal/pullrequest/pullrequest_row_test.go`
- Modify: `internal/pullrequest/pullrequest_row_internal_test.go`
- Modify: `testdata/pullrequests.json` (add `"draft": true` to one entry, placeholders only)
- ➕ Modify: `testdata/pullrequest.json` (add `"draft": false`): `PullRequestSuite.TestCanUnmarshal`
  round-trips that fixture with `JSONEq`, and the `omitempty`-free `draft` key must therefore be
  present in the fixture too (matching the real API, which always returns it)
- Modify: any existing test asserting `get`'s default header list

- [x] add `Draft bool \`json:"draft"\`` to `PullRequest` in `internal/pullrequest/pullrequest.go`
      (no `omitempty`), placed next to `CloseSourceBranch`
- [x] add the `draft` entry to the `columns` table (`false < true`, tie-break on `ID`) so
      `--columns draft` / `--sort draft` are accepted and completed on `get` and `list`
- [x] add `case "draft"` to `GetRow` rendering `strconv.FormatBool(pullrequest.Draft)`
- [x] append `draft` to `GetHeaders`' `get`-only defaults (after `description`); update the
      `GetHeaders` doc comment to say why `draft` is in `get`'s defaults but not `list`'s
- [x] set `Draft: true` on the fully-populated target in `TestPullRequestGetRowCoversEveryColumn`
      so the every-column guard exercises the new arm with a non-default value
- [x] extend `TestPullRequestGetRow` (headers + expected row) with `draft`; add a table-driven
      test that `GetRow([]string{"draft"})` renders `"true"`/`"false"`
- [x] update `TestPullRequestGetHeadersGetIncludesDescription` (and any other default-header
      assertion for `get`) for the new `draft` default; assert `list`'s defaults are unchanged
- [x] add a decode test: unmarshal `testdata/pullrequests.json` (or a small inline placeholder
      JSON) and assert the entry with `"draft": true` decodes to `Draft == true` and an entry
      without the key decodes to `false`; assert `json.Marshal` of a `PullRequest` always emits
      the `draft` key (both `true` and `false`)
- [x] add a `--sort draft` test (via `columns.SortBy("draft")` / `common.Sort`) proving
      non-drafts sort before drafts and ties fall back to id
- [x] run `go test -race ./internal/pullrequest/...` and `golangci-lint run ./internal/pullrequest/...` — must pass before task 2

### Task 2: Add `--ready`/`--draft` to `bb pullrequest update`

**Files:**
- Modify: `internal/pullrequest/update.go`
- Modify: `internal/pullrequest/update_test.go`
- Create: `internal/pullrequest/draft_state_flags_test.go` (isolated-command exclusivity tests,
  mirroring `description_file_test.go`)

- [x] add `registerDraftStateFlags(cmd *cobra.Command)` in `internal/pullrequest/update.go`
      registering `--ready` and `--draft` as `Bool` flags with help text stating they are mutually
      exclusive with each other and combinable with every other update flag in one request; call
      `cmd.MarkFlagsMutuallyExclusive("ready", "draft")`; invoke it from `init()`
- [x] extend `applySimpleFieldUpdates` with the two `cmd.Flag(...).Changed` blocks
      (`--ready` → `Draft=false`, `--draft` → `Draft=true`), reading off `cmd`, not a package var
- [x] have the test helper `registerUpdateFlags` in `update_test.go` call
      `registerDraftStateFlags(cmd)` instead of hand-registering copies
- [x] extend `TestApplySimpleFieldUpdates` with table cases: `--ready` on a draft PR → `Draft=false`,
      `changed=true`; `--draft` on a non-draft PR → `Draft=true`, `changed=true`; `--ready` alone
      with no other flag still reports `changed=true`
- [x] add `TestUpdateProcessReadyClearsDraftInPutBody`: httptest server whose GET returns
      `{"id":42,"title":"T","draft":true}`; run `updateProcess` with `--ready`; assert exactly
      GET+PUT, decode the PUT body as `map[string]any` and assert `body["draft"] == false` (i.e.
      the key is present and false — the `omitempty`-free contract), and that the printed result
      round-trips
- [x] add `TestUpdateProcessDraftSetsDraftInPutBody` (the symmetric case, `draft:false` → `--draft`
      → `body["draft"] == true`)
- [x] add `TestUpdateProcessReadyCombinesWithTitle`: `--ready --title "New"` produces one PUT whose
      body has both `"draft": false` and the new title (proves single-request combination)
- [x] add `TestUpdateProcessUntouchedDraftIsEchoedUnchanged`: GET returns `draft:true`, update with
      only `--title`; assert PUT body still has `"draft": true` (no accidental promotion)
- [x] add `TestUpdateProcessReadyDryRun`: with `--dry-run`, exactly one GET and no PUT; the
      stderr echo from `WhatIfPayload` contains `"draft": false`
- [x] in `draft_state_flags_test.go`, using an isolated throwaway command + `registerDraftStateFlags`
      + `cmd.Execute()`: `--ready --draft` together is rejected with cobra's "none of the others
      can be" mutual-exclusion error; `--ready` alone and `--draft` alone execute cleanly;
      neither flag given is not an error
- [x] run `go test -race ./internal/pullrequest/...` and `golangci-lint run ./internal/pullrequest/...` — must pass before task 3

### Task 3: Documentation (README + embedded SKILL.md)

**Files:**
- Modify: `README.md`
- Modify: `skill/bitbucket-cli/SKILL.md`

- [x] README `update` section (around `README.md:837`): add an example promoting a draft
      (`bb pullrequest update 1 --ready`) and one combining it with other flags
      (`--ready --add-reviewer ...`), state that `--ready` and `--draft` are mutually exclusive,
      and that either combines with every other `update` flag in a single request
- [x] README `create --draft` paragraph (around `:798`): add one sentence pointing to
      `update --ready` for promotion and to `get`'s `draft` column / `-o json` `draft` key for
      confirming the state
- [x] README `get`/`list` column documentation (locate the existing `--columns` prose for
      pullrequests): list `draft` as an available column and note it is in `get`'s defaults but
      not `list`'s
- [x] SKILL.md `update` synopsis (`:167-176`): add `[--ready | --draft]` to the synopsis and a
      sentence on exclusivity + single-request combination
- [x] SKILL.md `get`/`list` notes: mention `draft` as a `--columns` value and as an always-present
      `-o json` key, so an agent knows how to verify a draft state after `create --draft`
- [x] run `go test -race ./internal/cmd/...` (skill sync-guard) — must pass before task 4
- ⚠️ the plan's "`--sort draft` works on `get` and `list`" was not quite right: `--sort` is only
  registered on `list` (`internal/pullrequest/list.go:116`; `get` never reads it), consistent with
  the pre-existing SKILL.md statement that `--sort` is a `list`-only flag. The docs therefore say
  `--columns draft` works on both and `--sort draft` on `list` only. No code change needed.
- ⚠️ verified before documenting "`--ready --draft` is rejected before any request": cobra runs
  `PersistentPreRun`/`PreRun` ahead of `ValidateFlagGroups`, but nothing on the `pullrequest
  update` path defines either hook, so the mutual-exclusion error fires before `RunE` and before
  any HTTP.

### Task 4: Verify acceptance criteria
- [x] `bb pullrequest update <id> --ready` sends one PUT with `"draft": false`; `--draft` sends
      `"draft": true`; `--ready --draft` is rejected before any request
      — evidence: `TestUpdateProcessReadyClearsDraftInPutBody` / `TestUpdateProcessDraftSetsDraftInPutBody`
      (`update_test.go`, assert exactly GET+PUT and the decoded `draft` value);
      `TestRegisterDraftStateFlags` (`draft_state_flags_test.go`, isolated cmd + `cmd.Execute()`).
      "Before any request" is structural: nothing on the `pullrequest update` path defines
      `PreRun`/`PersistentPreRun` (`grep` over `internal/cmd` + `internal/pullrequest` finds only
      `install_skill.go`'s `PreRunE`), and the profile/HTTP client is only built inside `RunE`.
      Confirmed on the built binary offline: `bb --config /nonexistent/dir/config-cli.yml --profile
      does-not-exist pullrequest update 42 --ready --draft` exits 1 with the sole error `if any flags
      in the group [ready draft] are set none of the others can be; [draft ready] were all set`.
- [x] `--ready` combines with `--title`/`--description`/`--destination`/`--close-source-branch`/
      `--add-reviewer`/`--remove-reviewer` in one PUT (covered by the combination test plus code
      reading: no early return added)
      — evidence: `TestUpdateProcessReadyCombinesWithTitle` (title) and the new
      `TestUpdateProcessReadyCombinesWithSimpleFields` (description + destination +
      close-source-branch, all asserted in the single PUT body next to `"draft": false`). Reviewers by
      code reading: `applySimpleFieldUpdates` has no early return, and `updateProcess` runs
      `removeRequestedReviewers`/`addRequestedReviewers` on the same in-memory `pullrequest` after it,
      before the one `profile.Put` (`update.go:105-176`).
- [x] `bb pullrequest get <id> -o json` includes `draft`; the default `get` table shows a `draft`
      column; `bb pullrequest list --columns draft` and `--sort draft` work; `list` defaults
      unchanged
      — evidence: `TestPullRequestMarshalAlwaysEmitsDraft` (`-o json` key, both values) +
      `TestPullRequestDraftDecodesFromFixture`; `TestPullRequestGetHeadersGetIncludesDescription` and
      `TestPullRequestGetHeadersAuthorMode` (`get` defaults end in `description, draft`; `list`/nil
      defaults unchanged, `TestPullRequestGetHeadersDefault`); `TestPullRequestGetRowCoversEveryColumn`,
      `TestPullRequestGetRowDraft`, `TestPullRequestGetRow` (column rendering);
      `TestPullRequestColumnsSortByDraftOrdersNonDraftsFirst` (`--sort draft`). On the binary, offline:
      `bb __complete pullrequest {get,list} --columns ""` and `list --sort ""` all list `draft`;
      `list --columns draft --sort draft` passes flag parsing (fails later on local repository
      resolution), while `--columns bogus` is rejected at parse time with an enumeration containing `draft`.
- [x] `--dry-run` on `update --ready` still performs the GET (fails identically for a nonexistent
      PR) and echoes the payload with `draft` without PUTting
      — evidence: `TestUpdateProcessReadyDryRun` (exactly one GET, no PUT, stderr echo contains
      `"draft": false`) and the new `TestUpdateProcessReadyNonexistentPullRequest` (GET 404 with and
      without `--dry-run`: same `failed to get pullrequest 42` / `pull request not found` error, one GET,
      no PUT in both modes).
- [x] no real workspace/repository/user identifiers in any new test data or fixture change
      — evidence: `git diff master...HEAD | grep -inE 'sportscape|sportpursuit|privatesportshop|vits|rimer|avitsrimer'`
      matches nothing; a broader scan of added lines for `@`/`.com`/`.org`/`workspace` matches only
      `spf13/cobra` import/type lines. Fixture additions are a bare `"draft": true/false` key on existing
      entries; new tests use `42`, `T`, `main`, `develop` placeholders only.
- [x] run full suite: `go build ./... && go vet ./... && gofmt -l . && go test -race ./... && golangci-lint run`
      — `gofmt -l .` printed nothing, every package `ok`, `GOTOOLCHAIN=local golangci-lint run` → `0 issues.`
- [x] run `make cross-build` (portability guard) — must pass
      — `GOOS=linux CGO_ENABLED=0 GOTOOLCHAIN=local go build ./...` + `go vet ./...` exit 0
- ➕ added `TestUpdateProcessReadyCombinesWithSimpleFields` and `TestUpdateProcessReadyNonexistentPullRequest`
  (`internal/pullrequest/update_test.go`) to close the two criteria that previously rested on
  `--title`-only combination and on code reading alone
- ⚠️ `--columns`' `--help` text is the generic "Comma-separated list of columns to display" for every
  command (`common.RegisterColumnsFlag`), so the help never enumerates columns; `draft` is discoverable
  only via shell completion and the parse-time error for an invalid value. Pre-existing behavior, not
  a gap introduced here.

### Task 5: [Final] Update documentation and close out
- [x] update `CLAUDE.md` "What this is" paragraph: mention `pullrequest update`'s
      `--ready`/`--draft` pair (mutually exclusive, no `--force`, no confirmation — it is an
      ordinary mutating update gated by `WhatIfPayload` like the rest of `update`)
- [x] confirm no "Known Issues" section was added anywhere
- [x] move this plan to `docs/plans/completed/` and `git add -f` it
- [x] commit on a feature branch (conventional commit, e.g.
      `feat(pullrequest): add --ready/--draft to update and surface draft on get/list`), no AI
      mentions; do not include the pre-existing unrelated working-tree changes (`CLAUDE.md`'s
      jbcontext block, `.mcp.json`)
- ⚠️ `CLAUDE.md` carried an unrelated, pre-existing uncommitted `<!-- jbcontext-instructions-start -->`
  block appended at the end of the file. Only the "What this is" hunk was staged, via a hand-trimmed
  patch fed to `git apply --cached` (`git add CLAUDE.md` would have swept the jbcontext block in);
  the block stays in the working tree, uncommitted, together with the untracked `.mcp.json`.

## Post-Completion
*Items requiring manual intervention or external systems - no checkboxes, informational only*

**Manual verification** (operator, with their own Bitbucket Cloud profile and a throwaway
repository):
- `bb pullrequest create --title t --source b --destination main --reviewer none --draft`, then
  `bb pullrequest get <id> --columns id,draft` shows `true`; `bb pullrequest update <id> --ready`
  and `get` again shows `false`; `update <id> --draft` flips it back.
- `bb pullrequest update <id> --ready --add-reviewer <uuid>` lands both changes in one call
  (confirm in the web UI that draft status cleared and the reviewer was added).
- On a merged/declined PR, `update <id> --ready` returns Bitbucket's "only open pull requests can
  be mutated" style error rather than a fabricated success.

**External system updates**:
- Cut a release tag (`v0.5.0` or next appropriate) so the Homebrew cask picks up the new flags;
  the peer's environment was `bb 0.4.0` via Homebrew.
- Notify the requesting session that `--ready`/`--draft` and the `draft` column/JSON key are
  available once merged.

## Review

**What shipped** (branch `feat/pullrequest-draft-state`, five commits):

- `060b8d0` feat(pullrequest): add draft field and column to the pullrequest model — `Draft bool
  json:"draft"` (no `omitempty`) on `PullRequest`, a `draft` entry in the `columns` table
  (non-drafts first, tie-break on `ID`), a `case "draft"` arm in `GetRow` rendering
  `strconv.FormatBool`, `draft` appended to `get`'s default headers, and `"draft"` keys added to
  the `pullrequest.json`/`pullrequests.json` fixtures.
- `6558518` feat(pullrequest): add --ready/--draft to update — `registerDraftStateFlags` registers
  the mutually exclusive boolean pair on `updateCmd`; `applySimpleFieldUpdates` reads
  `cmd.Flag("ready"/"draft").Changed` directly and sets `Draft` false/true, marking the update
  wanted; everything else in `updateProcess` (GET → mutate → strip server-owned fields →
  `WhatIfPayload` → single PUT) is untouched, so either flag composes with every other `update`
  flag in one request.
- `4f7822d` docs(pullrequest): document --ready/--draft on update and the draft column — README
  `create --draft`/`update`/column prose and the embedded `skill/bitbucket-cli/SKILL.md` `update`
  synopsis and `get`/`list` notes.
- `1521d4f` test(pullrequest): cover --ready with other simple fields and nonexistent-target
  dry-run — `TestUpdateProcessReadyCombinesWithSimpleFields` and
  `TestUpdateProcessReadyNonexistentPullRequest`.
- this commit: `CLAUDE.md` "What this is" clause for the flag pair and the `draft` column/JSON key,
  plus this plan moved into `docs/plans/completed/`.

**Discoveries recorded during Tasks 1-4:**

- ➕ (Task 1) `testdata/pullrequest.json` also needed a `"draft": false` key:
  `PullRequestSuite.TestCanUnmarshal` round-trips that fixture with `JSONEq`, and an
  `omitempty`-free `draft` tag always marshals the key — matching the real API, which always
  returns it.
- ⚠️ (Task 3) the plan's "`--sort draft` works on `get` and `list`" was inaccurate: `--sort` is
  registered only on `list` (`internal/pullrequest/list.go:116`; `get` never reads it). Docs say
  `--columns draft` on both, `--sort draft` on `list` only. No code change.
- ⚠️ (Task 3) "`--ready --draft` is rejected before any request" was verified structurally: cobra
  runs `PersistentPreRun`/`PreRun` before `ValidateFlagGroups`, but nothing on the `pullrequest
  update` path defines either hook, so the mutual-exclusion error fires before `RunE` and before
  any HTTP.
- ➕ (Task 4) added `TestUpdateProcessReadyCombinesWithSimpleFields` and
  `TestUpdateProcessReadyNonexistentPullRequest` to close the two acceptance criteria that had
  rested on `--title`-only combination and on code reading alone.
- ⚠️ (Task 4) `--columns`' `--help` text is the generic "Comma-separated list of columns to
  display" for every command (`common.RegisterColumnsFlag`), so help never enumerates columns;
  `draft` is discoverable via shell completion and the parse-time error for an invalid value.
  Pre-existing behavior, not a gap introduced here.
- ⚠️ (Task 5) `CLAUDE.md`'s unrelated pre-existing jbcontext block was kept out of the commit with
  a hand-trimmed patch and `git apply --cached` (see the Task 5 note).

**Next:** a post-task automated review pass runs after this close-out — the quality,
implementation, testing, simplification, and documentation review agents plus a code-smells pass.
Their findings are logged in `docs/plans/progress-pullrequest-draft-state.txt`.
