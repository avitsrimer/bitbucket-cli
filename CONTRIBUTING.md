# Contributing to Bitbucket-cli

First off, thank you for taking the time to contribute to [bb](https://github.com/avitsrimer/bitbucket-cli)!

We welcome contributions from the community to help make this the best command-line interface for the Bitbucket platform.

It’s folks like you that make `bitbucket-cli` a better tool for everyone.

---

## Getting Started

1. **Fork the repository**: To get started, please **fork the repository** to your own GitHub account.
2. **Clone your fork**: Clone the forked repository to your local machine.
3. **Create a feature branch**: Create a branch for your changes, ensuring it is based off the latest code.

---

## Pull Request Guidelines

To maintain code quality and a streamlined workflow, we enforce the following rules for all Pull Requests:

### 1. Reporting Issues

If you find a bug, please check if the issue you are addressing has already been reported. If not, please create a new [issue](https://github.com/avitsrimer/bitbucket-cli/issues) with a clear description of the problem and link that issue in your Pull Request.

### 2. Target the `master` Branch

All Pull Requests **must** be targeted at the `master` branch (single-branch flow — there is no `dev` branch).

### 3. Signed Commits

Integrity is key. **All commits in Pull Requests must be signed** (GPG, SSH, or X.509).

* PRs containing unsigned commits will be closed or asked to be retargeted once the commits are signed.
* If you aren't sure how to do this, check out [GitHub's guide on signing commits](https://docs.github.com/en/authentication/managing-commit-signature-verification/about-commit-signature-verification).

### 4. Command Structure

* **This fork's scope is intentionally narrow:**  
Only the `bb pullrequest` command tree, `bb user`, and the `bb profile` authentication plumbing
they depend on are supported — see the README's `[!IMPORTANT]` note. Every other resource
inherited from upstream (`repository`, `project`, `workspace`, `issue`, `pipeline`, `branch`,
`commit`, `tag`, `artifact`, `gpg-key`, `ssh-key`, `cache`, `remote`, `component`) was removed
deliberately. **New top-level command groups are out of scope** for this fork; contributions
should extend the existing command trees (new subcommands, flags, or columns), not reintroduce a
removed resource or add a new one. `bb install-skill` is the one sanctioned exception: it ships
the embedded Claude Code skill (`skill/bitbucket-cli/`), not a Bitbucket resource, so it doesn't
reintroduce anything upstream removed. Packages now live under `internal/` (Go 1.26 minimum), not
`cmd/`.
* **Resources and commands:**  
`bb` is built as a modern CLI using subcommands. Ensure new features follow this pattern (e.g., `bb <resource> <subresource...> <command>`).  
Commands should be verbs (e.g., `list`, `create`, `delete`) and support the standard CRUD operations (Create -> `create`, Read -> `list` and `get`, Update -> `update`, Delete -> `delete`) where applicable, within the scope above.
* **Dry Run Support:**  
All commands that modify data on Bitbucket should support the --dry-run flag to allow users to preview changes.
* **Output Formats:**  
Ensure list and get commands remain compatible with various supported output formats (JSON, YAML, Table, etc.).

---

## Style & Standards

* **Formatting**:  
Ensure your code follows the standard Go language conventions (you can run `make fmt` in the project root).
* **Documentation**:  
If you are adding a feature, please update any relevant documentation or help text within the CLI and the [README.md](README.md) file.
* **Tests**:  
Verify your changes by running existing tests and adding new ones where applicable.  
If you add JSON paylods in the tests, make sure to add them in the `testdata` directory and reference them in your test code. You can find examples in the existing test files. An unreferenced file left in `testdata` will be flagged in review — delete it instead of leaving it behind.  
Please ensure that the payloads are anonymized enough and do not contain any sensitive information.  
You can run all tests with `make test`.
* **CI gate**:  
Every Pull Request must pass `go test -race ./...` and `golangci-lint run` (pinned to the version `.github/workflows/ci.yml` uses) before it can merge — this is the same gate CI enforces, so running both locally first avoids a red PR.

---

## Code of Conduct

We ask that all contributors adhere to the [Code of Conduct](CODE_OF_CONDUCT.md) to maintain a welcoming and inclusive environment for everyone.

---

## License

By contributing to Bitbucket-cli, you agree that your contributions will be licensed under the project's current license.

You can find the license details in the [LICENSE](LICENSE) file.

---

## Thank You!

Thank you again for your interest in contributing to Bitbucket-cli! We look forward to your contributions and are excited to see how you can help improve the project. If you have any questions or need assistance, please don't hesitate to reach out by opening an issue or joining our discussions. Happy coding!
