---
name: bitbucket-cli
description: Drive Bitbucket Cloud from the terminal via the bb CLI. Use when the user wants to open/create a pull request, review a pull request, list my pull requests, comment on a PR, approve a PR, request changes on a PR, unapprove a PR, merge a PR, decline a PR, check pull request activities/participants, list or read repositories, check the pipeline, why did the build fail, view pipeline step logs, trigger a pipeline, stop/cancel a running pipeline, download an artifact, list artifacts, clone a repository, list branches or commits, diff or patch a commit or PR, look up a Bitbucket user, list workspace members, or log in / set up a Bitbucket profile.
---

# bitbucket-cli

Drive Bitbucket Cloud from the terminal with `bb`. Requires `bb` installed (`brew install
--cask bb` or `make install`) and a profile (`bb profile create` / `bb profile authorize`)
before anything else works.

`bb` resolves the workspace and repository itself from the current git checkout or the
profile's defaults; you rarely need to pass either explicitly. Look an id up with the
resource's own `list` command before acting on it if you don't already have it (e.g. `bb
pipeline list` before `bb pipeline get <id>`) — never guess one.

## Profile / authentication

`bb profile create -n <name>` accepts exactly one credential shape at a time:

- API token (current Atlassian standard; app passwords are deprecated): `--user
  <email> --password <token>`, or `--password-stdin` to pipe it in instead of putting it on
  the command line (`op read op://vault/x/token | bb profile create -n work -u
  me@corp.com --password-stdin`).
- Access token (Repository/Project/Workspace Access Token): `--access-token <token>`, or
  `--access-token-stdin`.
- OAuth2 client (Authorization Code Grant or Client Credentials): `--client-id
  <id> --client-secret <secret>` (or `--client-secret-stdin`), then, for the Authorization
  Code Grant only, `bb profile authorize <name>` to complete the browser-based flow.

`--password`/`--access-token`/`--client-secret` each has a `-stdin` twin; the three `-stdin`
flags are mutually exclusive with each other and with their own non-stdin flag. Prefer the
`-stdin` form whenever a real secret value is available to you — it never lands in shell
history. Credentials are stored in the macOS Keychain by default; `--no-vault` stores them
in plaintext in the config file instead (only for testing, never recommend it otherwise).

Set `--default-workspace`/`--default-repository` on `create`/`update` so commands don't need
`--workspace`/`--repository` every time. `bb profile list`/`bb profile get` mask stored
secrets in table/csv/tsv output unconditionally; the value is shown in full only with an
EXPLICIT `-o json` or `-o yaml` on that command line.

## Workspaces, repositories, branches, commits

- `bb workspace list` / `bb workspace get [<slug>]` / `bb workspace members [<slug>]` — omit
  the slug to use the resolved current workspace.
- `bb repo list [--role member|contributor|admin|owner|all]` / `bb repo get
  [<slug-or-uuid>]` / `bb repo clone <slug-or-uuid> [destination]` (aliases `repository`).
  `repo get/list/clone` never take `--repository` — the repository is always their own
  positional (or, for `list`, every repository of the resolved workspace).
- `bb branch list` — branches of the current repository, no filter.
- `bb commit list [--query ...] [--include ref] [--exclude ref]`, `bb commit get
  [<hash>]` (no hash: the newest commit Bitbucket's API returns, NOT local git HEAD), `bb
  commit diff <ref> [<ref>]` (one ref: diff against its parent; `--stat` prints diffstat JSON
  instead), `bb commit patch <ref> <ref>` (patch needs BOTH refs — unlike `diff`, it has no
  single-ref/parent form). `diff`/`patch` accept a hash or a branch/tag ref (e.g.
  `release/1.0`); `get` only accepts a hash, not a ref. `diff`/`patch`/`--stat` always print
  raw text/JSON, ignoring `--output`.

## Pull requests

`bb pullrequest` (aliases `pr`, `pull-request`).

- `bb pullrequest list [--state open|merged|declined|superseded|all]* [--source <branch>]
  [--destination <branch>] [--query '<bitbucket query>'] [--commit <full-hash>]` — `--state`
  is repeatable and defaults to `open` alone when omitted; `all` fetches every state. `--commit`
  is mutually exclusive with `--state`/`--query`/`--source`/`--destination`.
- `bb pullrequest get <id>` (aliases `show`, `info`, `display`) — participants (per-reviewer approval
  state) are NOT in the default columns on `get` OR `list`. Always add `--columns
  participants` (or `-o json`/`-o yaml`, which include the full participant objects
  regardless of `--columns`) when the user's request is about who has/hasn't approved.
- `bb pullrequest create --title <t> --source <branch> [--destination <branch>] [--reviewer
  <user>]... [--description <text> | --description-file <path-or-->] [--draft]
  [--close-source-branch]` — `--title` and `--source` are required; `--destination` is
  optional (Bitbucket defaults it to the repository's main branch server-side when omitted).
  At most one of `--description`/`--description-file`. A reviewer value of `default` (as the
  first `--reviewer`) pulls the repository/project's default reviewers.
- `bb pullrequest update <id> [--title ...] [--description ... | --description-file ...]
  [--add-reviewer <user>]... [--remove-reviewer <user>]...`
- `bb pullrequest approve <id>` / `bb pullrequest unapprove <id>` / `bb pullrequest request-changes <id>` / `bb pullrequest
  remove-request-changes <id>` / `bb pullrequest decline <id>` — the pull request id is optional on
  each: omitted, `bb` tries the one open PR whose source is the current git branch.
- `bb pullrequest merge <id> [--message <text>] [--close-source-branch] [--merge-strategy
  merge_commit|squash|fast_forward] [--async]`. `--async` returns a task id; poll it with `bb
  pullrequest merge-status <id> --task-id <task-id>`.
- `bb pullrequest diff <id> [--stat]`, `bb pullrequest patch <id>`, `bb pullrequest commits <id>`, `bb pullrequest activities
  <id>` — id optional on all four (same current-branch fallback as approve/decline above).
  An activity kind newer than this build of `bb` recognizes is dropped from the list and
  warned about once on stderr, not fatal.

### Comments — `bb pullrequest comment <verb> <pullrequest-id> ...`

The pull request id is always the first, REQUIRED positional; there is no `--pullrequest`
flag anywhere in this subtree.

- `bb pullrequest comment list <pr-id>`
- `bb pullrequest comment get <pr-id> <comment-id>`
- `bb pullrequest comment create <pr-id> (--comment <text> | --comment-file <path-or-->) [--file
  <path-in-diff> --line <n>] [--parent <comment-id>]` (aliases `add`, `new`). `--comment` and
  `--comment-file` are mutually exclusive and exactly one is required.
  **Shell-quoting hazard: a markdown comment body with backticks or `$(...)` is a live
  command-substitution risk on the command line. ALWAYS write it to a file (or heredoc into
  `--comment-file -`) instead of passing it inline with `--comment` whenever the body
  contains backticks, `$(`, or is more than a short one-liner:**
  ```
  bb pullrequest comment create <pr-id> --comment-file - <<'EOF'
  Looks good, but run `go test ./...` and check $(git diff) first.
  EOF
  ```
  `--file` must name a path the pull request's own diff actually touches (validated against
  the diffstat even under `--dry-run`), not an arbitrary local file.
- `bb pullrequest comment update <pr-id> <comment-id> (--comment <text> | --comment-file
  <path-or-->)` (aliases `edit`)
- `bb pullrequest comment delete <pr-id> <comment-id>...` (aliases `remove`, `rm`)
- `bb pullrequest comment resolve <pr-id> <comment-id>`
- `bb pullrequest comment reopen <pr-id> <comment-id>`

### Tasks — `bb pullrequest task <verb> <pullrequest-id> ...`

Same shape as comments: pull request id is the required first positional, no `--pullrequest`.

- `bb pullrequest task list <pr-id>`
- `bb pullrequest task get <pr-id> <task-id>`
- `bb pullrequest task create <pr-id> --content <text> [--comment <comment-id>]` (aliases `add`, `new`)
- `bb pullrequest task update <pr-id> <task-id> [--content <text>] [--state RESOLVED|UNRESOLVED]`
  (aliases `edit`) — set `--state RESOLVED` to complete a task, `UNRESOLVED` to reopen it.
- `bb pullrequest task delete <pr-id> <task-id>...` (aliases `remove`, `rm`)

## Pipelines

`bb pipeline` (aliases `pipelines`, `pipe`, `pp`).

- `bb pipeline list [--query '<bitbucket query>']`
- `bb pipeline get <pipeline-uuid-or-build-number>` (aliases `show`, `info`, `display`)
- `bb pipeline trigger [--branch <branch>] [--commit <hash>] [--pattern <name>] |
  [--pullrequest <pr-id>] [--variable KEY=VALUE]...` — defaults to the current git branch
  when no target flag is given; `--pullrequest` is mutually exclusive with
  `--branch`/`--commit`/`--pattern`. `--variable` may repeat; variable VALUES are never
  logged, only their keys.
- `bb pipeline stop <pipeline-uuid-or-build-number>` (aliases `cancel`, `abort`).

**MANDATORY: always pass `--force` to `pipeline trigger` and `pipeline stop` when running as
an agent.** Both ask an interactive `y/N` confirmation before doing anything; with no TTY to
answer it (which is every agent invocation) the command fails immediately with:

```
input is not a terminal, use --force to skip confirmation
```

It does NOT hang waiting for input and does NOT proceed on its own — `--force` is the only
way to get past this. `--dry-run` also skips the prompt (and performs no write), which is
the right choice when the goal is only to preview the request, not send it.

### Pipeline steps — `bb pipeline step <verb> <pipeline> [<step>]`

Every subcommand's pipeline is a REQUIRED positional first argument — unlike `commit get`,
there is no "latest pipeline" fallback. The step positional accepts either a UUID or a step
NAME (case-insensitive, trimmed); `bb` resolves a name to its UUID itself. An unknown name
errors listing the available names; a name matching more than one step (Bitbucket allows
duplicates) errors listing the ambiguous UUIDs and asks for a UUID instead.

- `bb pipeline step list <pipeline>`
- `bb pipeline step get <pipeline> <step-uuid-or-name>` (aliases `show`, `info`, `display`)
- `bb pipeline step logs <pipeline> <step-uuid-or-name>` (alias `log`) — the actual build
  console output; this is almost always the right command for "why did the build fail".
- `bb pipeline step report <pipeline> <step-uuid-or-name>` — the test report (JSON).
- `bb pipeline step cases <pipeline> <step-uuid-or-name>` — individual test case results.

`logs`/`report`/`cases` print raw text/JSON straight through, ignoring `--output`/`--columns`.

## Artifacts

`bb artifact` operates on a repository's Downloads tab, NOT on pipeline build artifacts —
there is no `bb` command for the latter (use `pipeline step logs`/`report` instead for
build-time output).

- `bb artifact list [--query ...]`
- `bb artifact download <name>... [--destination <dir>]` (aliases `get`, `fetch`) —
  `--destination` defaults to the current directory and must already exist; multiple names
  can be given on one line.

## Users

- `bb user me` (alias `self`) — the authenticated user.
- `bb user get <user-id>` (aliases `show`, `info`, `display`) — the user id (an Account
  ID/UUID) is REQUIRED.

## Output formats, dry-run, and defaults

- **Output**: default is a table. `-o json`/`-o yaml`/`-o csv`/`-o tsv` (or `BB_OUTPUT_FORMAT`)
  give full, untruncated data for scripting; table output truncates any cell over 80
  characters (display-only — never affects json/yaml/csv/tsv). `--columns <a>,<b>` (or
  `--columns all`) picks which columns render; repeatable or comma-separated.
- **`--dry-run`** (also `--noop`/`--whatif`) runs the FULL preflight of a real invocation —
  every resolution GET, id/target validation — and prints the resolved API path and payload
  to stderr, but skips only the final write. A `--dry-run` against a nonexistent id fails
  exactly like the real command would; it never fabricates a success line.
- **Workspace/repository resolution** for every command follows the same order: the
  `--workspace`/`--repository` flag, then a Bitbucket git remote in the current directory,
  then the profile's `--default-workspace`/`--default-repository`. `--repository` (or
  `--default-repository`) may itself be `workspace/repository`, which overrides whatever
  `--workspace` would otherwise resolve to. If all three rungs are empty the error names all
  three ways to supply the value.
- **Narrowly-scoped tokens work fine.** `bb` does not require `read:workspace` for ordinary
  repository/pull-request/pipeline workflows — a token scoped to only the specific
  permissions a given command needs (e.g. `read:repository`, `read:pullrequest`,
  `write:pullrequest`, `read:pipeline`) is sufficient. Don't ask a user to grant broader
  scopes than the commands you're about to run actually need.
- **Multi-argument commands** (`comment delete`, `task delete`, `artifact download`, ...)
  accept several ids/names on one line and honor `--stop-on-error`/`--warn-on-error`/
  `--ignore-errors` for how failures among them are handled.
