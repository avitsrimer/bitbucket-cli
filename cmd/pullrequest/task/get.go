package task

import (
	"fmt"

	"github.com/gildas/bitbucket-cli/cmd/common"
	"github.com/gildas/bitbucket-cli/cmd/profile"
	prcommon "github.com/gildas/bitbucket-cli/cmd/pullrequest/common"
	"github.com/gildas/bitbucket-cli/cmd/repository"
	"github.com/gildas/go-flags"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var getCmd = &cobra.Command{
	Use:               "get [flags] <task-id>",
	Aliases:           []string{"show", "info", "display"},
	Short:             "get a pullrequest task by its <task-id>.",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: getValidArgs,
	RunE:              getProcess,
}

var getOptions struct {
	PullRequestID *flags.EnumFlag
	Columns       *flags.EnumSliceFlag
}

func init() {
	Command.AddCommand(getCmd)

	getOptions.PullRequestID = flags.NewEnumFlagWithFunc(getCmd, "", prcommon.GetPullRequestIDs)
	getOptions.Columns = flags.NewEnumSliceFlag(columns.Columns()...)
	getCmd.Flags().Var(getOptions.PullRequestID, "pullrequest", "Pullrequest to get tasks from")
	getCmd.Flags().Var(getOptions.Columns, "columns", "Comma-separated list of columns to display")
	_ = getCmd.MarkFlagRequired("pullrequest")
	_ = getCmd.RegisterFlagCompletionFunc(getOptions.PullRequestID.CompletionFunc("pullrequest"))
	_ = getCmd.RegisterFlagCompletionFunc(getOptions.Columns.CompletionFunc("columns"))
}

func getValidArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	taskIDs, err := GetPullRequestTaskIDs(cmd.Context(), cmd, getOptions.PullRequestID.Value)
	if err != nil {
		return []string{}, cobra.ShellCompDirectiveNoFileComp
	}
	return taskIDs, cobra.ShellCompDirectiveNoFileComp
}

func getProcess(cmd *cobra.Command, args []string) (err error) {
	profile, err := profile.GetProfileFromCommand(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get profile: %w", err)
	}

	repository, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get repository: %w", err)
	}

	lgr.Printf("[DEBUG] displaying pullrequest task %s", args[0])
	if !common.WhatIf(cmd, "Showing pullrequest task "+args[0]) {
		return nil
	}

	var task Task

	err = profile.Get(
		cmd.Context(),
		cmd,
		repository.GetPath("pullrequests", getOptions.PullRequestID.Value, "tasks", args[0]),
		&task,
	)
	if err != nil {
		return fmt.Errorf("failed to get pullrequest task %s: %w", args[0], err)
	}
	if err := profile.Print(cmd.Context(), cmd, task); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}
