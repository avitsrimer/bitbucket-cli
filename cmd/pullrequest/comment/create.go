package comment

import (
	"errors"
	"fmt"

	"github.com/gildas/bitbucket-cli/cmd/common"
	"github.com/gildas/bitbucket-cli/cmd/profile"
	"github.com/gildas/bitbucket-cli/cmd/repository"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

type CommentCreator struct {
	Content ContentCreator     `json:"content"           mapstructure:"content"`
	Anchor  *common.FileAnchor `json:"inline,omitempty"  mapstructure:"inline"`
	Parent  *ParentReference   `json:"parent,omitempty"  mapstructure:"parent"`
	Pending *bool              `json:"pending,omitempty" mapstructure:"pending"`
}

type ContentCreator struct {
	Raw string `json:"raw" mapstructure:"raw"`
}

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

	payload := CommentCreator{
		Content: ContentCreator{Raw: createOptions.Comment},
	}

	if createOptions.ParentID > 0 {
		payload.Parent = &ParentReference{ID: createOptions.ParentID}
	}

	if createOptions.File != "" {
		payload.Anchor = &common.FileAnchor{
			Path: createOptions.File,
		}
		if createOptions.From > 0 {
			payload.Anchor.From = uint64(createOptions.From)
		}
		if createOptions.To > 0 {
			payload.Anchor.To = uint64(createOptions.To)
		}
	} else if createOptions.From > 0 || createOptions.To > 0 {
		return errors.New("cannot specify from/to without a file")
	}
	if cmd.Flag("pending").Changed {
		payload.Pending = &createOptions.Pending
	}

	lgr.Printf("[DEBUG] creating pullrequest comment")
	if !common.WhatIf(cmd, "Creating comment for pullrequest %s", createOptions.PullRequestID.Value) {
		return nil
	}
	var comment Comment

	err = profile.Post(
		cmd.Context(),
		cmd,
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
