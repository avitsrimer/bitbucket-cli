package pullrequest

import (
	"fmt"

	"github.com/avitsrimer/bitbucket-cli/internal/commit"
	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var commitsCmd = &cobra.Command{
	Use:               "commits [flags] [<pullrequest-id>]",
	Short:             "Lists the commits of a pullrequest by its <pullrequest-id>",
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: openPullRequestIDsCompletion,
	RunE:              commitsProcess,
}

func init() {
	Command.AddCommand(commitsCmd)

	common.RegisterListFlags(commitsCmd, commit.Columns(), "commits")
}

func commitsProcess(cmd *cobra.Command, args []string) (err error) {
	repository, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot list commits of pull request: %w", err)
	}

	pullRequestID, err := GetPullRequestIDFromArgs(cmd.Context(), cmd, repository, args)
	if err != nil {
		return fmt.Errorf("cannot list commits of pull request: %w", err)
	}

	lgr.Printf("[DEBUG] listing commits of pullrequest %s", pullRequestID)
	if !common.WhatIf(cmd, "Listing commits of pullrequest %s", pullRequestID) {
		return nil
	}

	commits, err := profile.GetAll[commit.Commit](
		cmd.Context(),
		cmd,
		repository.GetPath("pullrequests", pullRequestID, "commits"),
	)
	if err != nil {
		return fmt.Errorf("failed to get the commits of pull request %s: %w", pullRequestID, err)
	}
	if sortValue := common.SortFlagValue(cmd); sortValue != "" {
		common.Sort(commits, commit.Columns().SortBy(sortValue))
	}
	if err := profile.Current.Print(cmd.Context(), cmd, commit.Commits(commits)); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}
