package comment

import (
	"errors"
	"fmt"
	"os"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/pullrequest/common"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var deleteCmd = &cobra.Command{
	Use:               "delete [flags] <comment-id...>",
	Aliases:           []string{"remove", "rm"},
	Short:             "delete pullrequest comments by their <comment-id>.",
	Args:              cobra.MinimumNArgs(1),
	ValidArgsFunction: deleteValidArgs,
	RunE:              deleteProcess,
}

var deleteOptions struct {
	PullRequestID *common.EnumFlag
}

func init() {
	Command.AddCommand(deleteCmd)

	deleteOptions.PullRequestID = common.NewEnumFlagWithFunc(deleteCmd, "", prcommon.GetPullRequestIDs)
	deleteCmd.Flags().Var(deleteOptions.PullRequestID, "pullrequest", "Pullrequest to delete comments from")
	_ = deleteCmd.MarkFlagRequired("pullrequest")
	_ = deleteCmd.RegisterFlagCompletionFunc(deleteOptions.PullRequestID.CompletionFunc("pullrequest"))
}

func deleteValidArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	commentIDs, err := GetPullRequestCommentIDs(cmd.Context(), cmd, args, toComplete)
	if err != nil {
		return []string{}, cobra.ShellCompDirectiveNoFileComp
	}
	return commentIDs, cobra.ShellCompDirectiveNoFileComp
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
	for _, commentID := range args {
		if common.WhatIf(cmd, "Deleting comment %s from pullrequest %s", commentID, deleteOptions.PullRequestID.Value) {
			err := profile.Delete(
				cmd.Context(),
				cmd,
				repository.GetPath("pullrequests", deleteOptions.PullRequestID.Value, "comments", commentID),
				nil,
			)
			if err != nil {
				if profile.ShouldStopOnError(cmd) {
					return fmt.Errorf("failed to delete pullrequest comment %s: %w", commentID, err)
				}
				errs = append(errs, err)
			}
			lgr.Printf("[DEBUG] pullrequest comment %s deleted", commentID)
		}
	}
	joined := errors.Join(errs...)
	if joined != nil && profile.ShouldWarnOnError(cmd) {
		fmt.Fprintf(os.Stderr, "Failed to delete these comments: %s\n", joined)
		return nil
	}
	if profile.ShouldIgnoreErrors(cmd) {
		lgr.Printf("[WARN] failed to delete these comments, but ignoring errors: %s", joined)
		return nil
	}
	return joined
}
