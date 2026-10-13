# Ship the bb skill as a Claude Code plugin marketplace

## Overview
- Make this repo a Claude Code plugin marketplace with one plugin, `bitbucket-cli`, carrying the existing
  `bitbucket-cli` skill, so users can run `/plugin marketplace add avitsrimer/bitbucket-cli` and
  `/plugin install bitbucket-cli@avitsrimer-bitbucket-cli` and get updates through Claude Code itself.
- `bb install-skill` stays as the offline/no-plugin route, but embeds the skill FROM the plugin directory: the
  plugin's `skills/bitbucket-cli/` is the one source of truth for both routes, so they can never drift.
- One PR against `master`: `feat: publish the bitbucket-cli skill as a claude code plugin marketplace`.

## Context (from discovery)
- `skill/embed.go` (package `skill`, `//go:embed bitbucket-cli`, `Files embed.FS`) + `skill/bitbucket-cli/SKILL.md`.
- Consumers of `skill.Files`: `internal/cmd/install_skill.go` (`fs.WalkDir(skill.Files, skillName, ...)`,
  `skill.Files.ReadFile`), `internal/cmd/skill_sync_test.go:86`, `skill/embed_test.go`.
- Docs naming the `skill/` path: CLAUDE.md:19 and its Layout block (:120); README "Agent skill" (~:1379).
- Stale install line: SKILL.md:8 and README:62-63 say `brew install --cask bb`, but `.goreleaser.yml:28-32`
  publishes the cask as `bitbucket-cli` since 4200bfa (homebrew/cask's own `bb` cask wins over the tap).
- Plugin format (code.claude.com/docs/en/plugins/{marketplace-reference,manifest-reference,loading,cli-reference}):
  marketplace at `<repo>/.claude-plugin/marketplace.json` (required: `name`, `owner.name`, `plugins[]` with
  `name` + `source`); relative `source` resolves from the repo root, must start with `./`, no `..`; plugin
  manifest at `<plugin>/.claude-plugin/plugin.json` (only `name` required); skills auto-discovered from
  `<plugin>/skills/<name>/SKILL.md`; only the plugin directory is copied into the user's plugin cache;
  with no `version` anywhere, the version is the commit SHA, so every merge to master is an update;
  `claude plugin validate <dir> [--strict]` validates marketplace + plugin (v2.1.289+; local is 2.1.296).
- Probe: a root-package file `//go:embed plugins/bitbucket-cli/skills/bitbucket-cli` builds (`go vet .` OK).

## Decisions (user, 2026-10-13)
- Versioning: no `version` field in either manifest — track commits.
- Embed host: a repo-root Go package, so the plugin directory holds plugin files only (no `.go` in the cache).
- Marketplace name `avitsrimer-bitbucket-cli` (install id `bitbucket-cli@avitsrimer-bitbucket-cli`); plugin and skill
  name `bitbucket-cli`. A user registers one marketplace per name (marketplace-reference: "Each user registers one
  marketplace per `name`"), so the bare `avitsrimer` stays free for a future cross-repo catalog repo that lists this
  plugin via a `git-subdir` source (`url: avitsrimer/bitbucket-cli`, `path: plugins/bitbucket-cli`); the plugin
  directory is reusable as-is from there. Upstream (gildas) ships no marketplace today.

## Development Approach
- **testing approach**: TDD — write the manifest/embed tests first, see them fail, then move files.
- complete each task fully before the next; every task includes tests; all tests pass before the next task.
- behaviour of `bb install-skill` is byte-identical: same destination, same files, same content.

## Testing Strategy
- unit tests only; table-driven where there are several cases.

## Progress Tracking
- mark items `[x]` immediately; ➕ for new tasks; ⚠️ for blockers.

## Solution Overview
```
.claude-plugin/marketplace.json                     # marketplace "avitsrimer-bitbucket-cli", one plugin, source ./plugins/bitbucket-cli
plugins/bitbucket-cli/.claude-plugin/plugin.json    # name, description, author, homepage, repository, license (no version)
plugins/bitbucket-cli/skills/bitbucket-cli/SKILL.md # git mv from skill/bitbucket-cli/SKILL.md
embed.go                                            # package bitbucketcli: //go:embed plugins/bitbucket-cli/skills/bitbucket-cli
embed_test.go                                       # moved from skill/embed_test.go + manifest tests
```
- `embed.go` exposes `SkillFiles fs.FS` = `fs.Sub(raw, "plugins/bitbucket-cli/skills")`, so paths stay
  `bitbucket-cli/SKILL.md` and `install_skill.go` only swaps `skill.Files` for `bitbucketcli.SkillFiles`
  (`skill.Files.ReadFile` → `fs.ReadFile`). `fs.Sub` on a literal, known-good prefix cannot fail at runtime,
  but its error is still checked (panic in a package-level init with a clear message), never ignored.
- The `skill/` package is deleted; the "leading . or _ is skipped by go:embed" doc note moves to `embed.go`.
- `make plugin-validate` runs `claude plugin validate .` locally (not `--strict`, see Task 2). CI does not run it (no `claude` on the
  runners); the Go manifest test covers the structural invariants there.

## Technical Details
- marketplace.json: `$schema` omitted unless the docs give a stable URL; `name: "avitsrimer-bitbucket-cli"`,
  `owner: {name: "Aleksandr Vits-Rimer", url: "https://github.com/avitsrimer"}`, `description`,
  `plugins: [{name: "bitbucket-cli", source: "./plugins/bitbucket-cli", description, category: "development"}]`.
- plugin.json: `name: "bitbucket-cli"`, `description` (says it needs the `bb` binary), `author`,
  `homepage`/`repository: "https://github.com/avitsrimer/bitbucket-cli"`, `license: "MIT"` (check LICENSE).
- Manifest test asserts: both files parse; marketplace `name` is `avitsrimer-bitbucket-cli`; exactly one plugin entry whose
  `name` equals plugin.json `name`; `source` starts with `./`, has no `..`, and points to a directory that
  contains `.claude-plugin/plugin.json`; neither manifest sets `version` (pins the track-commits decision);
  `<source>/skills/bitbucket-cli/SKILL.md` exists and its frontmatter `name` equals its directory name; the
  embedded `SkillFiles` tree equals the on-disk plugin skill tree file-for-file (guards a future embed-pattern
  change dropping a file).

## What Goes Where
- Implementation Steps: repo files, tests, docs.
- Post-Completion: a real `/plugin marketplace add` + install after merge (needs the pushed repo).

## Implementation Steps

### Task 1: Plugin layout and root embed

**Files:**
- Create: `.claude-plugin/marketplace.json`, `plugins/bitbucket-cli/.claude-plugin/plugin.json`, `embed.go`
- Move: `skill/bitbucket-cli/SKILL.md` → `plugins/bitbucket-cli/skills/bitbucket-cli/SKILL.md`
- Move+modify: `skill/embed_test.go` → `embed_test.go`
- Delete: `skill/embed.go`
- Modify: `internal/cmd/install_skill.go`, `internal/cmd/install_skill_test.go` (if it imports `skill`),
  `internal/cmd/skill_sync_test.go`

- [x] write `embed_test.go` (package `bitbucketcli_test`): existing frontmatter tests against `SkillFiles` plus the
  manifest test from Technical Details; run it and record that it fails (no package / no manifests)
- [x] create both manifests, `git mv` the skill, add `embed.go`, delete `skill/`
- [x] switch `install_skill.go` and the tests to `bitbucketcli.SkillFiles`; keep the install output identical
- [x] run `go test -race ./...` — must pass, including `install_skill_test.go` and the SKILL.md sync test
  - before: `go test .` → `no non-test Go files ... [build failed]`; after: all packages ok

### Task 2: Validate the plugin with Claude Code's own validator

**Files:**
- Modify: `Makefile`

- [x] add `plugin-validate` target: `claude plugin validate .`
  - ⚠️ not `--strict`: strict fails on the one expected warning, `plugin.json → version: No version specified`,
    which is the track-commits decision itself. Errors still fail the target.
- [x] run it; record the output here
  - `✔ Validation passed with warnings` (only the expected no-version warning). The validator also advised a
    one-step README install line, `/plugin install bitbucket-cli --marketplace avitsrimer/bitbucket-cli` — added.
- [x] smoke: `bb install-skill --to $(mktemp -d)` from a fresh `make build`; diff the written tree against
  `plugins/bitbucket-cli/skills/bitbucket-cli` — identical
  - ➕ local end-to-end: isolated `CLAUDE_CONFIG_DIR`, `claude plugin marketplace add <worktree>` +
    `claude plugin install bitbucket-cli@avitsrimer-bitbucket-cli` → installed, enabled, version = commit SHA
    (`a1255a8b23c3`), read from `plugins/bitbucket-cli`

### Task 3: Verify acceptance criteria
- [x] `go build ./...`, `go vet ./...`, `gofmt -l .` (empty), `go test -race ./...`, `golangci-lint run` (v2.12.2),
  `make cross-build`, all on a clean detached worktree of HEAD (a `git archive` export breaks
  `TestVersionStampedViaLdflags`)
- [x] `git grep -n 'skill/bitbucket-cli\|"github.com/avitsrimer/bitbucket-cli/skill"'` returns nothing

### Task 4: [Final] Documentation
- [x] README: new "Claude Code plugin" install subsection (marketplace add + install + update), "Agent skill"
  section names the plugin directory as the shipped source and keeps `install-skill` as the no-plugin route
- [x] fix the stale cask line in README and SKILL.md: `brew install --cask bitbucket-cli` (installed command stays
  `bb`)
- [x] CLAUDE.md: intro sentence and Layout block (`skill/` entry → `embed.go` + `plugins/bitbucket-cli/`,
  same go:embed/visibility reasoning, SKILL.md-sync rule now points at the plugin path; plugin manifests carry
  no version on purpose)
- [x] `install-skill` Long help: say the embedded tree is the plugin's skill
- [x] move this plan to `docs/plans/completed/` (`git add -f`)

## Post-Completion
- After merge: `claude plugin marketplace add avitsrimer/bitbucket-cli` then
  `claude plugin install bitbucket-cli@avitsrimer-bitbucket-cli` on a clean config dir; confirm the skill loads as
  `bitbucket-cli:bitbucket-cli`, and that a later master commit shows up through `claude plugin update`.

## Review

- Done as planned, plus: ➕ CONTRIBUTING.md and an install_skill_test.go comment still named `skill/`; updated.
  The tracked `docs/plans/20260810-inline-comment-anchor-fix.md` still says `skill/...`; left as a historical record.
- Lint initially reported 25 issues from a stale golangci-lint cache (paths into deleted worktrees);
  `golangci-lint cache clean` left one real finding in the new test (unwrapped error), fixed.
- planning:exec review phases run after implementation: phase 1 comprehensive (5 reviewers) → 16 findings, all
  fixed (aaf2c03), critical re-check clean; phase 2 smells → 7 minor, all fixed (ef3e1ad); phase 3 codex skipped
  (not installed); phase 4 critical clean. Visible change: install-skill errors now read
  `install skill: write skill tree: ...`.
