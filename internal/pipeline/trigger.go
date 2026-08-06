package pipeline

import (
	"fmt"
	"strings"

	"github.com/avitsrimer/bitbucket-cli/internal/branch"
	"github.com/avitsrimer/bitbucket-cli/internal/commit"
	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

// triggerBody is the payload sent to trigger a new pipeline.
type triggerBody struct {
	Target    Target     `json:"target"`
	Variables []Variable `json:"variables,omitempty"`
}

var triggerCmd = &cobra.Command{
	Use:     "trigger",
	Aliases: []string{"run", "start", "create"},
	Short:   "trigger a new pipeline",
	Args:    cobra.NoArgs,
	RunE:    triggerProcess,
}

func init() {
	Command.AddCommand(triggerCmd)

	triggerCmd.Flags().String("branch", "", "Branch to trigger the pipeline on. Defaults to the current git branch")
	triggerCmd.Flags().String("commit", "", "Commit hash to pin the pipeline to. Not compatible with --pullrequest: BitBucket derives the commit server-side for a pull request target")
	triggerCmd.Flags().Uint64("pullrequest", 0, "Pull request ID to trigger the pipeline for. Not compatible with --branch or --commit")
	triggerCmd.Flags().String("pattern", "", "Custom pipeline pattern to run (e.g. \"deploy-to-prod\"), for a repository's custom pipeline definitions. Not compatible with --pullrequest")
	triggerCmd.Flags().StringArray("variable", []string{}, "Pipeline variable in KEY=VALUE format. Can be specified multiple times")
	triggerCmd.Flags().Bool("force", false, "Skip the confirmation prompt")

	// A pull request target's source/destination/commit are all derived server-side from the pull
	// request itself, so --branch and --commit have no effect when combined with --pullrequest;
	// reject the combination outright instead of silently discarding one of them (--branch) or
	// sending a target BitBucket is likely to reject (--commit attached to a pull request target).
	// --pattern selects a repository's custom pipeline definition, which is orthogonal to a pull
	// request target and not something BitBucket's API accepts alongside one.
	triggerCmd.MarkFlagsMutuallyExclusive("branch", "pullrequest")
	triggerCmd.MarkFlagsMutuallyExclusive("commit", "pullrequest")
	triggerCmd.MarkFlagsMutuallyExclusive("pattern", "pullrequest")

	_ = triggerCmd.RegisterFlagCompletionFunc("branch", triggerBranchValidArgs)
	_ = triggerCmd.RegisterFlagCompletionFunc("commit", triggerCommitValidArgs)
}

// triggerBranchValidArgs backs shell completion of --branch via branch.GetBranchNames.
func triggerBranchValidArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	names, err := branch.GetBranchNames(cmd.Context(), cmd, args, toComplete)
	if err != nil {
		cobra.CompErrorln(err.Error())
		return []string{}, cobra.ShellCompDirectiveError
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

// triggerCommitValidArgs backs shell completion of --commit via commit.GetCommitHashes, which
// fetches every commit unbounded by --limit (completion candidates must never be truncated by a
// flag meant to cap a *listing*'s output). --commit is a plain string flag rather than an EnumFlag
// validated at parse time: commit hashes are an unbounded identifier space, unlike --branch's
// small, enumerable set.
func triggerCommitValidArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	hashes, err := commit.GetCommitHashes(cmd.Context(), cmd, args, toComplete)
	if err != nil {
		cobra.CompErrorln(err.Error())
		return []string{}, cobra.ShellCompDirectiveError
	}
	return hashes, cobra.ShellCompDirectiveNoFileComp
}

func triggerProcess(cmd *cobra.Command, args []string) error {
	repo, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get repository: %w", err)
	}

	target, description, err := buildTriggerTarget(cmd)
	if err != nil {
		return err
	}

	variables, keys, err := parseTriggerVariables(triggerVariablesFlagValue(cmd))
	if err != nil {
		return err
	}

	payload := triggerBody{Target: target, Variables: variables}

	// The payload (and therefore every variable value, routinely a secret or a token) must never
	// be logged -- only the variable keys are, and only their names, never their values.
	if len(keys) > 0 {
		lgr.Printf("[DEBUG] triggering pipeline on %s with variable(s): %s", description, strings.Join(keys, ", "))
	} else {
		lgr.Printf("[DEBUG] triggering pipeline on %s", description)
	}

	proceed, err := common.Confirm(cmd, fmt.Sprintf("Trigger a new pipeline on %s?", description))
	if err != nil {
		return fmt.Errorf("cannot confirm pipeline trigger: %w", err)
	}
	if !proceed {
		fmt.Println("Trigger canceled")
		return nil
	}

	if !common.WhatIf(cmd, "Triggering pipeline on %s", description) {
		return nil
	}

	profileCurrent, err := profile.GetProfileFromCommand(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get profile: %w", err)
	}

	var triggered Pipeline
	if err := profileCurrent.Post(cmd.Context(), repo.GetPath("pipelines"), payload, &triggered); err != nil {
		return fmt.Errorf("cannot trigger pipeline on %s: %w", description, err)
	}
	if err := profileCurrent.Print(cmd.Context(), cmd, triggered); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}

// buildTriggerTarget resolves the flat Target payload and a human-readable description of it from
// cmd's own --branch/--commit/--pullrequest/--pattern flags (read directly off cmd, not a
// package-level variable, per CLAUDE.md's flag-reading rule, so triggerProcess behaves identically
// whether cmd is the real triggerCmd or a standalone test command carrying its own flags). A pull
// request target takes the pull request id alone (BitBucket derives its source/destination/commit
// server-side); otherwise the target is a branch reference, defaulting to the current git branch
// (branch.GetCurrentBranch) when --branch is not set, optionally pinned to --commit and/or
// selecting a custom pipeline definition via --pattern (the only way to trigger a repository's
// custom pipelines, as opposed to its default/branch pipelines).
func buildTriggerTarget(cmd *cobra.Command) (target Target, description string, err error) {
	pullRequestID, _ := cmd.Flags().GetUint64("pullrequest")
	if pullRequestID != 0 {
		target = Target{Type: "pipeline_pullrequest_target", PullRequest: &pullRequestReference{Type: "pullrequest", ID: pullRequestID}}
		description = fmt.Sprintf("pull request #%d", pullRequestID)
	} else {
		branchName := common.StringFlagValue(cmd, "branch")
		if branchName == "" {
			branchName, err = branch.GetCurrentBranch(cmd.Context())
			if err != nil {
				return Target{}, "", fmt.Errorf("cannot determine branch to trigger: use --branch or --pullrequest, or run inside a git repository: %w", err)
			}
			lgr.Printf("[DEBUG] using current branch: %s", branchName)
		}
		target = Target{Type: "pipeline_ref_target", RefType: "branch", RefName: branchName}
		description = fmt.Sprintf("branch %q", branchName)
	}

	commitHash := common.StringFlagValue(cmd, "commit")
	if commitHash != "" {
		target.Commit = &commit.CommitReference{Hash: commitHash}
	}

	pattern := common.StringFlagValue(cmd, "pattern")
	if pattern != "" {
		target.Selector = &common.Selector{Type: "custom", Pattern: pattern}
		description = fmt.Sprintf("%s (custom pipeline %q)", description, pattern)
	}
	return target, description, nil
}

// triggerVariablesFlagValue reads cmd's own --variable values.
func triggerVariablesFlagValue(cmd *cobra.Command) []string {
	return common.StringArrayFlagValue(cmd, "variable")
}

// parseTriggerVariables converts "KEY=VALUE" strings into Variables, returning the keys separately
// so callers can log them without ever touching -- let alone logging -- a value. It rejects entries
// missing "=" or carrying an empty key.
func parseTriggerVariables(raw []string) (variables []Variable, keys []string, err error) {
	if len(raw) == 0 {
		return nil, nil, nil
	}
	variables = make([]Variable, 0, len(raw))
	keys = make([]string, 0, len(raw))
	for _, entry := range raw {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 || parts[0] == "" {
			return nil, nil, fmt.Errorf("invalid --variable %q: expected KEY=VALUE with a non-empty key", entry)
		}
		variables = append(variables, Variable{Key: parts[0], Value: parts[1]})
		keys = append(keys, parts[0])
	}
	return variables, keys, nil
}
