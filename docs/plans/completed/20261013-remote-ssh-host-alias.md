# Detect Bitbucket remotes behind SSH host aliases

## Overview
- `bb` auto-resolves workspace/repository from the git remote (`internal/remote`), but only accepts a remote
  whose URL contains `bitbucket.org`. A remote that goes through an `~/.ssh/config` Host alias
  (`git@bitbucket-work:ws/repo.git`, or the bare `bitbucket-work:ws/repo.git` with the user set in ssh config)
  is rejected, so every command needs an explicit `--repository`/`--workspace`.
- Ports the intent of upstream 88f6883 (gildas/bitbucket-cli: plain `strings.Contains(url, "bitbucket")`, later
  narrowed on upstream/dev to `"://bitbucket"`/`"@bitbucket"`) with a stricter, host-based check (decisions.md D4).
- One PR against `master`: `fix(remote): detect bitbucket remotes behind ssh host aliases`.

## Context (from discovery)
- `internal/remote/remote.go:62,76` — `strings.Contains(url, "bitbucket.org")` in both branches of
  `GetRemoteFromReader`.
- `internal/remote/remote.go:105-126` — `RepositoryName` handles `bitbucket.org:` prefix, `git@` scp-form,
  `https://`/`ssh://`; anything else returns the raw URL (wrong for a bare alias).
- `internal/remote/remote_test.go` — existing testify tests feed a git-config payload to `GetRemoteFromReader`.
- Consumers: `internal/repository/repository.go:235-257`, `internal/workspace/workspace.go:113-115` (doc comment at
  repository.go:235 mentions "not bitbucket.org" — update wording).
- `skill/bitbucket-cli/SKILL.md` and `CLAUDE.md` may describe remote detection — grep and update if so.

## Development Approach
- **testing approach**: TDD — write the failing tests first, run them and record the failure in the progress
  file, then implement.
- complete each task fully before moving to the next; small focused changes
- **CRITICAL: every task MUST include new/updated tests**; **all tests must pass before the next task**
- **CRITICAL: update this plan file when scope changes during implementation**
- maintain backward compatibility: every Bitbucket-hosted remote form accepted today stays accepted with the same
  result (non-Bitbucket hosts that the old substring check accepted by accident, e.g.
  `https://github.com/me/bitbucket.org.git`, are now rejected — intended)

## Testing Strategy
- unit tests only (no e2e in this repo); table-driven with subtests for new code (CLAUDE.md Go conventions).

## Progress Tracking
- mark completed items `[x]` immediately; ➕ for new tasks; ⚠️ for blockers.

## Solution Overview
- Add an unexported `remoteHost(rawURL string) string` that extracts the host from:
  - URL form `scheme://[user[:pw]@]host[:port]/path` (use `net/url`; lower-case `Hostname()`);
  - scp-like form `[user@]host:path` (no `://`): host = text before the first `:`, after the last `@`; if that
    text contains a `/` it is a local path (git's own rule) and the host is empty.
- Unexported `isBitbucketHost(host string) bool`: true iff host == `bitbucket.org`, has suffix `.bitbucket.org`,
  or has prefix `bitbucket` (the ssh Host-alias convention: `bitbucket-work`, `bitbucket.work`, `bitbucket_me`).
- `GetRemoteFromReader` uses `isBitbucketHost(remoteHost(url))` in both branches instead of the substring check.
- `RepositoryName` parses generically: any `://` form via `url.Parse(...).Path` (so `http://`, `git+ssh://`,
  `ssh://...:22` all work, not only `https://`/`ssh://`), any scp-like form as the path after the first `:`;
  then trim leading/trailing `/` FIRST and a `.git` suffix SECOND (so `repo.git/` -> `repo`). Keep
  `TestCanGetWorkspaceNameWithoutSlash` (remote_test.go:137, `https://bitbucket.org` -> empty workspace) green.
- Accepted side effect (record in PR): a self-hosted `bitbucket.example.com` is now accepted (upstream also
  accepts it); it fails later at the Cloud API instead of at detection.

## Technical Details
Must-accept table (URL → RepositoryName):
- `git@bitbucket.org:ws/repo.git` → `ws/repo`; `git@bitbucket.org:ws/repo` → `ws/repo`
- `bitbucket.org:ws/repo.git` → `ws/repo`
- `https://bitbucket.org/ws/repo.git`, `https://user@bitbucket.org/ws/repo.git`,
  `https://user:secret@bitbucket.org/ws/repo.git` → `ws/repo`
- `ssh://git@bitbucket.org/ws/repo.git`, `ssh://git@bitbucket.org:22/ws/repo.git` → `ws/repo`
- `git@bitbucket-work:ws/repo.git`, `bitbucket-work:ws/repo.git`, `ssh://git@bitbucket-work/ws/repo.git`,
  `git@BitBucket-Work:ws/repo.git` → `ws/repo`
- `git@altssh.bitbucket.org:ws/repo.git` → `ws/repo`
- `ssh://git@bitbucket-work:22/ws/repo.git`, `http://bitbucket.org/ws/repo.git`, `git+ssh://git@bitbucket-work/ws/repo.git`,
  `git@bitbucket-work:ws/repo.git/` → `ws/repo`
Must-reject (named lookup errors with "argument remote is invalid"; unnamed lookup skips it):
- `git@github.com:me/bitbucket-cli.git`, `https://github.com/bitbucket/repo.git`,
  `https://github.com/me/bitbucket.org.git`, `git@gitlab.com:bitbucket/x.git`, `github.com:bitbucket/x`,
  `https://evil.com/@bitbucket/x.git` (upstream's substring check would accept the last one), local paths
  `../bitbucket-x/repo`, `/srv/bitbucket:repo`, `bitbucket-mirror/x:y`, empty URL.
- Unnamed lookup: a config whose first remote is github and second is a `bitbucket-work` alias returns the alias.

## What Goes Where
- Implementation Steps: code, tests, docs in this repo.
- Post-Completion: nothing external.

## Implementation Steps

### Task 1: Host-based Bitbucket remote detection

**Files:**
- Modify: `internal/remote/remote.go`
- Modify: `internal/remote/remote_test.go`

- [x] write table-driven tests for the must-accept and must-reject URL tables above, only through the exported
  `GetRemoteFromReader` (named and unnamed lookup) in the existing external `remote_test.go` — no internal test file
- [x] run them and record in this plan (a ⚠️/➕ note under the box) that the alias/bare-alias/false-positive cases FAIL on the
  unchanged code (failing-then-passing evidence)
  ➕ on unchanged code: every `bitbucket-work` alias row (incl. bare `bitbucket-work:ws/repo.git`, `git+ssh://`, `:22`,
  trailing `/`, `BitBucket-Work`) failed with `argument remote is invalid (value: origin)` (named) / `no remote found`
  (unnamed); false positive `https://github.com/me/bitbucket.org.git` was wrongly accepted; the github-then-alias
  unnamed case failed with `no remote found`. All pass after the fix.
- [x] implement `remoteHost` and `isBitbucketHost`; switch both checks in `GetRemoteFromReader`
- [x] update the `GetRemoteFromReader` doc comment (no longer "does not contain bitbucket.org")
- [x] run `go test -race ./internal/remote/...` — must pass

### Task 2: Repository/workspace name parsing for alias hosts

**Files:**
- Modify: `internal/remote/remote.go`
- Modify: `internal/remote/remote_test.go`

- [x] write tests asserting `RepositoryName()`/`WorkspaceName()` for every must-accept URL (incl. bare alias,
  ssh:// with port, trailing `.git`/no `.git`); run and record in this plan the bare-alias/`http://`/port failures first
  ➕ `TestRemoteNamesForBitbucketHosts` (plus an extra `https://bitbucket.org/ws/repo/` row). On unchanged code:
  `bitbucket-work:ws/repo.git` -> actual `"bitbucket-work:ws/repo.git"` / workspace `"bitbucket-work:ws"`;
  `http://bitbucket.org/ws/repo.git` -> `"http://bitbucket.org/ws/repo.git"` / `"http:"`;
  `git+ssh://git@bitbucket-work/ws/repo.git` -> unchanged URL / `"git+ssh:"`; `git@bitbucket-work:ws/repo.git/` ->
  `"ws/repo.git/"`; `https://bitbucket.org/ws/repo/` -> `"repo/"`.
  ⚠️ the `ssh://...:22` port rows already PASSED on unchanged code (the old last-two-path-segments logic is
  port-agnostic), so there was no port failure to record. All rows pass after the fix.
- [x] rewrite `RepositoryName` so scp-like forms are parsed host-agnostically and URL forms keep current output;
  every existing test still passes unchanged
- [x] update `internal/repository/repository.go:235` comment wording to match the new detection rule
- [x] run `go test -race ./...` — must pass

### Task 3: Verify acceptance criteria
- [x] verify every row of the Technical Details tables is covered by a test
  ➕ all 17 must-accept rows are in both `TestGetRemoteFromReaderAcceptsBitbucketHosts` (named + unnamed) and
  `TestRemoteNamesForBitbucketHosts` (RepositoryName/WorkspaceName); all 10 must-reject rows (incl. empty URL) are in
  `TestGetRemoteFromReaderRejectsNonBitbucketHosts` (named -> "argument remote is invalid", unnamed -> "no remote
  found"); the github-then-alias unnamed case is `TestGetRemoteFromReaderUnnamedSkipsNonBitbucketRemote`. No row
  missing, no test added.
- [x] run `go build ./...`, `go vet ./...`, `gofmt -l .` (empty), `go test -race ./...`, `golangci-lint run`
  (v2.12.2), `make cross-build`
  ➕ all clean: build/vet OK, gofmt empty, every package `ok`, golangci-lint 2.12.2 `0 issues.`, cross-build exit 0.
- [x] manual smoke on the built binary in a temp git repo with `origin = git@bitbucket-work:ws/repo.git`:
  `./bb pullrequest list --dry-run --debug` (or another read-only command with `--dry-run`) shows `ws/repo` resolved
  from the remote; record the output in this plan
  ➕ ran from a scratch `git init` repo with `HOME`/`XDG_CONFIG_HOME` pointed at an empty scratch dir (no profile, real
  config/keychain untouched): `env -u BB_CONFIG HOME=$S/home bb pullrequest list --dry-run --debug`.
  ⚠️ no command can reach its `Dry run:` line offline: every repository-scoped command resolves through
  `repository.GetRepository`, which GETs the repository (needs a profile) before the `WhatIf` gate. The `--debug`
  resolution line is the evidence instead. Fixed binary (9d067b5):
  ```
  [DEBUG] {repository/repository.go:257 repository.GetRepositoryName} repository name found in git config: ws/repo, from remote: git@bitbucket-work:ws/repo.git
  [DEBUG] {repository/repository.go:291 repository.GetRepositoryBySlugOrID} repository slug ws/repo contains a workspace, extracting workspace and repository name
  [DEBUG] {repository/repository.go:328 repository.GetRepositoryBySlugOrID} getting repository repo in workspace ws
  Error: cannot get repository: cannot get profile: no profiles configured
  ```
  Unfixed binary (built from `git archive 4200bfa`), same repo/command -- the remote is not detected:
  ```
  [WARN]  {profile/profile.go:230 profile.CurrentOrFromCommand} cannot load profile to resolve a default repository: no profiles configured
  Error: cannot get repository: argument repository is missing: pass --repository, run from a Bitbucket git checkout, or set a default repository with `bb profile update --default-repository`
  ```

### Task 4: [Final] Update documentation
- [x] add a short note at `skill/bitbucket-cli/SKILL.md:119` and `:377` ("Bitbucket git remote") that a remote
  through an ssh Host alias starting with `bitbucket` (e.g. `git@bitbucket-work:ws/repo.git`) is detected too;
  grep `CLAUDE.md` for remote-detection wording and fix only what is now wrong; sync tests stay green
- [x] move this plan to `docs/plans/completed/` (`git add -f` — docs/plans/* is gitignored)

## Post-Completion
- none (no external systems).
