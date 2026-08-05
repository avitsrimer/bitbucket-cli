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

var getCmd = &cobra.Command{
	Use:               "get [flags] <pullrequest-id>",
	Aliases:           []string{"show", "info", "display"},
	Short:             "get a pullrequest by its <pullrequest-id>.",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: getValidArgs,
	RunE:              getProcess,
}

var getOptions struct {
	Columns *common.EnumSliceFlag
}

func init() {
	Command.AddCommand(getCmd)

	getOptions.Columns = common.NewEnumSliceFlag(columns.Columns()...)
	getCmd.Flags().Var(getOptions.Columns, "columns", "Comma-separated list of columns to display")
	_ = getCmd.RegisterFlagCompletionFunc(getOptions.Columns.CompletionFunc("columns"))
}

func getValidArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	ids, err := prcommon.GetPullRequestIDsWithState(cmd.Context(), cmd, "ALL")
	if err != nil {
		cobra.CompErrorln(err.Error())
		return []string{}, cobra.ShellCompDirectiveError
	}
	return common.FilterValidArgs(ids, args, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func getProcess(cmd *cobra.Command, args []string) error {
	profile, err := profile.GetProfileFromCommand(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get profile: %w", err)
	}

	repository, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get repository: %w", err)
	}

	lgr.Printf("[DEBUG] displaying pull request %s", args[0])
	if !common.WhatIf(cmd, "Showing pull request "+args[0]) {
		return nil
	}
	var pullrequest PullRequest

	err = profile.Get(
		cmd.Context(),
		cmd,
		repository.GetPath("pullrequests", args[0]),
		&pullrequest,
	)
	if err != nil {
		return fmt.Errorf("failed to get pullrequest %s: %w", args[0], err)
	}

	if err := profile.Print(cmd.Context(), cmd, pullrequest); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}
