package pullrequest

import (
	"github.com/gildas/bitbucket-cli/cmd/common"
	"github.com/gildas/bitbucket-cli/cmd/profile"
	prcommon "github.com/gildas/bitbucket-cli/cmd/pullrequest/common"
	"github.com/gildas/bitbucket-cli/cmd/repository"
	"github.com/gildas/bitbucket-cli/cmd/user"
	"github.com/gildas/go-errors"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var approveCmd = &cobra.Command{
	Use:               "approve [flags] <pullrequest-id>",
	Short:             "approve a pullrequest by its <pullrequest-id>. If not provided, it will try to approve the only open pullrequest.",
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: approveValidArgs,
	RunE:              approveProcess,
}

func init() {
	Command.AddCommand(approveCmd)
}

func approveValidArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
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

func approveProcess(cmd *cobra.Command, args []string) (err error) {
	profile, err := profile.GetProfileFromCommand(cmd.Context(), cmd)
	if err != nil {
		return errors.Join(errors.Errorf("Cannot approve Pull Request"), err)
	}

	repository, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return errors.Join(errors.Errorf("Cannot approve Pull Request"), err)
	}

	pullRequestID, err := GetPullRequestIDFromArgs(cmd.Context(), cmd, repository, args)
	if err != nil {
		return errors.Join(errors.Errorf("Cannot approve Pull Request"), err)
	}

	if !common.WhatIf(cmd, "Approving pullrequest %s", pullRequestID) {
		return nil
	}
	var participant user.Participant

	err = profile.Post(
		cmd.Context(),
		cmd,
		repository.GetPath("pullrequests", pullRequestID, "approve"),
		nil,
		&participant,
	)
	if err != nil {
		return errors.Join(errors.Errorf("Failed to approve Pull Request %s", pullRequestID), err)
	}
	return profile.Print(cmd.Context(), cmd, participant)
}
