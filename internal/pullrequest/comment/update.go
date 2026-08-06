package comment

import (
	"fmt"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:               "update [flags] <pullrequest-id> <comment-id>",
	Aliases:           []string{"edit"},
	Short:             "update a pull request comment by its <comment-id> on the pullrequest identified by <pullrequest-id>.",
	Args:              cobra.ExactArgs(2),
	ValidArgsFunction: pullRequestAndCommentIDValidArgs,
	RunE:              updateProcess,
}

var updateOptions commentEditOptions

func init() {
	Command.AddCommand(updateCmd)

	registerCommentEditFlags(updateCmd, &updateOptions, "Updated comment of the pullrequest")
}

func updateProcess(cmd *cobra.Command, args []string) (err error) {
	pullRequestID, commentID := args[0], args[1]
	if validateErr := common.ValidatePathIdentifier("pullrequest-id", pullRequestID); validateErr != nil {
		return fmt.Errorf("cannot update comment: %w", validateErr)
	}

	profile, err := profile.GetProfileFromCommand(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get profile: %w", err)
	}

	repository, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get repository: %w", err)
	}

	payload, err := updateOptions.payload(cmd)
	if err != nil {
		return err
	}

	lgr.Printf("[DEBUG] updating pullrequest comment")
	if !common.WhatIf(cmd, "Updating comment %s for pullrequest %s", commentID, pullRequestID) {
		return nil
	}
	var comment Comment

	err = profile.Put(
		cmd.Context(),
		repository.GetPath("pullrequests", pullRequestID, "comments", commentID),
		payload,
		&comment,
	)
	if err != nil {
		return fmt.Errorf("failed to update comment for pullrequest %s: %w", pullRequestID, err)
	}
	if err := profile.Print(cmd.Context(), cmd, comment); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}
