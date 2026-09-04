# Profile secret masking + authorize browser launch

## Overview

Two independent defects, landing as **two sequential PRs** against `master`. Both were found by
diffing `upstream/master` (gildas v0.18.5) against our fork point `4eb91df` (v0.18.4) and then
stress-testing the conclusions; both were reproduced locally before this plan was written. The plan
was then reviewed and revised — see Review Findings Applied at the bottom for what changed and why.

**PR A — mask stored secrets in every json/yaml profile output.** `bb profile get`, `bb profile
list` and `bb profile which` render `password`, `clientSecret` and `accessToken` in cleartext
whenever the output format arrives from a profile's configured `outputFormat` or from
`BB_OUTPUT_FORMAT`, rather than from an explicit `-o json|yaml`. The existing gate
(`explicitJSONOrYAMLOutput`) suppresses only the *vault fetch*; a `--no-vault` profile's secrets are
already in memory from the config file, so gating the fetch changes nothing about what gets printed.
This is currently **documented behaviour** (`skill/bitbucket-cli/SKILL.md:67-81`) rather than an
unknown bug — the fix deliberately removes the caveat by masking with `secretMask` whenever the
**resolved** output format is json/yaml and no explicit `-o` opted in, so the sanctioned scripting
path remains the only way to see a real secret.

Benefit: the documented "renders in ANY json/yaml output regardless of how that format was chosen"
footgun stops existing. A profile-level `outputFormat: json`, a `BB_OUTPUT_FORMAT` exported in a
shell or CI job, or a `.env`, can no longer turn a routine `bb profile list` into a credential dump.

PR A also makes dry-run gating consistent across all four profile-display commands: `list` and `get
<name>` honour `common.WhatIf` today, while `get --current` and `which` return before it and
silently ignore `--dry-run`. This is bundled deliberately, not incidentally — the masking fix has to
touch all four call sites anyway, and leaving two of them inconsistent after editing all four would
be the odd outcome. Called out explicitly in the PR description.

**PR B — make `bb profile authorize` actually open a browser.** `openBrowser` appends the URL
wrapped in literal double quotes on every platform, when only the WSL `cmd.exe /C start` branch
re-parses them; it launches with `.Start()`, so the child's non-zero exit is never observed; and the
URL is printed only through `common.Verbose`, i.e. only under `--verbose`/`--debug`. On macOS the
net effect is that `bb profile authorize <name>` prints `waiting for browser authorization...` and
blocks forever with no browser and no clickable URL.

Benefit: OAuth profile setup works on the fork's only release target (darwin/arm64), and a browser
launch failure leaves the user a URL to click instead of a hang.

## Context (from discovery)

- **Files involved:**
  - PR A: `internal/profile/get.go`, `internal/profile/list.go`, `internal/profile/which.go`,
    `internal/profile/profile.go`, `skill/bitbucket-cli/SKILL.md`, `README.md`
  - PR B: `internal/profile/authorize.go`
- **Patterns found:**
  - `Profile.forDisplay()` (`internal/profile/profile.go:913`) already returns a copy with all three
    secrets replaced by `secretMask` (`"********"`), only when non-empty. Its only current caller is
    `internal/profile/update.go:180` — `return profile.Print(ctx, cmd, profile.forDisplay())`, which
    is exactly the shape PR A needs (real receiver for format resolution, masked payload).
  - `explicitJSONOrYAMLOutput` (`internal/profile/profile.go:524`) is the existing opt-in predicate
    and keeps its meaning unchanged.
  - `setupGitShim` (`internal/repository/clone_test.go`) is the repo's argv-recording
    fake-executable harness, mandated by CLAUDE.md for shell-out code. PR B needs the same shape.
  - `captureStdout` exists **twice**: `internal/profile/display_mask_test.go:24` (package
    `profile_test`) and `internal/profile/update_internal_test.go:22` (package `profile`). Reuse the
    one matching the test file's package — redeclaring either is a compile error.
- **Dependencies identified:** none new. `golang.org/x/term` is banned in-tree
  (`internal/common/secret.go:53`) and is not needed by either PR.
- **Constraint that drives the whole PR A design (1):** `Profile.MarshalYAML`
  (`internal/profile/profile.go:855`) is **shared with the persistence path** — `profileForSave`
  embeds `Profile` to inherit it (`:869-877`) and `saveProfilesConfig` marshals through it
  (`internal/profile/profiles.go:226-240`). Clearing or masking secrets inside the marshalers would
  write `""` or `********` into the user's config file on the next save, destroying a `--no-vault`
  user's credentials. **The marshalers must not be touched.** Masking happens at print call sites
  only. Verified that `saveProfilesConfig` is reachable only from `create.go:139`, `delete.go:81`,
  `use.go:45` and `update.go:173` — none reachable from `get`/`list`/`which`.
- **Constraint that drives the whole PR A design (2):** masking must be gated on the **resolved**
  output format, not merely on the absence of an explicit `-o`. `GetRow` renders the table/csv/tsv
  `accesstoken` cell as `redactWithHash(profile.AccessToken)` (`profile.go:288`); feeding it a
  masked value yields `redactWithHash("********")` — one identical `REDACTED-xxxxxxxxxx` for every
  profile, destroying the property `redactWithHash` exists for (`profile.go:26-27`: "keeping a short
  hash so repeated values remain distinguishable"). Masking therefore applies **only** when the
  resolved format is json or yaml.
- **Test-mechanics constraints:**
  - `saveProfilesConfig` is unexported (`profiles.go:226`), so an external (`profile_test`) test
    cannot call it; drive a real save through `bb profile update` instead.
  - `common.WhatIf` writes to `os.Stderr` (`internal/common/whatif.go:22`), not `cmd.ErrOrStderr()`,
    so dry-run assertions need an `os.Stderr` capture, not the existing stdout helpers.
  - `internal/testutil` imports `profile`, so an internal (`package profile`) test file cannot
    import testutil. Nothing in `package profile` does today; keep it that way.

## Development Approach

- **testing approach**: **TDD (tests first)** — every defect here is already reproduced, so each task
  starts by writing the test that fails against current `master`, then makes it pass.
- complete each task fully before moving to the next
- make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
  - tests are not optional - they are a required part of the checklist
  - table-driven with subtests, per CLAUDE.md
  - cover both success and error scenarios
- **CRITICAL: all tests must pass before starting next task** - no exceptions
- **CRITICAL: update this plan file when scope changes during implementation**
- run `go test -race ./...` after each change
- maintain backward compatibility (see the compatibility note under Technical Details)

## Testing Strategy

- **unit tests**: required for every task (see Development Approach above)
- **e2e tests**: not applicable — CLI with no UI-based e2e harness. The equivalent end-to-end
  coverage is (a) driving the real cobra command with a temp `--config` file and capturing
  stdout/stderr, and (b) the PATH-shim technique that runs the real `os/exec` path against a fake
  executable. Both are used below and held to the same rigour as unit tests.
- **regression guards specific to PR A** (the two failure modes this design is most exposed to):
  1. masking must never reach the config file — after a gate-closed print, a real save must still
     write the true plaintext secret, never `********`;
  2. table/csv/tsv output must stay byte-identical — in particular the `accesstoken` cell must
     remain a per-value `REDACTED-<hash>`, distinct across profiles with different tokens.
- **lint**: `golangci-lint run` (v2.12.2, matching CI) green before each PR is pushed.

## Progress Tracking

- mark completed items with `[x]` immediately when done
- add newly discovered tasks with ➕ prefix
- document issues/blockers with ⚠️ prefix
- update plan if implementation deviates from original scope
- keep plan in sync with actual work done

## Solution Overview

### PR A

Add two small helpers to `internal/profile/profile.go` and route **all four** display call sites
through them:

```go
// resolvedOutputFormat reports the output format Print will actually use for cmd, applying the
// same --output > BB_OUTPUT_FORMAT > profile.OutputFormat precedence Print applies.
func (profile Profile) resolvedOutputFormat(cmd *cobra.Command) string

// maskSecrets reports whether payloads printed for cmd must have their secret fields masked: the
// resolved format is one that renders them verbatim, and no explicit -o json|yaml opted in.
func (profile Profile) maskSecrets(cmd *cobra.Command) bool
```

`resolvedOutputFormat` is **extracted from** `Print`'s existing resolution block
(`profile.go:539-552`), which then calls it — so this removes duplication rather than adding a
second, drifting copy of the precedence rules.

The four call sites, each gated identically:

| # | site | payload today | note |
|---|---|---|---|
| 1 | `getProcess` named-profile branch (`get.go:93`) | `profile` | gate must sit AFTER the `Default` adjustment at `:89-91` |
| 2 | `getProcess` `--current` branch (`get.go:59`) | `Current` | also needs `common.WhatIf` |
| 3 | `listProcess` (`list.go:70`) | `Profiles` | receiver is the *current* profile, payload is every profile |
| 4 | `whichProcess` (`which.go:46`) | `Current` | also needs `common.WhatIf` |

For `list`, the masked payload must be a **fresh `profiles` slice of fresh pointers**. `Profiles` is
the same package-level slice `saveProfilesConfig` reads, so masking in place risks `********` being
persisted. Build the masked copies *after* the existing `Profiles[0].Default = true` adjustment so
the `default` column stays correct. Verified that `profiles`' `GetHeaders`/`GetRowAt`/`Size` are
value receivers on the slice type (`profiles.go:85`, `:94`, `:104`), and that `Profile`'s
`GetHeaders`/`GetRow` are value receivers (`profile.go:249`, `:256`), so both a masked `Profile`
value and a `profiles` of masked pointers satisfy the printer unchanged.

Rejected alternatives (recorded so they are not revisited):

- **Mask inside `MarshalJSON`/`MarshalYAML`.** Rejected: shared with persistence, would corrupt the
  config file.
- **Gate on `!explicitJSONOrYAMLOutput(cmd)` alone, without resolving the format.** Rejected: it
  masks table/csv/tsv too and degrades the `accesstoken` column to a constant hash.
- **Clear the fields instead of masking.** Viable, and what upstream does, but `omitempty` makes the
  keys vanish, giving the reader no signal a secret exists. Decided against in favour of `********`.
- **Add upstream's `--show-secrets` flag.** Rejected: an explicit `-o json|yaml` already *is* our
  opt-in, so the flag would be a second, redundant control surface plus new doc/completion surface.
- **Add upstream's cleartext `password`/`clientsecret` table columns.** Rejected outright — they
  would reintroduce exactly the exposure this PR removes.

### PR B

Split the platform decision out of the side-effecting launch, so the argv shape becomes testable on
any host OS:

```
browserEnv{goos, wsl, interopDisabled, sshSession}  ->  browserCommand(env, url) (name, args, err)
```

`browserCommand` is pure: it owns the `runtime.GOOS` switch, the SSH / WSL-interop guards, and the
one place quoting is legitimate (the WSL `cmd.exe /C start` branch). `openBrowser` becomes: build
the env from the real runtime, call `browserCommand`, then `exec.CommandContext(...).Run()` with a
captured `stderr` so a child failure surfaces as a real error.

Deliberately **not** ported from upstream: upstream's fix *deletes* the `--stop-on-error` branch and
the manual-URL fallback and hard-fails instead. We keep the fallback — a browser failure must still
leave the user a URL to click.

`.Run()` vs `.Start()`: `.Run()` is required for the child's exit status to be observable at all,
which is the whole point; it also reaps the child, which today's `.Start()`-with-no-`.Wait()` never
does. The honest risk assessment: `open`, `xdg-open` and `rundll32` normally hand off and return
promptly, but a generic-fallback `xdg-open` that blocks until the browser exits would block
`openBrowser`. Authorization would still *complete* in that case — the callback server is already
listening (`authorize.go:70-74`) and `resultchan` is buffered — but the CLI would not return until
the browser process exits. Accepted: linux is not a release target (`.goreleaser.yml` builds
darwin/arm64 only). A watchdog-timeout variant was considered and rejected as YAGNI.

## Technical Details

**PR A — shape at each call site** (illustrative; note the gate sits after `Default`):

```go
// get.go, named-profile branch
if explicitJSONOrYAMLOutput(cmd) {
    _ = profile.LoadSecrets(ctx)
}
if len(Profiles) == 1 {
    profile.Default = true
}
if profile.maskSecrets(cmd) {
    return profile.Print(ctx, cmd, profile.forDisplay())
}
return profile.Print(ctx, cmd, profile)
```

```go
// list.go — masked copies built AFTER the Default adjustment, into a fresh slice
payload := any(Profiles)
if profile.maskSecrets(cmd) {
    masked := make(profiles, len(Profiles))
    for i, p := range Profiles {
        displayed := p.forDisplay()
        masked[i] = &displayed
    }
    payload = masked
}
return profile.Print(ctx, cmd, payload)
```

**Compatibility note (behaviour change, intentional):** a script reading
`bb profile get x -o json | jq -r .accessToken` with an *explicit* `-o json` is unaffected. A script
relying on a profile-configured `outputFormat: json` or on `BB_OUTPUT_FORMAT` to harvest a secret
**will** now receive `********`. That is the point of the change; it goes in the PR description and
the SKILL.md/README updates.

**PR B — argv table that `browserCommand` must produce:**

| goos | condition | name | args |
|---|---|---|---|
| darwin | — | `open` | `[<url>]` |
| windows | — | `rundll32` | `["url.dll,FileProtocolHandler", <url>]` |
| linux | plain | `xdg-open` | `[<url>]` |
| linux | WSL, interop on | `cmd.exe` | `["/C", "start", "\"<url>\""]` |
| linux | `SSH_CONNECTION` set | — | error `cannot open browser in SSH session` |
| linux | WSL, interop off | — | error `cannot open browser in WSL without interop enabled` |
| other | — | — | error `unsupported platform` |

The WSL row is the **only** one that keeps the quotes. Its argv is **preserved byte-identically as-is
and is unverified** — `cmd.exe /C start "<url>"` plausibly makes `start` treat the quoted argument as
a window *title* rather than a target, but nobody on this project can test WSL, so this plan
deliberately preserves current behaviour there rather than "fixing" it blind. Not a regression
either way.

**PR B — output changes in `authorizeProcess`:** the two `common.Verbose` calls
(`internal/profile/authorize.go:79`, `:89`) become unconditional writes to `cmd.ErrOrStderr()`.
Stderr keeps stdout clean (matching `common.Verbose`'s own documented rationale,
`internal/common/verbose.go:11-15`), and `cmd.ErrOrStderr()` rather than `os.Stderr` makes it
assertable via `cmd.SetErr`. The existing fallback at `:97` moves to the same destination so the
command has one consistent stream for interactive guidance. `waiting for browser authorization...`
(`:100`) stays on stdout as-is.

## What Goes Where

- **Implementation Steps** (`[ ]` checkboxes): code, tests, doc updates, branch/PR/CI work
- **Post-Completion** (no checkboxes): manual verification needing a real OAuth consumer and
  browser, plus the local-config hygiene item

---

## Implementation Steps

### Task 1: Failing tests for the json/yaml secret disclosure

**Files:**
- Create: `internal/profile/secret_output_gate_test.go` (package `profile_test`)

- [x] add a helper writing a temp config with one `--no-vault`-shaped profile carrying all three
      secrets in plaintext and a configurable `outputFormat`, driving the real `profile` command tree
      via `--config`; reuse `display_mask_test.go:24`'s `captureStdout` (do NOT redeclare it)
- [x] write table-driven test: `profile list` with profile-configured `outputFormat: json` must print
      `********` and must NOT contain any plaintext secret value (currently FAILS)
- [x] write table-driven test: `profile list` with `BB_OUTPUT_FORMAT=yaml` via `t.Setenv` and
      `outputFormat: table` must likewise mask (currently FAILS)
- [x] write the same two cases for `profile get <name>` (currently FAILS)
- [x] write the same two cases for `profile which` (currently FAILS — fourth call site)
- [x] write preserved-behaviour cases: explicit `-o json` and `-o yaml` must still print the real
      plaintext values (currently PASSES — guards against over-fixing)
- [x] write preserved-behaviour case: `profile get <name>` on a single-profile config still reports
      `default: true` (guards the `Default` ordering trap)
- [x] run tests — confirm exactly the expected subtests fail, for the expected reason, before any
      production code change

### Task 2: Add the resolved-format gate helpers

**Files:**
- Modify: `internal/profile/profile.go`
- Create: `internal/profile/output_gate_internal_test.go` (package `profile`)

- [x] extract `Print`'s format-resolution block (`profile.go:539-552`) into
      `func (profile Profile) resolvedOutputFormat(cmd *cobra.Command) string`, and have `Print`
      call it so there is exactly one copy of the precedence rules
- [x] add `func (profile Profile) maskSecrets(cmd *cobra.Command) bool` returning true only when
      `resolvedOutputFormat` is `json`/`yaml` AND `explicitJSONOrYAMLOutput(cmd)` is false
- [x] write table-driven tests for `resolvedOutputFormat` over the full precedence matrix (explicit
      `-o`, `BB_OUTPUT_FORMAT`, profile `OutputFormat`, and the empty/default fallthrough)
- [x] write table-driven tests for `maskSecrets`: true for config/env-derived json+yaml, false for
      explicit `-o json|yaml`, false for table/csv/tsv however chosen
- [x] verify `Print`'s behaviour is unchanged by the extraction (existing profile tests still pass)
- [x] run `go test -race ./...` — must pass before next task

### Task 3: Route all four display call sites through the gate

**Files:**
- Modify: `internal/profile/get.go`
- Modify: `internal/profile/list.go`
- Modify: `internal/profile/which.go`

- [x] `getProcess` named-profile branch: print `profile.forDisplay()` when `maskSecrets(cmd)`,
      placing the gate AFTER the `Default` adjustment at `get.go:89-91`
- [x] `getProcess` `--current` branch: apply the same gate to the `Current` payload
- [x] `whichProcess`: apply the same gate to the `Current` payload
- [x] `listProcess`: build a fresh `profiles` slice of fresh pointers to `forDisplay()` copies when
      `maskSecrets(cmd)`, positioned AFTER the existing `Profiles[0].Default = true` adjustment
- [x] confirm `Profiles` itself is never mutated by the masking (no in-place secret overwrite)
- [x] run Task 1's tests — every masking subtest must now pass, every preserved-behaviour subtest
      must still pass
- [x] run `go test -race ./...` — must pass before next task

### Task 4: Regression guards — persistence and table output

**Files:**
- Modify: `internal/profile/secret_output_gate_test.go`

- [x] write a test that runs a gate-closed `profile list`, then triggers a real save via
      `profile update <name> --description x` (`update.go:173`, the production save path), and
      asserts the written config still holds the real plaintext secret, never `********`
- [x] write the mirror case for a gate-closed `profile get <name>`
- [x] write a test asserting the table `accesstoken` cell is a per-value `REDACTED-<hash>` and that
      two profiles with DIFFERENT tokens produce DIFFERENT hashes (this is what catches the
      masked-value-into-`redactWithHash` degradation; the existing `display_mask_test.go:98`-`:122`
      assertions only check the `REDACTED-` prefix and cannot)
- [x] write the same distinctness assertion for csv and tsv output
- [x] run tests — must pass before next task

### Task 5: Make dry-run gating consistent across all four commands

**Files:**
- Modify: `internal/profile/get.go`
- Modify: `internal/profile/which.go`
- Modify: `internal/profile/secret_output_gate_test.go`

- [x] add a `captureStderr` helper (or use `root.SetErr`) — `common.WhatIf` writes to `os.Stderr`
      (`internal/common/whatif.go:22`), so the existing stdout helpers cannot see the dry-run line
- [x] write failing test: `profile get --current --dry-run` must print the dry-run line and must NOT
      print a profile table (currently FAILS — it prints the table)
- [x] write failing test: `profile which --dry-run` must do the same (currently FAILS)
- [x] write preserved-behaviour test: `profile list --dry-run` and `profile get <name> --dry-run`
      still behave as today (currently PASSES)
- [x] add `common.WhatIf(cmd, "Showing current profile")` to `getProcess`'s `--current` branch,
      ordered to match the named-profile branch
- [x] add the equivalent `common.WhatIf` to `whichProcess`
- [x] run tests — must pass before next task

### Task 6: Correct the stale doc comments and user-facing docs

**Files:**
- Modify: `internal/profile/list.go` (comment at :53-57)
- Modify: `internal/profile/get.go` (comment at :84-85)
- Modify: `internal/profile/profile.go` (six comments, listed below)
- Modify: `skill/bitbucket-cli/SKILL.md` (:67-81)
- Modify: `README.md`

- [x] rewrite `list.go`'s comment: it claims the vault gate prevents cleartext rendering, which was
      only ever true of the *fetch*
- [x] update `get.go`'s comment for the new gate
- [x] update `profile.go:236-248` (`GetHeaders`) — it asserts `get`/`list` "intentionally still show
      the real secret … when `-o/--output` json or yaml is given EXPLICITLY" and that `forDisplay`
      "only covers confirmation output after a mutation"; both change
- [x] update `profile.go:771-786` (`MarshalJSON`) — "It always shows every field, including any
      secret…" is still true of the marshaler but must not read as a statement about command output
- [x] update `profile.go:845-852` (`MarshalYAML`) — same
- [x] update `profile.go:59-68` (the `vault` field) — "Display paths (profile get/list -o yaml/json)
      marshal the Profile directly and show the secret"
- [x] update `explicitJSONOrYAMLOutput`'s godoc (`:515-523`) and `forDisplay`'s (`:902-912`) to name
      their new roles; describe current behaviour only, lowercase, per CLAUDE.md
- [x] rewrite SKILL.md:67-81: delete the now-false "renders in ANY json/yaml output regardless of how
      that format was chosen" caveat and the `--current` gate-skip paragraph; state that only an
      explicit `-o json|yaml` reveals a stored secret
- [x] keep every `bb <command>` example in its own code span — `extractBBCommandPaths`
      (`internal/cmd/skill_sync_test.go:52-77`) would read `` `bb profile list/get` `` as the command
      path `profile list/get` and fail
- [x] update README's profile/secret section, and `README.md:478-487` (`profile which`, documented as
      the equivalent of `get --current`)
- [x] run `go test -race ./...` including `internal/cmd/skill_sync_test.go` and `skill/embed_test.go`
      — must pass
- [x] run `golangci-lint run` — must be clean

### Task 7: Land PR A

- [x] create branch `fix/profile-secret-output-masking` off `master`
- [x] run `make test` and `make lint` locally, both green
- [x] commit with a conventional message, no AI/Claude/Anthropic mention anywhere
- [x] push and open the PR against `master`; the description must state the `outputFormat`/
      `BB_OUTPUT_FORMAT` compatibility break and that the PR deliberately carries the dry-run
      consistency fix alongside the masking fix
- [x] confirm CI (`build` on macos-latest + `cross-build` on ubuntu-latest) green
- [x] report the PR URL and CI state; do NOT merge

### Task 8: Failing tests for `browserCommand` argv per platform

**Files:**
- Create: `internal/profile/authorize_internal_test.go` (package `profile`)

- [x] write a table-driven test over every row of the argv table in Technical Details, asserting the
      exact `name` and `args` (or the exact error) for each `browserEnv`
- [x] assert explicitly that ONLY the WSL row carries the `"`-wrapped URL, and that
      darwin/windows/plain-linux carry the bare URL
- [x] include the three error rows: SSH session, WSL interop disabled, unsupported platform
- [x] reuse `update_internal_test.go:22`'s `captureStdout` if stdout capture is needed — do NOT
      redeclare it (same package)
- [x] run tests — the `profile` test binary will fail to COMPILE until Task 9 adds the function;
      that is the expected pre-implementation state here (not a per-subtest failure)

### Task 9: Extract `browserCommand` and fix the launch

**Files:**
- Modify: `internal/profile/authorize.go`

- [x] add the `browserEnv` struct and pure `browserCommand(env, url) (name, args, err)`, carrying the
      existing SSH / WSL-interop / unsupported-platform guards over verbatim
- [x] move the quoted-URL append inside the WSL branch only; every other branch appends the bare URL
- [x] rewrite `openBrowser` to build `browserEnv` from `runtime.GOOS`, `common.IsWSL()`,
      `SSH_CONNECTION`, and `wslInteropDisabled()` — calling `wslInteropDisabled()` only when
      `IsWSL()` is true, preserving today's conditional `/etc/wsl.conf` read; do not name a local
      variable `wslInteropDisabled` (it shadows the function)
- [x] launch via `exec.CommandContext(...).Run()` with a captured `stderr` buffer, returning the
      trimmed stderr text in the error when the child wrote any
- [x] keep the existing `//nolint:gosec` (G204) justification next to the call
- [x] run Task 8's tests — must pass
- [x] run `go test -race ./...` — must pass before next task

### Task 10: Print the authorization URL unconditionally, keep the fallback

**Files:**
- Modify: `internal/profile/authorize.go`
- Modify: `internal/profile/authorize_internal_test.go`

- [x] replace the two `common.Verbose` calls (`:79`, `:89`) with unconditional `cmd.ErrOrStderr()`
      writes; move the existing fallback write (`:97`) to the same stream
- [x] write test asserting the URL reaches stderr with `--verbose` unset, driving `authorizeProcess`
      with `--stop-on-error=true` and a failing browser launch so it returns at `:95` instead of
      blocking on `<-resultchan` at `:102` — a test that lets execution reach `:102` DEADLOCKS until
      the `go test` panic, so this ordering is mandatory
- [x] assert via `cmd.SetErr(&buf)` rather than swapping `os.Stderr`
- [x] confirm the `--stop-on-error` branch and the manual-URL fallback both still exist and still
      fire (explicitly NOT adopting upstream's deletion of them)
- [x] run tests — must pass before next task

### Task 11: PATH-shim test for the real exec path

**Files:**
- Modify: `internal/profile/authorize_internal_test.go`

- [x] add an `open` shim in a `t.TempDir()` prepended to `PATH`, recording its exact argv to a file,
      modelled on `setupGitShim` in `internal/repository/clone_test.go`
- [x] assert the shim received the bare, unquoted URL as a single argv entry
- [x] assert a non-zero shim exit surfaces as a non-nil error from `openBrowser`, with the shim's
      stderr text present in the error
- [x] assert a zero-exit shim yields a nil error
- [x] guard the shim test with `runtime.GOOS == "darwin"` so it does not fail when run on a local
      linux machine (note: CI's `cross-build` job runs only `go build`/`go vet` on ubuntu and never
      executes tests — tests run on macos-latest only, so this guard is for local runs)
- [x] run `go test -race ./...` — must pass before next task

### Task 12: Verify acceptance criteria

- [x] verify PR A: no cleartext secret in json/yaml from a profile-configured `outputFormat` or
      `BB_OUTPUT_FORMAT`, for `get <name>`, `get --current`, `list` AND `which` (verified on
      `fix/profile-secret-output-masking`, where the gate tests live)
- [x] verify PR A: explicit `-o json|yaml` still reveals secrets (verified on
      `fix/profile-secret-output-masking`)
- [x] verify PR A: table/csv/tsv unchanged, `accesstoken` still a per-value distinct `REDACTED-<hash>`
      (verified on `fix/profile-secret-output-masking`)
- [x] verify PR A: `--dry-run` honoured by all four display commands (verified on
      `fix/profile-secret-output-masking`)
- [x] verify PR A: no `********` ever written to a config file (verified on
      `fix/profile-secret-output-masking`)
- [x] verify PR B: rebuild and run `bb profile authorize` against a throwaway profile, confirming the
      URL prints without `--verbose` and a browser actually opens — a rebuilt `bb` run against a
      throwaway `--no-vault` profile printed both stderr lines with `--verbose` unset, an `open` shim
      first on `PATH` recorded exactly one argv entry holding the bare unquoted URL, a shim exiting
      non-zero surfaced `cannot open browser: <shim stderr>` under `--stop-on-error` and printed the
      manual-URL fallback without it, and an unshimmed run handed the URL to the real `open`
      successfully (the OAuth round-trip itself is not automatable — see Post-Completion)
- [x] run `make test` (`go test -race ./...`)
- [x] run `make lint` (`golangci-lint run`, v2.12.2)
- [x] run `make cross-build` (proves the linux/CGO_ENABLED=0 path still builds)

### Task 13: Update documentation and land PR B

**Files:**
- Modify: `skill/bitbucket-cli/SKILL.md`
- Modify: `README.md`
- Modify: `CLAUDE.md` (only if a new pattern was established)

- [x] update SKILL.md's authorize text (`:10`, `:53-54`) if the always-printed URL changes what is
      documented — `:10` needed nothing; the create/authorize bullet now states that the command
      prints the authorization URL and opens a browser, falls back to the printed URL when it cannot,
      and returns the launch failure under `--stop-on-error`
- [x] update `README.md:397` — "You can also use the `--verbose` to get some information about the
      authorization process" becomes misleading once the URL is unconditional
- [x] update CLAUDE.md only if the `browserCommand` extraction establishes a pattern worth recording
      (skipped: the explicit-argv/no-shell and PATH-shim conventions CLAUDE.md already records cover
      this split exactly, so there is no new pattern to add)
- [x] create branch `fix/authorize-browser-launch` off updated `master`
- [x] `make test` + `make lint` green locally
- [x] commit with a conventional message, no AI/Claude/Anthropic mention anywhere
- [x] push and open the PR against `master` (owned by the landing session, not by the documentation
      task that finished this plan)
- [x] confirm CI green (same: checked from the landing session)
- [x] move this plan to `docs/plans/completed/` and `git add -f` it into PR B (the finishing PR)
- [x] report both PR URLs and CI state; do NOT merge either (reported from the landing session)

## Post-Completion

*Items requiring manual intervention or external systems - no checkboxes, informational only*

**Manual verification:**

- PR B's end-to-end path needs a real Bitbucket OAuth consumer (client id + secret + callback port)
  and a real browser; the shim tests cover argv and exit-status handling but cannot prove the OAuth
  round-trip. Worth one manual `bb profile authorize` run before merging PR B.
- The WSL argv row is preserved as-is and remains unverified — nobody on this project can test WSL.
- PR A's masking is verified by automated stdout capture, but a quick eyeball of
  `BB_OUTPUT_FORMAT=json bb profile list` against the real local config is cheap reassurance.

**Local config hygiene (unrelated to both PRs, found while investigating):**

- `~/Library/Application Support/bitbucket/config-cli.yml` holds two leftover test profiles,
  `temptest` and `vault-leak-test`. `temptest` carries a plaintext `accesstoken`. Deleting those
  profiles — and rotating that token if valid — is independent of this plan and worth doing
  regardless of whether either PR lands.

**Explicitly out of scope (decided, not deferred):**

- `internal/remote/remote.go`'s `bitbucket.org` substring match. Upstream's `://bitbucket` /
  `@bitbucket` replacement would break `ssh://git@altssh.bitbucket.org:443/ws/repo.git`, which our
  substring test handles; our own false-positive class (a non-Bitbucket URL merely containing the
  string, e.g. `github.com/acme/bitbucket.org-tools.git`) is a separate concern from anything
  upstream shipped and deserves its own plan if worth fixing at all.
- Upstream's terminal-width description truncation (needs the banned `golang.org/x/term` plus a
  `tmux` shell-out; our fixed-width `truncateCell` is a documented deliberate choice).
- Upstream's addition of `author` to `pullrequest list`'s default columns (our narrower `list`
  defaults are a documented design decision).
- Upstream's cleartext `password`/`clientsecret` table columns (actively rejected).
- Upstream's chocolatey/snap/Debian packaging and hand-edited `version.go` bump (no counterpart in a
  GoReleaser + Homebrew-cask repo with an ldflags-stamped version).

## Review Findings Applied

Recorded so the reasoning is not lost; all were verified against the code before revision.

1. **`profile which` is a fourth leaking call site** (`which.go:46`). It printed the entire `Current`
   profile and was absent from the first draft — PR A would have shipped a bypass of its own fix
   while deleting the doc caveat that warned about it. Now covered in Tasks 1, 3, 5, 6, 12.
2. **Masking on `!explicit` alone degrades table output.** `redactWithHash("********")` collapses the
   `accesstoken` column to one constant hash across all profiles. Fixed by gating on the **resolved**
   output format (new Task 2 helpers), which also removes the first draft's false "table output
   unchanged" claim, and by adding the hash-distinctness guard in Task 4.
3. **The `get.go` snippet dropped `Default = true`** (`get.go:89-91`) by returning before it. Snippet
   and task wording corrected; a preserved-behaviour test added in Task 1.
4. **Task 9's original test would deadlock** on `<-resultchan` (`authorize.go:102`), because the only
   non-blocking exit (`--stop-on-error=true`, `:94-96`) returns *before* the fallback print. Respecified
   in Task 10 to assert the pre-launch write with `--stop-on-error=true`.
5. **Stderr destinations.** Authorize writes go to `cmd.ErrOrStderr()` (assertable via `cmd.SetErr`),
   and Task 5 needs an `os.Stderr` capture because `common.WhatIf` writes there directly.
6. **Task 3's save guard could not compile where it was placed** — `saveProfilesConfig` is unexported
   and the test file is external. Now drives the real save through `profile update`.
7. **Four more stale doc comments** in `profile.go` assert the removed behaviour; all six are now
   itemised in Task 6.
8. **Dry-run bundling made explicit** rather than incidental, in the Overview and the PR description
   checkbox.
9. **`.Run()` linux rationale corrected** — the first draft wrongly claimed ctx cancellation bounds a
   hang; replaced with an accurate description and an explicit "accepted, linux is not a release
   target".
10. **Small corrections:** `wslInteropDisabled()` stays behind `IsWSL()`; no local variable shadowing
    it; Task 11's guard rationale fixed (the ubuntu job never runs tests); the WSL argv row labelled
    preserved-but-unverified; a `skill_sync_test.go` code-span trap added to Task 6; `captureStdout`
    reuse-not-redeclare noted for both packages; a single gate helper instead of four repetitions.

## Deviations from the plan as written

Recorded so the commit history reads honestly against the task boundaries above.

- ⚠️ **Tasks 8 and 9 landed as one commit.** Task 8's tests call `browserCommand`, which Task 9
  adds, so committing Task 8 on its own would have put a non-compiling `internal/profile` test
  binary in history. The TDD order was still followed inside the working tree (tests written first,
  run against the missing function, then the implementation added); only the commit boundary moved.
- ⚠️ **Tasks 10 and 11 likewise landed as one commit,** for the same reason: both touch the same
  rewritten launch path and its single test file.
- ➕ **Task 10's deadlock avoidance is stronger than specified.** The plan mandated
  `--stop-on-error=true` as the only non-blocking exit, so only that path would have been asserted.
  The test instead occupies the callback port (`occupiedCallbackPort`), which makes
  `authorizeProcess`'s own `ListenAndServe` fail to bind and deliver that failure into the buffered
  `resultchan` — so `<-resultchan` never blocks and BOTH `--stop-on-error` values are covered,
  including the manual-URL fallback the fork deliberately keeps.
