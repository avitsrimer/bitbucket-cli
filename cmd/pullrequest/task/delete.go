package task

import (
	"errors"
	"fmt"
	"os"

	"github.com/gildas/bitbucket-cli/cmd/common"
	"github.com/gildas/bitbucket-cli/cmd/profile"
	prcommon "github.com/gildas/bitbucket-cli/cmd/pullrequest/common"
	"github.com/gildas/bitbucket-cli/cmd/repository"
	"github.com/gildas/go-flags"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var deleteCmd = &cobra.Command{
	Use:               "delete [flags] <task-id...>",
	Aliases:           []string{"remove", "rm"},
	Short:             "delete pullrequest tasks by their <task-id>.",
	Args:              cobra.MinimumNArgs(1),
	ValidArgsFunction: deleteValidArgs,
	RunE:              deleteProcess,
}

var deleteOptions struct {
	PullRequestID *flags.EnumFlag
}

func init() {
	Command.AddCommand(deleteCmd)

	deleteOptions.PullRequestID = flags.NewEnumFlagWithFunc(deleteCmd, "", prcommon.GetPullRequestIDs)
	deleteCmd.Flags().Var(deleteOptions.PullRequestID, "pullrequest", "Pullrequest to delete comments from")
	_ = deleteCmd.MarkFlagRequired("pullrequest")
	_ = deleteCmd.RegisterFlagCompletionFunc(deleteOptions.PullRequestID.CompletionFunc("pullrequest"))
}

func deleteValidArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	taskIDs, err := GetPullRequestTaskIDs(cmd.Context(), cmd, deleteOptions.PullRequestID.Value)
	if err != nil {
		return []string{}, cobra.ShellCompDirectiveNoFileComp
	}
	return taskIDs, cobra.ShellCompDirectiveNoFileComp
}

func deleteProcess(cmd *cobra.Command, args []string) error {
	profile, err := profile.GetProfileFromCommand(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get profile: %w", err)
	}

	repository, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get repository: %w", err)
	}

	var errs []error
	for _, taskID := range args {
		if common.WhatIf(cmd, "Deleting task %s from pullrequest %s", taskID, deleteOptions.PullRequestID) {
			err := profile.Delete(
				cmd.Context(),
				cmd,
				repository.GetPath("pullrequests", deleteOptions.PullRequestID.Value, "tasks", taskID),
				nil,
			)
			if err != nil {
				if profile.ShouldStopOnError(cmd) {
					return fmt.Errorf("failed to delete pullrequest task %s: %w", taskID, err)
				}
				errs = append(errs, err)
			}
			lgr.Printf("[DEBUG] pullrequest task %s deleted", taskID)
		}
	}
	joined := errors.Join(errs...)
	if joined != nil && profile.ShouldWarnOnError(cmd) {
		fmt.Fprintf(os.Stderr, "Failed to delete these tasks: %s\n", joined)
		return nil
	}
	if profile.ShouldIgnoreErrors(cmd) {
		lgr.Printf("[WARN] failed to delete these tasks, but ignoring errors: %s", joined)
		return nil
	}
	return joined
}
