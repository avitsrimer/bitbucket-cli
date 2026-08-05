package pullrequest

import (
	"fmt"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	prcommon "github.com/avitsrimer/bitbucket-cli/internal/pullrequest/common"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/spf13/cobra"
)

var unapproveCmd = &cobra.Command{
	Use:               "unapprove [flags] <pullrequest-id>",
	Short:             "unapprove a pullrequest by its <pullrequest-id>. If not provided, it will try to unapprove the only open pullrequest.",
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: unapproveValidArgs,
	RunE:              unapproveProcess,
}

func init() {
	Command.AddCommand(unapproveCmd)
}

func unapproveValidArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	ids, err := prcommon.GetPullRequestIDsWithState(cmd.Context(), cmd, "OPEN")
	if err != nil {
		cobra.CompErrorln(err.Error())
		return []string{}, cobra.ShellCompDirectiveError
	}
	return common.FilterValidArgs(ids, args, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func unapproveProcess(cmd *cobra.Command, args []string) (err error) {
	profile, err := profile.GetProfileFromCommand(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot unapprove pull request: %w", err)
	}

	repository, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot unapprove pull request: %w", err)
	}

	pullRequestID, err := GetPullRequestIDFromArgs(cmd.Context(), cmd, repository, args)
	if err != nil {
		return fmt.Errorf("cannot unapprove pull request: %w", err)
	}

	if !common.WhatIf(cmd, "Unapproving pullrequest %s", pullRequestID) {
		return nil
	}
	err = profile.Delete(
		cmd.Context(),
		cmd,
		repository.GetPath("pullrequests", pullRequestID, "approve"),
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to unapprove pull request %s: %w", pullRequestID, err)
	}
	return nil
}
