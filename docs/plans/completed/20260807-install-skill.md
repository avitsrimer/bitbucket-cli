# bb install-skill — embedded Claude skill (cycle B, last work before first release)

## Overview

Ship an embedded Claude skill inside the `bb` binary, installable via `bb install-skill`,
matching `jcli install-skill` and grafanapi. The skill teaches an agent to drive bb for
the workflows this fork actually supports, accurate to the POST-cycle-A command surface.
Spec: `docs/plans/field-report-findings.md`, final section ("PHASE 3 … `bb
install-skill`") — every checkbox there is binding. The SKILL.md CONTENT is the substance
of this cycle; the plumbing is mechanical.

## Context (from discovery)

- Reference implementation `~/Code/my/go/jcli` (read these before coding):
  - `skill/embed.go` — `package skill`, `//go:embed jenkins-cli`, `Files embed.FS`, doc
    comment "embeds the skill tree so the installed binary can write it from any
    directory, independent of the source checkout". Also `skill/embed_test.go`.
  - `skill/jenkins-cli/SKILL.md` — YAML frontmatter with `name:` and a long trigger-rich
    `description:`; body is a compact command reference with a "Core flow" list, exit
    codes, and per-command gotchas written FOR AN AGENT (imperatives, exact flags,
    confirmation-prompt warnings).
  - `internal/cli/cmd_install_skill.go` — `runInstallSkill()`: `--to` default
    `os.UserHomeDir()/.claude`, dest `<to>/skills/<name>`, `os.RemoveAll` for idempotent
    re-install, `fs.WalkDir` writing dirs 0o750 / files 0o600, every error wrapped with
    context, prints `installed <name> skill to <dest>`.
  - `internal/cli/cmd_install_skill_test.go` — the test shape to mirror.
- bb registers commands in `internal/cmd/root.go` `init()` via
  `RootCmd.AddCommand(...)` (root.go:84-92). jcli uses go-flags; bb is cobra — register
  the cobra way: a `Command` in a new file, `--to` string flag with
  `MarkFlagDirname("to")` completion.
- The post-cycle-A surface the skill must document (master c281286+, PRs #37..#57):
  positional PR ids on the comment/task trees (no `--pullrequest` THERE; note that
  `bb pipeline trigger --pullrequest <id>` still exists as a flag and trigger takes NO
  positionals — document that form correctly),
  `--comment-file`/`--description-file` with `-` = stdin, full-preflight `--dry-run`
  (resolution GETs run, write skipped, resolved payload + target path echoed to stderr),
  workspace/repository resolution `flag > bitbucket git remote > profile default
  (--default-workspace/--default-repository)`, `pr get` participants (column
  `participants`, full objects in json/yaml), repeatable `pr list --state` +
  `--source`/`--destination` filters, `pipeline step` fully positional
  (`step list <pipeline>`; `step get|logs|report|cases <pipeline> <step-uuid-or-name>`,
  name→UUID resolution), `pipeline trigger/stop` y/N confirmation + `--force`, profile
  secrets masked in table output / shown in explicit `-o json|yaml` only,
  `commit diff/patch` accept slashed refs while `commit get` is hash/single-segment only.
  VERIFY EVERY STATEMENT AGAINST THE SOURCE — the `Use:` string, `Args:` validator, AND
  the flag registrations of each command; `--help` alone is NOT authoritative (two `Use`
  strings are known-stale: `pr activities` omits its optional `<pr-id>` positional
  (activities.go, MaximumNArgs(1)) and `user get` omits its required positional
  (user/get.go, ExactArgs(1)) — Task 1 fixes both). Watch arg-count asymmetries such as
  `commit patch` (ExactArgs(2)) vs `commit diff` (1-2 args).

## Binding rule sets (read before ANY task)

1. `docs/plans/field-report-findings.md` PHASE 3 section — the spec; every `- [ ]` line
   there is a requirement.
2. `CLAUDE.md` — conventions: lowercase current-behavior-only comments, no AI mentions in
   commits/PRs/code comments (the SKILL.md itself is Claude-facing content and may of
   course say "Claude"), stdlib errors with `%w` wrapping, table-driven tests, cobra
   registration, RunE reads flags off `cmd`.
3. Workflow: same non-negotiables as cycle A (worktree per task, explicit-path git add,
   commit -F tempfile, full gate, PR → CI watch → squash-merge → sync master).

## Development Approach

- **testing approach**: Regular (code first, then tests); the install command's tests
  mirror jcli's `cmd_install_skill_test.go` shape. Pre-fix-failure proof is N/A for a
  brand-new command (there is no pre-fix behavior) — instead, prove test value by
  CONCRETE mutation checks: temporarily flip `0o600`→`0o644` (permission test must
  fail), drop the `os.RemoveAll` (idempotency/stale-file test must fail), and hardcode
  the default dest ignoring `--to` (the `--to` test must fail); record the three
  outcomes in the progress log, then restore.
- one task = one PR against master. Full PR flow per task (see cycle A Workflow; CI is
  operational — `gh pr checks --watch` in the foreground, merge after green).
- **CRITICAL: every task MUST include new/updated tests.**
- **CRITICAL: all tests must pass before starting next task.**
- **CRITICAL: update this plan file when scope changes during implementation.**

## Testing Strategy

- unit tests for the embed package (SKILL.md present, frontmatter parses, expected
  `name:`) and the install command (fresh install, overwrite, `--to`, default `~/.claude`
  via temp HOME, permission bits, every embedded file lands byte-identical).
- acceptance: the BUILT binary run from OUTSIDE the source checkout installs a tree
  identical to `skill/bitbucket-cli/` (diff -r).
- skill-content accuracy: every command/flag named in SKILL.md is verified against the
  SOURCE (`Use` string + `Args` validator + flag registrations); the built binary's
  `--help` is a secondary cross-check, not the authority.

## Progress Tracking

- mark completed items with `[x]` immediately when done; ➕ for discovered tasks, ⚠️ for
  blockers.
- append per-task entries to `docs/plans/progress-install-skill.txt`.

## Solution Overview

Copy jcli's proven pattern, adapted to cobra. The skill tree lives at the repo root
(`skill/bitbucket-cli/`) so the embed package is importable by `internal/cmd`; content is
one SKILL.md (no extra files needed — keep parity with jcli). Two implementation PRs:
the feature (embed + command + skill content + docs), then acceptance + plan completion.
Then the review loop.

## Implementation Steps

### Task 1: skill content + embed package + `bb install-skill` command (one PR)

**Files:**
- Create: `skill/embed.go`, `skill/embed_test.go`
- Create: `skill/bitbucket-cli/SKILL.md`
- Create: `internal/cmd/install_skill.go`, `internal/cmd/install_skill_test.go`
- Modify: `internal/cmd/root.go` (AddCommand)
- Modify: `README.md` ("Agent skill" section), `CLAUDE.md` (Layout: `skill/` tree +
  embed shim; new rule: skill content must be updated whenever the command surface
  changes — it is documentation shipped inside the binary and goes stale silently)

- [x] `skill/embed.go`: `package skill`, `//go:embed bitbucket-cli`, exported
      `Files embed.FS`, jcli-style doc comment
- [x] WRITE THE SKILL CONTENT (the substance): `skill/bitbucket-cli/SKILL.md` with
      frontmatter `name: bitbucket-cli` and a trigger-rich `description:` enumerating
      real phrasings ("open a PR", "review a pull request", "comment on PR", "approve /
      request changes on a PR", "merge PR", "check the pipeline", "why did the build
      fail", "download an artifact", "list my pull requests", …). Body covers, at
      minimum: profile login (all three credential shapes — app password, API/access
      token, OAuth client — incl. the `-stdin` variants and the macOS Keychain vault),
      listing/reading PRs (incl. `--state`/`--source`/`--destination`), creating a PR,
      commenting with a markdown body from a file (`--comment-file`, `-` = stdin — call
      out the shell-quoting hazard explicitly), approving / requesting changes /
      merging, reading `pr activities`, `pr get` participants (approval state per
      reviewer), pipelines (list/get/step logs with the positional
      `<pipeline> <step-uuid-or-name>` forms/trigger with y/N confirmation + `--force`),
      repo + workspace + commit + branch reads, artifact list/download, full-preflight
      `--dry-run` semantics, an "output formats" note (`-o json` for scripting; table
      truncation is display-only; profile secrets only in explicit `-o json|yaml`), a
      "defaults" note (workspace/repository precedence and `--default-workspace`/
      `--default-repository`), and a "narrowly-scoped tokens" note (workspace-scoped
      tokens work; never demand `read:workspace`). MANDATORY imperatives the skill must
      state: (a) ALWAYS pass `--force` on `pipeline trigger`/`pipeline stop` — without a
      TTY the y/N prompt fails with "input is not a terminal, use --force to skip
      confirmation" (it does not hang or auto-proceed) — quote that exact error so the
      agent recognizes it; (b) participant approval state is NOT in default columns —
      use `bb pr get <id> --columns participants` or `-o json`
- [x] COMPLETENESS CHECKLIST: the skill must cover every command in README's
      supported-surface bullet list (README.md ~:29-42) — including pr decline/update/
      merge-status/diff/patch/commits/unapprove/remove-request-changes, comment
      resolve/reopen/delete, the full task tree, workspace members, user me, repo clone,
      pipeline stop, and profile list/use — a one-line mention with the exact invocation
      shape is enough for rarely-used verbs
- [x] fix the two stale `Use` strings found during planning review: `pr activities`
      gains its optional `[<pullrequest-id>]` positional and `user get` its required
      `<user-id>` (same class as FR-13's doc-truthfulness rule); adjust any tests
      asserting the old strings
- [x] verify every command and flag named in SKILL.md against the SOURCE (`Use` +
      `Args` + flag registrations); then build `./bb` and use its help output as a
      secondary cross-check; fix drift in the skill text, not from memory
- [x] `internal/cmd/install_skill.go`: cobra `installSkillCmd` with `--to` (string,
      default resolved at run time to `os.UserHomeDir()/.claude`; described as "path to
      a .claude folder (default ~/.claude)"; `MarkFlagDirname`), RunE reads the flag off
      `cmd`; dest `<to>/skills/bitbucket-cli`; a `common.WhatIf(cmd, "Installing the
      bitbucket-cli skill to %s", dest)` gate honors the root `--dry-run` flag (local
      disk writes ARE writes) — with a subtest asserting dry-run writes nothing;
      `os.RemoveAll` then `fs.WalkDir` writing dirs 0o750 / files 0o600; every error
      wrapped with context; prints `installed bitbucket-cli skill to <dest>` to
      `cmd.OutOrStdout()`; registered in root.go `init()`
- [x] tests mirroring jcli's, in `internal/cmd/install_skill_test.go` declared
      `package cmd` (INTERNAL — the three existing files there are `package cmd_test`,
      but an external test could only drive the process-global `RootCmd`, firing
      `cobra.OnInitialize` → real config loads; instead build a STANDALONE
      `*cobra.Command` carrying its own `--to`/`--dry-run` flags and invoke the RunE
      directly, which is exactly why RunE reads flags off `cmd`): fresh install;
      overwrite of an existing dir with stale extra files (idempotency — stale file is
      gone after re-install); `--to` honored; default `~/.claude` resolution driven with
      a temp HOME (`t.Setenv("HOME", ...)`); permission bits asserted with
      `Mode().Perm()` EXACTLY 0o750/0o600 (CI's macos-latest umask is 022; if it ever
      flakes, mask the expectation, don't loosen the mode); every embedded file lands
      byte-identical to `skill.Files`; ERROR PATH: `--to` pointing at a regular file so
      `MkdirAll` fails ENOTDIR — assert the wrapped error names the offending path
      (jcli has the same test)
- [x] sync-guard test in `internal/cmd`: extract every `bb <words>` command path from
      the embedded SKILL.md — take only the LEADING command words, stopping at the
      first token beginning with `-`, `<` or `[` — and assert `RootCmd.Find(...)`
      returns ZERO leftover args AND the found command's `CommandPath()` equals the
      extracted path. `err == nil` alone is vacuous: cobra's Find does not error on an
      unknown LEAF verb (it returns the deepest matched parent + leftovers), which is
      exactly the staleness this test exists to catch
- [x] `skill/embed_test.go`: embedded SKILL.md exists, YAML frontmatter parses, `name:`
      == `bitbucket-cli`, `description:` non-empty and contains a few expected trigger
      phrases
- [x] README "Agent skill" section (what the skill enables, `bb install-skill [--to]`);
      CLAUDE.md Layout + stale-skill rule. ALSO update the other surface enumerations
      that now change: README's IMPORTANT scope box (~:13-22) and supported-surface
      bullets (~:29-42), CLAUDE.md's "What this is" paragraph, and CONTRIBUTING.md's
      "new top-level command groups are out of scope" line
- [x] full gate + PR flow

### Task 2: acceptance + plan completion (one PR)

**Files:**
- Modify: none expected (verification); this plan moves to `docs/plans/completed/`

- [x] `make build`, copy `bb` outside the checkout (e.g. the scratchpad), run
      `bb install-skill --to <tmpdir>` from that outside directory, then
      `diff -r skill/bitbucket-cli <tmpdir>/skills/bitbucket-cli` — identical
- [x] `goreleaser check` still passes; module count unchanged (`go list -m all | wc -l`
      == 38; the yaml frontmatter test must not add a dependency — `gopkg.in/yaml.v3` is
      already in the module graph)
- [x] `bb install-skill --help` matches README wording
- [x] every PHASE 3 spec checkbox in `docs/plans/field-report-findings.md` maps to
      landed work — record the mapping in the progress log
- [x] tick this plan's boxes, move it to `docs/plans/completed/` (`git add -f`), commit
      with this PR
- [x] full gate + PR flow

### Task 3: Review pass over the cycle-B diff

Scoped review (this is a two-PR cycle, not cycle A's 20-PR diff): ONE review pass
covering (a) skill-content accuracy against the source (Use/Args/flags — accuracy is a
correctness property here) and (b) the install command's Go code. Fixer + critical
re-check only if findings warrant; loop until the re-check is clean.

- [ ] review pass (skill-content accuracy + Go correctness/tests); findings logged
- [ ] fixer PR(s) for confirmed findings if any, full gate each; re-check until clean
- [ ] log the outcome

## Post-Completion

*Owner-only; do NOT do these:*

- add `HOMEBREW_TAP_TOKEN` secret (fine-grained PAT, write access to
  `avitsrimer/homebrew-apps`), then cut the release via `/release-tools:new`
  (suggest v0.19.0). Verify with `brew tap avitsrimer/apps && brew install --cask bb &&
  bb --version`, then `bb install-skill`.
