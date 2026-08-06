package comment

import (
	"fmt"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var reopenCmd = &cobra.Command{
	Use:               "reopen [flags] <pullrequest-id> <comment-id>",
	Short:             "reopen a pullrequest comment by its <comment-id> on the pullrequest identified by <pullrequest-id>.",
	Args:              cobra.ExactArgs(2),
	ValidArgsFunction: pullRequestAndCommentIDValidArgs,
	RunE:              reopenProcess,
}

func init() {
	Command.AddCommand(reopenCmd)
}

func reopenProcess(cmd *cobra.Command, args []string) (err error) {
	pullRequestID, commentID := args[0], args[1]
	if validateErr := common.ValidatePathIdentifier("pullrequest-id", pullRequestID); validateErr != nil {
		return fmt.Errorf("cannot reopen comment: %w", validateErr)
	}

	profile, err := profile.GetProfileFromCommand(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get profile: %w", err)
	}

	repository, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get repository: %w", err)
	}

	if !common.WhatIf(cmd, "Reopening comment %s from pullrequest %s", commentID, pullRequestID) {
		return nil
	}

	err = profile.Delete(
		cmd.Context(),
		repository.GetPath("pullrequests", pullRequestID, "comments", commentID, "resolve"),
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to reopen pullrequest comment %s: %w", commentID, err)
	}
	lgr.Printf("[DEBUG] pullrequest comment %s reopened", commentID)
	return nil
}
