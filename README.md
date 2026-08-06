# Bitbucket Command Line Interface

[![build](https://github.com/avitsrimer/bitbucket-cli/actions/workflows/ci.yml/badge.svg?branch=master)](https://github.com/avitsrimer/bitbucket-cli/actions/workflows/ci.yml)

> [!NOTE]
> **This project is a fork of [gildas/bitbucket-cli](https://github.com/gildas/bitbucket-cli).** All credit for the original design and implementation goes to Gildas Cherruel and the upstream contributors. This fork is maintained independently of the upstream project and is **detached** from it — it does not track upstream releases and does not aim for feature parity.

`bb` is the missing command line interface for Bitbucket. It brings the power of the Bitbucket platform to your command line. Creating and merging Pull Requests and more are now just a few keystrokes away.

> [!IMPORTANT]
> **This is an opinionated fork, deliberately narrower than upstream.**
>
> The supported surface is `bb pullrequest` (full command tree), `bb user`, `bb profile`
> (authentication plumbing), `bb pipeline` (`get`, `list`, `trigger`, `stop`, plus the `step`
> subgroup: `get`, `list`, `logs`, `report`, `cases`), `bb repo`/`bb repository` (`get`, `list`,
> `clone` — read-only, no create/delete/fork/update), `bb workspace` (`get`, `list`, `members` —
> no permission administration), `bb commit`/`bb branch` (read-only: `commit get/list/diff/patch`,
> `branch list`), and `bb artifact` (`list`, `download` — no upload/delete). Every other command
> group inherited from upstream — `issue`, `tag`, `project`, `gpg-key`, `ssh-key`, `cache`,
> `remote`, `component` — remains **removed** from this fork, as does every admin/destructive verb
> of the groups above (repository create/delete/fork/update, `repo get --forks`, workspace
> permission management, pipeline `--tag` targets) and the deprecated `pullrequest activity` alias
> (use `pullrequest activities`).
>
> `bb pipeline trigger` and `bb pipeline stop` are, deliberately, the only commands in this fork
> that ask for a `y`/`N` confirmation before running (or accept `--force` to skip it) — every
> other state-changing command, including `pullrequest merge`/`decline`, runs immediately. Use
> `--dry-run` to preview any of them first.

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

## Usage

`bb` is a modern command line interface. It uses subcommands to perform actions. You can get help on any subcommand by running `bb <subcommand> --help`.

General help is also available by running `bb --help` or `bb help`.

By default `bb` works in the current git repository. You can specify a Bitbucket repository with the `--repository` flag.

Many commands and flags are dynamically auto-completed. See the [Completion](#completion) section for more information about completion.

Most `delete` commands support multiple arguments. You can pass a list of arguments or a file with one argument per line:

```bash
bb pullrequest comment delete --pullrequest 1 452466 452467 452468
```

You can tell `bb` to stop on the first error, warn on errors, or ignore errors when processing multiple arguments with the `--stop-on-error`, `--warn-on-error`, or `--ignore-errors` flags.

All commands that would modify something on Bitbucket now allow you to preview the changes before applying them. You can use the `--dry-run` flag to see what would happen.

```bash
bb pullrequest decline 1 --dry-run
```

Most commands will support the `--workspace` and `--repository` flags to specify the workspace and repository to use. If not provided, the workspace and repository will be determined from the git configuration or the profile configuration (in that order). The workspace and repository can be combined in the `--repository` flag in the form `workspace/repository`. For example:

```bash
bb pullrequest list --repository myrepository
bb pullrequest list --workspace myworkspace --repository myrepository
bb pullrequest list --repository myworkspace/myrepository
```

The `--workspace` flag is also dynamically auto-completed with the workspaces you have access to.

`get` and `list` commands support the `--columns` flag to specify which columns to display in the output. You can pass a comma-separated list of columns, repeat the flag, or use `all` to display all columns. If you do not provide this flag, the default columns are displayed.

```bash
bb pullrequest list --columns all
bb pullrequest list --columns id,title,state
bb pullrequest list --columns id --columns title
```

`list` commands also support the `--sort` flag to sort the output by a specific column. You can pass a comma-separated list of columns, repeat the flag, or use `all` to sort by all columns. If you do not provide this flag, the default sorting is used.

```bash
bb pullrequest list --sort title
```

`list` commands also support the `--query` flag to filter the output by a specific query. The query syntax is similar to the one used in the Bitbucket web interface. For example, to filter Pull Requests updated after a specific date:

```bash
bb pullrequest list --query 'updated_on > 2025-12-31'
```

Please refer to the [Bitbucket API documentation](https://developer.atlassian.com/cloud/bitbucket/rest/intro/#filtering) for more information about the supported query syntax and fields.

`list` commands also support the 'page-length' flag to set the number of items to retrieve per request to Bitbucket API at a time. By default, the page length is set on the profile and the default is 50. You can set it to a value between 1 and 100.

```bash
bb pullrequest list --page-length 25
```

### Output

`bb` outputs a table by default. You can change the output format with the `--output` flag,  by setting the `BB_OUTPUT_FORMAT` environment variable, or by modifying the profile configuration (See [Profiles](#profiles)).

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
+----+---------------------------+--------------------------------+---------------------+-------------+----------+
| ID |           TITLE           |          DESCRIPTION           |       SOURCE        | DESTINATION |  STATE   |
+----+---------------------------+--------------------------------+---------------------+-------------+----------+
|  1 | Merge feature/links       | Feature links. Do not delete   | feature/links       | dev         | DECLINED |
|    |                           | the feature branch after the   |                     |             |          |
|    |                           | merge.                         |                     |             |          |
|  2 | Merge feature/links       | Feature links. Do not delete   | feature/links       | dev         | MERGED   |
|    |                           | the feature branch after the   |                     |             |          |
|    |                           | merge.                         |                     |             |          |
|  3 | Merge release/1.0.0       | Feature 1.0.0. Do not delete   | release/1.0.0       | master      | MERGED   |
|    |                           | the feature branch after the   |                     |             |          |
|    |                           | merge.                         |                     |             |          |
|  4 | Merge feature/bb          | Feature bb. Do not delete the  | feature/bb          | dev         | DECLINED |
|    |                           | feature branch after the merge |                     |             |          |
|  5 | Merge feature/bb          | Feature bb. Do not delete the  | feature/bb          | dev         | MERGED   |
|    |                           | feature branch after the merge |                     |             |          |
|  6 | Merge feature/bb-doc      | Feature bb-doc. Do not delete  | feature/bb-doc      | dev         | MERGED   |
|    |                           | the feature branch after the   |                     |             |          |
|    |                           | merge.                         |                     |             |          |
+----+---------------------------+--------------------------------+---------------------+-------------+----------+
```

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

You should define the default workspace for the profile with the `--default-workspace` flag. This will allow you to use `bb` without specifying the workspace every time.

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

Permission Scopes:

- [OAuth 2.0 scopes](https://developer.atlassian.com/cloud/bitbucket/rest/intro/#bitbucket-oauth-2-0-scopes)
- [API token permissions](https://support.atlassian.com/bitbucket-cloud/docs/api-token-permissions/)

When you use a user/password, the password is stored in the vault of the operating system (Windows Credential Manager, macOS Keychain, or Linux Secret Service). You can pass the `--no-vault` flag to disable this feature and store the password in plain text in the configuration file. This is not recommended, but can be useful for testing purposes. On Linux and macOS, you can also pass the `--vault-key` flag to set the key to use in the system keychain. By default, the key is `bitbucket-cli`. On Windows, this option is not available.

> [!NOTE]
> `--clone-protocol` and `--clone-user` are functional again: `bb repo clone` uses them as the
> default protocol (overridable per invocation with `--protocol`) and, for the `https` protocol,
> the username embedded in the clone URL.

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

If you do not provide a workspace, the one resolved from `--workspace`/git config/profile default is used. You can narrow the list down to repositories you have a given role in with `--role` (`all`, `owner`, `admin`, `contributor`, `member`; default `owner`):

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

You can get the details of a commit with the `bb commit get` command. If you don't provide a hash, the latest commit is used:

```bash
bb commit get 123456
bb commit get
```

You can get the diff between two commits with the `bb commit diff` command (or between one commit and its parent, if only one hash is given), and the diffstat alone with `--stat`:

```bash
bb commit diff 123456 654321
bb commit diff 123456
bb commit diff --stat 123456
```

You can get the patch between two commits with the `bb commit patch` command:

```bash
bb commit patch 123456 654321
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

If the first reviewer is `default`, the command will try to get the default reviewers from the project settings.

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

To add or remove reviewers from a pull request, you can use the `--add-reviewer` and `--remove-reviewer` flags:

```bash
bb pullrequest update 1 \
  --add-reviewer    username1 --add-reviewer {userUUID2} \
  --remove-reviewer username3 --remove-reviewer {userUUID4}
```

You can `approve`or `unapprove` a pull request with the `bb pullrequest approve` or `bb pullrequest unapprove` command:

```bash
bb pullrequest approve 1
```

If no pull request is provided, the command will try to approve the opened pull request with the current branch.

You can `decline` a pull request with the `bb pullrequest decline` command:

```bash
bb pullrequest decline 1
```

If no pull request is provided, the command will try to decline the opened pull request with the current branch.

You can `merge` a pull request with the `bb pullrequest merge` command:

```bash
bb pullrequest merge 1
```

If no pull request is provided, the command will try to merge the opened pull request with the current branch.

You can also merge the pull request asynchronously with the `--async` flag:

```bash
bb pullrequest merge 1 --async
```

In that case, you will receive a merge task ID in return, and you can check the status of the merge task with the `bb pullrequest merge-status` command:

```bash
bb pullrequest merge-status 1 --task-id 6a0ddb61-40cf-4224-b9e8-bbb5852c66ba
```

If no pull request is provided, the command will try to request the merge status of the opened pull request with the current branch.

You can request changes on a pull request with the `bb pullrequest request-changes` command:

```bash
bb pullrequest request-changes 1
```

If no pull request is provided, the command will try to request changes on the opened pull request with the current branch.

To remove the request for changes, you can use the `bb pullrequest remove-request-changes` command:

```bash
bb pullrequest remove-request-changes 1
```

If no pull request is provided, the command will try to remove the request for changes on the opened pull request with the current branch.

You can see the activities of a pull request with the `bb pullrequest activities` command:

```bash
bb pullrequest activities 1
```

If no pull request is provided, the command will try to list the activities of the opened pull request with the current branch.

You can list the commits of a pull request with the `bb pullrequest commits` command:

```bash
bb pullrequest commits 1
```

If no pull request is provided, the command will try to list the commits of the opened pull request with the current branch.

You can see the diff of a pull request with the `bb pullrequest diff` command:

```bash
bb pullrequest diff 1
```

If no pull request is provided, the command will try to show the diff of the opened pull request with the current branch.

You can get the diffstat of a pull request with the `bb pullrequest diff --stat` command:

```bash
bb pullrequest diff --stat 1
```

If no pull request is provided, the command will try to show the diffstat of the opened pull request with the current branch.

You can also get the patch of a pull request with the `bb pullrequest patch` command:

```bash
bb pullrequest patch 1
```

If no pull request is provided, the command will try to show the patch of the opened pull request with the current branch.

#### Pull Request Comments

You can list the comments of a pull request with the `bb pullrequest comment list` command:

```bash
bb pullrequest comment list --pullrequest 1
```

You can add a comment to a pull request with the `bb pullrequest comment create` or `bb pullrequest comment add` command:

```bash
bb pullrequest comment add --pullrequest 1 \
  --comment "My comment" \
  --file    README.md \
  --line    404
```

You can resolve a comment with the `bb pullrequest comment resolve` command:

```bash
bb pullrequest comment resolve --pullrequest 1 452466
```

You can re-open a comment with the `bb pullrequest comment reopen` command:

```bash
bb pullrequest comment reopen --pullrequest 1 452466
```

You can get the details of a comment with the `bb pullrequest comment get` or `bb pullrequest comment show` command:

```bash
bb pullrequest comment get --pullrequest 1 452466
```

You can update a comment with the `bb pullrequest comment update` command:

```bash
bb pullrequest comment update --pullrequest 1 452466 \
  --comment "My comment"
```

You can delete a comment with the `bb pullrequest comment delete` command:

```bash
bb pullrequest comment delete --pullrequest 1 452466
```

#### Pull Request Tasks

You can list the tasks of a pull request with the `bb pullrequest task list` command:

```bash
bb pullrequest task list --pullrequest 1
```

You can add a task to a pull request with the `bb pullrequest task create` or `bb pullrequest task add` command:

```bash
bb pullrequest task create --pullrequest 1 \
  --content "My task"
```

You can reference a comment in a task with the `--comment` flag:

```bash
bb pullrequest task create --pullrequest 1 \
  --content "My task" \
  --comment 452466
```

You can complete a task with the `bb pullrequest task update` command:

```bashbb pullrequest task update --pullrequest 1 7643545 \
  --state RESOLVED
```

You can also re-open a task with the same command:

```bashbb pullrequest task update --pullrequest 1 7643545 \
  --state UNRESOLVED
```

You can delete a task with the `bb pullrequest task delete` command:

```bash
bb pullrequest task delete --pullrequest 1 7643545
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

By default, the pipeline is triggered on the current git branch (`--branch` overrides it). You can pin the target to a specific `--commit`, or trigger it for a pull request instead with `--pullrequest` (Bitbucket then resolves the source/destination/commit server-side):

```bash
bb pipeline trigger --pullrequest 42
```

`trigger` asks for a `y`/`N` confirmation before sending the request. Pass `--force` to skip the prompt, or `--dry-run` to preview what would be sent without ever showing the prompt:

```bash
bb pipeline trigger --branch master --force
```

> [!NOTE]
> There is no `--tag` target (the upstream `tag` package stays removed) and no
> `--show-logs-command`/`logs-command` output.

You can stop a running pipeline with the `bb pipeline stop` command (aliases `cancel`, `abort`), which asks for the same `y`/`N` confirmation (or accepts `--force`):

```bash
bb pipeline stop 123456
```

#### Pipeline Steps

You can list the steps of a pipeline with the `bb pipeline step list` command:

```bash
bb pipeline step list --pipeline 123456
```

You can get the details of a step with the `bb pipeline step get` command:

```bash
bb pipeline step get --pipeline 123456 {stepUUID}
```

You can get the logs, test report, and test cases of a step with the `bb pipeline step logs`, `bb pipeline step report`, and `bb pipeline step cases` commands:

```bash
bb pipeline step logs   --pipeline 123456 {stepUUID}
bb pipeline step report --pipeline 123456 {stepUUID}
bb pipeline step cases  --pipeline 123456 {stepUUID}
```

### Artifacts

You can list the artifacts of the current repository with the `bb artifact list` command:

```bash
bb artifact list
```

You can download one or more artifacts by name with the `bb artifact download` command (aliases `get`, `fetch`):

```bash
bb artifact download myartifact.zip
bb artifact download myartifact.zip other.zip --destination ./downloads
```

`--destination` defaults to the current directory and must already exist. Each artifact is written under the base name of its `<name>` (any directory components are stripped, so a name cannot write outside the destination directory), overwriting a file already there; a download only replaces the destination file once it has completed successfully, so a failed attempt never leaves a stray empty or partial file behind.

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
  only — no upload/delete, no `--progress`). `pipeline trigger`/`stop` are the only commands in
  this fork with a `y`/`N` confirmation prompt (`--force` to skip it).
- **Eight command groups remain removed**: `project`, `issue`, `tag`, `gpg-key`, `ssh-key`,
  `cache`, `remote`, `component`, and the deprecated `pullrequest activity` alias (use
  `pullrequest activities`).
- **`--log <file>` / `-l` and the `LOG_DESTINATION`/`LOG_LEVEL`/`DEBUG` environment variables are
  gone.** Logs always go to stderr; use `--debug 2> bb.log` instead — see
  [Obtaining logs for debugging](#obtaining-logs-for-debugging).
- **`bb cache clear` is gone.** Delete the cache directory directly instead — see [Cache](#cache).
- **Config file format is unchanged** (plain YAML, same `profiles:` shape) but the on-disk
  filename is `config-cli.yml`, not `config-cli.json` as some older docs suggested — see
  [Profiles](#profiles).

## Maturity

> [!WARNING]
> **This fork is under active development and maintained independently, on a best-effort basis.**
> There are no on-call support or SLAs. Bugs and issues are tracked and addressed as time allows.

This project should be considered a personal, opinionated fork, not an officially supported release of `bitbucket-cli`. Pull request, user, profile, pipeline, repository (read-only + clone), workspace (read-only), commit/branch (read-only), and artifact (list/download) management are the only features kept working; every other command group from upstream, and every admin/destructive verb of the ones above, has been removed from this fork.
