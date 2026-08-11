# Bitbucket Command Line Interface

[![build](https://github.com/avitsrimer/bitbucket-cli/actions/workflows/ci.yml/badge.svg?branch=master)](https://github.com/avitsrimer/bitbucket-cli/actions/workflows/ci.yml)

> [!NOTE]
> **This project is a fork of [gildas/bitbucket-cli](https://github.com/gildas/bitbucket-cli).** All credit for the original design and implementation goes to Gildas Cherruel and the upstream contributors. This fork is maintained independently of the upstream project and is **detached** from it — it does not track upstream releases and does not aim for feature parity.

`bb` is the missing command line interface for Bitbucket. It brings the power of the Bitbucket platform to your command line. Creating and merging Pull Requests and more are now just a few keystrokes away.

**Who this is for.** Working engineers who don't administer Bitbucket and track their work mostly
in Jira. The feature surface is opinionatedly reduced to make everyday use easier and to make it
hard to break anything by accident. And it is deliberately **not AI-native**: merging a pull
request always requires a human pressing the button — `bb pullrequest merge` prompts
interactively and has no automation bypass.

> [!IMPORTANT]
> **This is an opinionated fork, deliberately narrower than upstream.**
>
> The supported surface is `bb pullrequest` (full command tree), `bb user`, `bb profile`
> (authentication plumbing), `bb pipeline` (`get`, `list`, `trigger`, `stop`, plus the `step`
> subgroup: `get`, `list`, `logs`, `report`, `cases`), `bb repo`/`bb repository` (`get`, `list`,
> `clone` — read-only, no create/delete/fork/update), `bb workspace` (`get`, `list`, `members` —
> no permission administration), `bb commit`/`bb branch` (read-only: `commit get/list/diff/patch`,
> `branch list`), `bb artifact` (`list`, `download` — no upload/delete), and `bb install-skill`
> (writes an embedded Claude Code skill teaching an agent this whole surface — see [Agent
> skill](#agent-skill)). Every other command group inherited from upstream — `issue`, `tag`,
> `project`, `gpg-key`, `ssh-key`, `cache`,
> `remote`, `component` — remains **removed** from this fork, as does every admin/destructive verb
> of the groups above (repository create/delete/fork/update, `repo get --forks`, workspace
> permission management, pipeline `--tag` targets) and the deprecated `pullrequest activity` alias
> (use `pullrequest activities`).
>
> `bb pipeline trigger` and `bb pipeline stop` ask for a `y`/`N` confirmation before running, and
> accept `--force` to skip it. `pullrequest merge` also asks for a `y`/`N` confirmation, but
> deliberately has no `--force` of any kind — merging is not automatable via `bb`, on purpose (see
> [Pull Requests](#pull-requests)). Every other state-changing command, including `pullrequest
> decline`, runs immediately. Use `--dry-run` to preview any of them first.

The supported surface is:

- **Workspaces** — `bb workspace` → `get`, `list`, `members`
- **Repositories** — `bb repo` (alias `repository`) → `get`, `list`, `clone`
- **Branches** — `bb branch` → `list`
- **Commits** — `bb commit` → `get`, `list`, `diff`, `patch`
- **Pull requests** — `bb pullrequest` → `list`, `get`, `create`, `update`, `decline`, `merge`, `merge-status`, `approve`, `unapprove`, `request-changes`, `remove-request-changes`, `diff`, `patch`, `commits`, `activities`
- **Comments** — `bb pullrequest comment` → `list`, `get`, `create`, `update`, `delete`, `resolve`, `reopen`
- **Tasks** — `bb pullrequest task` → `list`, `get`, `create`, `update`, `delete`
- **Pipelines** — `bb pipeline` (aliases `pipelines`, `pipe`, `pp`) → `get`, `list`, `trigger`, `stop`
- **Pipeline steps** — `bb pipeline step` (alias `steps`) → `get`, `list`, `logs`, `report`, `cases`
- **Artifacts** — `bb artifact` → `list`, `download`
- **Users** — `bb user` → `get`, `me`
- **Authentication** — `bb profile` (including API tokens stored in the macOS Keychain, see [Profiles](#profiles)) and `bb completion`
- **Agent skill** — `bb install-skill` (see [Agent skill](#agent-skill))

## Installation

This fork releases macOS (darwin/arm64) binaries only.

### Homebrew

```bash
brew tap avitsrimer/apps
brew install --cask bb
```

### Go

If you have Go installed, you can install `bb` with:

```bash
go install github.com/avitsrimer/bitbucket-cli/cmd/bb@latest
```

### Binaries

You can download the latest `bb` tar.gz from the [releases](https://github.com/avitsrimer/bitbucket-cli/releases) page and copy the extracted `bb` executable anywhere in your `$PATH`.

> [!NOTE]
> A binary downloaded this way is unsigned and stays quarantined by macOS Gatekeeper (the
> Homebrew cask strips the quarantine attribute for you as part of installation, but a manual
> download does not). Run `xattr -dr com.apple.quarantine /path/to/bb` after extracting it,
> or use Homebrew instead.

### Version

`bb --version` prints a single git-describe-derived string (e.g. `v0.19.0` or
`v0.19.0-3-gabc1234-dirty` between tags); there is no separate `bb version` subcommand. A
`go install`ed or plain `go build` binary reports `dev` unless the build stamps the `version`
variable via `-ldflags` the way the release `Makefile` does. Building `bb` requires Go 1.26+.

### Prerequisites

`bb repo clone` and `bb pipeline trigger` (whenever `--branch` is omitted) shell out to the real
`git` executable, so a `git` binary must be on `PATH`. No other command touches it — `bb`
otherwise reads `.git/config` by hand and never needs `git` itself.

## Usage

`bb` is a modern command line interface. It uses subcommands to perform actions. You can get help on any subcommand by running `bb <subcommand> --help`.

General help is also available by running `bb --help` or `bb help`.

By default `bb` works in the current git repository. You can specify a Bitbucket repository with the `--repository` flag.

Many commands and flags are dynamically auto-completed. See the [Completion](#completion) section for more information about completion.

Most `delete` commands, and `bb artifact download`, accept multiple ids/names as separate positional arguments on the same command line:

```bash
bb pullrequest comment delete 1 452466 452467 452468
bb artifact download build.log other.zip
```

You can tell `bb` to stop on the first error, warn on errors, or ignore errors when processing multiple arguments with the `--stop-on-error`, `--warn-on-error`, or `--ignore-errors` flags.

All commands that would modify something on Bitbucket now allow you to preview the changes before applying them. You can use the `--dry-run` flag to see what would happen.

```bash
bb pullrequest decline 1 --dry-run
```

`--dry-run` runs full preflight: every resolution GET a real invocation would make (looking up the
pull request/comment/task/pipeline, validating a `--file` diff anchor against the PR's diffstat,
and so on) still runs, and the command echoes the resolved target API path and payload it would
have sent, to stderr — only the final write is skipped. A nonexistent pull request id or an empty
comment/task body fails under `--dry-run` with the same error a real invocation would produce; the
output is never a fabricated success line for input that would actually fail.

```bash
bb pullrequest decline 999999 --dry-run   # fails: pull request 999999 not found
```

Most commands will support the `--workspace` and `--repository` flags to specify the workspace and repository to use. If not provided, each is resolved independently in the same three-rung order: the flag itself, then a Bitbucket git remote in the current checkout (a GitHub or other non-Bitbucket remote is ignored and falls through), then the profile's default (`--default-workspace`/`--default-repository` on `bb profile create`/`update`). If all three rungs come up empty, the error names all three ways to supply the value. The workspace and repository can be combined in the form `workspace/repository`, whether supplied via `--repository` or via `--default-repository` on the profile -- either way, the workspace segment it carries overrides `--default-workspace`/a git remote/`--workspace`. For example:

```bash
bb pullrequest list --repository myrepository
bb pullrequest list --workspace myworkspace --repository myrepository
bb pullrequest list --repository myworkspace/myrepository
```

The `--workspace` flag is also dynamically auto-completed with the workspaces you have access to.

> [!NOTE]
> `bb repo get`, `bb repo list`, and `bb repo clone` reject `--repository` (it is also hidden
> from their `--help`): the repository is always the command's own positional argument (`get`,
> `clone`) or the current workspace's repositories (`list`), so a separate `--repository` override
> would be ambiguous or meaningless. `bb pullrequest list --author/--mine` rejects it too, for the
> same reason: that listing spans every repository of the workspace (see
> [Listing one author's pull requests across a whole workspace](#listing-one-authors-pull-requests-across-a-whole-workspace)).

`get` and `list` commands support the `--columns` flag to specify which columns to display in the output. You can pass a comma-separated list of columns, repeat the flag, or use `all` to display all columns. If you do not provide this flag, the default columns are displayed.

```bash
bb pullrequest list --columns all
bb pullrequest list --columns id,title,state
bb pullrequest list --columns id --columns title
```

`list` commands also support the `--sort` flag to sort the output by a single column. If you do not provide this flag, the command's default -- either a sensible column-based sort, or (`bb pipeline step/pipeline/commit list`) the order Bitbucket's API itself returned -- is used.

```bash
bb pullrequest list --sort title
```

Most `list` commands also support the `--query` flag to filter the output by a specific query. The query syntax is similar to the one used in the Bitbucket web interface. For example, to filter Pull Requests updated after a specific date:

```bash
bb pullrequest list --query 'updated_on > 2025-12-31'
```

Please refer to the [Bitbucket API documentation](https://developer.atlassian.com/cloud/bitbucket/rest/intro/#filtering) for more information about the supported query syntax and fields.

> [!NOTE]
> `bb workspace list`, `bb workspace members`, `bb repo list`, and `bb pipeline step list` have no
> `--query`: Bitbucket's list endpoints for workspaces, members, and pipeline steps don't support
> server-side filtering. `bb repo list` filters by `--role` instead.

`list` commands also support the `--page-length` flag to set the number of items to retrieve per request to Bitbucket API at a time. By default, the page length is set on the profile and the default is 50. You can set it to a value between 1 and 100.

```bash
bb pullrequest list --page-length 25
```

`list` commands also support the `--limit` flag to cap the *total* number of items retrieved across every page (unlike `--page-length`, which only sizes each individual request). Shell-completion lookups (e.g. completing a pull request ID) deliberately ignore `--limit` on the same command, since a completion candidate list must never be truncated by a flag meant to bound a different, unrelated listing.

```bash
bb pullrequest list --limit 10
```

### Output

`bb` outputs a table by default. You can change the output format with the `--output` flag,  by setting the `BB_OUTPUT_FORMAT` environment variable, or by modifying the profile configuration (See [Profiles](#profiles)).

> [!NOTE]
> `bb commit diff`, `bb commit patch`, and `bb pipeline step logs`/`report`/`cases` ignore
> `--output`/`--columns` entirely: they stream Bitbucket's raw response (a diff, a patch, log
> text, or a JSON report) straight to stdout instead of rendering a table/JSON/etc. `bb commit
> diff --stat` is a further special case: it hits the diffstat endpoint and always prints raw
> JSON, regardless of `--output`.

The following formats are supported:

- `csv`: CSV
- `json`: JSON
- `yaml`: YAML
- `tsv`: TSV
- `table`: Table

For example:

```bash
bb --output json pullrequest list
```

Or

```bash
bb pullrequest list --output json
```

Changing the format with the environment variable `BB_OUTPUT_FORMAT` can be done like this:

```bash
export BB_OUTPUT_FORMAT=json
bb pullrequest list
```

The Table output format displays the data in a human-readable format. Here is an example of the output for the `bb pullrequest list` command:

```bash
$ bb pr list --state all
+----+----------------------+---------------+-------------+----------+
| ID |        TITLE         |    SOURCE     | DESTINATION |  STATE   |
+----+----------------------+---------------+-------------+----------+
|  1 | Merge feature/links  | feature/links | dev         | DECLINED |
|  2 | Merge feature/links  | feature/links | dev         | MERGED   |
|  3 | Merge release/1.0.0  | release/1.0.0 | master      | MERGED   |
|  4 | Merge feature/bb     | feature/bb    | dev         | DECLINED |
|  5 | Merge feature/bb     | feature/bb    | dev         | MERGED   |
|  6 | Merge feature/bb-doc | feature/bb-doc| dev         | MERGED   |
+----+----------------------+---------------+-------------+----------+
```

> [!NOTE]
> The table format truncates a fixed set of six free-text-ish columns -- `title`, `description`,
> `message`, `content`, `reason`, and `participants` -- to 80 display columns (ellipsized, and
> internal newlines collapsed to spaces first); "display columns" accounts for double-width
> runes (CJK, most emoji) rather than a plain character/rune count, so those still render at up
> to 80 terminal columns, not double that. Every other column, including every identifier such
> as a UUID or an artifact name, always renders at full length regardless of size. This cap
> exists so a single multi-paragraph field -- a pull request/comment/task body, a commit message
> -- can no longer blow one column, and with it every other column, out to an unreadable width.
> This cap is cosmetic: it only affects the rendered table. `csv`, `tsv`,
> `json`, and `yaml` output always contain the complete, untruncated value, since those formats
> are meant for scripting. For the same reason, `participants` is not among `bb pullrequest
> get`/`list`'s default columns, and `description` is not among `list`'s (though it IS a default
> column on `get`) -- an unbounded, list-shaped value is rarely useful in a rendered table -- pass
> `--columns description`/`--columns participants` (or `--columns all`) to include them anyway on
> whichever subcommand doesn't already. In table/csv/tsv, `participants` renders a compact
> `nickname:state` summary per reviewer; `-o json`/`yaml` carry the full participant objects
> (role, approval state, participation date) regardless of `--columns`.

### Environment variables

`bb` reads a `.env` file in the current directory on startup, if one exists (via
[`joho/godotenv`](https://github.com/joho/godotenv)), so any of the variables below can also be
set there instead of in your shell.

| Variable                        | Equivalent flag | Effect                                                                                                   |
| -------------------------------- | ---------------- | --------------------------------------------------------------------------------------------------------- |
| `BB_PROFILE`                     | `--profile`       | Profile to use, overriding the default profile.                                                            |
| `BB_OUTPUT_FORMAT`                | `--output`        | Output format (`csv`, `json`, `table`, `tsv`, `yaml`).                                                      |
| `BB_CONFIG`                      | `--config`        | Path to the configuration file. See [Profiles](#profiles) for the default search order.                    |
| `BITBUCKET_CLI_CACHE_DURATION`    | _(none)_          | How long repository/user/workspace lookups are cached on disk. See [Cache](#cache).                        |

### Cache

`bb` caches repository, user, and workspace lookups on disk for 5 minutes by default, to avoid
repeated round trips to the Bitbucket API. The duration can be changed with the
`BITBUCKET_CLI_CACHE_DURATION` environment variable (any Go duration string, e.g. `30s`, `10m`):

```bash
export BITBUCKET_CLI_CACHE_DURATION=10m
```

The cache is stored under `os.UserCacheDir()/bitbucket` (`~/Library/Caches/bitbucket` on macOS,
`~/.cache/bitbucket` on Linux). There is no `bb cache clear` command; delete that directory
directly if you need to invalidate a stale entry.

### Token permissions

`bb` is designed to work with **narrowly scoped tokens** — grant only what the commands you use
need. Scope names below use Bitbucket's API-token form (`read:repository:bitbucket`);
app-password equivalents in parentheses.

**Required — the core every command relies on**

- `read:repository` *(Repositories: Read)* — repository/branch/commit reads, artifact
  `list`/`download`, and resolving the target repository for every other command
- `read:pullrequest` *(Pull requests: Read)* — `pullrequest list/get/activities/diff/patch/commits`,
  comment and task reads[^readpr]
- `read:user` *(Account: Read)* — `bb user get`/`me` (including `--emails`), and
  `pullrequest list --mine`, which resolves your own account through the same endpoint and fails
  without it (`pullrequest create`'s default-reviewer lookup also calls it, but only degrades to a
  `[WARN]`)

**[Optional] — grant per workflow**

- `write:pullrequest` *(Pull requests: Write)* — `pullrequest
  create/update/approve/unapprove/request-changes/remove-request-changes/decline/merge`, comment
  and task create/update/delete. Skip it for a read-only setup
- `read:pipeline` *(Pipelines: Read)* — `pipeline get/list` and all of `pipeline step`, including
  `logs`, `report`, and `cases` (Bitbucket's pipeline read scope covers logs, tests, and
  artifacts)
- `write:pipeline` *(Pipelines: Write)* — `pipeline trigger` and `pipeline stop`
- `read:workspace` *(Workspaces: Read)* — **only** `workspace get/list/members` and the
  workspace-name validation in `profile update --default-workspace`. Everything else deliberately
  works without it: explicitly supplied `--workspace` values, `--default-workspace` on
  `profile create`, and every repository/pull-request/pipeline operation, none of which needs a
  workspace *lookup*. `pullrequest list --author/--mine` does address a workspace-level path
  (`/workspaces/{workspace}/pullrequests/{user}`), but it is a pull-request endpoint and only needs
  `read:pullrequest` — the workspace slug is never resolved through a workspace endpoint. A
  Repository Access Token nonetheless cannot reach it: see
  [Listing one author's pull requests across a whole workspace](#listing-one-authors-pull-requests-across-a-whole-workspace)

**Not needed**

- `read:test`, `read:runner` — reading test reports is covered by `read:pipeline`; no `bb`
  command touches runner endpoints
- admin, webhook, project, and snippet scopes — no `bb` command calls them

**No token needed at all** — `completion`, `help`, `install-skill`, and every `profile` command
(local config + Keychain) except the `read:workspace` case above; `profile authorize`
authenticates through its own OAuth client credentials rather than token scopes.

[^readpr]: Bitbucket documents `read:pullrequest` as also permitting PR comments; `bb` has not
verified comment writes on a read-only token, so grant `write:pullrequest` if you comment.

> [!NOTE]
> `repo clone` authenticates through git itself (an SSH key or an app password over HTTPS), not
> through the API token above.

### Profiles

#### Setting up OAUTH 2.0

To add an OAuth 2.0 profile, you need to create an OAuth consumer on Bitbucket. First, go to the settings page <https://bitbucket.org/xxxx/workspace/settings> of the Bitbucket workspace you want a consumer for (where `xxxx` is the workspace name/ID). On that page, click on the `OAuth clients` link in the `Apps and features` section. Then click on the `Create OAuth client` button. Fill in the form.

![OAuth clients](images/bitbucket-add-oauth.png)

To use an [OAuth 2.0 with Authorization Code Grant](https://developer.atlassian.com/cloud/bitbucket/rest/intro/#1--authorization-code-grant--4-1-), you will need to fill in the `Callback URL` with a link like <http://localhost:yyyy> (where `yyyy` is the port you want to use and provide to the `--callback-port` flag of `bb profile create`) and **do not** enable the check box for `This is a private consumer`.

To use an [OAuth 2.0 with Client Credentials](https://developer.atlassian.com/cloud/bitbucket/rest/intro/#3--client-credentials-grant--4-4-), you will need to enable the check box for `This is a private consumer` and add a _dummy_ `Callback URL`.

In both cases, you will need to fill in the permissions you want to grant to the consumer.

Once you hit the `Save` button, your OAuth consumer will be created and you can use the credentials (client identifier and secret) provided to configure your profile with `bb`.

#### Managing Profiles

`bb` uses profiles to store your Bitbucket credentials. You can create a profile with the `bb profile create` command:

```bash
bb profile create \
  --name              myprofile \
  --default-workspace myworkspace \
  --client-id         <your-client-id> \
  --client-secret     <your-client-secret> \
  --callback-port     8080
```

You should define the default workspace for the profile with the `--default-workspace` flag. This will allow you to use `bb` without specifying the workspace every time. Likewise, `--default-repository` sets a default repository, used whenever `--repository` is not given and the current directory has no Bitbucket git remote to fall back to.

You can also pass the `--default` flag to make this profile the default one, or pass a `--output` flag to change the profile output format. If you use only one profile, it will be used as the default profile.

> [!NOTE]
> `--default-project` and `--progress` are still accepted and stored in the profile (for
> compatibility with config files written by upstream) but have no effect in this fork: the
> commands that read them were removed from the command surface. `--default-ssh-key-file` is
> functional again: `bb repo clone` uses it as the default SSH private key file when cloning over
> the `git`/`ssh` protocols and no `--ssh-key-file` flag is passed.

By default, the password or client secret is stored in the vault of the operating system (Windows Credential Manager, macOS Keychain, or Linux Secret Service). You can pass the `--no-vault` flag to disable this feature and store the password or client secret in plain text in the configuration file. This is not recommended, but can be useful for testing purposes.

Once the profile is created in `bb`, for an [OAuth 2.0 with Authorization Code Grant](https://developer.atlassian.com/cloud/bitbucket/rest/intro/#1--authorization-code-grant--4-1-), you will need to authorize the profile with the following command:

```bash
bb profile authorize myprofile
```

You can also use the `--verbose` to get some information about the authorization process.

Profiles support the following authentications:

- [OAuth 2.0 with Authorization Code Grant](https://developer.atlassian.com/cloud/bitbucket/rest/intro/#1--authorization-code-grant--4-1-) with the `--client-id`, `--client-secret`, and `--callback-port` flags. See the [Setting Up an OAUTH 2.0 Profile](#setting-up-oauth-20) section for more information about how to create an OAuth client and authorize the profile.
- [OAuth 2.0 with Client Credentials](https://developer.atlassian.com/cloud/bitbucket/rest/intro/#3--client-credentials-grant--4-4-) with the `--client-id` and `--client-secret` flags. See the [Setting Up an OAUTH 2.0 Profile](#setting-up-oauth-20) section for more information about how to create an OAuth client.
- [API tokens](https://support.atlassian.com/bitbucket-cloud/docs/api-tokens/) with the `--user` and `--password` flags. The user is the **Atlassian account email** and the password is the API token in this case.
- ~~[App passwords](https://support.atlassian.com/bitbucket-cloud/docs/app-passwords/) with the `--user` and `--password` flags.~~ [App passwords are deprecated by Atlassian in favour of API tokens as of June 9, 2025 and will stop working entirely on June 9, 2026](https://www.atlassian.com/blog/bitbucket/bitbucket-cloud-transitions-to-api-tokens-enhancing-security-with-app-password-deprecation). Use API tokens instead.
- [Repository Access Tokens](https://support.atlassian.com/bitbucket-cloud/docs/repository-access-tokens/), [Project Access Tokens](https://support.atlassian.com/bitbucket-cloud/docs/project-access-tokens/), [Workspace Access Tokens](https://support.atlassian.com/bitbucket-cloud/docs/workspace-access-tokens/) with the `--access-token` flags. Using Project/Workspace Access Tokens requires a Premium plan on Bitbucket Cloud. Using Repository Access Tokens does not require a Premium plan, but the token will only have access to the repository it was created for.

##### Passing secrets without shell history

`--password` and `--access-token` both take their value directly on the command line, where most shells record it in history and any other local process can see it for as long as the command runs. `bb profile create` and `bb profile update` also accept `--password-stdin` and `--access-token-stdin`, which read the secret from stdin instead (the whole input, trimmed of its trailing newline/whitespace) -- the same convention as `docker login --password-stdin` and `gh auth login --with-token`:

```bash
op read op://vault/bitbucket/token | bb profile create -n work -u me@corp.com --password-stdin
```

The OAuth2 client secret has the same convention: `--client-secret-stdin` reads it from stdin instead of `--client-secret`.

`--password-stdin` is mutually exclusive with `--password`, `--access-token-stdin` is mutually exclusive with `--access-token`, `--client-secret-stdin` is mutually exclusive with `--client-secret`, and the three `-stdin` flags are mutually exclusive with each other (only one secret can be piped in at a time).

If you run `bb profile create` with `-u/--user` set but no password source at all (neither `--password` nor `--password-stdin`), `bb` prompts for it interactively instead, with terminal echo disabled -- unless the vault already holds a password for that user (e.g. reused from an earlier profile), in which case that stored password is used and there is no prompt at all:

```console
$ bb profile create -n work -u me@corp.com
Password or API token for me@corp.com:
```

`bb profile update` prompts the same way when `-u/--user` is given without a password source, and additionally when a password source is given _without_ `-u/--user` on a profile that already has one (rotating a secret never requires retyping `--user`). Both prompts require a real terminal; in a non-interactive context (CI, a script, a piped command) `bb` fails fast with an error naming `--password-stdin`/`--access-token-stdin` instead of hanging.

Creating a profile with none of `--user`, `--client-id`/`--client-secret`, or `--access-token` given at all never prompts and never fails: the profile is created with no credentials on record, deferring resolution to the vault (already populated by another tool, e.g. for CI provisioning) the first time it is actually used, or to a later `bb profile update`. `--no-vault` is the one exception -- it cannot defer to the vault by definition, so it still requires a credential up front.

Switching a profile's credential shape (password, access token, or OAuth2 client) via `bb profile update` clears the other two shapes -- both their value and, if applicable, their vault entry -- so the profile only ever authenticates with the shape you most recently set.

Permission Scopes:

- [OAuth 2.0 scopes](https://developer.atlassian.com/cloud/bitbucket/rest/intro/#bitbucket-oauth-2-0-scopes)
- [API token permissions](https://support.atlassian.com/bitbucket-cloud/docs/api-token-permissions/)

When you use a user/password, the password is stored in the vault of the operating system (Windows Credential Manager, macOS Keychain, or Linux Secret Service). You can pass the `--no-vault` flag to disable this feature and store the password in plain text in the configuration file. This is not recommended, but can be useful for testing purposes. On Linux and macOS, you can also pass the `--vault-key` flag to set the key to use in the system keychain. By default, the key is `bitbucket-cli`. On Windows, this option is not available.

> [!NOTE]
> `--clone-protocol` and `--clone-user` are functional again: `bb repo clone` uses them as the
> default protocol (overridable per invocation with `--protocol`) and, for the `https` protocol,
> the username embedded in the clone URL.

> [!NOTE]
> `bb profile list`/`bb profile get` mask the access token in table/csv/tsv output (a masked
> placeholder even under the explicit `--columns accesstoken`). For json/yaml output, the
> EXPLICIT `-o json`/`-o yaml` gate on that command line only controls whether the command
> *fetches* a vault-provenance secret to show it -- that's the supported way to script retrieval
> of a token stored in the vault. It does not gate secrets by any other route: a secret that is
> already sitting in memory for another reason renders in ANY json/yaml output regardless of how
> that format was chosen (an explicit `-o`, a profile-configured `outputformat`, or
> `BB_OUTPUT_FORMAT`). That's the case for a profile created with `--no-vault` (its secret lives
> in plaintext in the config file and loads into memory the moment the profile is read, with no
> vault fetch involved) and for a profile whose vault store failed at creation/update time and
> fell back to plaintext -- for those, a bare `bb profile list`/`bb profile get` combined with a
> profile-configured `outputformat: json`/`yaml` or `BB_OUTPUT_FORMAT=json`/`yaml` renders the
> secret in full even with no `-o` flag on the command line at all. `bb profile get --current`
> skips the vault fetch too, so a VAULT-backed profile's secret stays absent from `--current`
> output -- but the same plaintext-secret profiles above render in full there as well, since
> their secret is already in memory with no vault fetch needed. `client-secret` and `password`
> have no `--columns` value at all, so they never appear in table/csv/tsv output regardless.

You can get the list of your profiles with the `bb profile list` command:

```bash
bb profile list
```

You can get the details of a profile with the `bb profile get` or `bb profile show` command:

```bash
bb profile get myprofile
```

You can ge the details of the current profile:

```bash
bb profile get --current
```

Or:

```bash
bb profile which
```

You can update a profile with the `bb profile update` command:

```bash
bb profile update myprofile \
  --client-id <your-client-id> \
  --client-secret <your-client-secret>
```

If the profile was not using the vault to store the credentials and you update it with new credentials, the updated profile will keep using plain text to store the credentials. If the profile was using the vault and you update it with new credentials, the updated profile will keep using the vault to store the credentials.

You can move the profile credentials to the vault with the `bb profile update`:

```bash
bb profile update myprofile --to-vault
```

During that process, you cannot change the credentials. But you can specify the vault key for non Windows OS with the `--vault-key` flag:

```bash
bb profile update myprofile --to-vault --vault-key my-vault-key
```

The default vault key is `bitbucket-cli`.

You can delete a profile with the `bb profile delete` command:

```bash
bb profile delete myprofile
```

You can set the default profile with the `bb profile use` command:

```bash
bb profile use myprofile
```

You can also set the profile with the environment variable `BB_PROFILE`:

```bash
export BB_PROFILE=myprofile
```

The profile can also come from your current `.git/config` file. You can set the `bb.profile` variable in the `[bitbucket "cli"]` section of your `.git/config` file:

```ini
[bitbucket "cli"]
  profile = myprofile
```

```bash
git config --local bitbucket.cli.profile myprofile
```

The current profile comes in order from:

- the `--profile` flag
- the `BB_PROFILE` environment variable
- the `profile` variable in the `[bitbucket "cli"]` section of your `.git/config` file,  
  if the profile does not exist, the command will print a warning and use the default profile
- the profile marked `default` in the configuration file
- the first profile in the configuration file

Profiles are stored in the configuration file, a plain YAML file written atomically with `0600`
permissions. By default, the configuration file is located:

- on Linux: `$XDG_CONFIG_HOME/bitbucket/config-cli.yml`, or `~/.config/bitbucket/config-cli.yml`, then `~/.bitbucket-cli`
- on macOS: `$HOME/Library/Application Support/bitbucket/config-cli.yml`, then `~/.bitbucket-cli`
- on Windows: `%AppData%\bitbucket\config-cli.yml`, then `$HOME/.bitbucket-cli`
- on Plan 9: `$home/lib/bitbucket/config-cli.yml`, then `~/.bitbucket-cli`

You can also override the location of the configuration file with the environment variable `BB_CONFIG` or the `--config` flag:

```bash
export BB_CONFIG=~/.bb/config.yml
```

```bash
bb --config ~/.bb/config.yml pullrequest list
```

### Users

You can get the details of your user with the `bb user me` command:

```bash
bb user me
```

You can get the details of a user with the `bb user get` or `bb user show` command:

```bash
bb user get {UUID}
```

Or,

```bash
bb user get UUID
```

### Workspaces

You can list the workspaces you have access to with the `bb workspace list` command:

```bash
bb workspace list
```

You can get the details of a workspace with the `bb workspace get` command. If you don't provide a slug or ID, the current workspace (resolved the same way as `--workspace`) is used:

```bash
bb workspace get myworkspace
bb workspace get
```

You can list the members of a workspace with the `bb workspace members` command, which resolves the current workspace the same way when no slug is given:

```bash
bb workspace members myworkspace
bb workspace members
```

> [!NOTE]
> Workspace permission administration (`bb workspace permission get/list` upstream) is not part of this fork.

### Repositories

You can list the repositories of a workspace with the `bb repo list` command (aliased `bb repository list`):

```bash
bb repo list --workspace myworkspace
```

If you do not provide a workspace, the one resolved from `--workspace`/git config/profile default is used. You can narrow the list down to repositories you have a given role in with `--role` (`all`, `owner`, `admin`, `contributor`, `member`; default `member`):

```bash
bb repo list --role contributor
```

You can get the details of a repository with the `bb repo get` command. If you don't provide a slug or UUID, the current repository is used:

```bash
bb repo get myrepository
bb repo get
```

You can clone a repository with the `bb repo clone` command:

```bash
bb repo clone myrepository
bb repo clone myrepository myfolder
```

The destination is an optional second positional argument (it defaults to the repository's slug), not a `--destination` flag. The protocol is resolved from `--protocol` (`git`, `https`, or `ssh`), then the profile's clone-protocol setting, then `git`:

```bash
bb repo clone --protocol https myrepository
```

For the `git`/`ssh` protocols, an SSH private key file can be set with `--ssh-key-file` (or the profile's SSH key file setting); it is passed to the underlying `git clone` through `GIT_SSH_COMMAND`. For `https`, the profile's clone-user setting, if any, is used as the username embedded in the clone URL.

> [!NOTE]
> `bb repo` does not support `create`, `delete`, `fork`, `update`, or `get --forks`; `list` filters
> only by `--role` — upstream's `--project`, `--project-key`, `--has-issues`, `--has-wiki`,
> `--is-private`, `--language`, and `--main-branch` filters are not implemented.

### Branches and Commits

You can list the branches of the current repository with the `bb branch list` command:

```bash
bb branch list
```

You can list the commits of the current repository with the `bb commit list` command, optionally filtered with `--query`, or narrowed with `--include`/`--exclude` (commit hashes or branch names):

```bash
bb commit list
bb commit list --include develop --exclude master
```

You can get the details of a commit with the `bb commit get` command. If you don't provide a hash, the newest commit Bitbucket's `/commits` endpoint returns for the repository is used — this is a server-side lookup, not your local git `HEAD`, so it can differ if your working copy is checked out to a different branch, is behind, or the repository is resolved without a local git checkout at all:

```bash
bb commit get 123456
bb commit get
```

You can get the diff between two commits with the `bb commit diff` command (or between one commit and its parent, if only one hash is given). Each argument may be a commit hash or a branch/tag ref (e.g. `release/1.0`); `--stat` switches to the diffstat endpoint instead of the diff itself, which returns a JSON summary rather than a text diff (both are printed raw, ignoring `--output`; see [Output](#output)):

```bash
bb commit diff 123456 654321
bb commit diff 123456
bb commit diff --stat 123456
bb commit diff release/1.0 main
```

You can get the patch between two commits with the `bb commit patch` command; each argument may likewise be a commit hash or a branch/tag ref:

```bash
bb commit patch 123456 654321
bb commit patch release/1.0 main
```

> [!NOTE]
> There is no commit or branch mutation of any kind in this fork (no `commit ancestor`, no branch create/delete/merge).

### Pull Requests

You can list pull requests with the `bb pullrequest list` command:

```bash
bb pullrequest list
```

You can get the list of pull requests for a given commit hash (it must be the full hash) with the `--commit` flag:

```bash
bb pullrequest list --commit ae86d5323477989fab3bf3879cd1234543565753
```

`--state` filters by pull request state and is repeatable (each occurrence adds a state to fetch); `all` is sugar for every state (`declined`, `merged`, `open`, `superseded`) rather than a real Bitbucket state. With no `--state` at all, the default is `open`:

```bash
bb pullrequest list --state merged
bb pullrequest list --state open --state merged
bb pullrequest list --state all
```

`--source` and `--destination` filter the list by source/destination branch name (composing with `--state` and `--query`, ANDed together). This is a *list filter*, distinct from `create`/`update`'s `--source`/`--destination`, which set a pull request's branches:

```bash
bb pullrequest list --source my-branch
bb pullrequest list --destination master --state open
```

`--commit` is mutually exclusive with `--state`, `--query`, `--source`, and `--destination` (Bitbucket's by-commit endpoint takes no other filter).

#### Listing one author's pull requests across a whole workspace

`--mine` lists your own open pull requests across **every repository** of the workspace, in a single request, and `--author` does the same for somebody else:

```bash
bb pullrequest list --mine
bb pullrequest list --mine --state all --sort updated_on
bb pullrequest list --author '{01234567-89ab-cdef-0123-456789abcdef}'
bb pullrequest list --author 557058:11111111-2222-3333-4444-555555555555 --workspace myworkspace
```

Either flag switches the command to Bitbucket's workspace-level `/workspaces/{workspace}/pullrequests/{user}` endpoint, which changes a few things:

- **No repository is resolved at all**, so this works from any directory — you are not required to be in a Bitbucket checkout, as long as the workspace resolves (`--workspace`, a Bitbucket git remote, or the profile's `--default-workspace`). Passing `--repository` explicitly is rejected rather than silently ignored, since the listing spans every repository.
- `repository` joins the **default columns** (`ID`, `Title`, `repository`, `source`, `destination`, `state`), so each row shows which repository its pull request belongs to. Repository-scoped `list` and `pullrequest get` keep their own defaults; `--columns repository` works on both, and `--sort repository` on any `list`.
- Pull request ids are only unique per repository, so the default `+id` sort is of limited use across repositories — prefer `--sort repository` or `--sort updated_on`.
- `--state`, `--query`, `--source`, and `--destination` compose exactly as they do on the repository-scoped listing. `--author`, `--mine`, and `--commit` are mutually exclusive with each other.

`--author` accepts the author's **UUID in braces** (quote it: most shells treat braces specially), their **Atlassian account ID**, or — per Bitbucket's API documentation for this endpoint — their **username**. The UUID is the reliable form: usernames are largely legacy post-GDPR, and the nicknames in the `Name` column of `bb workspace members` are *not* usernames — that command's `ID` column carries the UUID this flag wants. An identifier the endpoint cannot resolve to an author comes back either as a 404 (which `bb` wraps with the accepted forms) or as an empty listing, so an empty result does not by itself prove the author has no matching pull requests. There is no `me` sentinel; use `--mine`, which resolves your own account through `GET /user` (this needs the `read:user` scope).

> [!NOTE]
> This endpoint is workspace-level, so a [Repository Access Token](#profiles) cannot use it (it 403s no matter which workspace you name). Use an API token, a Workspace Access Token, or OAuth 2.0 for `--author`/`--mine`.

You can create a pull request with the `bb pullrequest create` command:

```bash
bb pullrequest create \
  --title "My pull request" \
  --source "my-branch" \
  --destination "master"
```

You can add reviewers to the pull request with the `--reviewer` flag:

```bash
bb pullrequest create \
  --title "My pull request" \
  --source "my-branch" \
  --destination "master" \
  --reviewer    username1 --reviewer {userUUID2}
```

Default reviewers are pulled from the repository/project's effective default-reviewers setting
whenever `--reviewer` is either omitted entirely or given with `default` as its first value; any
further `--reviewer` values after that first `default` are silently discarded, so never mix
`default` with real reviewers in one command. A failure to resolve the default reviewers hard-
fails the command when `--reviewer default` was given explicitly, but when `--reviewer` was
omitted (the fallback fired implicitly) the same failure instead follows the usual
`--stop-on-error`/`--warn-on-error`/`--ignore-errors` tolerance -- the pull request is still
created, just with no reviewers, since the caller never asked for any.

Pass `--reviewer none` (exactly, and alone) to create the pull request with no reviewers and skip
the default-reviewers lookup entirely -- useful when you explicitly want no reviewers rather than
falling back to the repository's defaults:

```bash
bb pullrequest create \
  --title "My pull request" \
  --source "my-branch" \
  --destination "master" \
  --reviewer none
```

Combining `none` with any other `--reviewer` value is an error, regardless of whether the values
arrive as repeated flags (`--reviewer none --reviewer username1`) or a single comma-separated list
(`--reviewer none,username1`).

Pass `--reviewer all` (exactly, and alone) to add every workspace member as a reviewer. The
current user is excluded when identifiable; with a token that cannot read the user identity,
Bitbucket rejects the self-review server-side instead:

```bash
bb pullrequest create \
  --title "My pull request" \
  --source "my-branch" \
  --destination "master" \
  --reviewer all
```

You can create the pull request as a draft with the `--draft` flag:

```bash
bb pullrequest create \
  --title "My pull request" \
  --source "my-branch" \
  --destination "master" \
  --draft
```

Writing a markdown description on the command line means fighting shell quoting -- backticks and
`$(...)` inside double quotes are a live command-substitution hazard. `--description-file` reads
the description from a file instead, or from stdin with `-`, and is mutually exclusive with
`--description`:

```bash
bb pullrequest create \
  --title "My pull request" \
  --source "my-branch" \
  --destination "master" \
  --description-file description.md
```

```bash
bb pullrequest create \
  --title "My pull request" \
  --source "my-branch" \
  --destination "master" \
  --description-file -  <<'EOF'
Fixes the flaky test by running `go test -race ./...` and checking $(git diff) first.
EOF
```

You can get the details of a pull request with the `bb pullrequest get` or `bb pullrequest show` command:

```bash
bb pullrequest get 1
```

You can also modify a pull request with the `bb pullrequest update` command:

```bash
bb pullrequest update 1 \
  --title "My pull request" \
  --description "My pull request description"
```

`--description-file`/`-` work the same way on `update` as they do on `create`.

To add or remove reviewers from a pull request, you can use the `--add-reviewer` and `--remove-reviewer` flags:

```bash
bb pullrequest update 1 \
  --add-reviewer    username1 --add-reviewer {userUUID2} \
  --remove-reviewer username3 --remove-reviewer {userUUID4}
```

`--add-reviewer` accepts the same `default` and `all` sentinels `--reviewer` on `create` does, but
`default`'s semantics differ slightly between the two commands: on `update`, `default` only needs
to be the *first* value -- `--add-reviewer default,bob` resolves the repository/project's
effective default reviewers and still adds `bob` on top, whereas the equivalent on `create`
(`--reviewer default,bob`) discards `bob` entirely. `--add-reviewer all` (exactly, and alone --
`all` must be the only value, same as on `create`) adds every workspace member as a reviewer. Both
sentinels exclude the current user when identifiable; with a token that cannot read the user
identity, Bitbucket rejects the self-review server-side instead. `--remove-reviewer` does not
resolve either word specially -- `--remove-reviewer default` or `--remove-reviewer all` only match
a current reviewer literally named `default` or `all`.

You can `approve`or `unapprove` a pull request with the `bb pullrequest approve` or `bb pullrequest unapprove` command:

```bash
bb pullrequest approve 1
```

If no pull request is provided, `bb` fetches every open pull request in the current repository
(`GET pullrequests?state=OPEN` — this is not related to the current git branch): with exactly one
open pull request it acts on that one; with more than one it errors `too many open pullrequests,
specify one: <id>, <id>, ...`; with none it errors `no open pullrequest found for repository
<repo>`. Always pass the explicit pull request id to `approve` instead of relying on this
fallback — relying on it risks approving the wrong pull request the moment a second one is open.

You can `decline` a pull request with the `bb pullrequest decline` command:

```bash
bb pullrequest decline 1
```

If no pull request is provided, `bb` fetches every open pull request in the current repository
(`GET pullrequests?state=OPEN` — this is not related to the current git branch): with exactly one
open pull request it acts on that one; with more than one it errors `too many open pullrequests,
specify one: <id>, <id>, ...`; with none it errors `no open pullrequest found for repository
<repo>`. Always pass the explicit pull request id to `decline` instead of relying on this
fallback — relying on it risks declining the wrong pull request the moment a second one is open,
and that action is not reversible.

You can `merge` a pull request with the `bb pullrequest merge` command:

```bash
bb pullrequest merge 1
```

If no pull request is provided, `bb` fetches every open pull request in the current repository
(`GET pullrequests?state=OPEN` — this is not related to the current git branch): with exactly one
open pull request it acts on that one; with more than one it errors `too many open pullrequests,
specify one: <id>, <id>, ...`; with none it errors `no open pullrequest found for repository
<repo>`. Always pass the explicit pull request id to `merge` instead of relying on this
fallback — relying on it risks merging the wrong pull request the moment a second one is open,
and that action is not reversible.

`merge` always asks `Merge pullrequest <id>? [y/N]` before sending the request, and — unlike
every other confirmation-gated command in this fork — there is **no `--force`** to skip it, in
any form: merging is deliberately not automatable via `bb`. `--dry-run` skips the prompt
entirely (and sends no write), including on non-interactive stdin — it is the one way to preview
a merge without answering the prompt. Declining prints `Merge canceled` and exits `0`. What
counts as "asking" depends on what stdin actually is:

- Piped, redirected, or otherwise non-interactive stdin (`echo y | bb pullrequest merge 1`, `<
  file`) errors immediately, before any prompt is shown: `cannot confirm merge: Merge pullrequest
  <id>?: merging requires an interactive terminal`.
- `/dev/null` on stdin (cron, `nohup`, most agent harnesses) passes the character-device check —
  `/dev/null` IS a character device — but the read immediately returns EOF with nothing typed.
  That EOF-without-input case is also an error, with the same message: nobody answered, so nothing
  is allowed to look like a handled decline. (An interactive session pressing Ctrl-D at the prompt
  hits this same rule, so it now exits non-zero with that error instead of printing `Merge
  canceled`.)
- A real terminal or pty prompts and blocks waiting for a line of input — including a pty an
  automated process is puppeting, which `bb` cannot distinguish from a human.

You can also merge the pull request asynchronously with the `--async` flag:

```bash
bb pullrequest merge 1 --async
```

In that case, you will receive a merge task ID in return, and you can check the status of the merge task with the `bb pullrequest merge-status` command:

```bash
bb pullrequest merge-status 1 --task-id 6a0ddb61-40cf-4224-b9e8-bbb5852c66ba
```

If no pull request is provided, `bb` fetches every open pull request in the current repository
(`GET pullrequests?state=OPEN` — this is not related to the current git branch): with exactly one
open pull request it acts on that one; with more than one it errors `too many open pullrequests,
specify one: <id>, <id>, ...`; with none it errors `no open pullrequest found for repository
<repo>`.

You can request changes on a pull request with the `bb pullrequest request-changes` command:

```bash
bb pullrequest request-changes 1
```

If no pull request is provided, `bb` fetches every open pull request in the current repository
(`GET pullrequests?state=OPEN` — this is not related to the current git branch): with exactly one
open pull request it acts on that one; with more than one it errors `too many open pullrequests,
specify one: <id>, <id>, ...`; with none it errors `no open pullrequest found for repository
<repo>`. Always pass the explicit pull request id to `request-changes` instead of relying on this
fallback — relying on it risks requesting changes on the wrong pull request the moment a second
one is open.

To remove the request for changes, you can use the `bb pullrequest remove-request-changes` command:

```bash
bb pullrequest remove-request-changes 1
```

If no pull request is provided, `bb` fetches every open pull request in the current repository
(`GET pullrequests?state=OPEN` — this is not related to the current git branch): with exactly one
open pull request it acts on that one; with more than one it errors `too many open pullrequests,
specify one: <id>, <id>, ...`; with none it errors `no open pullrequest found for repository
<repo>`. Always pass the explicit pull request id to `remove-request-changes` instead of relying
on this fallback — relying on it risks acting on the wrong pull request the moment a second one
is open.

You can see the activities of a pull request with the `bb pullrequest activities` command:

```bash
bb pullrequest activities 1
```

If no pull request is provided, `bb` fetches every open pull request in the current repository
(`GET pullrequests?state=OPEN` — this is not related to the current git branch): with exactly one
open pull request it acts on that one; with more than one it errors `too many open pullrequests,
specify one: <id>, <id>, ...`; with none it errors `no open pullrequest found for repository
<repo>`.

An activity kind BitBucket adds in the future that this version of `bb` does not recognize is
silently dropped from the printed list (rather than failing the whole command) and reported once,
per distinct unknown kind, as a `[WARN]` line on stderr.

You can list the commits of a pull request with the `bb pullrequest commits` command:

```bash
bb pullrequest commits 1
```

If no pull request is provided, `bb` fetches every open pull request in the current repository
(`GET pullrequests?state=OPEN` — this is not related to the current git branch): with exactly one
open pull request it acts on that one; with more than one it errors `too many open pullrequests,
specify one: <id>, <id>, ...`; with none it errors `no open pullrequest found for repository
<repo>`.

You can see the diff of a pull request with the `bb pullrequest diff` command:

```bash
bb pullrequest diff 1
```

If no pull request is provided, `bb` fetches every open pull request in the current repository
(`GET pullrequests?state=OPEN` — this is not related to the current git branch): with exactly one
open pull request it acts on that one; with more than one it errors `too many open pullrequests,
specify one: <id>, <id>, ...`; with none it errors `no open pullrequest found for repository
<repo>`.

You can get the diffstat of a pull request with the `bb pullrequest diff --stat` command:

```bash
bb pullrequest diff --stat 1
```

If no pull request is provided, `bb` fetches every open pull request in the current repository
(`GET pullrequests?state=OPEN` — this is not related to the current git branch): with exactly one
open pull request it acts on that one; with more than one it errors `too many open pullrequests,
specify one: <id>, <id>, ...`; with none it errors `no open pullrequest found for repository
<repo>`.

You can also get the patch of a pull request with the `bb pullrequest patch` command:

```bash
bb pullrequest patch 1
```

If no pull request is provided, `bb` fetches every open pull request in the current repository
(`GET pullrequests?state=OPEN` — this is not related to the current git branch): with exactly one
open pull request it acts on that one; with more than one it errors `too many open pullrequests,
specify one: <id>, <id>, ...`; with none it errors `no open pullrequest found for repository
<repo>`.

#### Pull Request Comments

Every `pullrequest comment` subcommand takes the pull request id as its first, required
positional argument (there is no `--pullrequest` flag).

You can list the comments of a pull request with the `bb pullrequest comment list` command:

```bash
bb pullrequest comment list 1
```

You can add a comment to a pull request with the `bb pullrequest comment create` or `bb pullrequest comment add` command:

```bash
bb pullrequest comment add 1 \
  --comment "My comment" \
  --file    README.md \
  --line    404
```

`--line` anchors the comment to line 404 of `README.md` as it reads at the pull request's head
(the **new** side of the diff) — this is the line number you'd get counting in the file as it
exists on the branch. Use `--from` instead of `--line` to anchor to a line number in the file's
**old** (base) version, e.g. to comment on a line that was deleted. `--line` and `--from` are
mutually exclusive; there is no `--to`.

`--file` names a path inside the pull request's own diff, not a local file: every invocation
(with or without `--dry-run`) validates it against the pull request's actual diffstat before
sending the comment, which is deliberately stricter than what the write endpoint itself enforces.
`README.md` above must actually be a file the pull request's diff touches, or the command fails
before any comment is created.

A markdown comment body containing backticks or `$(...)` is a shell command-substitution hazard
on the command line. `--comment-file <path>` reads the body from a file instead, or from stdin
with `--comment-file -`; it is mutually exclusive with `--comment`, and exactly one of the two is
required:

```bash
bb pullrequest comment add 1 --comment-file review-notes.md
```

```bash
bb pullrequest comment add 1 --comment-file -  <<'EOF'
Looks good, but run `go test ./...` and check $(git diff) first.
EOF
```

`bb pullrequest comment update` accepts the same `--comment`/`--comment-file` pair.

You can resolve a comment with the `bb pullrequest comment resolve` command:

```bash
bb pullrequest comment resolve 1 452466
```

You can re-open a comment with the `bb pullrequest comment reopen` command:

```bash
bb pullrequest comment reopen 1 452466
```

You can get the details of a comment with the `bb pullrequest comment get` or `bb pullrequest comment show` command:

```bash
bb pullrequest comment get 1 452466
```

You can update a comment with the `bb pullrequest comment update` command:

```bash
bb pullrequest comment update 1 452466 \
  --comment "My comment"
```

You can delete a comment with the `bb pullrequest comment delete` command:

```bash
bb pullrequest comment delete 1 452466
```

#### Pull Request Tasks

Every `pullrequest task` subcommand takes the pull request id as its first, required
positional argument (there is no `--pullrequest` flag).

You can list the tasks of a pull request with the `bb pullrequest task list` command:

```bash
bb pullrequest task list 1
```

You can add a task to a pull request with the `bb pullrequest task create` or `bb pullrequest task add` command:

```bash
bb pullrequest task create 1 \
  --content "My task"
```

You can reference a comment in a task with the `--comment` flag:

```bash
bb pullrequest task create 1 \
  --content "My task" \
  --comment 452466
```

You can complete a task with the `bb pullrequest task update` command:

```bash
bb pullrequest task update 1 7643545 \
  --state RESOLVED
```

You can also re-open a task with the same command:

```bash
bb pullrequest task update 1 7643545 \
  --state UNRESOLVED
```

You can delete a task with the `bb pullrequest task delete` command:

```bash
bb pullrequest task delete 1 7643545
```

### Pipelines

You can list the pipelines of the current repository with the `bb pipeline list` command (aliases `pipelines`, `pipe`, `pp`):

```bash
bb pipeline list
```

You can get the details of a pipeline by its UUID or build number with the `bb pipeline get` command:

```bash
bb pipeline get 123456
```

You can trigger a new pipeline with the `bb pipeline trigger` command (aliases `run`, `start`, `create`):

```bash
bb pipeline trigger --branch master --variable KEY1=VALUE1 --variable KEY2=VALUE2
```

By default, the pipeline is triggered on the current git branch (`--branch` overrides it). You can pin the target to a specific `--commit`, trigger a repository's custom pipeline definition with `--pattern` (e.g. `--pattern deploy-to-prod`), or trigger it for a pull request instead with `--pullrequest` (Bitbucket then resolves the source/destination/commit server-side; `--pullrequest` is not compatible with `--branch`, `--commit`, or `--pattern`):

```bash
bb pipeline trigger --pullrequest 42
bb pipeline trigger --branch master --pattern deploy-to-prod
```

`--variable KEY=VALUE` may be repeated; each entry must contain `=` and a non-empty `KEY` or the command errors before sending anything. Variable *values* are never logged, even under `--debug` — only their keys are. `--variable` has no way to mark a variable as secured (Bitbucket-side "secured" variables can only be managed from the web UI or the variables API directly).

`trigger` asks for a `y`/`N` confirmation before sending the request. Pass `--force` to skip the prompt, or `--dry-run` to preview what would be sent without ever showing the prompt:

```bash
bb pipeline trigger --branch master --force
```

Declining the confirmation prints `Trigger canceled` and exits `0` (not an error) — same for `bb pipeline stop`'s equivalent `Stop canceled`. On success, `trigger` prints the newly created pipeline, honoring `--output` like any other command.

With stdin on `/dev/null` and no `--force` (cron, `nohup`, most agent harnesses), `trigger`/`stop` still print the prompt, read EOF, and treat that as a silent decline — `Trigger canceled`/`Stop canceled`, exit `0` — one more reason scripts driving these commands should pass `--force`.

> [!NOTE]
> There is no `--tag` target (the upstream `tag` package stays removed) and no
> `--show-logs-command`/`logs-command` output.

You can stop a running pipeline with the `bb pipeline stop` command (aliases `cancel`, `abort`), which asks for the same `y`/`N` confirmation (or accepts `--force`):

```bash
bb pipeline stop 123456
```

#### Pipeline Steps

> [!IMPORTANT]
> The pipeline is a required positional argument on every command in this subgroup — unlike `bb
> commit get`, there is no "latest pipeline" fallback when it is omitted. `<pipeline-step-uuid-or-name>`
> accepts either form: a UUID is used as-is, a name is resolved against the pipeline's steps
> (case-insensitively, trimmed). An unknown name errors listing the available step names; a name
> matching more than one step (BitBucket allows duplicate step names) errors listing the ambiguous
> candidates and their UUIDs, and asks you to pass a UUID instead.

You can list the steps of a pipeline with the `bb pipeline step list` command:

```bash
bb pipeline step list 123456
```

You can get the details of a step with the `bb pipeline step get` command:

```bash
bb pipeline step get 123456 {stepUUID}
bb pipeline step get 123456 "Build and Test"
```

You can get the logs, test report, and test cases of a step with the `bb pipeline step logs`, `bb pipeline step report`, and `bb pipeline step cases` commands:

```bash
bb pipeline step logs   123456 {stepUUID}
bb pipeline step report 123456 {stepUUID}
bb pipeline step cases  123456 {stepUUID}
```

### Artifacts

> [!IMPORTANT]
> `bb artifact` operates on a repository's **Downloads** (the files attached under a repository's
> "Downloads" tab in the Bitbucket web UI), not on a pipeline *build* artifact. There is no `bb`
> command for pipeline build artifacts specifically; despite appearing right after the Pipeline
> Steps section above, `bb artifact` is a repository-level feature, unrelated to any particular
> pipeline run.

You can list the artifacts (repository downloads) of the current repository with the `bb artifact list` command:

```bash
bb artifact list
```

You can download one or more artifacts by name with the `bb artifact download` command (aliases `get`, `fetch`):

```bash
bb artifact download myartifact.zip
bb artifact download myartifact.zip other.zip --destination ./downloads
```

`--destination` defaults to the current directory and must already exist. Each artifact is written under the base name of its `<name>` (any directory components are stripped, so a name cannot write outside the destination directory), overwriting a file already there and preserving that file's existing permissions (a newly downloaded file gets a fixed mode of 0644, not the process umask-adjusted mode and not a restricted one); a download only replaces the destination file once it has completed successfully, so a failed attempt never leaves a stray empty or partial file behind, nor corrupts a file already there.

> [!NOTE]
> There is no `bb artifact upload`/`delete`, and no `--progress` flag.

### Completion

`bb` supports completion for Bash, fish, Powershell, and zsh.

#### Bash

To enable completion, run the following command:

```bash
source <(bb completion bash)
```

You can also add this line to your `~/.bashrc` file to enable completion for every new shell.

```bash
bb completion bash >> ~/.bashrc
```

#### fish

To enable completion, run the following command:

```bash
bb completion fish | source
```

You can also add this line to your `~/.config/fish/config.fish` file to enable completion for every new shell.

```bash
bb completion fish > ~/.config/fish/completions/bb.fish
```

#### Powershell

To enable completion, run the following command:

```pwsh
bb completion powershell | Out-String | Invoke-Expression
```

You can also add the output of the above command to your `$PROFILE` file to enable completion for every new shell.

#### zsh

To enable completion, run the following command:

```bash
source <(bb completion zsh)
```

You can also add this line to your functions folder to enable completion for every new shell.

```bash
bb completion zsh > "~/${fpath[1]}/_bb"
```

On macOS, you can add the completion to the brew functions:

```bash
bb completion zsh > "$(brew --prefix)/share/zsh/site-functions/_bb"
```

### Agent skill

`bb install-skill [--to <dir>]` writes an embedded [Claude Code](https://claude.com/claude-code)
skill to `<to>/skills/bitbucket-cli`, defaulting `--to` to `$CLAUDE_CONFIG_DIR` when that
environment variable is set, or `~/.claude` otherwise. The skill teaches an agent
the full command surface documented in this README — profile setup, pull request/comment/task
management, pipeline triggering and log inspection, repository/workspace/commit/branch reads, and
artifact download — including the fork-specific behaviors an agent needs to get right (positional
pull request/pipeline ids, the `pipeline trigger`/`stop` confirmation prompt and `--force`,
`pullrequest merge`'s interactive-only confirmation with no `--force` (an agent should hand the
merge off to the user rather than invoke it directly),
`--comment-file`/`--description-file` for shell-quoting-hazard-free bodies, `--dry-run`'s
guarantee against sending writes without guaranteeing zero network traffic on reads, and
output-format/masking rules).
Re-running the command always replaces the destination directory wholesale — any files you added
under it yourself since the last install are deleted along with it, not merged — so it stays in
sync with whatever skill content shipped in the `bb` binary you ran it with. `install-skill`
itself honors `--dry-run`: it prints the destination it would write to and exits without
touching the filesystem.

```bash
bb install-skill
```

### Obtaining logs for debugging

`bb` always logs to stderr; there is no log-file flag or `LOG_DESTINATION`/`LOG_LEVEL`
environment variable. By default only `[WARN]`/`[ERROR]` lines are shown. Pass
`--debug` to also see `[DEBUG]` lines, with the source file, line number, and function
name of each log call:

```bash
bb --debug pullrequest list
```

Redirect stderr to a file if you want to capture the output:

```bash
bb --debug pullrequest list 2> bb.log
```

**Note**: `bb` tries hard to not log sensitive information, but be careful when
sharing logs, and make sure to remove any sensitive information before sharing them.

## Upgrading from upstream 0.18.x

This fork diverges from [gildas/bitbucket-cli](https://github.com/gildas/bitbucket-cli) 0.18.4 in
ways that break existing scripts and installs:

- **Install source moved.** The Homebrew tap is now `avitsrimer/apps` (`brew tap avitsrimer/apps
  && brew install --cask bb`), not `gildas/tap/bitbucket-cli`; the snap/chocolatey/scoop packages
  are gone. `go install` now targets
  `github.com/avitsrimer/bitbucket-cli/cmd/bb@latest` — the module path changed along with it.
- **Six command groups restored, narrower than upstream**: `repository`/`repo` (`get`, `list`,
  `clone` only — no create/delete/fork/update, no `get --forks`), `workspace` (`get`, `list`,
  `members` only — no permission administration), `branch` (`list` only), `commit` (`get`,
  `list`, `diff`, `patch`), `pipeline` (`get`, `list`, `trigger`, `stop`, plus the `step`
  subgroup — no `--tag` target, no `--show-logs-command`), and `artifact` (`list`, `download`
  only — no upload/delete, no `--progress`). `pipeline trigger`/`stop` ask for a `y`/`N`
  confirmation prompt with `--force` to skip it; `pullrequest merge` also asks for one, but
  **breaks scripts**: it has no `--force` at all, and a piped or `/dev/null` stdin now errors
  (`...: merging requires an interactive terminal`) instead of merging unattended.
- **Eight command groups remain removed**: `project`, `issue`, `tag`, `gpg-key`, `ssh-key`,
  `cache`, `remote`, `component`, and the deprecated `pullrequest activity` alias (use
  `pullrequest activities`).
- **`--pullrequest` is gone from every `pullrequest comment`/`pullrequest task` subcommand.** The
  pull request id is now a required first positional argument on all of them (e.g. `bb
  pullrequest comment list 1`, `bb pullrequest task delete 1 452466`); top-level `pullrequest`
  commands (`get`, `approve`, ...) are unaffected.
- **`--pipeline` is gone from every `pipeline step` subcommand.** The pipeline is now a required
  first positional, and the step is the next positional, identified by its UUID or its name (e.g.
  `bb pipeline step get 42 Build`); an ambiguous or unknown step name fails with the available
  names listed.
- **`--log <file>` / `-l` and the `LOG_DESTINATION`/`LOG_LEVEL`/`DEBUG` environment variables are
  gone.** Logs always go to stderr; use `--debug 2> bb.log` instead — see
  [Obtaining logs for debugging](#obtaining-logs-for-debugging).
- **`bb cache clear` is gone.** Delete the cache directory directly instead — see [Cache](#cache).
- **Config file format is unchanged** (plain YAML, same `profiles:` shape) but the on-disk
  filename is `config-cli.yml`, not `config-cli.json` as some older docs suggested — see
  [Profiles](#profiles).
- **`bb pullrequest comment create`/`update --line` now anchors to the new (head) side of the
  file, not the old (base) side.** Previously `--line` was sent as the API's `inline.from`
  (old-side), which mis-anchored the comment whenever lines were added or removed above it;
  `--line` now maps to `inline.to` (new-side), matching how users actually count line numbers —
  against the file as it reads in the pull request. Use `--from` to anchor to the old side (e.g.
  a deleted line) instead. `--to` is removed — it never had working "range end" semantics.

## Maturity

> [!WARNING]
> **This fork is under active development and maintained independently, on a best-effort basis.**
> There are no on-call support or SLAs. Bugs and issues are tracked and addressed as time allows.

This project should be considered a personal, opinionated fork, not an officially supported release of `bitbucket-cli`. Pull request, user, profile, pipeline, repository (read-only + clone), workspace (read-only), commit/branch (read-only), and artifact (list/download) management are the only features kept working; every other command group from upstream, and every admin/destructive verb of the ones above, has been removed from this fork.
