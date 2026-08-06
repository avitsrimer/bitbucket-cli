package pullrequest

import (
	"fmt"

	"github.com/avitsrimer/bitbucket-cli/internal/commit"
	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	prcommon "github.com/avitsrimer/bitbucket-cli/internal/pullrequest/common"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/gildas/go-core"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var commitsCmd = &cobra.Command{
	Use:               "commits [flags] <pullrequest-id>",
	Short:             "Lists the commits of a pullrequest by its <pullrequest-id>",
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: commitsValidArgs,
	RunE:              commitsProcess,
}

func init() {
	Command.AddCommand(commitsCmd)

	common.RegisterListFlags(commitsCmd, commit.Columns(), "commits")
}

func commitsValidArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
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
		core.Sort(commits, commit.Columns().SortBy(sortValue))
	}
	if err := profile.Current.Print(cmd.Context(), cmd, commit.Commits(commits)); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}
