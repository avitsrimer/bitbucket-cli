package comment

import (
	"fmt"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var getCmd = &cobra.Command{
	Use:               "get [flags] <pullrequest-id> <comment-id>",
	Aliases:           []string{"show", "info", "display"},
	Short:             "get a pullrequest comment by its <comment-id> on the pullrequest identified by <pullrequest-id>.",
	Args:              cobra.ExactArgs(2),
	ValidArgsFunction: pullRequestAndCommentIDValidArgs,
	RunE:              getProcess,
}

func init() {
	Command.AddCommand(getCmd)

	common.RegisterColumnsFlag(getCmd, columns)
}

func getProcess(cmd *cobra.Command, args []string) (err error) {
	pullRequestID, commentID := args[0], args[1]
	if validateErr := common.ValidatePathIdentifier("pullrequest-id", pullRequestID); validateErr != nil {
		return fmt.Errorf("cannot get comment: %w", validateErr)
	}
	if validateErr := common.ValidatePathIdentifier("comment-id", commentID); validateErr != nil {
		return fmt.Errorf("cannot get comment: %w", validateErr)
	}

	ctx := cmd.Context()

	profile, err := profile.GetProfileFromCommand(ctx, cmd)
	if err != nil {
		return fmt.Errorf("cannot get profile: %w", err)
	}

	repository, err := repository.GetRepository(ctx, cmd)
	if err != nil {
		return fmt.Errorf("cannot get repository: %w", err)
	}

	lgr.Printf("[DEBUG] displaying pullrequest comment %s", commentID)
	if !common.WhatIf(cmd, "Showing pullrequest comment "+commentID) {
		return nil
	}

	var comment Comment

	err = profile.Get(
		ctx,
		repository.GetPath("pullrequests", pullRequestID, "comments", commentID),
		&comment,
	)
	if err != nil {
		return fmt.Errorf("failed to get pullrequest comment %s: %w", commentID, err)
	}
	if err := profile.Print(ctx, cmd, comment); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}
