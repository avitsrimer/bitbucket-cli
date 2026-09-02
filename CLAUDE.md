# CLAUDE.md

Guidance for Claude Code when working in this repository.

## What this is

`bb` — an opinionated, macOS-first fork of [gildas/bitbucket-cli](https://github.com/gildas/bitbucket-cli),
deliberately narrower than upstream. The supported CLI surface is `bb pullrequest` (full
command tree; `pullrequest list` additionally takes `--author <id>`/`--mine`, which switch it
to a workspace-wide author mode — `GET /workspaces/{ws}/pullrequests/{user}`, no repository
resolution at all, an explicitly-passed `--repository` rejected, and `repository` added to the
default column set), `bb user`, `bb profile` (authentication plumbing), `bb pipeline` (`get`,
`list`, `trigger`, `stop`, plus the `step` subgroup: `get`, `list`, `logs`, `report`,
`cases`), `bb repository`/`bb repo` (`get`, `list`, `clone` — read-only, no
create/delete/fork/update, no `get --forks`), `bb workspace` (`get`, `list`, `members` —
no permission administration), `bb commit`/`bb branch` (read-only: `commit
get/list/diff/patch`, `branch list`), `bb artifact` (`list`, `download` — no
upload/delete, no `--progress`), and `bb install-skill` (writes the embedded
`skill/bitbucket-cli/` Claude skill to `<to>/skills/bitbucket-cli`). `draft` is a `PullRequest`
field surfaced as a column: it sits in the defaults of `get`, `update` and `create` (on the latter
two the printed row is the server's own response to the write, which is what makes those mutations
self-confirming) but not in `list`'s — `merge` prints a single row without `--columns` too, but its
response is always a non-draft, so it carries no `draft` column. It is reachable
via `--columns draft` on `get`/`list`, `--sort draft` on `list` only, and always present in `-o
json|yaml`. `pipeline trigger`/`stop` ask for a `y`/`N` confirmation prompt with `--force` to skip it.
`pullrequest merge` also asks for a `y`/`N` confirmation but deliberately has no
`--force` of any kind — merging is not automatable via `bb` (see
`internal/common/confirm.go`'s `ConfirmInteractive`). Every other state-changing
command runs immediately. `pullrequest update` takes `--ready`/`--draft` (mutually
exclusive; they clear/set the pull request's draft state — each applying its own value, so
`--ready=false` is `--draft` — combine with every other `update` flag in the same PUT, and are an
ordinary `WhatIfPayload`-gated mutation — no confirmation prompt, no `--force` of any kind).

Every other command group inherited from upstream (`project`, `issue`, `tag`, `gpg-key`,
`ssh-key`, `cache`, `remote`, `component`) has been removed from the CLI surface, along
with every admin/destructive verb of the groups above (`repository`
create/delete/fork/update, workspace permission management, pipeline `--tag` targets). A
few packages for groups with no restored surface (`project`, `remote`) remain as internal
libraries other commands depend on — see Layout.

Config lives in `~/Library/Application Support/bitbucket/config-cli.yml` (or
`os.UserConfigDir()`'s platform equivalent, falling back to `~/.bitbucket-cli`),
plain YAML, 0600. Credentials are stored in the OS vault (macOS Keychain via
`zalando/go-keyring`) unless `--no-vault` is passed. Logging goes to stderr via
`go-pkgz/lgr`; there is no file-logging flag.

`internal/common/cache.go` is a small persistent TTL cache for repository/user/workspace
lookups, mirrored to disk under `os.UserCacheDir()/bitbucket/<sha256(key)>` as JSON. Default
TTL is 5 minutes, overridable via `BITBUCKET_CLI_CACHE_DURATION` (a Go duration string, parsed
with plain `time.ParseDuration` — the ISO-8601 form (`PT30S`) go-core's env helper used to also
accept is not supported; an unset, empty, or unparseable value silently falls back to 5 minutes).
There is no encryption (dropped as a simplification: it protected non-sensitive cached metadata
while the actual OAuth token used a separate, unencrypted mechanism) and no `bb cache clear` command —
delete the directory directly to invalidate it.

`Profile` carries five fields (`Progress`, `CloneProtocol`, `CloneUser`, `SshKeyFilename`,
`DefaultProject`) that are persisted (read from and written back to the config file, for
compatibility with configs upstream wrote). `CloneProtocol`, `CloneUser`, and `SshKeyFilename`
are functional again: `internal/repository/clone.go` (`bb repo clone`) resolves protocol and SSH
key file as `--protocol`/`--ssh-key-file` flag > profile field > default, and uses `CloneUser` as
the `https` clone URL's username. `Progress` and `DefaultProject` remain inert — no restored
command reads them. Don't wire those two up to "fix" a seemingly-dead flag, and don't delete any
of the five either — removing a field would silently drop that data from a user's existing config
file on the next save. `DefaultRepository` sits outside this group of five: like `DefaultWorkspace`,
it is a fully functional persisted field, not upstream-compat dead weight. It is the last rung of
`internal/repository.GetRepositoryName`'s `--repository` flag > Bitbucket git remote > profile
default precedence chain (mirroring `internal/workspace.GetWorkspaceName`'s workspace chain), set
via `--default-repository` on `bb profile create`/`update`.

The fork is permanently detached from upstream (different module path, no shared
history intent) — do not try to keep it merge-compatible.

## Layout

```
cmd/bb/main.go            # entry point: load .env, set up lgr, cmd.Execute
internal/cmd/             # cobra RootCmd, global flags, version
internal/common/          # config load/save, EnumFlag, local TTL cache, error helpers,
                           # Confirm/ConfirmInteractive (y/N prompt; the latter has no
                           # --force and errors on EOF-without-input, for merge only),
                           # exported flag-hiding helpers, WhatIf/WhatIfPayload
                           # (dry-run gate + resolved-request echo),
                           # ValidatePathIdentifier (GetPath positional guard),
                           # ReadBodyFromFileOrStdin
internal/profile/         # profile CRUD, OAuth2 authorize flow, HTTP client (net/http).
                           # download.go is the one request path that deliberately
                           # breaks two invariants the rest of the client relies on: it
                           # streams straight to the destination writer instead of
                           # buffering the body, so it is never retried on a transient
                           # failure (doRequestWithRetry's retry logic needs a buffered
                           # body to resend); and it depends on the stdlib http.Client's
                           # default redirect policy stripping the Authorization header
                           # on a cross-host redirect, since Bitbucket's downloads
                           # endpoint 302s to a different upload host. Keep both
                           # properties if you ever touch that file. table.go is a local
                           # ASCII-table renderer reproducing kataras/tablewriter's
                           # NewWriter defaults byte-for-byte (numeric right-align,
                           # header centering/Title()-casing, multi-line/ragged-row
                           # rules, runewidth-based widths); table_golden_test.go pins
                           # that output against bytes captured from the real
                           # tablewriter before it was dropped -- any change to table.go
                           # must keep those goldens byte-identical, not just "close
                           # enough" or "still readable".
internal/pullrequest/     # pullrequest command tree + shared action helper
  /comment, /task, /common # subcommand packages; /common (prcommon) holds shared getters
                           # (GetPullRequestIDs, PullRequestIDValidArgs), ExistsPullRequest, and
                           # the preflight-aware DeleteSubResources
internal/user/            # bb user get/me
internal/workspace/       # bb workspace get/list/members
internal/repository/      # bb repo(sitory) get/list/clone
internal/branch/          # bb branch list (+ dep-free GetCurrentBranch)
internal/commit/          # bb commit get/list/diff/patch
internal/pipeline/        # bb pipeline get/list/trigger/stop
  /common                 # plcommon: getters shared with pipeline/step (breaks the
                           # pipeline->step->pipeline import cycle)
  /step                   # bb pipeline step get/list/logs/report/cases
internal/artifact/        # bb artifact list/download
internal/cmd/install_skill.go # bb install-skill: writes skill.Files to <to>/skills/bitbucket-cli
skill/                    # embed.go (package skill, //go:embed bitbucket-cli, Files embed.FS)
                           # + bitbucket-cli/SKILL.md, the Claude skill bb install-skill writes.
                           # Lives at the repo root, not under internal/ (no import cycle is
                           # possible either way -- skill has no dependency on internal/cmd to
                           # cycle back through). The real constraints are go:embed's own-
                           # package-directory rule (the embedded tree must sit under skill's own
                           # directory) and internal/'s visibility rule, which would otherwise
                           # confine the package to importers inside this module. SKILL.md is
                           # documentation shipped inside the binary: update it in the same PR as
                           # any change to a documented command/flag, or it goes stale silently
                           # (a sync-guard test in internal/cmd catches a renamed/removed command
                           # path, but not a changed flag or behavior description).
internal/testutil/        # shared test harness (profile/fixture setup); imports
                           # common, profile, repository, user, and workspace, so an internal
                           # test file (one declared "package common"/"package profile"/
                           # "package repository"/"package workspace"/"package user", as opposed
                           # to "package foo_test") in any of those five packages would cycle
                           # importing it and must instead duplicate the specific helpers it
                           # needs in a local helpers_test.go. An external test file in one of
                           # those five packages (package foo_test) sits outside the cycle and
                           # can still import testutil normally -- see
                           # internal/workspace/allowed_slugs_test.go.
internal/project/, /remote/
                           # library packages consumed by profile/pullrequest/user;
                           # no cobra command tree of their own (no restore task covers
                           # them)
```

Implementation plans live in `docs/plans/`, but `.gitignore` tracks only
`docs/plans/completed/` — `docs/plans/*` is otherwise ignored. In-progress plans are
local-only; a plan is committed only once moved into `completed/`. Editing a plan's
checkboxes mid-work therefore never shows up in `git status`.

## Build / test / lint

```bash
make build       # go build -ldflags "-X .../internal/cmd.version=$(git describe)" -o bb ./cmd/bb
make test        # go test -race ./...
make lint        # GOTOOLCHAIN=local golangci-lint run (config: .golangci.yml)
make fmt         # gofmt -s -w . && goimports -w .
make cross-build # GOOS=linux CGO_ENABLED=0 go build/vet ./... (proves the repo stays portable)
make install     # build + install to ~/bin (override INSTALL_DIR=/usr/local/bin)
```

`make fmt` and `make lint` are not self-contained: `goimports` and `golangci-lint` are neither
`go.mod` `tool` directives nor vendored, so both must already be on `PATH` (`go install
golang.org/x/tools/cmd/goimports@latest`; `golangci-lint` pinned to **v2.12.2** to match CI's
`golangci-lint-action` version — a different local version can report findings that don't match
what CI reports).

The Makefile is plain POSIX recipes (no GNU-only `!=` shell-assignment tricks; the four
`?=` conditional-assignments it does use are supported by both GNU make and BSD/macOS
make, so they don't break portability), so it runs under both modern GNU make and
macOS's stock BSD/GNU make 3.81. If `make` on your
machine still resolves to that ancient 3.81 (`brew install make` installs a current one
as `gmake`), prefer `/opt/homebrew/bin/gmake` — this repo's Makefile itself doesn't need
it, but `goreleaser` and other tooling referenced below may only be on `PATH` at
`/opt/homebrew/bin`.

## CI & lint config

`.github/workflows/ci.yml` runs on every push/PR: a `build` job on `macos-latest`
(`go test -race ./...` + `golangci-lint-action` pinned to `v2.12.2`) and a
`cross-build` job on `ubuntu-latest` (`GOOS=linux CGO_ENABLED=0 go build/vet ./...`).
No `shellcheck` job — the repo has zero `.sh` files.

`.github/workflows/release.yml` runs GoReleaser (`.goreleaser.yml`) on `macos-latest`
when a `v*` tag is pushed: darwin/arm64 only, tar.gz archive, version stamped through
`-X github.com/avitsrimer/bitbucket-cli/internal/cmd.version={{.Version}}`. It also
publishes a Homebrew **cask** (`project_name: bb` pins the cask token to `bb`, not the
repo name) to the `avitsrimer/homebrew-apps` tap via `homebrew_casks` — the cross-repo
push needs a fine-grained PAT in the `HOMEBREW_TAP_TOKEN` secret (the built-in
`GITHUB_TOKEN` can't push to another repo). The cask's post-install hook runs
`xattr -dr` so brew-installed binaries skip Gatekeeper's quarantine prompt
automatically; a `go install`ed or manually-downloaded binary is unsigned/ad-hoc and
stays quarantined until stripped by hand.

`.golangci.yml` (golangci-lint v2, `default: none`, ~45 explicitly enabled linters,
zero temporary/task-scoped exclusions) drives both CI and `make lint`. Two permanent,
deliberate deviations from the umputun/jcli baseline this config was derived from —
don't "fix" them without a real reason:

- `gochecknoinits` is not enabled — cobra's `init()`-based command registration
  (profile/pullrequest/user, ~40 registrations total) is a kept-forever architectural
  decision, not something to refactor away.
- `importShadow` is added to `gocritic.disabled-checks` — long-standing cobra
  param/receiver names (`cmd`, `context`) shadow imports in ~166 places repo-wide;
  renaming them is low-value busywork.

`issues.max-{same,per-linter}-issues: 0` so `make lint` output is always the honest,
uncapped count. `errcheck` excludes `fmt.Fprint{,f,ln}` (diagnostic writes never need
their error checked in a CLI). `_test.go` files are exempt from `gosec`, `dupl`,
`noctx`, `usestdlibvars`, and unused-parameter checks.

## Go conventions

- Go 1.26. Run `go build ./...`, `go vet ./...`, `gofmt -l .`, `go test -race ./...`,
  and `golangci-lint run` clean before declaring work done.
- In-code comments are lowercase (except godoc on exported symbols) and describe
  current behavior only — never history, never what changed or why it used to be
  different.
- Conventional commit messages; no AI/Claude/Anthropic mentions anywhere in commits,
  PR descriptions, or comments.
- Errors: stdlib only. Wrap with `fmt.Errorf("...: %w", err)`; local sentinel
  `var Err... = errors.New(...)` only where an `errors.Is` check actually exists
  somewhere in the codebase — don't pre-declare unused sentinels. Aggregate multiple
  errors with stdlib `errors.Join`.
- Logging: package-level `go-pkgz/lgr` calls (`lgr.Printf("[DEBUG] ...")`,
  `[WARN]`, `[ERROR]`) — no logger-in-context threading, no per-call-site logger
  construction. Both streams route to stderr; `--debug` adds caller file/func/msec
  prefixes and enables `[DEBUG]` output, otherwise `[DEBUG]` lines are filtered out.
  There is no file-logging flag — `lgr` always writes to stderr.
- Config: plain `gopkg.in/yaml.v3`, atomic writes, 0600 permissions. No viper.
- CLI flags via `spf13/cobra` (kept over `jessevdk/go-flags` deliberately — dynamic
  shell completion via `ValidArgsFunction`/`RegisterFlagCompletionFunc` is a core
  feature this CLI relies on and go-flags has no equivalent).
- Tests: table-driven with subtests for new code; HTTP-facing tests use
  `net/http/httptest` (see `internal/profile/profile_client_test.go`,
  `internal/pullrequest/action_test.go`); existing testify suites remain suite-style
  where already established, but new suite-style assertions must use `suite.NoError`
  rather than `suite.Require().NoError` inside a handler goroutine (`require` calls
  `runtime.Goexit`, which only halts that goroutine, not the test — a failing
  assertion there hangs instead of failing cleanly). No mock framework unless an
  interface boundary appears, then `matryer/moq`.
- One task/change = one PR against `master` (single-branch flow; `dev` was retired).
  Every PR gate is `go test -race ./...` + `golangci-lint run` green in CI before
  merge.
- Every file under `testdata/` must be referenced by at least one test; delete orphaned
  fixtures rather than leaving them (three were removed for exactly this reason during
  the modernization — a stale fixture with no reader is a trap for the next person who
  assumes it's exercised).
- Shelling out to an external command (`internal/repository/clone.go`'s `git clone`,
  `internal/branch/current.go`'s `git symbolic-ref`) always uses `exec.CommandContext` with an
  explicit argv, never a shell (`sh -c` or similar) — string-concatenating user input into a
  single command line reopens exactly the injection class explicit argv exists to close.
  `os.Environ()`-derived env vars that a subprocess's own subshell *will* re-parse (e.g. git's
  `GIT_SSH_COMMAND`, which git passes to `/bin/sh`) must still be shell-quoted/escaped even though
  the outer `exec.Command` call itself needs no quoting. Wire stdin/stdout/stderr straight through
  unless the command's output must be captured. A justified `//nolint:gosec` (G204) sits next to
  the call. Never build a URL carrying userinfo into a log line. Test this kind of code with a
  fake executable of the same name placed first on `PATH` in a `t.TempDir()`, recording its exact
  argv to a file for the test to assert on (see `setupGitShim` in
  `internal/repository/clone_test.go`) — this runs the real code path through the real
  `os/exec` machinery without ever invoking the real external binary.
- A `RunE` function reads every flag it needs directly off the `cmd *cobra.Command` it was
  called with (`cmd.Flags().GetString("query")`, `common.SortFlagValue(cmd)`, etc.), never off a
  package-level variable a flag was bound to at registration time. This is what lets a test drive
  the function with a standalone `*cobra.Command` carrying its own flags and get identical
  behavior to the real, fully-wired command — a package-level binding is only ever populated on
  the one real command instance, so a test built around one is exercising something a production
  invocation with a different `cmd` value could never see. Where earlier code still binds a flag
  to a package-level struct field (e.g. `internal/repository/list.go`'s `listOptions.Role`),
  treat it as legacy, not a second sanctioned pattern — write new flag-reading code the direct-off-`cmd` way.
- All `gildas/*` dependencies were replaced by stdlib or small local code during the
  modernization (`go-logger` → `go-pkgz/lgr`, `go-errors` → stdlib `errors`/`fmt`,
  `go-request` → `net/http`, `go-cache`/`go-flags` → local code in
  `internal/common/{cache,flags}.go`; `go-core`'s generics (`core.Map`/`core.Sort`/`core.Filter`)
  and env helpers (`GetEnvAsString`/`GetEnvAsDuration`) → `internal/common/{generics,env}.go`;
  its `core.URL`/`core.Time`/`core.Timestamp` JSON-marshaling types → `internal/common/coretypes.go`,
  ported to cover only the surface their four consumers — `common/link.go`, `profile/profile.go`,
  `profile/token.go`, `pullrequest/task/task.go` — actually use). `gildas/go-core` is no longer a
  dependency; each ported type's byte-for-byte compatibility with the dropped dependency is pinned
  by golden-fixture tests in `internal/common/coretypes_test.go` and alongside each of those four
  consumers. `kataras/tablewriter` is also gone the same way, replaced by the local renderer in
  `internal/profile/table.go` (see Layout). `github.com/mattn/go-runewidth` remains a direct
  dependency -- table.go calls it to measure display width, the same way `truncateCell` already
  did -- even though tablewriter, its original reason to appear in the module graph, is gone.
- Every user-supplied positional (or flag value) that reaches a `repository.Repository.GetPath`
  call — a pull request/comment/task id, a pipeline id, a pipeline step UUID-or-name, a commit
  hash, an artifact name — is validated via `common.ValidatePathIdentifier` before it does:
  `GetPath` is a bare `path.Join` with no escaping, so an unvalidated value can splice extra path
  segments into the request. The one sanctioned exception is a value that legitimately spans two
  segments in the `workspace/repository` form (`repository.GetRepositoryBySlugOrID`, reached via
  `--repository`/`--default-repository`), which validates that shape on its own terms instead.
  A uripath built by hand instead of through `GetPath` needs BOTH `ValidatePathIdentifier` *and*
  `url.PathEscape` on every interpolated segment — `internal/pullrequest/list.go`'s author mode is
  the reference (it validates the workspace and `--author` segments and escapes both).
  `url.PathEscape` is the non-negotiable half: validation deliberately permits `?` and `#`, and
  `profile.resolveRequestURL` splits the uripath on the first `?` and takes the remainder as the
  request's `RawQuery`, so an unescaped segment can replace the caller's own query parameters
  wholesale. Validation is the half that buys a clear "argument X is invalid" instead of an
  unexplained 404 — escaping alone is safe but opaque, which is what
  `internal/workspace/workspace.go`'s workspace paths (:171, :191) currently do; new hand-built
  paths should do both.
- Every mutating `RunE` gates its write on `common.WhatIfPayload` (or, for `pipeline
  trigger`/`stop`, `common.Confirm`; for `pullrequest merge`, `common.ConfirmInteractive`, gated
  after `WhatIfPayload` so `--dry-run` still short-circuits first), called only AFTER every
  resolution GET a real invocation would make (looking up the target resource, validating a
  `--file` diff anchor, resolving reviewers, ...) — a `--dry-run` must fail identically to a real
  invocation for a nonexistent target or an invalid input, never report a fabricated success for
  something that would actually fail. `WhatIfPayload` echoes the resolved target path and payload
  to stderr; a payload carrying a secret (e.g. a pipeline trigger's `--variable` values) must be
  redacted by the caller before it ever reaches that call. Read-only commands
  (`get`/`list`/`diff`/...) keep the plain `common.WhatIf` short-circuit, since there is no write to
  gate. It is checked before any resolution that only exists to *find the target*, but a read-only
  command whose dry-run line cannot even be printed without a lookup resolves that lookup first —
  `pullrequest list --mine` issues `GET /user` before the gate, since the author it would query is
  part of what `--dry-run` reports (and resolving it is itself a read, never a write).
- HTTP status classification lives in `internal/profile/error.go`: `mapErrorResponse` wraps every
  non-2xx response's error in the unexported `statusError{StatusCode, err}`, whose `Error()` renders
  the wrapped error verbatim (so messages are unchanged) and which `Unwrap()`s to it (so an
  `errors.As` for `*BitBucketError` still finds the API's own payload underneath). Callers that need
  to react to a specific status use the exported `profile.IsNotFound` — or add a sibling helper next
  to it — never a string match on the message and never a new status field on `BitBucketError`.
- Read paths tolerate an unrecognized variant/type VALUE (a new activity kind, comment shape,
  ...) rather than failing the whole decode: the offending entries are skipped with one `[WARN]`
  per distinct unrecognized kind (deduped locally to the call, never via package-level state) and
  every entry of a known kind still renders. Malformed JSON, wrong shapes, or a missing required
  identity field are still hard errors — permissiveness applies only to unrecognized ENUM-shaped
  values, never to structural or identity validation. See `internal/pullrequest/activity.go`'s
  `Activity.UnmarshalJSON` for the reference implementation.

## Plans convention

Implementation plans live in `docs/plans/*.md` while in progress and are
`.gitignore`d (see Layout). A finished plan — every checkbox `[x]`, every discovery
note (➕/⚠️) recorded — moves to `docs/plans/completed/` and is committed with the PR
that finishes it (`git add -f`, since the file was previously untracked/ignored).
