package comment

import (
	"fmt"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var createCmd = &cobra.Command{
	Use:     "create",
	Aliases: []string{"add", "new"},
	Short:   "create a pullrequest comment",
	Args:    cobra.NoArgs,
	RunE:    createProcess,
}

var createOptions commentEditOptions

func init() {
	Command.AddCommand(createCmd)

	registerCommentEditFlags(createCmd, &createOptions, "Comment of the pullrequest", "Pullrequest to create comments to")
}

func createProcess(cmd *cobra.Command, args []string) (err error) {
	profile, err := profile.GetProfileFromCommand(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get profile: %w", err)
	}

	repository, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get repository: %w", err)
	}

	payload, err := createOptions.payload(cmd)
	if err != nil {
		return err
	}

	lgr.Printf("[DEBUG] creating pullrequest comment")
	if !common.WhatIf(cmd, "Creating comment for pullrequest %s", createOptions.PullRequestID.Value) {
		return nil
	}
	var comment Comment

	err = profile.Post(
		cmd.Context(),
		repository.GetPath("pullrequests", createOptions.PullRequestID.Value, "comments"),
		payload,
		&comment,
	)
	if err != nil {
		return fmt.Errorf("failed to create comment for pullrequest %s: %w", createOptions.PullRequestID.Value, err)
	}
	if err := profile.Print(cmd.Context(), cmd, comment); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}
