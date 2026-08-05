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

var mergeStatusCmd = &cobra.Command{
	Use:               "merge-status <pull-request-id>",
	Short:             "Get the status of a pull request merge task",
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: mergeStatusValidArgs,
	RunE:              mergeStatusProcess,
}

var mergeStatusOptions struct {
	TaskID string
}

func init() {
	Command.AddCommand(mergeStatusCmd)

	mergeStatusCmd.Flags().StringVar(&mergeStatusOptions.TaskID, "task-id", "", "ID of the merge task to check the status of")
	_ = mergeStatusCmd.MarkFlagRequired("task-id")
}

func mergeStatusValidArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
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

func mergeStatusProcess(cmd *cobra.Command, args []string) (err error) {
	profile, err := profile.GetProfileFromCommand(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("failed to get the profile: %w", err)
	}

	repository, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot merge pull request: %w", err)
	}

	pullRequestID, err := GetPullRequestIDFromArgs(cmd.Context(), cmd, repository, args)
	if err != nil {
		return fmt.Errorf("cannot merge pull request: %w", err)
	}

	lgr.Printf("[DEBUG] getting the pull request merge status for %s", pullRequestID)
	if !common.WhatIf(cmd, "Getting the merge status for pull request %s", pullRequestID) {
		return nil
	}

	var status PullRequestMergeStatus

	err = profile.Get(
		cmd.Context(),
		repository.GetPath("pullrequests", pullRequestID, "merge", "task-status", mergeStatusOptions.TaskID),
		&status,
	)
	if err != nil {
		return fmt.Errorf("failed to get the merge status for pull request %s: %w", pullRequestID, err)
	}
	status.ID = mergeStatusOptions.TaskID

	if err := profile.Print(cmd.Context(), cmd, status); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}
