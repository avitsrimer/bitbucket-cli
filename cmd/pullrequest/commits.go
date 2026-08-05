package pullrequest

import (
	"github.com/gildas/bitbucket-cli/cmd/commit"
	"github.com/gildas/bitbucket-cli/cmd/common"
	"github.com/gildas/bitbucket-cli/cmd/profile"
	prcommon "github.com/gildas/bitbucket-cli/cmd/pullrequest/common"
	"github.com/gildas/bitbucket-cli/cmd/repository"
	"github.com/gildas/go-core"
	"github.com/gildas/go-errors"
	"github.com/gildas/go-flags"
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

var commitsOptions struct {
	Columns    *flags.EnumSliceFlag
	SortBy     *flags.EnumFlag
	PageLength int
}

func init() {
	Command.AddCommand(commitsCmd)

	commitsOptions.Columns = flags.NewEnumSliceFlagWithAllAllowed(commit.Commit{}.GetColumnDefinitions().Columns()...)
	commitsOptions.SortBy = flags.NewEnumFlag(commit.Commit{}.GetColumnDefinitions().Sorters()...)
	commitsCmd.Flags().Var(commitsOptions.Columns, "columns", "Comma-separated list of columns to display")
	commitsCmd.Flags().Var(commitsOptions.SortBy, "sort", "Column to sort by")
	commitsCmd.Flags().IntVar(&commitsOptions.PageLength, "page-length", 0, "Number of items per page to retrieve from Bitbucket. Default is the profile's default page length")
	_ = commitsCmd.RegisterFlagCompletionFunc(commitsOptions.Columns.CompletionFunc("columns"))
	_ = commitsCmd.RegisterFlagCompletionFunc(commitsOptions.SortBy.CompletionFunc("sort"))
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
		return errors.Join(errors.Errorf("Cannot list commits of Pull Request"), err)
	}

	pullRequestID, err := GetPullRequestIDFromArgs(cmd.Context(), cmd, repository, args)
	if err != nil {
		return errors.Join(errors.Errorf("Cannot list commits of Pull Request"), err)
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
		return errors.Join(errors.Errorf("Failed to get the commits of Pull Request %s", pullRequestID), err)
	}
	core.Sort(commits, commit.Commit{}.GetColumnDefinitions().SortBy(commitsOptions.SortBy.Value))
	return profile.Current.Print(cmd.Context(), cmd, commit.Commits(commits))
}
