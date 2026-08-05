# Umputun-Style Modernization of bitbucket-cli

## Overview

Modernize the trimmed-down bitbucket-cli (now: `profile`, `pullrequest`, `user` commands only)
to match the engineering conventions used in umputun's recent Go repos and — where they diverge —
the house style already established in `~/Code/my/go/jcli` (jcli wins on conflicts: it encodes
the owner's actual preferences, e.g. `internal/` usage and the exact release flow).

Problems solved:
- No CI at all today; no automated quality gate.
- 226-module dependency graph for a small CLI (gildas/go-logger alone drags cloud.google.com/grpc/otel).
- Hand-built 3-file Makefile packaging zoo (deb/rpm/snap/chocolatey) for platforms the owner doesn't use.
- Hardcoded `VERSION` constant, manual bumps, chglog-based changelog machinery.
- Copy-pasted command boilerplate; low, uneven test coverage; no codified conventions.

End state: a macOS-first CLI with jcli's CI/release flow (GitHub Actions + goreleaser +
Homebrew cask on `avitsrimer/homebrew-apps`), a minimal dependency graph, `internal/` layout,
stdlib errors, lgr logging, strict golangci-lint v2 config, and a repo CLAUDE.md codifying it all.

## Context (from discovery)

- Repo: `github.com/avitsrimer/bitbucket-cli` (fork of gildas/bitbucket-cli), local branch `master`,
  PR base branch `master` (single-branch flow; the upstream-style `dev` branch was retired). `gh` CLI at `/opt/homebrew/bin/gh`, authenticated as `avitsrimer`.
- Working tree ALREADY CONTAINS the uncommitted feature trim (deleted artifact/cache/component/
  gpg-key/issue/pipeline/ssh-key/tag; branch/commit/project/repository/workspace demoted to
  libraries; go-git + progressbar deps dropped; README updated). Build/vet/tests green. This must
  land as PR #1 before anything else.
- Reference for release flow: `~/Code/my/go/jcli/.github/workflows/{ci,release}.yml`,
  `~/Code/my/go/jcli/.goreleaser.yml`, `~/Code/my/go/jcli/.golangci.yml`, `~/Code/my/go/jcli/Makefile`.
- Key current facts: root `main.go` + everything under `cmd/`; 40 cobra `init()` registrations;
  `gildas/go-logger` in 65 files (context-passed); `gildas/go-errors` in 57 files
  (`errors.Join(errors.Errorf(...), err)` idiom); viper+godotenv+ini config;
  testify suites writing per-suite log files; coverage 8–35%; `testdata/pipeline-*.json` stale.
- Platform rule from owner: macOS (darwin/arm64) is the only release target; keep code
  cross-buildable only where it costs nothing (jcli does the same with a linux cross-build CI stub).
- Deliberate deviation from umputun: KEEP cobra (dynamic shell completion via ValidArgsFunction
  is a core feature; go-flags has no equivalent). Do not migrate to jessevdk/go-flags.

## Development Approach

- **Testing approach**: Regular (code first, then tests in the same task) — matches umputun/jcli style.
- One task = one PR. Sequential: each task branches from freshly-updated `master` after the
  previous PR merges. Never push to `master` directly.
- **PR workflow (every task)**:
  1. `git checkout master && git pull origin master && git checkout -b <task-branch>`
  2. implement + test locally: `go build ./... && go vet ./... && go test -race ./...`
     (plus `golangci-lint run` once Task 2 has landed)
  3. commit (conventional, no AI mentions in messages), push, open PR against `master`
     with `/opt/homebrew/bin/gh pr create --base master`
  4. wait for CI: `/opt/homebrew/bin/gh pr checks <n> --watch` (Task 1 predates CI — verify locally only)
  5. merge when green: `/opt/homebrew/bin/gh pr merge <n> --squash --delete-branch`
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task —
  success and error scenarios, listed as separate checklist items. All tests must pass
  (`go test -race ./...`) before the PR is opened and before starting the next task.
- **CRITICAL: update this plan file when scope changes during implementation.**
- Comments/docs follow umputun rules: lowercase except godoc, describe current state only
  (never history), no AI/Claude mentions anywhere in commits or PRs.
- Workers (implementation subagents) run on **sonnet**; plan/review agents on **opus**.

## Testing Strategy

- **unit tests**: required for every task; table-driven with subtests for new code
  (existing testify suites may remain suite-style until Task 8 normalizes them).
- HTTP-facing tests use `net/http/httptest` (already the repo pattern in profile_client_test.go);
  no mock framework unless an interface boundary appears, then `matryer/moq`.
- `go test -race ./...` is the gate locally and in CI.
- no e2e/UI tests in this project.

## Progress Tracking

- mark completed items with `[x]` immediately when done
- add newly discovered tasks with ➕ prefix
- document issues/blockers with ⚠️ prefix
- record each task's PR number next to the task heading when opened

## Solution Overview

Twelve sequential PRs (ten planned tasks plus 5b/5c, added mid-flight — see Technical Details),
ordered so the safety net (CI) lands right after the already-done trim,
then the three big dependency-killing migrations (logging, errors, config) run under CI
protection, then structure/boilerplate refactors, then release flow, tests, and docs.
Each migration is mechanical-per-file and parallelizable across sonnet workers within a task.

Key design decisions:
- **lgr over slog** for logging: matches umputun repos and keeps the current leveled-debug
  behavior with near-zero transitive deps. (slog considered; lgr chosen for ecosystem consistency —
  reviewer may overrule, both kill the cloud/otel/grpc tree.)
- **stdlib errors** (`fmt.Errorf("...: %w", err)` + local sentinel `var Err...` where needed)
  over gildas/go-errors.
- **plain YAML config** (gopkg.in/yaml.v3, already a dep) over viper. Keep godotenv (tiny, useful),
  keep ini (reads .git/config), keep go-cache/go-core/go-flags/go-request/keyring/uuid/tablewriter.
  Drop spinner (single OAuth wait message replaces it).
- **layout**: `cmd/bb/main.go` (binary `bb`) + `internal/` for all packages (jcli convention,
  compiler-enforced privacy — deviates from umputun's `pkg/`, matches the house style).
- **release**: goreleaser v2, darwin/arm64 only, tar.gz, Homebrew cask with quarantine-strip hook
  into `avitsrimer/homebrew-apps` `Casks/`; tag-triggered release workflow on macos-latest;
  `revision`-style version via ldflags from `git describe`; delete packaging/ + chglog machinery.

## Technical Details

- Module identity: the module is renamed `github.com/gildas/bitbucket-cli` →
  `github.com/avitsrimer/bitbucket-cli` in Task 6 (same PR as the internal/ move — one
  mechanical repo-wide import rewrite covers both). Without this, `go install` of the fork
  can never work (Go requires the declared module path to match the requested one).
- Version: replace `version.go` (`VERSION = "0.18.4"`) with `var version = "dev"` in
  `internal/cmd` (mirrors jcli's `internal/cli.version`), stamped via
  `-ldflags "-X github.com/avitsrimer/bitbucket-cli/internal/cmd.version=$(git describe --tags --always --dirty)"`
  in Makefile and `{{.Version}}` in goreleaser. INTENTIONAL BEHAVIOR CHANGE: the old
  branch-aware `VERSION[+stamp.commit]` scheme is dropped in favor of a single
  git-describe-derived string; `bb version` / `--version` prints it.
- Logging migration pattern: `logger.Must(logger.FromContext(ctx)).Child(x, y)` call sites →
  package-level `lgr` printf-style calls (`lgr.Printf("[DEBUG] ...")`); `--debug` flag sets
  `lgr.Setup(lgr.Debug, lgr.CallerFile, lgr.CallerFunc, lgr.Msec, lgr.LevelBraces, lgr.Out(os.Stderr),
  lgr.Err(os.Stderr))`, non-debug drops the four debug-only options (still routing both streams to
  stderr so `[WARN]`/`[ERROR]` are always visible there with no stdout duplication); go-logger's
  `Infof`/`Debugf` both map to `[DEBUG]` (filtered out unless `--debug`), `Warnf`→`[WARN]`,
  `Errorf`→`[ERROR]` (its trailing-error-arg convenience re-expressed as an explicit `%v`).
  DECISION (2026-08-05): the `--log <file>`/`-l` flag and `LOG_DESTINATION` env var are DROPPED
  entirely — lgr always writes to stderr, and file-based logging was a go-logger feature nobody
  used for this CLI. Test suites stop creating `./log/*.log` FileStreams (and the now-dead `log/`
  directories were deleted from every package).
- Error migration pattern: `errors.Join(errors.Errorf("Cannot X"), err)` →
  `fmt.Errorf("cannot X: %w", err)`; go-errors sentinels (ArgumentMissing, NotFound, …) →
  stdlib `errors.Is`-compatible local sentinels only where actually checked; audit each
  `errors.Is` call site during migration. Task 4 ALSO wraps bare cross-package
  `return err` sites flagged by wrapcheck (161 findings include many of these, not just the
  Join idiom) — otherwise wrapcheck exclusions can't be lifted in Task 8.
- Config: viper usage is confined to `cmd/common/config.go` + 5 profile files; replace with
  a small loader: resolve path (--config → UserConfigDir/bitbucket/config-cli.yml → ~/.bitbucket-cli),
  yaml.Unmarshal into a struct, explicit save on profile mutations (atomic write, 0600 like jcli).
- Boilerplate helper: one generic "resolve profile/repo/PR-id → optional WhatIf → call API verb →
  print result" helper in the pullrequest package used by approve/unapprove/decline/merge/
  request-changes/remove-request-changes (±6 near-identical files today).
- CI (`.github/workflows/ci.yml`, jcli-shaped): `build` job on macos-latest
  (checkout v7, setup-go v6 with go-version-file, `go test -race ./...`,
  golangci-lint-action v9 pinned version) + `cross-build` job on ubuntu-latest
  (`GOOS=linux CGO_ENABLED=0 go build ./... && go vet ./...`). Runs on push + pull_request.
- Lint: adopt jcli's `.golangci.yml` (v2, default:none, ~45 linters; file named `.golangci.yml`,
  replacing the current `.golangci.yaml`) with these PERMANENT documented deviations from jcli
  (a dry run of jcli's config against this repo yields 815 findings, and these two categories
  are structural, never fixed by any task):
  - `gochecknoinits` disabled — cobra `init()` command registration is a kept-forever
    architectural decision (38 findings)
  - `importShadow` added to `gocritic.disabled-checks` — long-standing param/receiver names
    shadow imports in ~166 places; renaming them is low-value busywork
  Temporary exclusions are allowed only for findings that Tasks 3–8 (inclusive — Task 8 owns
  testifylint/suite-style findings) will eliminate, each annotated with the owning task number
  and removed in Task 8. `new-from-rev` is NOT used. jcli's `shellcheck` CI job is deliberately
  omitted (repo has zero `.sh` files).
- `.gitignore`: add jcli's plan convention — ignore `docs/plans/*` except `docs/plans/completed/`.

## What Goes Where

- **Implementation Steps** (`[ ]` checkboxes): all code/CI/docs changes below.
- **Post-Completion** (no checkboxes): HOMEBREW_TAP_TOKEN secret creation, first release cut,
  brew install verification — owner-only actions.

## Implementation Steps

### Task 1: Land the feature trim (PR #1 — pre-CI, local verification only)

**Files:**
- Modify: (already-modified working tree: deleted packages, trimmed libraries, go.mod/go.sum, README.md)
- Delete: `testdata/pipeline-*.json` (stale fixtures of removed feature)
- Modify: `.gitignore` (add `docs/plans/*` ignore + `!docs/plans/completed/` exception)

- [x] delete stale `testdata/pipeline-*.json` fixtures
- [x] add docs/plans gitignore rules (in-progress plans stay local, jcli convention)
- [x] verify locally: `go build ./... && go vet ./... && go test -race ./...` all green
- [x] verify built binary lists only profile/pullrequest/user commands (`go run . -- help` smoke)
- [x] branch `refactor/trim-to-pullrequest-core`, commit working tree as one commit, push, open PR to `master`
- [x] merge PR (no CI yet — local verification above is the gate)

### Task 2: CI + lint baseline (PR #2)

**Files:**
- Create: `.github/workflows/ci.yml`
- Create: `.golangci.yml` (jcli-derived v2 config)
- Delete: `.golangci.yaml` (replaced)
- Delete: `.custom-gcl.yml` (upstream nilaway plugin config pinning golangci-lint v2.8.0)

- [x] port jcli's `.golangci.yml` with the two permanent deviations from Technical Details
      (`gochecknoinits` disabled, `importShadow` added to gocritic.disabled-checks)
- [x] run `golangci-lint run` locally; fix all findings that don't belong to later tasks
      (mechanical fixes: errcheck, unconvert, misspell, whitespace, usestdlibvars, etc.)
- [x] add temporary, commented, per-linter exclusions ONLY for findings that Tasks 3–8 will
      eliminate (each exclusion annotated with the owning task number; testifylint/suite-style
      findings belong to Task 8)
- [x] create `ci.yml` per jcli shape (macos build job: test -race + lint; ubuntu cross-build stub)
- [x] write/adjust tests for any behavior-affecting lint fixes (error paths newly checked)
- [x] run `go test -race ./...` — must pass
- [x] ➕ delete upstream `.custom-gcl.yml`: it made golangci-lint-action build a custom v2.8.0
      nilaway binary, so CI linted with a different version than the pinned v2.12.2 and reported
      findings that contradicted the local run (stale modernize hit, "unused" gosec directive)
- [x] open PR, confirm CI executes and is green, merge

### Task 3: Logging migration go-logger → lgr (PR #3)

**Files:**
- Modify: `main.go`, `cmd/root.go`, `cmd/common/config.go` (logger init/flag wiring)
- Modify: all files importing `gildas/go-logger` (~65)
- Modify: all `_test.go` suites creating FileStream loggers
- Modify: `go.mod`/`go.sum` (drop go-logger, add go-pkgz/lgr; `go mod tidy`)

- [x] add `go-pkgz/lgr`; implement setup: default quiet, `--debug` → lgr.Debug+caller,
      decide and record `--log <file>` mapping (file writer or drop the flag)
- [x] migrate main.go + root.go + common/config.go init path off logger-in-context
- [x] migrate all command packages (fan out per-package to sonnet workers; identical mechanical pattern)
- [x] remove FileStream test-logger setup from all test suites (and now-dead `log/` dirs expectation)
- [x] drop `gildas/go-logger` from go.mod; `go mod tidy`; record module-count before/after in PR body
- [x] update tests for logging-adjacent behavior changes (config init, flag handling) — success + error cases
- [x] `go test -race ./...` + lint green; open PR, wait CI, merge
- ⚠️ `gildas/go-logger` could NOT be fully removed from the module graph: `gildas/go-request`'s
      `request.Options.Logger` field is typed `*logger.Logger`, so its own go.mod unconditionally
      requires go-logger (and go-logger's own cloud.google.com/grpc/otel tree) regardless of whether
      our code sets that field. `cmd/profile/profile_client.go` no longer sets `Options.Logger` at all
      (go-request falls back to its own internal void logger) — zero glue needed, but the transitive
      tree is unavoidable without replacing go-request (explicitly out of scope for this task).
      Module count: 226 → 227 (net +1, solely from adding `go-pkgz/lgr`; verified with
      `diff <(go list -m all) <(go list -m all)` against HEAD — the only line that differs is the
      added `github.com/go-pkgz/lgr` entry). `github.com/gildas/go-logger` moved from a direct to an
      `// indirect` requirement in go.mod, confirming no first-party code imports it anymore.
- ➕ two `Profile`/`Token` debug-log call sites (`cmd/profile/update.go`, `cmd/profile/token.go`) used
      to log the full struct via go-logger's `Record()` + the `logger.Redactable` interface, which
      auto-redacted `ClientSecret`/`Password`/`AccessToken`/`RefreshToken` before printing. lgr has no
      such interface, so `Profile.Redact()`/`Token.Redact()` are kept as plain methods (no longer
      documented as "implements logger.Redactable") and called explicitly at each of those two
      `lgr.Printf` call sites; the `logger.RedactWithHash` helper was reimplemented locally as
      `redactWithHash` (sha256-based) in `cmd/profile/profile.go` so no go-logger import is needed
      for it.
- ➕ `cmd/profile/authorize.go`'s local OAuth callback server used `log.HttpHandler()` (a go-logger
      HTTP access-log middleware) with no lgr equivalent; dropped rather than replaced — it wrapped a
      single local loopback callback endpoint from the user's own browser, logging it was low value.

### Task 4: Error handling migration go-errors → stdlib (PR #4)

**Files:**
- Modify: all files importing `gildas/go-errors` (~57)
- Modify: `cmd/common/errors.go` (or equivalent error-processing helpers) as needed
- Modify: `go.mod`/`go.sum`

- [x] inventory all `errors.Is(err, errors.X)` sentinel checks; define local sentinels for the
      ones actually needed (e.g. `ErrNotFound` for API 404 handling in profile_client)
- [x] migrate wrapping idiom `errors.Join(errors.Errorf("Cannot X"), err)` → `fmt.Errorf("cannot X: %w", err)`
      across all command files (fan out per-package to sonnet workers)
- [x] migrate argument-validation errors (ArgumentMissing/ArgumentInvalid) to clear stdlib errors
- [x] drop `gildas/go-errors`; `go mod tidy`
- [x] update error-path tests (profile error_test.go asserts go-errors types today — rewrite for stdlib)
- [x] add tests covering sentinel `errors.Is` behavior for the new local sentinels
- [x] `go test -race ./...` + lint green; open PR, wait CI, merge
- ⚠️ `gildas/go-errors` could NOT be fully dropped: `cmd/profile/profile_client.go` still imports it
      directly (not indirect) because `gildas/go-request` returns error values of that package's
      `Error` type (`errors.JSONUnmarshalError` on unmarshal failures, `errors.FromHTTPStatusCode`
      results, an `*errors.Error` carrying the HTTP status code) and the client has to inspect them
      (`errors.Is`/`errors.As`, `errors.NewSentinel`) to preserve behavior. This mirrors Task 3's
      go-logger finding: go.mod's `github.com/gildas/go-errors` line is unchanged (still a direct,
      non-indirect requirement) after `go mod tidy` — module count stays at 227. Task 5b (go-request
      → net/http) is the only way to drop it for good; the import is annotated in profile_client.go
      with that note.
- ➕ sentinel inventory result: of the go-errors sentinels in play (ArgumentMissing, ArgumentInvalid,
      NotFound, Empty, InvalidType, JSONMarshalError, JSONUnmarshalError, DuplicateFound,
      RuntimeError), only two were ever actually matched via `errors.Is` anywhere in the repo:
      `errors.Empty` (profile.go's `GetProfileFromCommand` returning "no profiles yet", checked by
      7 profile subcommands) → became the package-level `profile.ErrNoProfiles` sentinel; and
      `errors.JSONUnmarshalError` on our own `UnmarshalTokenFromBitbucketData` output (checked once
      in profile_client.go's OAuth callback handler) → became `profile.ErrUnmarshalJSON`. Every
      other sentinel was only ever used to *build* an error, never matched, so those became plain
      `fmt.Errorf`/`errors.New` messages with no sentinel var, per the plan's guidance.
- ➕ `cmd/common/link.go`'s `errors.Is(err, errors.JSONUnmarshalError)` guards around each
      `json.Unmarshal` call were dead code: `encoding/json.Unmarshal` never returns a
      `gildas/go-errors` type, so the condition was always false and every call fell through to the
      `else if err != nil` branch. Simplified to a plain `if err != nil` during the migration.
- ➕ found and fixed two latent bugs uncovered while translating go-errors' `.With()`/sentinel
      templates to plain messages (both sentinels have a fixed `Text` format string with fewer `%`
      verbs than arguments passed to `.With()`, so part of the intended message was silently
      dropped by the old code, never rendered): `cmd/pullrequest/comment/{create,update}.go` used
      `errors.RuntimeError.With("Cannot specify from/to without a file")` — `RuntimeError`'s text has
      zero `%` verbs, so the actual `.Error()` output was always just "Runtime Error", never the
      intended message; now `errors.New("cannot specify from/to without a file")`. And
      `cmd/pullrequest/create.go`'s `resolveCreateDefaultReviewers` called
      `errors.Join(errors.New(...), err, errMe)` — go-errors' `Join` returns `nil` whenever its
      *last* argument is `nil`, so whenever `user.GetMe` succeeded (`errMe == nil`, the common case)
      the function silently returned `(nil, nil)` even though `GetEffectiveDefaultReviewers` had
      genuinely failed; replaced with stdlib `errors.Join(fmt.Errorf(...), errMe)`, which skips nil
      arguments instead of swallowing the whole result.
- ➕ removed the Task 4 `wrapcheck` exclusion in `.golangci.yml` and made the repo genuinely
      wrapcheck-clean: besides the ~150 go-errors call sites, this also required wrapping ~110 bare
      cross-package `return err`/tail-call passthroughs repo-wide (not limited to the 57
      go-errors files) with `fmt.Errorf("...: %w", err)` context — e.g. every cobra `RunE` that
      returned `profile.GetProfileFromCommand(...)`/`repository.GetRepository(...)`/
      `profile.Print(...)` errors unwrapped. One exception: `cmd/root.go`'s `Execute` keeps
      `//nolint:wrapcheck` on `cobra.Command.ExecuteContext` — `main.go` prints that error verbatim,
      and wrapping it would prefix *every* command failure with redundant "cannot execute
      command:" noise instead of the command's own, already-specific error.
- ➕ replaced every `errors.MultiError` (repository.go, pullrequest.go, activity.go x2,
      comment.go, profile.go, and the two `task`/`comment` `delete` loops) with stdlib
      `errors.Join`/a plain `[]error` accumulator — a straight drop-in that also fixes a formatting
      wart in the two `delete` loops: `errors.MultiError.Error()` has a pointer receiver, so passing
      the value type to `%s` in `fmt.Fprintf`/`lgr.Printf` never actually invoked it; stdlib
      `errors.Join` returns a plain `error` interface value, so `%s`/%v` always renders correctly.

### Task 5: Config simplification — drop viper + spinner (PR #5)

**Files:**
- Modify: `cmd/common/config.go`, `cmd/profile/{use,create,update,delete,profiles}.go`
- Modify: `cmd/profile/authorize.go` (spinner removal)
- Modify: `go.mod`/`go.sum`

- [x] implement plain-YAML config loader/saver (path resolution preserved: --config →
      os.UserConfigDir()/bitbucket/config-cli.yml → ~/.bitbucket-cli; atomic write, 0600)
- [x] migrate profile persistence off viper (profiles slice marshals to the same YAML shape —
      config file format MUST stay backward compatible; verify against a real existing config)
- [x] replace spinner in `profile authorize` with a plain "waiting for browser authorization..." message
- [x] drop `spf13/viper` + `briandowns/spinner`; `go mod tidy`
- [x] tests: config load/save round-trip (existing-file, missing-file, malformed-file cases)
- [x] tests: profile CRUD persistence against a temp config dir
- [x] `go test -race ./...` + lint green; open PR, wait CI, merge
- ➕ `cmd/common/config.go`'s new `Config` type/loader replaced viper as a small package-level
      singleton (`common.CurrentConfig()`), mirroring viper's own global-state shape so
      `cmd/profile/profiles.go`'s `Load()`/`saveProfilesConfig()` needed only mechanical
      call-site swaps (`config.GetSection("profiles", ...)` / `config.SetSection("profiles", ...)`
      in place of `viper.UnmarshalKey`/`viper.Set`+`WriteConfig`). `use.go` and `delete.go`
      previously duplicated a simplified inline `viper.Set`+`WriteConfig` (without the
      `WriteConfigAs` fallback `profiles.go`'s own save path had); both now call the shared
      `saveProfilesConfig()` instead, removing that duplication and its latent
      "fails to write when no config file was ever found" gap.
- ➕ removed the Task-5-owned temporary `gocritic`/`govet` exclusion for `cmd/common/config.go`
      from `.golangci.yml` as planned; the new `Config.Save()` needed one more genuine fix beyond
      that (an `errcheck` on a bare `defer os.Remove(tempPath)` and a `govet` shadow on a nested
      `err`) — both fixed properly, `golangci-lint run` reports 0 issues with no exclusions left
      for this file.
- ⚠️ discovered (not fixed, pre-existing, out of scope for this task): `BB_CONFIG` only ever
      supplied the `--config` flag's *default value* on `cmd/root.go`'s `RootCmd`; because
      `ConfigPath` (like the viper code it replaced) only reads the flag's value when
      `PersistentFlags().Changed("config")` is true, setting `BB_CONFIG` alone (without also
      passing `--config`) is silently ignored and path resolution falls through to
      `UserConfigDir()`. Verified this reproduces identically on master before this change.
      Path resolution logic was preserved exactly per this task's scope; the smoke test below
      used `--config` directly instead of `BB_CONFIG`.
- module count: 227 → 213 (-14): `spf13/viper`, `briandowns/spinner`, and their transitive-only
  chain (`spf13/afero`, `spf13/cast`, `go-viper/mapstructure/v2`, `pelletier/go-toml/v2`,
  `fsnotify/fsnotify`, `sagikazarmark/locafero`, `subosito/gotenv`, `fatih/color`,
  `mattn/go-colorable`, `mattn/go-isatty`, `frankban/quicktest`, `sourcegraph/conc`) all left
  `go list -m all`, confirmed via diff against master.
- backward compatibility proof: `cmd/profile/profiles_test.go`'s
  `TestLoadParsesTestdataConfigYAMLProfilesIdentically` loads `testdata/config.yml` (the exact
  fixture viper used to read) through the new loader and asserts the two profiles decode with
  the same field values as before; `TestProfileCreateAndDeletePersistAcrossReloads` drives real
  `profile create`/`profile delete` commands against a temp config file (reloading `Profiles`
  from disk between commands) and asserts the on-disk YAML keeps the same `profiles:` list shape,
  and a manual binary smoke test (`bb profile list|create|delete --config <tmp copy of
  testdata/config.yml>`) confirmed the file stays parseable and 0600 throughout.

### ➕ Task 5b: Replace go-request with net/http (PR #5b)

➕ Added after Task 3 discovery: `gildas/go-request`'s `Options.Logger` field is typed
`*logger.Logger`, so go-request unconditionally keeps `gildas/go-logger` AND its
cloud.google.com/grpc/otel tree in the module graph (226→227 after Task 3, not the expected
drop). Task 10's acceptance target (module count <100, no go-logger in the graph) is
unreachable without this. Stdlib `net/http` also matches umputun's stdlib-first philosophy.

**Files:**
- Modify: `cmd/profile/profile_client.go` (rewrite request layer on net/http)
- Modify: `cmd/profile/profile_client_test.go`
- Modify: `go.mod`/`go.sum`

- [x] rewrite the request layer on stdlib net/http, preserving the public surface commands use
      (Get/GetRaw/Post/Put/Patch/Delete and payload/error semantics: auth header, JSON
      encode/decode, API error mapping, pagination/links behavior if go-request handled any)
- [x] drop `gildas/go-request`; `go mod tidy`; VERIFY gildas/go-logger and the whole
      cloud.google.com / google.golang.org/grpc / go.opentelemetry.io tree leave `go list -m all`;
      record before/after module counts
- [x] audit remaining `gildas/*` deps (go-core, go-cache, go-flags) for heavy transitive baggage;
      report residuals (report only — no further replacements in this task)
- [x] tests: httptest coverage for the rewritten client — success, API error body mapping,
      auth header presence, non-2xx handling
- [x] `go test -race ./...` + lint green; PR, CI green, merge
- ⚠️ the VERIFY step did NOT achieve a clean graph: `gildas/go-logger` (and therefore its entire
      `cloud.google.com/*`, `google.golang.org/grpc`, `go.opentelemetry.io/*`,
      `google.golang.org/api`, `google.golang.org/genproto` tree) is still present in
      `go list -m all` after dropping go-request. Root cause found via
      `go mod graph | grep gildas`: both `gildas/go-cache` (kept per Technical Details) and
      `gildas/go-flags` (kept — cobra completion is a hard requirement) declare their own direct
      dependency on `gildas/go-logger` (v1.9.2 and v1.9.4 respectively) in their own go.mod files,
      completely independent of go-request. Dropping go-request was still worth doing (it also
      dropped `gildas/go-errors` as a *direct* import of this repo — profile_client.go now imports
      only stdlib `errors`), but the go-logger/cloud/otel/grpc tree cannot be removed from this
      repo's module graph without either replacing go-cache and go-flags (both explicitly
      out-of-scope replacements) or those two libraries dropping their go-logger dependency
      upstream. Module count: 213 → 209 (net -4: go-request itself plus its exclusive deps
      `google/go-pkcs11`, `google/martian/v3`, `golang/snappy`, `zeebo/errs`, `golang.org/x/term`,
      `google.golang.org/appengine` all left the graph; `github.com/creack/pty`, `github.com/kr/pty`,
      and `github.com/pkg/diff` entered as newly-surfaced transitive test-only deps of the
      remaining gildas/* modules). `gildas/go-errors` moved from a direct to an `// indirect`
      requirement in go.mod (still present transitively via go-logger/go-cache/go-flags, but no
      first-party file imports it anymore — profile_client.go's former go-errors import, called
      out as the last one in Task 4, is gone).
- ➕ gildas/* residual audit (report only, per this task's scope):
      - `gildas/go-core`: light — only pulls in `google/uuid`, `stretchr/testify` (test-only),
        `golang.org/x/exp`, and `gopkg.in/yaml.v3`. No logger/grpc/otel baggage of its own.
      - `gildas/go-cache`: pulls in `gildas/go-logger` directly (v1.9.2), and therefore the same
        cloud.google.com/go-logging + grpc + otel + google.golang.org/api tree go-logger always
        drags in; also `gildas/go-errors`, `joho/godotenv` (already a direct dep here).
      - `gildas/go-flags`: pulls in `gildas/go-logger` directly (v1.9.4) — same tree again — plus
        `gildas/go-errors`, `joho/godotenv`, and (necessarily) `spf13/cobra`/`spf13/pflag`, which
        this repo already depends on directly.
      - Net effect: go-cache and go-flags are two independent, sufficient reasons the
        go-logger/cloud/grpc/otel tree stays in the graph; go-request was never the only cause,
        just the one Task 3 happened to notice first.
- ➕ `PostWithResult`'s return type changed from `*request.Content` to the new local `*Response`
      type (`StatusCode`, `StatusText`, `Headers http.Header`, `Body []byte`); the only external
      caller (`cmd/pullrequest/merge.go`) only ever read `.Headers.Get("Location")`, which the new
      type still exposes under the same field name — no caller changes needed beyond the type name
      itself resolving via type inference.
- ➕ fixed a latent nil-pointer panic in `GetRaw`: the old code called `result.Reader()` on the
      `*request.Content` returned by `send()` unconditionally, including on the error paths where
      `send()` returns `(nil, err)` (e.g. a failed OAuth authorization) — calling a pointer-receiver
      method that dereferences a nil `*Content` panics. The rewrite checks `result == nil` first
      and returns `(nil, err)` in that case.
- ➕ fixed a latent bug in `writeAuthorizationErrorResponse` (the local OAuth callback's HTTP error
      response to the browser): when the token endpoint's error body failed to parse as JSON, the
      old code silently `return`ed without writing anything, so the browser would have received an
      implicit `200 OK` with an empty body instead of an error. It now falls back to
      `http.Error(w, err.Error(), http.StatusInternalServerError)`, matching the `result == nil`
      branch just above it.
- ➕ the OAuth2 token endpoint (`https://bitbucket.org/site/oauth2/access_token`) requires a
      form-encoded body, not JSON — go-request picked this encoding automatically because the
      payload was a `map[string]string` without an explicit `PayloadType`. The rewrite makes this
      explicit: `sendOAuthTokenRequest` always builds `application/x-www-form-urlencoded` bodies,
      called out in a doc comment so it isn't "simplified" to JSON by a future change.
- ➕ the go-errors-specific status-code plumbing in `bitbucketAuthError`/
      `writeAuthorizationErrorResponse` (`errors.As(err, &*errors.Error)` to recover the HTTP status
      code) is gone: the new local `*Response` type carries `StatusCode` directly from the HTTP
      response, so both functions read `result.StatusCode` instead of unwrapping an error type.

### ➕ Task 5c: Replace gildas/go-cache + go-flags with local implementations

➕ Added after Task 5b discovery: `gildas/go-cache` and `gildas/go-flags` each declare their
own direct dependency on `gildas/go-logger`, so the cloud.google.com/grpc/otel tree survives
both prior removals (module count stuck at 209). Task 10's <100-module target requires this.
Both libraries are used narrowly and are replaceable with small local code.

**Files:**
- Create: `cmd/common/flags.go` (or similar) — local EnumFlag implementing pflag.Value + completion
- Modify: `cmd/common/cache.go` — local persistent TTL cache replacing go-cache
- Modify: all EnumFlag call sites (~26 files incl. cmd/root.go), cache call sites (repository/user/workspace)
- Modify: `go.mod`/`go.sum`

- [x] implement local EnumFlag (pflag.Value + cobra completion func) with the same behaviors
      the repo uses from go-flags (allowed values, dynamic allowed-func for --workspace, env default);
      migrate all call sites; drop gildas/go-flags
- [x] implement local persistent TTL cache (os.UserCacheDir JSON file, expiry, and the
      BITBUCKET_CLI_CACHE_DURATION / BITBUCKET_CLI_CACHE_ENCRYPTIONKEY env behaviors preserved —
      AES-GCM when key set); migrate RepositoryCache/UserCache/WorkspaceCache; drop gildas/go-cache
- [x] audit gildas/go-core (the last gildas dep): what does it pull, which helpers do we use,
      is it droppable with trivial local code? If ≤ ~30 call sites of simple helpers
      (GetEnvAsString etc.), replace it too in this task; otherwise report only
- [x] `go mod tidy`; VERIFY gildas/go-logger and the cloud/grpc/otel/genproto tree finally leave
      `go list -m all`; record before/after module counts
- [x] tests: EnumFlag (set valid/invalid, completion), cache round-trip (fresh/expired/encrypted)
- [x] `go test -race ./...` + lint green; PR, CI green, merge
- ➕ go-flags call-site inventory (26 files, all mechanical): `EnumFlag`/`EnumSliceFlag` struct
      types (one field access, one struct literal in `cmd/root.go`); constructors `NewEnumFlag`,
      `NewEnumFlagWithFunc`, `NewEnumSliceFlag`, `NewEnumSliceFlagWithAllAllowed`,
      `NewEnumSliceFlagWithAllAllowedAndFunc` (the plain `NewEnumSliceFlagWithFunc`, without
      "AllAllowed", was never called anywhere — not implemented); methods `Type`/`String`/`Set`/
      `CompletionFunc` plus `pflag.SliceValue`'s `Append`/`Replace`/`GetSlice` on the slice flag.
      All landed in `cmd/common/flags.go`, matching this repo's existing local-pflag.Value
      convention (`cmd/common/error_processing.go`'s `ErrorProcessing` type predates this task and
      was the template). 25 of the 26 files already imported `cmd/common` for other reasons
      (`common.WhatIf`, etc.); only `cmd/user/me.go` needed the import added.
- ➕ go-cache call-site inventory: only `Get(key string) (*T, error)` and
      `Set(item T, key ...string) error` are ever called (`RepositoryCache`/`UserCache`/
      `WorkspaceCache`, each with exactly one key argument) — no `Clear`/`SetWithExpiration`/
      `core.Identifiable` auto-keying in use. The new `cmd/common/cache.go` implements exactly
      those two methods; the exported `common.NewCache[T]()` keeps its original zero-argument
      signature (hardcoded "bitbucket" cache name) so the three call sites in
      `cmd/repository/repository.go`, `cmd/user/user.go`, `cmd/workspace/workspace.go` needed no
      changes at all. Cache-entry filenames are derived with `crypto/sha256` (not `sha1`/`uuid` as
      upstream used) purely to avoid gosec's weak-crypto import blocklist on `crypto/sha1` — the
      hash is a filename derivation, not a security boundary, sha256 has no such downside.
- ➕ go-core audit (report only, confirming Task 5b's finding): 27 files import it; usage is not
      trivial env-helpers — `core.Map`/`core.Sort`/`core.Filter` (generic collection helpers, used
      in every list/sort command), `core.URL`/`core.Time`/`core.Timestamp` (JSON-marshaling types
      embedded directly as struct fields across `profile`, `pullrequest/task`, `common/link.go`),
      and `core.TypeCarrier` (an interface several domain types implement). This matches the
      plan's "deeply embedded" criterion exactly (types in struct fields, not simple env getters)
      — go-core is kept, no replacement attempted. It pulls no gildas/logger/grpc/otel baggage of
      its own (confirmed again via `go mod graph | grep gildas`).
- module count: 209 → 38 (-171): dropping go-flags and go-cache removed their shared
  `gildas/go-logger` dependency, taking its entire `cloud.google.com/*` /
  `google.golang.org/grpc` / `go.opentelemetry.io/*` / `google.golang.org/api` /
  `google.golang.org/genproto` tree with it — the last gildas/go-logger consumer left, so this
  is now completely absent from `go list -m all`. `gildas/go-errors` (transitive via go-flags/
  go-cache since Task 5b) is also gone. `go list -m all | grep -E "gildas|cloud.google|grpc|
  opentelemetry|genproto|google.golang.org/api"` now returns only `github.com/gildas/go-core`
  (kept, per the audit above) plus the repo's own module line.

### Task 6: Layout — cmd/bb + internal/ (PR #6)

**Files:**
- Create: `cmd/bb/main.go` (moved from root `main.go` + `version.go` contents)
- Move: `cmd/<pkg>` → `internal/<pkg>` for: root(→`internal/cmd`), common, profile, pullrequest(+subpkgs),
  user, branch, commit, project(+reviewer), repository, workspace, remote
- Modify: `go.mod` (module rename → `github.com/avitsrimer/bitbucket-cli`); all import paths; `Makefile` build path

- [x] rename module: `go mod edit -module github.com/avitsrimer/bitbucket-cli` (fork is permanently
      detached; required for `go install` of the fork to ever work)
- [x] move root main.go/version.go to `cmd/bb/`; rewrite ALL import paths repo-wide in one
      mechanical sweep (module rename + internal/ move together, gofmt-verified)
- [x] move packages under `internal/`; merge trivia: fold `project/reviewer` (1 struct) into `project`
- [x] verify no exported-surface consumers exist outside the module (it's a CLI — safe)
- [x] update Makefile default target to `./cmd/bb`, binary name `bb`
- [x] full test suite + lint green (imports-only change should not alter behavior)
- [x] open PR, wait CI, merge
- ⚠️ `version.go` was relocated as-is (VERSION constant, branch/commit/stamp vars unchanged);
      the plan's checklist line mentioned "version var lands in `internal/cmd` per Technical
      Details" but that rename/ldflags rework is explicitly Task 9's job per the Technical
      Details bullet and Task 9's own checklist — Task 6 only moves the file. Makefile's awk
      extraction of `VERSION`/`APP` was repointed at `cmd/bb/version.go` and verified to still
      match.
- ➕ local environment had no GNU Make ≥4 installed (`make` resolved to Apple's ancient BSD/GNU
      Make 3.81, and the Makefile hard-requires 4+ via its `OSTYPE != uname -s` POSIX
      shell-assignment syntax, pre-existing before this task); installed `make` via `brew install
      make` (available as `gmake`) to validate the Makefile build path end-to-end. Not a repo
      change — noted for anyone hitting the same gate locally on macOS.
- module count: unchanged at 38 (`go mod tidy` after the rename/move is a no-op — confirmed via
  diff against the pre-task go.mod/go.sum); this task is import-path-only and touches no
  dependency.

### Task 7: Pullrequest action boilerplate helper (PR #7)

**Files:**
- Create: `internal/pullrequest/action.go` (+ `action_test.go`)
- Modify: `internal/pullrequest/{approve,unapprove,decline,merge,request-changes,remove-request-changes}.go`

- [x] extract shared skeleton (ValidArgs PR-id completion + RunE: resolve profile/repo/id →
      WhatIf → API verb → print) into one helper with per-command parameters
- [x] rewrite the 6 action files onto the helper (each becomes ~20 lines: metadata + call)
- [x] table-driven tests for the helper via httptest (success, API error, dry-run, invalid id)
- [x] verify each command's CLI contract unchanged (flags, output, completion) — golden help-text check
- [x] `go test -race ./...` + lint green; open PR, wait CI, merge
- ➕ approve/unapprove/decline/request-changes/remove-request-changes now each declare a
      package-level `actionSpec` value (e.g. `approveSpec`) consumed by both their `init()` and
      `action_test.go`'s table-driven tests, so tests exercise the exact same spec data the real
      command registers instead of a hand-duplicated copy that could drift.
- ➕ merge.go keeps its own `mergeCmd`/`mergeProcess` (extra flags, async handling, Location-header
      parsing via the Task 5b `*profile.Response` type) exactly as planned; only its
      `mergeValidArgs` was rewritten to call the new shared `openPullRequestIDsCompletion` helper.
- ➕ the five simple actions' WhatIf/error-message phrasing turned out uniform enough to carry as
      two short spec fields (`whatIf` gerund phrase, `errVerb` infinitive phrase) rather than a
      hardcoded verb; `errVerb` doubles as the phrase in both the "cannot %s pull request" and
      "failed to %s pull request %s" templates for all five actions, including the two multi-word
      ones ("request changes on", "remove request changes on").
- ➕ preserved a pre-existing inconsistency rather than "fixing" it during the extraction: three of
      the five actions (approve, request-changes, remove-request-changes) logged
      `[DEBUG] fetched %d pullrequest ids` in their ValidArgsFunction while unapprove/decline did
      not; carried forward verbatim as a per-spec `logFetch` bool so behavior is unchanged.
- CLI contract verification: captured `bb pullrequest --help` and each of the six actions'
  `--help` before this task's changes and diffed against the same captures afterward — byte
  identical (`diff -r` reported no differences); `bb completion bash` output is also unchanged.
- LOC: the five simple action files went from 76/67/74/77/71 (365 total) to 17/16/16/18/18 (85
  total); merge.go went from 116 to 106 (only its `ValidArgsFunction` body shrank); the new
  `internal/pullrequest/action.go` helper is 111 lines. `golangci-lint run` reports 0 issues with
  the Task 7 `dupl` exclusion removed from `.golangci.yml`.

### Task 8: Tests hardening + lint tightening (PR #8)

**Files:**
- Modify: `.golangci.yml` (remove Task-2 temporary exclusions)
- Create: tests for `internal/user`, `internal/pullrequest/task`, thin spots in `internal/profile`
- Delete: any remaining stale testdata fixtures

- [x] remove all temporary lint exclusions added in Task 2; fix whatever surfaces
- [x] add table-driven httptest coverage for user (get/me) and pullrequest/task subcommands
- [x] raise pullrequest package coverage to ≥40% (from 14%) focusing on list/get/create parsing
- [x] prune unused testdata fixtures; ensure `go test ./... -cover` reports no 0% package with logic
- [x] `go test -race ./...` + lint green; open PR, wait CI, merge
- ➕ the only remaining Task 2 temporary exclusion was `testifylint` on `_test.go` files (the
      Task 8 note in `.golangci.yml` already anticipated this — the exclusion block had no other
      leftover entries from Tasks 3-7 to remove). Deleting it surfaced 17 findings across the
      existing suite-style tests, all fixed as real bugs/idiom cleanups rather than suppressed:
      `error-is-as`/`go-require`/`require-error`/`empty`/`float-compare` findings in
      `internal/profile/{error_test,profile_client_test,row_test}.go`,
      `internal/pullrequest/comment/create_test.go`, and `internal/remote/remote_test.go`. Notably,
      `profile_client_test.go`'s `suite.Require().NoError(...)` inside an `httptest` handler
      goroutine was a latent risk (`require` calls `runtime.Goexit` on failure, which only halts
      the handler goroutine, not the test goroutine, so a real assertion failure there would have
      hung the test instead of failing it cleanly) — changed to `suite.NoError`, matching
      testifylint's go-require rule. `golangci-lint run` now reports 0 issues with 0 exclusions
      left in `.golangci.yml`'s `exclusions.rules` beyond the permanent, non-task ones already
      documented in Technical Details.
- ➕ new httptest-pattern coverage added, reusing/extending `internal/pullrequest/action_test.go`'s
      approach: `internal/pullrequest/{list,get,create}_test.go` (list/get/create success, API
      error, dry-run, plus a missing-title and a reviewers-lookup-failure case for create) and a
      new `internal/pullrequest/pullrequest_row_test.go` for the pure `GetHeaders`/`GetRow`/
      `String`/`Validate`/`GetPullRequestIDFromArgs`/`PullRequests` methods (no network needed);
      `internal/pullrequest/common/getters_test.go` (previously untested, 0% → 90.0%) covering
      `GetPullRequestIDsWithState`/`GetPullRequestIDs`' OPEN-then-ALL fallback; a full
      `internal/pullrequest/task/{helpers,list,get,create}_test.go` scaffold (package had zero
      tests, 0% → 44.3%) mirroring `action_test.go`'s cache-priming pattern in its own package;
      and `internal/user/{helpers,get,me,participant}_test.go` (0% → 56.3%), including a
      table-output rendering case for both `get` and `me` per the task's "output rendering"
      requirement. `action_test.go`'s `setupTest` was split into a thin wrapper over a new
      `setupTestNamed(t, profileName, handler, dryRun)` so the new create tests (which exercise
      `user.GetMe`, cached under `profile.Name + ":me"`) can use a unique profile name per
      sub-test and never collide with the shared `UserCache`/`RepositoryCache`/`WorkspaceCache`
      singletons other tests populate.
- ➕ per-package coverage, before → after this task (`go test ./... -cover`): `pullrequest` 27.7%
      → 40.8% (plan's own baseline said ~14%; the higher real starting number reflects Task 7's
      `action_test.go` landing since the plan was drafted — the ≥40% target is still met with
      room to spare); `pullrequest/common` 0.0% → 90.0%; `pullrequest/task` 0.0% → 44.3%; `user`
      0.0% → 56.3%; `project` 0.0% → 88.9% (String/MarshalJSON + `Reviewer` JSON round-trip, per
      the plan's "small unmarshal/String test suffices" guidance); `repository` 0.0% → 44.9%
      (String/GetPath/Validate/MarshalJSON/UnmarshalJSON, using the existing
      `testdata/repository.json` fixture instead of a hand-built one). Unrelated packages
      untouched: `common` 57.1%, `profile` 34.2%, `branch` 31.4%, `commit` 26.1%,
      `activity` 38.3%, `comment` 17.5%, `remote` 38.3%, `workspace` 18.3%.
- ➕ testdata fixture audit (grep every file in `testdata/` against every `_test.go`): three
      fixtures were genuinely unreferenced anywhere and deleted as orphans — `tags.json` (the
      removed tags/pipeline feature's stale fixture, missed by Task 1's
      `testdata/pipeline-*.json` cleanup), `config-empty.yml` (0 bytes, no test loads it), and
      `config-one.yml` (a single-profile config fixture with no remaining reader after Task 5's
      viper removal). Four other apparently-unused fixtures turned out to be a good fit for this
      task's new tests instead of deletion: `testdata/repository.json` and
      `testdata/participant.json` (real BitBucket API response shapes, now exercised by the new
      `repository`/`user` unmarshal tests), `testdata/pullrequests.json` (a realistic
      multi-item paginated list response, now the fixture for `list_test.go`'s success case), and
      `testdata/error-badrequest-nobranch.json` (a "branch not found" BitBucket error body, now
      the fixture for `create_test.go`'s POST-failure case).
- ⚠️ `cmd/bb` and `internal/cmd` remain at 0.0% coverage and were deliberately left untested:
      both are pure entrypoint/wiring (`cmd/bb/main.go`'s `main()` calls `os.Exit`; `internal/cmd/
      root.go`'s `init()` only registers cobra flags/subcommands and its `Execute` is a one-line
      delegate to `cobra.Command.ExecuteContext`), with no business logic of their own — matching
      the plan's own "no logic package at 0%" bar rather than violating it. Every package that
      does carry business/domain logic now has non-zero coverage.

### Task 9: Release flow — goreleaser + version via ldflags (PR #9)

**Files:**
- Create: `.goreleaser.yml`, `.github/workflows/release.yml`
- Modify: `Makefile` (rewrite jcli-style), `cmd/bb/main.go` (version var)
- Delete: `Makefile.linux`, `Makefile.windows`, `packaging/` (chocolatey/nfpm/snap), `changelog.yaml`,
  `CHANGELOG.md` chglog machinery (keep CHANGELOG.md file with a pointer to GitHub Releases)

- [x] replace VERSION constant with ldflags-stamped `version` var in `internal/cmd` (default "dev");
      version output simplified to the single git-describe string (intentional behavior change —
      replaces the old branch-aware `VERSION[+stamp.commit]` scheme)
- [x] rewrite Makefile: build/test/lint/fmt/install (+ cross-build stub), REV via git describe — mirror jcli
- [x] write `.goreleaser.yml`: `project_name: bb` (repo name is bitbucket-cli, so the cask token
      must be pinned explicitly or it publishes as `bitbucket-cli`), darwin/arm64, CGO_ENABLED=0,
      tar.gz, homebrew_casks → avitsrimer/homebrew-apps `Casks/` on branch `main` with
      quarantine-strip post-install hook, binary `bb`
- [x] write `release.yml`: on tag `v*`, macos-latest, goreleaser-action, GITHUB_TOKEN + HOMEBREW_TAP_TOKEN
- [x] delete legacy packaging files/targets; `goreleaser check` + `goreleaser release --snapshot --clean` locally must pass
- [x] test: version stamping verified in built snapshot binary (`./dist/.../bb version`)
- [x] `go test -race ./...` + lint green; open PR, wait CI, merge
- ➕ neither `goreleaser` nor GNU Make 4+ (`gmake`) were on `PATH` in the local shell, but both were
      already installed via Homebrew at `/opt/homebrew/bin/{goreleaser,gmake}` — used those absolute
      paths (prefixing `PATH` with `/opt/homebrew/bin`) for all local validation instead of installing
      anything new. `/usr/bin/make` on this Mac is still the ancient BSD/GNU Make 3.81 Task 6 already
      flagged; the new Makefile is plain POSIX recipes (no `!=`/`?=` GNU-only assignment tricks) so it
      also runs fine under 3.81, unlike the old Makefile it replaced.
- ➕ `RootCmd.Use` was previously set from `main.go` (`cmd.RootCmd.Use = APP`, `APP` a const in the
      now-deleted `cmd/bb/version.go`); relocated as a hardcoded `Use: "bb"` field directly in the
      `RootCmd` struct literal in `internal/cmd/root.go` — it's a static command name matching the
      binary, not a build-time value, so a constant one file over serves no purpose. `RootCmd.Version`
      is now set the same way (`Version: Version()` field in the same literal) instead of being
      assigned from `main.go` after construction; `main.go` no longer references `APP`/`Version()` at
      all (both lines deleted). The unused `PACKAGE` const from the old `version.go` had no remaining
      reference anywhere once the old Makefile's packaging targets were deleted, so it was dropped
      with no replacement.
- ➕ there is no `bb version` subcommand in this codebase (cobra only auto-adds a `--version` *flag*
      when `Command.Version` is set, never a `version` subcommand) — confirmed via `cobra@v1.10.2`'s
      `InitDefaultVersionFlag`/`Execute` source and grepping the repo/README for any prior `version`
      command wiring; found none. The plan's "`bb version` and `--version` print..." phrasing is read
      as informal shorthand for "the bb version output" via `--version`, the only mechanism that
      exists; no new subcommand was added, since Task 9's scope is the ldflags/version-var plumbing,
      not new UX.
- ➕ added `internal/cmd/version_test.go` (`TestVersionStampedViaLdflags`): builds `cmd/bb` with the
      exact `-ldflags -X .../internal/cmd.version=<rev>` the Makefile uses (via `go build` from a
      temp dir, `rev` from a live `git describe --tags --always --dirty`), runs the resulting binary
      with `--version`, and asserts the output contains that revision string — an end-to-end proof
      the ldflags path actually reaches the linked binary, not just that `Version()` returns whatever
      `version` holds. Skipped under `-short` (it's a real build + subprocess, not a pure unit test).
- verified locally beyond the plan's own bar: `make build` (stamps a real `git describe` revision
  into `./bb --version`), `make test`, `make lint` (0 issues), `make cross-build` (linux stub) all
  pass via the rewritten Makefile; `goreleaser check` validates the config; `goreleaser release
  --snapshot --clean` produces `dist/bb_<snapshot-version>_darwin_arm64.tar.gz` and
  `dist/homebrew/Casks/bb.rb` (cask token `bb`, `desc`/`homepage` populated, quarantine-strip
  `postflight` hook present) and the extracted snapshot binary's `--version` output matches the
  snapshot version goreleaser computed. No tag was pushed and nothing was published (snapshot mode
  skips both, per the plan's own note); `dist/` was removed afterward (already gitignored).

### Task 10: Verify acceptance criteria + docs (PR #10)

**Files:**
- Create: `CLAUDE.md` (repo conventions: layout, build/test/lint commands, comment/commit rules, release flow)
- Modify: `README.md` (CI badge, brew install instructions, remove stale packaging docs)
- Modify: `CONTRIBUTING.md` (upstream URL + retired `dev`-branch PR-target rule)
- Move: this plan → `docs/plans/completed/`

- [x] verify all Overview goals: CI green on master, module count reported (target: <100, from 226),
      only 3 commands exposed, goreleaser snapshot builds, no gildas/go-logger|go-errors|viper|spinner in go.mod
- [x] write CLAUDE.md (jcli's as template, adapted)
- [x] update README (badges, install via `brew tap avitsrimer/apps && brew install --cask bb`, usage intact)
- [x] sweep ALL remaining `gildas/bitbucket-cli` references out of README/docs: `go install` lines
      (now `github.com/avitsrimer/bitbucket-cli/cmd/bb@latest`), releases/issues links,
      starchart badge — nothing may point at upstream
- [x] full suite: `go test -race ./... && golangci-lint run && goreleaser check`
- [x] move plan to `docs/plans/completed/`, commit in this PR
- [x] open PR, wait CI, merge

**Acceptance verification results (2026-08-05):**
- CI green on master: last 3 runs (`build` job, PRs #9/#10/#11) all `completed success`
  (`gh run list --branch master --limit 3`).
- Module count: `go list -m all | wc -l` → **38** (target was <100; baseline was 226 —
  a 188-module, 83% reduction). Only `github.com/gildas/go-core` remains from the
  `gildas/*` family, kept by design per Task 5c's audit (deeply-embedded generics/JSON
  types, no logger/grpc/otel baggage of its own).
- Binary surface: `bb --help` lists exactly `profile`, `pullrequest`, `user` plus
  cobra's built-ins (`completion`, `help`, `--version` flag) — no `repository`,
  `project`, `workspace`, `branch`, `commit`, `remote`, or any of the other
  upstream/removed command groups.
- `goreleaser check` — 1 configuration file validated, no errors.
- `make build` (stamps a real `git describe` revision into `./bb --version`),
  `make test`, `make lint` (0 issues) all pass via the Task 9 Makefile.
- `go.mod`/`go list -m all` grep for `go-logger|go-errors|go-request|go-cache|
  go-flags|viper|spinner` — zero matches. `gildas/go-core` is the sole surviving
  `gildas/*` module.
- Repo-wide grep for `gildas/bitbucket-cli` (outside `.git/`): two matches remain,
  both the single intentional upstream-attribution links this task's own scope
  permits — the `[!NOTE]` fork-credit line at the top of `README.md`, and the
  equivalent line at the top of `CLAUDE.md`. `CONTRIBUTING.md`'s stale upstream
  issue-tracker link and its "target the `dev` branch" rule (retired per the
  Post-Completion notes) were also fixed even though not separately called out in
  this task's file list, since the repo-wide sweep criterion covers all docs, not
  just `README.md`.
- `go build ./... && go vet ./... && gofmt -l . && go test -race ./... &&
  golangci-lint run && goreleaser check` — all clean, no output/errors.

## Post-Completion

**Owner actions (release cut):**
- Create a fine-grained PAT with write access to `avitsrimer/homebrew-apps` and add it as the
  `HOMEBREW_TAP_TOKEN` secret on `avitsrimer/bitbucket-cli` (Settings → Secrets → Actions).
- Run `/release-tools:new` to tag the first modernized release (suggest starting at `v0.19.0`);
  the tag push triggers `release.yml` → goreleaser publishes the GitHub release + Homebrew cask.
- Verify: `brew tap avitsrimer/apps && brew install --cask bb` (cask token pinned to `bb` via
  goreleaser `project_name`), then `bb --version` shows the tag (there is no `bb version`
  subcommand, only the `--version` flag — see Task 9's note above).

**Notes:**
- Single-branch flow on `master` (owner decision 2026-08-05): the upstream-style `dev` branch was
  merged into `master` and deleted; all PRs base on `master` and releases tag `master`.
- Upstream (gildas) divergence is now permanent; future upstream merges are impractical by design.
