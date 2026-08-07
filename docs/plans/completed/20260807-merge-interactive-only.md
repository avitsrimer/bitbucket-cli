# `bb pullrequest merge` — interactive-only confirmation (no --force)

## Overview

Make `bb pullrequest merge` require an interactive y/N confirmation from a human, with
NO `--force` escape hatch (OWNER DECISION 2026-08-07: "Strict, no --force" — merging is
deliberately not automatable via bb; scripts and agents must hand the merge to the
user). Extends the fork's visible-guardrails philosophy to the one irreversible PR
action.

## The stdin contract (decided here, binding — docs must state EXACTLY this)

`StdinIsInteractive` tests `os.Stdin`'s `ModeCharDevice` bit, so three real stdin
shapes exist and behave differently. The decided behavior:

1. **pipe / file redirect** (`echo y | bb pr merge 1`, `< file`): not a char device →
   ERROR before any prompt.
2. **`/dev/null`** (cron, `nohup`, many agent harnesses): IS a char device, so the
   mode check passes — but the read returns EOF with no input. ConfirmInteractive
   treats EOF-without-input as an ERROR (not a silent decline): nobody answered, so
   nothing may look like a handled cancel. (This is stricter than `Confirm`'s current
   silent-decline-on-EOF; that difference is deliberate and documented.) Pinned
   predicate: the shared read helper returns `(line string, err error)` (Confirm keeps
   discarding err); ConfirmInteractive errors iff
   `line == "" && errors.Is(err, io.EOF)` — a reader like `strings.NewReader("y")`
   (answer, no trailing newline) returns `"y", io.EOF` and is a REAL answer, not an
   error. Consequence to document in README's merge passage: an interactive Ctrl-D at
   the prompt now exits non-zero with `cannot confirm merge: …` rather than printing
   `Merge canceled`.
3. **real terminal or pty**: prompts and waits. A pty-puppeting agent could answer —
   undetectable client-side; the mitigation is the SKILL.md imperative ("never invoke
   `bb pullrequest merge` yourself — on a pty it blocks your session and there is no
   `--force`; surface the ready-to-merge state and ask the user").

Pinned strings (code, docs, and tests must agree verbatim):
- prompt: `Merge pullrequest %s?` (Confirm's shared body appends ` [y/N] `)
- decline output: `Merge canceled` (to `cmd.OutOrStdout()`; note: trigger/stop use
  bare `fmt.Println` — do NOT copy that, use `cmd.OutOrStdout()`)
- non-interactive/EOF error: `%s: merging requires an interactive terminal` — must NOT
  contain the word "force"
- call-site wrapper: `cannot confirm merge: %w`

## Context (from discovery — master 4d84f55 / v0.1.0; verified by plan review)

- `internal/pullrequest/merge.go` — `mergeProcess`: profile → repository → PR id
  (`GetPullRequestIDFromArgs`, NOT branch-aware) → `ExistsPullRequest` → uripath +
  payload → `common.WhatIfPayload` gate (--dry-run stops here) → `Post` (sync) or
  `PostWithResult` (`--async`). No `--force` flag registered; none exists at root
  either (only trigger.go:39 / stop.go:24 register one, locally).
- `internal/common/confirm.go` — `Confirm`: y/N via `cmd.InOrStdin()`; --dry-run
  short-circuit; `--force` skip; non-char-device real-stdin → error suggesting
  --force. Unsuitable verbatim for merge (force read + force-suggesting error +
  silent decline on EOF).
- Pattern siblings: `Confirm` called at `trigger.go:105` (decline 109-110, prompt
  `"Trigger a new pipeline on %s?"`) and `stop.go:48` (decline 52-53, prompt
  `"Stop pipeline %s?"`) — wrappers `cannot confirm pipeline trigger/stop: %w`.
- Existing merge tests in `internal/pullrequest/action_test.go`:
  `TestMergeProcessSyncSuccess` (:290), `TestMergeProcessAsyncSuccessUsesLocationHeader`
  (:337), `TestMergeProcessAPIError` (:384) reach the gate and need
  `cmd.SetIn(strings.NewReader("y\n"))`. The other four (:417, :451, :481 dry-run,
  :503 invalid id) return before the gate — leave them alone. Failure mode if one is
  missed: `go test` stdin is `/dev/null` → EOF → (post-fix) error — the test fails
  loudly, not hangs.
- Test helpers `poisonStdin`/`swapStdinToNonInteractivePipe` live UNEXPORTED in
  `internal/pipeline/helpers_test.go:46-66` (and `swapStdin`/`poisonReader` in
  `internal/common/confirm_test.go:26-70`) — the merge-level tests need local copies
  in `internal/pullrequest` (or move shared ones to `internal/testutil` if no import
  cycle; `testutil` imports repository/user/workspace only — pullrequest package
  internal tests CAN import it, check first).
- `testutil.CaptureStderr` (`internal/testutil/testutil.go:178`) exists for the
  prompt-content assertion.

## Binding rules

1. OWNER DECISION: strict — no `--force` on merge, no bypass of any kind (no env var,
   no config). The stdin contract above is the behavior.
2. `--dry-run` UNCHANGED: full preflight, payload echo, no prompt, no write.
3. CLAUDE.md conventions (lowercase comments, no AI mentions, `%w` wrapping,
   table-driven tests, flags off `cmd`).
4. Workflow non-negotiables: throwaway worktree, explicit-path add, commit -F
   tempfile, full gate (`go build ./... && go vet ./... && gofmt -l .` (empty) `&&
   go test -race ./... && golangci-lint run` (v2.12.2) `&& goreleaser check`),
   PR → `gh pr checks --watch` foreground → squash-merge → sync master. Never touch
   the main checkout's tree; a CLAUDE.md pull conflict → STOP and report.
5. Regression tests proven to fail on pre-fix master via throwaway worktree — copy
   ONLY the merge-level tests (decline/EOF/non-TTY ones) there; the ConfirmInteractive
   unit tests won't compile pre-fix and a compile error proves nothing.

## Solution Overview

`common.ConfirmInteractive(cmd, prompt) (bool, error)`: shares the y/N prompt/read
body with `Confirm` via an unexported helper (no duplicated reader); keeps the
`isDryRun` short-circuit (defense in depth — unreachable from merge's call site since
WhatIfPayload fires first, so it gets its own direct unit test); NO `--force` read even
when a force flag exists; non-char-device real-stdin AND EOF-without-input both yield
the pinned error. Gate `mergeProcess` after `WhatIfPayload`, before the async/sync
split. Decline prints `Merge canceled`, returns nil.

## Implementation Steps

### Task 1: strict interactive confirmation on merge (one PR)

**Files:**
- Modify: `internal/common/confirm.go`, `internal/common/confirm_test.go`,
  `internal/common/whatif.go` (godoc only)
- Modify: `internal/pullrequest/merge.go`, `internal/pullrequest/action_test.go`,
  new `internal/pullrequest/merge_confirm_test.go`
- Modify: `skill/bitbucket-cli/SKILL.md`, `README.md`, `CLAUDE.md`

- [x] `common.ConfirmInteractive` per Solution Overview; godoc states the strict
      contract (no force, EOF = error, why it differs from Confirm). Refactor the
      shared prompt/read body out of `Confirm` without changing Confirm's behavior
      (Confirm keeps silent-decline-on-EOF). Update `isDryRun`'s godoc in whatif.go
      ("Shared by WhatIf … and Confirm" → include ConfirmInteractive)
- [x] gate `mergeProcess`: after `WhatIfPayload`, before the async/sync split:
      `proceed, err := common.ConfirmInteractive(cmd, fmt.Sprintf("Merge pullrequest %s?", pullRequestID))`;
      error → `cannot confirm merge: %w`; !proceed → `Merge canceled` to
      `cmd.OutOrStdout()`, return nil
- [x] update `mergeCmd.Short` (and help text) to mention the interactive confirmation
      and the absence of `--force`
- [x] wire `cmd.SetIn(strings.NewReader("y\n"))` into exactly:
      `TestMergeProcessSyncSuccess`, `TestMergeProcessAsyncSuccessUsesLocationHeader`,
      `TestMergeProcessAPIError`; leave the four pre-gate tests alone
- [x] merge-level tests (table-driven where shapes repeat): "y" and "yes" proceed
      (POST sent); "n" and bare-Enter cancel — output contains `Merge canceled`, ZERO
      write requests (request-recording handler); `--async` decline sends nothing;
      `--dry-run` unchanged (preflight GETs run, payload echoed, no prompt consumed —
      poison reader — no POST); prompt CONTENT test: omit the positional, serve one
      open PR, `testutil.CaptureStderr` must contain the resolved id (the fallback is
      not branch-aware — the prompt showing the real target is the point of the gate);
      EOF/no-input at the merge level in BOTH shapes — `cmd.SetIn(strings.NewReader(""))`
      (skips the mode check, exercises the EOF rule) and the non-interactive-pipe
      stdin swap with no SetIn (exercises the mode check) — each asserting the error
      wraps as `cannot confirm merge:` and ZERO write requests; `--force` guard:
      structural `mergeCmd.Flags().Lookup("force") == nil` AND behavioral
      `mergeCmd.ParseFlags([]string{"--force"})` returning an `unknown flag: --force`
      error (in-package — `package pullrequest` tests cannot reach RootCmd:
      internal/cmd imports internal/pullrequest)
- [x] ConfirmInteractive unit tests in confirm_test.go: y/yes/n/Enter; registered+set
      force flag is IGNORED (still prompts); non-char-device real-stdin error equals
      the pinned string and does not contain "force"; EOF-without-input (empty reader)
      errors with the pinned string; dry-run flag set + poison reader → true with no
      read
- [x] docs — the COMPLETE checklist (all assert trigger/stop-only today):
      README.md:27-30 (scope box: merge no longer "runs immediately"),
      README.md:759-772 (merge section: the stdin contract verbatim, no --force),
      README.md:1178 (install-skill section), README.md:1226-1227
      (upstream-differences list), CLAUDE.md:17-18 (opening claim),
      CLAUDE.md:63 (layout comment on Confirm), CLAUDE.md:258-262 (mutating-RunE
      convention: WhatIfPayload / Confirm / ConfirmInteractive for merge),
      SKILL.md:3 (frontmatter "merge PR" phrasing → "hand merge to the user"),
      SKILL.md:19-32 (CRITICAL section: merge moves out of agent-drivable
      state-changers), SKILL.md:136 + :139-141 (merge lines: stdin contract, no
      bypass), SKILL.md:205-218 (MANDATORY --force block gains the merge carve-out:
      "never invoke bb pullrequest merge yourself — on a pty it blocks your session
      and there is no --force; surface the ready-to-merge state and ask the user").
      SKILL.md:273 (merge stays in the --dry-run write list — still accurate)
- [x] pre-fix proof: throwaway worktree at current master tip; copy ONLY
      merge_confirm_test.go's decline/EOF tests (+ the SetIn-less versions of the
      three wired tests if needed); pre-fix, merge posts without consuming stdin —
      decline-sends-nothing and EOF-errors tests must FAIL there; record evidence
- [x] full gate + PR flow

### Task 2: scoped review pass

- [x] one review pass over the diff (Go correctness + doc accuracy on every rewritten
      passage, incl. the stdin contract's three-way claim vs the actual code);
      findings logged to the progress file. Reviewer note: the doubled punctuation in
      `cannot confirm merge: Merge pullrequest 42?: merging requires an interactive
      terminal` matches the trigger/stop sibling shape and is intentional — not a
      defect. Review pass found 4 major + 5 sub-major findings.
- [x] fixer PR(s) if findings warrant; re-check until clean. All 9 findings fixed in
      PR #67; closing re-verification clean at master 4cd1949.
- [x] tick this plan's boxes, move it to `docs/plans/completed/` (`git add -f`) with
      the final PR (or a tiny wrap-up PR); log the outcome. Done in this wrap-up PR.

## Post-Completion

- Ships in the next release (suggest v0.2.0 — minor with a breaking note: any script
  that piped or /dev/null'd `bb pr merge` now errors on the confirmation). Cutting it
  is the owner's call.
