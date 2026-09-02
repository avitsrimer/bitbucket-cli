---
name: bitbucket-cli
description: Drive Bitbucket Cloud from the terminal via the bb CLI. Use when the user wants to open/create a pull request, review a pull request, list my pull requests, comment on a PR, approve a PR, request changes on a PR, unapprove a PR, hand a pull request merge to the user, decline a PR, check pull request activities/participants, list or read repositories, check the pipeline, why did the build fail, view pipeline step logs, trigger a pipeline, stop/cancel a running pipeline, download an artifact, list artifacts, clone a repository, list branches or commits, diff or patch a commit or PR, look up a Bitbucket user, list workspace members, or log in / set up a Bitbucket profile.
---

# bitbucket-cli

Drive Bitbucket Cloud from the terminal with `bb`. Requires `bb` installed (`brew tap
avitsrimer/apps && brew install --cask bb`, or `make install`) and a profile (`bb profile create` /
`bb profile authorize`) before anything else works.

`bb` resolves the workspace and repository itself from the current git checkout or the
profile's defaults; you rarely need to pass either explicitly. Look an id up with the
resource's own `list` command before acting on it if you don't already have it (e.g. `bb
pipeline list` before `bb pipeline get <id>`) — never guess one.

## CRITICAL: the omitted-<pullrequest-id> fallback is NOT branch-aware

`approve`, `unapprove`, `request-changes`, `remove-request-changes`, `decline`, `merge`,
`merge-status`, `diff`, `patch`, `commits`, and `activities` (i.e. `bb pullrequest <verb>
[<pullrequest-id>]` for each of these) all accept `<pullrequest-id>` as an OPTIONAL positional.
When it is omitted, `bb` fetches **every OPEN pull request of the current repository** (`GET
pullrequests?state=OPEN`, no relation to the current git branch whatsoever): if there is
exactly one, it silently acts on that one; if there is more than one, it errors `too many open
pullrequests, specify one: <id>, <id>, ...`; if there are none, it errors `no open pullrequest
found for repository <repo>`. There is no "the PR for my current branch" resolution anywhere in
this path.

**For decline/approve/request-changes/remove-request-changes/unapprove — every state-changing
pullrequest command you may run yourself — always pass the explicit `<pullrequest-id>` yourself
(look it up first with `bb pullrequest list` if you don't have it) instead of relying on this
fallback.** Relying on it risks silently declining/approving the wrong pull request the moment a
second open PR exists in the repository, and that action is not reversible. `merge` is not in
this list: never invoke it yourself at all — see the MANDATORY note under Pipelines below.

## Profile / authentication

`bb profile create -n <name>` accepts one credential shape at a time — `--user`/`--client-id`/
`--access-token`/`--access-token-stdin` are mutually exclusive with each other (as are each
secret flag with its own `-stdin` twin), so mixing sources across an unrelated pair (e.g.
`--client-id` with `--password-stdin`) is not guaranteed to be caught by flag validation the way
same-shape conflicts are — stick to exactly one of the three shapes below:

- API token (current Atlassian standard; app passwords are deprecated): `--user
  <email> --password <token>`, or `--password-stdin` to pipe it in instead of putting it on
  the command line (`op read op://vault/x/token | bb profile create -n work -u
  me@corp.com --password-stdin`).
- Access token (Repository/Project/Workspace Access Token): `--access-token <token>`, or
  `--access-token-stdin`.
- OAuth2 client (Authorization Code Grant or Client Credentials): `--client-id
  <id> --client-secret <secret>` (or `--client-secret-stdin`). For the Authorization Code Grant
  only, also pass `--callback-port <port>` at create time (a profile created without it cannot
  authorize: `bb profile authorize` errors "profile <name> does not support Authorization Code
  Grant" if `--callback-port` was never set), then run `bb profile authorize <profile-name>` to
  complete the browser-based flow.

`--password`/`--access-token`/`--client-secret` each has a `-stdin` twin; the three `-stdin`
flags are mutually exclusive with each other and with their own non-stdin flag. Prefer the
`-stdin` form whenever a real secret value is available to you — it never lands in shell
history. Credentials are stored in the macOS Keychain by default; `--no-vault` stores them
in plaintext in the config file instead (only for testing, never recommend it otherwise).

Set `--default-repository` on `create`/`update` so commands don't need `--repository` every
time. `--default-workspace` is also available on both, but only `profile update
--default-workspace` validates the value against the workspace list (which needs
`read:workspace`); `profile create --default-workspace` stores whatever string you give it
unvalidated, so a scoped-down token that lacks `read:workspace` can still set it at create time.
`bb profile list`/`bb profile get` mask stored secrets in table/csv/tsv output unconditionally.
For json/yaml output, the EXPLICIT `-o json`/`-o yaml` gate on that command line only controls
whether the command *fetches* a vault-provenance secret to show it — a secret that is already
sitting in memory for another reason renders in ANY json/yaml output regardless of how that
format was chosen (an explicit `-o`, a profile-configured `outputFormat`, or `BB_OUTPUT_FORMAT`).
That's the case for a profile created with `--no-vault` (its secret lives in plaintext in the
config file and loads into memory the moment the profile is read, with no vault fetch involved)
and for a profile whose vault store failed at creation/update time and fell back to plaintext.
`bb profile get --current` skips the json/yaml secret gate entirely — it never calls the vault
fetch, so a VAULT-backed profile's secret stays absent from `--current` output no matter the
format. That does NOT make `--current` safe in general: a profile whose secret lives in the
config file (`--no-vault`, or a vault store that failed and fell back to plaintext) already has
that secret in memory with no vault fetch needed, so it renders in full in any json/yaml
output, `--current` included. `bb profile get <profile-name>` requires either the name
positional or `--current`; given neither, it errors `argument profile is missing`.

## Workspaces, repositories, branches, commits

- `bb workspace list` / `bb workspace get [<slug>]` / `bb workspace members [<slug>]` — omit
  the slug to use the resolved current workspace.
- `bb repo list [--role member|contributor|admin|owner|all]` / `bb repo get
  [<slug-or-uuid>]` / `bb repo clone <slug-or-uuid> [destination]` (aliases `repository`).
  `repo get/list/clone` never take `--repository` — the repository is always their own
  positional (or, for `list`, every repository of the resolved workspace). `list` defaults to
  `--role member`, not `all` — a repository the caller can see but isn't a member of (rare in a
  team workspace, where every repo is workspace-owned) is left out unless you pass `--role all`.
- `bb branch list [--query ...]` — branches of the current repository.
- `bb commit list [--query ...] [--include ref] [--exclude ref]`, `bb commit get
  [<hash-or-single-segment-ref>]` (no argument: the newest commit Bitbucket's API returns, NOT
  local git HEAD; a ref like `main` works exactly like a hash, but a multi-segment ref such as
  `release/1.0` is rejected — that one only works on `diff`/`patch` below), `bb
  commit diff <ref> [<ref>]` (one ref: diff against its parent; `--stat` prints diffstat JSON
  instead), `bb commit patch <ref> <ref>` (patch needs BOTH refs — unlike `diff`, it has no
  single-ref/parent form). `diff`/`patch` accept a hash or a branch/tag ref of any number of
  segments (e.g. `release/1.0`). `diff`/`patch`/`--stat` always print raw text/JSON, ignoring
  `--output`.

## Pull requests

`bb pullrequest` (aliases `pr`, `pull-request`).

- `bb pullrequest list [--state open|merged|declined|superseded|all]* [--source <branch>]
  [--destination <branch>] [--query '<bitbucket query>'] [--commit <full-hash>]` — `--state`
  is repeatable and defaults to `open` alone when omitted; `all` fetches every state. `--commit`
  is mutually exclusive with `--state`/`--query`/`--source`/`--destination`.
- `bb pullrequest list --mine` / `bb pullrequest list --author <user>` — **author mode**: lists
  pull requests across EVERY repository of the resolved workspace in one request, instead of the
  repository-scoped listing above. Use this for "my open PRs" or "what has X got open" rather
  than looping over `bb repo list`. No repository is resolved at all, so it works from any
  directory (including a non-Bitbucket checkout) as long as the workspace resolves via
  `--workspace` > Bitbucket git remote > profile `--default-workspace`. Passing `--repository`
  explicitly is rejected with an error rather than silently ignored. `--author` and `--mine` are
  mutually exclusive with each other and with `--commit`; `--state`/`--query`/`--source`/
  `--destination` all still apply and compose exactly as above.
  `--author` takes a **UUID in braces** (`'{01234567-89ab-cdef-0123-456789abcdef}'` — braces
  required, quote it so the shell doesn't eat them), an Atlassian account ID, or, per Bitbucket's
  API docs, a username. Prefer the UUID: usernames are largely legacy post-GDPR, and the nicknames
  `bb workspace members` prints in its `Name` column are NOT usernames — that command's `ID` column
  is the UUID this flag wants. There is no `me` sentinel: `--author` is a verbatim pass-through,
  use `--mine` for yourself. An identifier the endpoint cannot resolve to an author comes back
  either as a 404 (whose message spells out the accepted forms) or as an empty list, so an empty
  result is not proof the author has no open pull requests — re-check the identifier.
  Author mode adds `repository` to the default columns (`ID, Title, repository, source,
  destination, state`); repository-scoped `list`/`get` defaults are unchanged. `--columns
  repository` works on `list` and `get` alike; `--sort` is a `list`-only flag (`get` never
  registers it), so `--sort repository` is available on any `list` — repository-scoped or author
  mode — and on no `get`. The default sort is still `+id`, which is meaningless across
  repositories — pass `--sort repository` or `--sort updated_on` in author mode.
  Token scope caveat: this endpoint is workspace-level, so a **Repository** Access Token that
  works fine for repository-scoped `list` gets a 403 here. Use a workspace-scoped token, an API
  token, or fall back to per-repository listing.
- `bb pullrequest get <id>` (aliases `show`, `info`, `display`) — the id is REQUIRED here (no
  fallback — unlike approve/unapprove/request-changes/remove-request-changes/decline/merge/
  merge-status/diff/patch/commits/activities below; see the CRITICAL section above for what
  their fallback does). Participants (per-reviewer approval
  state) are NOT in the default columns on `get` OR `list`. Always add `--columns
  participants` (or `-o json`/`-o yaml`, which include the full participant objects
  regardless of `--columns`) when the user's request is about who has/hasn't approved.
  `draft` (`true`/`false`) IS a default column on `get`, `update` and `create`, but NOT on
  `list` — pass `--columns draft` on `list` (and `--sort draft`, `list`-only, orders non-drafts
  before drafts); `-o json`/`-o yaml` always carry the `draft` key on all of them. Because
  `update`/`create` print the server's own response to the write, `create --draft` and `update
  <id> --ready` already show the resulting draft state — no follow-up call is needed (and
  `--columns` is not registered on `update`/`create`, nor on `merge`, so never pass it to any of
  the three; `merge` prints a single row too, but its response is always a non-draft and carries
  no `draft` column). To check a draft state at any other time: `bb pullrequest get <id> --columns
  id,draft`.
- `bb pullrequest create --title <t> --source <branch> [--destination <branch>] [--reviewer
  <user>]... [--description <text> | --description-file <path-or-->] [--draft]
  [--close-source-branch]` — `--title` and `--source` are required; `--destination` is
  optional (Bitbucket defaults it to the repository's main branch server-side when omitted).
  At most one of `--description`/`--description-file`. Default reviewers are pulled from the
  repository/project settings whenever `--reviewer` is either omitted entirely OR given with
  `default` as its FIRST value; any further `--reviewer` values after that first `default` are
  silently discarded (they are ONLY read when the first value is not `default`), so never mix
  `default` with real reviewers in one command. A failure to resolve the default reviewers hard-
  fails the command when `--reviewer default` was given explicitly, but when `--reviewer` was
  omitted (the fallback fired implicitly) the same failure instead follows the usual
  `--warn-on-error`/`--ignore-errors` tolerance — the pullrequest is still created, just with no
  reviewers, since the caller never asked for any. `--reviewer none` (exactly, and alone) skips
  reviewer resolution entirely — no default-reviewers lookup, no reviewers on the created
  pullrequest; use it when the caller explicitly wants no reviewers rather than falling back to
  the repository's defaults. Combining `none` with any other `--reviewer` value (in any position,
  whether repeated flags or one comma-separated list) is a hard error before any write is sent
  (profile and repository resolution run first, and a repository lookup on a cache miss can still
  issue a GET). `--reviewer all` (exactly, and alone) adds every workspace member as a reviewer.
  The current user is excluded when identifiable; with a token that cannot read the user
  identity, Bitbucket rejects the self-review server-side instead. A pullrequest created with
  `--draft` is promoted later with `bb pullrequest update <id> --ready` — never send the user to
  the web UI for this.
- `bb pullrequest update <id> [--title ...] [--description ... | --description-file ...]
  [--destination <branch>] [--close-source-branch] [--ready | --draft] [--add-reviewer <user>]...
  [--remove-reviewer <user>]...` (aliases `edit`) — `--ready` clears the pullrequest's draft
  status (marks it ready for review), `--draft` sets it; the two are mutually exclusive with each
  other (a hard error before any request), and either one combines with every other `update` flag
  in the same single PUT — `update <id> --ready --add-reviewer <user>` promotes and adds the
  reviewer in one call, so never split it into two invocations. Passing `--ready` on a
  pullrequest that is already ready (or `--draft` on a draft) still sends the update, like
  `--close-source-branch`; both are ordinary booleans, so `--ready=false` means `--draft` and
  `--draft=false` means `--ready`. `update` (including `--ready`/`--draft`) only works on OPEN
  pullrequests — Bitbucket's API documents the update endpoint as mutating open pull requests
  only, and rejects a merged/declined/superseded target server-side.
  `--add-reviewer` accepts the same `default`
  and `all` sentinels as `create`'s `--reviewer`, but `default` only has to be the FIRST value
  here: `--add-reviewer default,bob` resolves the default reviewers AND still adds `bob`, unlike
  `create`'s `--reviewer default,bob` which discards `bob`. `all` must be the ONLY value on both
  commands. Both sentinels exclude the current user when identifiable; with a token that cannot
  read the user identity, Bitbucket rejects the self-review server-side instead.
  `--remove-reviewer` resolves neither, matching `default`/`all` literally against the
  pullrequest's current reviewers.
- `bb pullrequest approve <id>` / `bb pullrequest unapprove <id>` / `bb pullrequest request-changes <id>` / `bb pullrequest
  remove-request-changes <id>` / `bb pullrequest decline <id>` / `bb pullrequest merge <id>` —
  the pull request id is optional on each (see the CRITICAL section above for what "omitted"
  actually does — always pass it explicitly for these). `merge` additionally has its own
  never-invoke-yourself rule below.
- `bb pullrequest merge <id> [--message <text>] [--close-source-branch] [--merge-strategy
  merge_commit|squash|fast_forward] [--async]`. Per Bitbucket's own documentation a draft
  pullrequest is NOT eligible for merging, manually or automatically: if a merge is refused
  because the pullrequest is a draft, promote it with `bb pullrequest update <id> --ready` first. `--async` returns a task id; poll it with `bb
  pullrequest merge-status <id> --task-id <task-id>` (`<id>` optional here too, same fallback).
  `merge` always asks an interactive `Merge pullrequest <id>? [y/N]` confirmation and has **no
  `--force`** of any kind: piped/redirected stdin errors immediately, before any prompt is
  shown, with `cannot confirm merge: ...: merging requires an interactive terminal`;
  `/dev/null` stdin prompts to stderr first and only then hits the same error, since the empty
  read still counts as nobody answering; and a real or pty-backed terminal blocks waiting for a
  human's answer. `--dry-run` is the one exception: it skips the prompt entirely (and sends no
  write), including on non-interactive stdin. See the MANDATORY note under Pipelines below for
  why this means you must never run this command yourself.
- `bb pullrequest diff <id> [--stat]`, `bb pullrequest patch <id>`, `bb pullrequest commits <id>`, `bb pullrequest activities
  <id>` — id optional on all four (same fallback as above; these are all read-only so the risk
  of the fallback here is "wrong PR shown", not an unrecoverable write). An activity kind newer
  than this build of `bb` recognizes is dropped from the list; `bb` warns once per DISTINCT
  unrecognized kind on stderr (not once overall, and not fatal).

### Comments — `bb pullrequest comment <verb> <pullrequest-id> ...`

The pull request id is always the first, REQUIRED positional; there is no `--pullrequest`
flag anywhere in this subtree.

- `bb pullrequest comment list <pr-id>`
- `bb pullrequest comment get <pr-id> <comment-id>`
- `bb pullrequest comment create <pr-id> (--comment <text> | --comment-file <path-or-->) [--file
  <path-in-diff> [--line <n>] [--from <n>]] [--pending] [--parent <comment-id>]`
  (aliases `add`, `new`). `--comment` and `--comment-file` are mutually exclusive and exactly one
  is required. `--line` anchors to the line number in the **new** (head) version of the file —
  this is what most people mean when they say "comment on line N", since that's the file as it
  reads in the PR. `--from` anchors to the line number in the **old** (base) version of the file
  instead, for commenting on a line that was deleted. `--line` and `--from` are mutually
  exclusive; there is no `--to`. `--pending` marks it a pending (draft) comment.
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
  <path-or-->) [--file <path-in-diff> [--line <n>] [--from <n>]] [--pending]
  [--parent <comment-id>]` (aliases `edit`) — same flag set as `create` above.
- `bb pullrequest comment delete <pr-id> <comment-id>...` (aliases `remove`, `rm`)
- `bb pullrequest comment resolve <pr-id> <comment-id>`
- `bb pullrequest comment reopen <pr-id> <comment-id>`

### Tasks — `bb pullrequest task <verb> <pullrequest-id> ...`

Same shape as comments: pull request id is the required first positional, no `--pullrequest`.

- `bb pullrequest task list <pr-id>`
- `bb pullrequest task get <pr-id> <task-id>`
- `bb pullrequest task create <pr-id> --content <text> [--comment <comment-id>] [--pending]`
  (aliases `add`, `new`)
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
an agent.** Both ask an interactive `y/N` confirmation before doing anything. The behavior
without `--force` depends on what stdin actually is:

- Piped/redirected stdin (every normal agent invocation) fails immediately with an error ending
  `: input is not a terminal, use --force to skip confirmation` (prefixed with `cannot confirm
  pipeline trigger:`/`cannot confirm pipeline stop:` and the specific `y/N` prompt text, e.g.
  `Trigger a new pipeline on <target>?` / `Stop pipeline <id>?`).
- `/dev/null` on stdin (cron, `nohup`, most agent harnesses) is a character device, so it passes
  the same check that piped/redirected stdin fails: the command prompts to stderr as normal,
  then reads EOF with nothing typed and silently DECLINES — `Trigger canceled`/`Stop canceled`,
  exit `0`, nothing run. This is not the error case above; it looks like a normal decline.
- A real or pty-backed terminal on stdin is treated as interactive: the command prints the
  prompt and BLOCKS waiting for a line of input — it does not detect "no human is actually
  there" in that case. Don't assume it always fails fast; only `--force` reliably avoids the
  error, the silent decline, and the block.

`--dry-run` also skips the prompt (and performs no write), which is the right choice when the
goal is only to preview the request, not send it.

**MANDATORY: never invoke `bb pullrequest merge` yourself.** It asks the same kind of
interactive confirmation as `pipeline trigger`/`stop` above, but has **no `--force`** — piped or
`/dev/null` stdin does not fall back to unattended success, it errors; a real or pty-backed
terminal on stdin blocks your whole session waiting for a line of input nobody may ever type.
`--dry-run` still skips the prompt entirely (and sends no write), same as `pipeline
trigger`/`stop` above, but that only previews the merge — it never substitutes for actually
running it, so this never-invoke rule still applies. Surface the pull request as ready to merge
(its id, title, and current state) and ask the user to run `bb pullrequest merge` themselves
instead.

### Pipeline steps — `bb pipeline step list <pipeline>` / `bb pipeline step <get|logs|report|cases> <pipeline> <step>`

Every subcommand's pipeline is a REQUIRED positional first argument — unlike `commit get`,
there is no "latest pipeline" fallback. `list` takes only the pipeline; `get`/`logs`/`report`/
`cases` additionally require the step positional (no verb makes it optional). The step
positional accepts either a UUID or a step NAME (case-insensitive, trimmed); `bb` resolves a
name to its UUID itself. An unknown name errors listing the available names; a name matching
more than one step (Bitbucket allows duplicates) errors listing the ambiguous UUIDs and asks
for a UUID instead.

- `bb pipeline step list <pipeline>`
- `bb pipeline step get <pipeline> <step-uuid-or-name>` (aliases `show`, `info`, `display`)
- `bb pipeline step logs <pipeline> <step-uuid-or-name>` (alias `log`) — the actual build
  console output; this is almost always the right command for "why did the build fail".
- `bb pipeline step report <pipeline> <step-uuid-or-name>` — the test report (JSON).
- `bb pipeline step cases <pipeline> <step-uuid-or-name>` — individual test case results.

`logs`/`report`/`cases` print raw text/JSON straight through: `--output` is accepted (inherited
from the root command) but has no effect on them, while `--columns` is not registered on these
three at all and is a hard `unknown flag: --columns` error — don't pass it.

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
  give full, untruncated data for scripting; table output caps exactly six free-text-ish
  columns (`title`, `description`, `message`, `content`, `reason`, `participants` — never an
  identifier such as a UUID or an artifact name, which always render at full length) at 80
  display columns, collapsing internal whitespace/newlines to single spaces first — display-only,
  never affects json/yaml/csv/tsv. `--columns <a>,<b>` (or `--columns all`) picks which columns
  render; repeatable or comma-separated. Every API-backed `list` subcommand also takes `--limit
  <n>` (maximum total items fetched) and `--page-length <n>` (Bitbucket page size per request);
  most also take `--sort <column>` to sort client-side. `bb profile list` is the one exception —
  profiles come from the local config file, not a paged API, so it registers only `--columns`/
  `--sort`; `--limit`/`--page-length` aren't accepted there.
- **`--dry-run`** (also `--noop`/`--whatif`) behaves differently for write vs. read commands:
  - Write commands (create/update/delete/merge/approve/decline/trigger/stop/comment/task/...)
    run their FULL preflight first — every resolution GET, id/target validation the real write
    would also need — then print the resolved API path and, when there is a request body, its
    JSON payload to stderr, and skip only the final write. A `--dry-run` against a nonexistent
    id still fails exactly like the real command would; it never fabricates a success line.
  - `--dry-run` guarantees no write/mutating request is ever sent. On read commands
    (`get`/`list`/`diff`/`patch`/`commits`/`logs`/`report`/`cases`/...) it skips the main/final
    request, printing just a "Dry run: <description>" line on stderr instead — but do not rely on
    it for zero network traffic. Depending on the command, target resolution (repository lookup,
    PR id lookup, or even fetching the resource itself) may still run BEFORE the dry-run check and
    issue real GETs, and caching may skip some of those. A `--dry-run` against a nonexistent id can
    still fail with a real error from that resolution. To validate inputs without side effects,
    prefer running the real read command (reads are safe) over read+`--dry-run`.
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
  scopes than the commands you're about to run actually need. (`profile update
  --default-workspace` is one narrow exception — see the Profile section above.)
- **Multi-argument commands** (`comment delete`, `task delete`, `artifact download`, ...)
  accept several ids/names on one line and honor `--stop-on-error`/`--warn-on-error`/
  `--ignore-errors` for how failures among them are handled.
