package pullrequest

import (
	"fmt"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	prcommon "github.com/avitsrimer/bitbucket-cli/internal/pullrequest/common"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/avitsrimer/bitbucket-cli/internal/user"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var requestChangesCmd = &cobra.Command{
	Use:               "request-changes [flags] <pullrequest-id>",
	Aliases:           []string{"requestChanges", "requestchanges"},
	Short:             "Request changes on a pullrequest by its <pullrequest-id>. If not provided, it will try to request changes on the only open pullrequest.",
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: requestChangesValidArgs,
	RunE:              requestChangesProcess,
}

func init() {
	Command.AddCommand(requestChangesCmd)
}

func requestChangesValidArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
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

func requestChangesProcess(cmd *cobra.Command, args []string) (err error) {
	profile, err := profile.GetProfileFromCommand(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot request changes on pull request: %w", err)
	}

	repository, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot request changes on pull request: %w", err)
	}

	pullRequestID, err := GetPullRequestIDFromArgs(cmd.Context(), cmd, repository, args)
	if err != nil {
		return fmt.Errorf("cannot request changes on pull request: %w", err)
	}

	if !common.WhatIf(cmd, "Requesting changes on pullrequest %s", pullRequestID) {
		return nil
	}
	var participant user.Participant

	err = profile.Post(
		cmd.Context(),
		cmd,
		repository.GetPath("pullrequests", pullRequestID, "request-changes"),
		nil,
		&participant,
	)
	if err != nil {
		return fmt.Errorf("failed to request changes on pull request %s: %w", pullRequestID, err)
	}
	if err := profile.Print(cmd.Context(), cmd, participant); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}
