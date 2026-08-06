# CLAUDE.md

Guidance for Claude Code when working in this repository.

## What this is

`bb` — an opinionated, macOS-first fork of [gildas/bitbucket-cli](https://github.com/gildas/bitbucket-cli)
that does one thing: work with Bitbucket pull requests from the terminal. Only the
`bb pullrequest` command tree, `bb user`, and the `bb profile` authentication plumbing
they depend on are supported — every other command group inherited from upstream
(`repository`, `project`, `workspace`, `issue`, `pipeline`, `branch`, `commit`, `tag`,
`artifact`, `gpg-key`, `ssh-key`, `cache`, `remote`, `component`) has been removed from
the CLI surface (a few of the underlying packages remain as internal libraries other
commands depend on — see Layout).

Config lives in `~/Library/Application Support/bitbucket/config-cli.yml` (or
`os.UserConfigDir()`'s platform equivalent, falling back to `~/.bitbucket-cli`),
plain YAML, 0600. Credentials are stored in the OS vault (macOS Keychain via
`zalando/go-keyring`) unless `--no-vault` is passed. Logging goes to stderr via
`go-pkgz/lgr`; there is no file-logging flag.

`internal/common/cache.go` is a small persistent TTL cache for repository/user/workspace
lookups, mirrored to disk under `os.UserCacheDir()/bitbucket/<sha256(key)>` as JSON. Default
TTL is 5 minutes, overridable via `BITBUCKET_CLI_CACHE_DURATION` (a Go duration string). There
is no encryption (dropped as a simplification: it protected non-sensitive cached metadata while
the actual OAuth token used a separate, unencrypted mechanism) and no `bb cache clear` command —
delete the directory directly to invalidate it.

`Profile` carries five fields (`Progress`, `CloneProtocol`, `CloneUser`, `SshKeyFilename`,
`DefaultProject`) that are persisted (read from and written back to the config file, for
compatibility with configs upstream wrote). `CloneProtocol`, `CloneUser`, and `SshKeyFilename`
are functional again: `internal/repository/clone.go` (`bb repo clone`) resolves protocol and SSH
key file as `--protocol`/`--ssh-key-file` flag > profile field > default, and uses `CloneUser` as
the `https` clone URL's username. `Progress` and `DefaultProject` remain inert — no restored
command reads them. Don't wire those two up to "fix" a seemingly-dead flag, and don't delete any
of the five either — removing a field would silently drop that data from a user's existing config
file on the next save.

The fork is permanently detached from upstream (different module path, no shared
history intent) — do not try to keep it merge-compatible.

## Layout

```
cmd/bb/main.go            # entry point: load .env, set up lgr, cmd.Execute
internal/cmd/             # cobra RootCmd, global flags, version
internal/common/          # config load/save, EnumFlag, local TTL cache, error helpers
internal/profile/         # profile CRUD, OAuth2 authorize flow, HTTP client (net/http)
internal/pullrequest/     # pullrequest command tree + shared action helper
  /comment, /task, /common # subcommand packages + shared getters
internal/user/            # bb user get/me
internal/branch/, /commit/, /project/, /repository/, /workspace/, /remote/
                           # library packages consumed by profile/pullrequest/user;
                           # no longer exposed as their own cobra command trees
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

The Makefile is plain POSIX recipes (no GNU-only `!=`/`?=` assignment tricks), so it
runs under both modern GNU make and macOS's stock BSD/GNU make 3.81. If `make` on your
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
- Five `gildas/*` dependencies were replaced by stdlib or small local code during the
  modernization (`go-logger` → `go-pkgz/lgr`, `go-errors` → stdlib `errors`/`fmt`,
  `go-request` → `net/http`, `go-cache`/`go-flags` → local code in
  `internal/common/{cache,flags}.go`). `gildas/go-core` is the one dependency kept —
  its `core.Map`/`core.Sort`/`core.Filter` generics and `core.URL`/`core.Time`/
  `core.Timestamp` JSON-marshaling types are embedded directly in domain struct
  fields across `profile`, `pullrequest/task`, and `common/link.go`, not just called
  as env-var helpers, so it isn't a trivial drop-in replacement candidate.

## Plans convention

Implementation plans live in `docs/plans/*.md` while in progress and are
`.gitignore`d (see Layout). A finished plan — every checkbox `[x]`, every discovery
note (➕/⚠️) recorded — moves to `docs/plans/completed/` and is committed with the PR
that finishes it (`git add -f`, since the file was previously untracked/ignored).
