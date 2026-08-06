package comment

import (
	"fmt"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var resolveCmd = &cobra.Command{
	Use:               "resolve [flags] <pullrequest-id> <comment-id>",
	Short:             "resolve a pullrequest comment by its <comment-id> on the pullrequest identified by <pullrequest-id>.",
	Args:              cobra.ExactArgs(2),
	ValidArgsFunction: pullRequestAndCommentIDValidArgs,
	RunE:              resolveProcess,
}

func init() {
	Command.AddCommand(resolveCmd)
}

func resolveProcess(cmd *cobra.Command, args []string) (err error) {
	pullRequestID, commentID := args[0], args[1]
	if validateErr := common.ValidatePathIdentifier("pullrequest-id", pullRequestID); validateErr != nil {
		return fmt.Errorf("cannot resolve comment: %w", validateErr)
	}
	if validateErr := common.ValidatePathIdentifier("comment-id", commentID); validateErr != nil {
		return fmt.Errorf("cannot resolve comment: %w", validateErr)
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

	if err = existsComment(ctx, cmd, repository, pullRequestID, commentID); err != nil {
		return fmt.Errorf("cannot resolve comment: %w", err)
	}

	uripath := repository.GetPath("pullrequests", pullRequestID, "comments", commentID, "resolve")

	if !common.WhatIfPayload(cmd, uripath, nil, "Resolving comment %s from pullrequest %s", commentID, pullRequestID) {
		return nil
	}

	err = profile.Post(
		ctx,
		uripath,
		nil,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to resolve pullrequest comment %s: %w", commentID, err)
	}
	lgr.Printf("[DEBUG] pullrequest comment %s resolved", commentID)
	return nil
}
