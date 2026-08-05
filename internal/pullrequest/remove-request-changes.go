package pullrequest

import (
	"fmt"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	prcommon "github.com/avitsrimer/bitbucket-cli/internal/pullrequest/common"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var removeRequestChangesCmd = &cobra.Command{
	Use:               "remove-request-changes [flags] <pullrequest-id>",
	Aliases:           []string{"removeRequestChanges", "remove-requestChanges", "removerequestchanges", "cancel-request-changes"},
	Short:             "Remove request changes on a pullrequest by its <pullrequest-id>. If not provided, it will try to remove request changes on the only open pullrequest.",
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: removeRequestChangesValidArgs,
	RunE:              removeRequestChangesProcess,
}

func init() {
	Command.AddCommand(removeRequestChangesCmd)
}

func removeRequestChangesValidArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	ids, err := prcommon.GetPullRequestIDsWithState(cmd.Context(), cmd, "OPEN")
	if err != nil {
		cobra.CompErrorln(err.Error())
		return []string{}, cobra.ShellCompDirectiveError
	}
	lgr.Printf("[DEBUG] fetched %d pullrequest ids", len(ids))
	return common.FilterValidArgs(ids, args, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func removeRequestChangesProcess(cmd *cobra.Command, args []string) (err error) {
	profile, err := profile.GetProfileFromCommand(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot remove request changes on pull request: %w", err)
	}

	repository, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot remove request changes on pull request: %w", err)
	}

	pullRequestID, err := GetPullRequestIDFromArgs(cmd.Context(), cmd, repository, args)
	if err != nil {
		return fmt.Errorf("cannot remove request changes on pull request: %w", err)
	}

	if !common.WhatIf(cmd, "Removing request changes on pullrequest %s", pullRequestID) {
		return nil
	}

	err = profile.Delete(
		cmd.Context(),
		cmd,
		repository.GetPath("pullrequests", pullRequestID, "request-changes"),
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to remove request changes on pull request %s: %w", pullRequestID, err)
	}
	return nil
}
