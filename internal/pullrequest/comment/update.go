package comment

import (
	"errors"
	"fmt"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

type CommentUpdator struct {
	Content ContentUpdator     `json:"content"           mapstructure:"content"`
	Anchor  *common.FileAnchor `json:"inline,omitempty"  mapstructure:"inline"`
	Parent  *ParentReference   `json:"parent,omitempty"  mapstructure:"parent"`
	Pending *bool              `json:"pending,omitempty" mapstructure:"pending"`
}

type ContentUpdator struct {
	Raw string `json:"raw" mapstructure:"raw"`
}

var updateCmd = &cobra.Command{
	Use:               "update [flags] <comment-id>",
	Aliases:           []string{"edit"},
	Short:             "update an issue comment by its <comment-id>.",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: updateValidArgs,
	RunE:              updateProcess,
}

var updateOptions commentEditOptions

func init() {
	Command.AddCommand(updateCmd)

	registerCommentEditFlags(updateCmd, &updateOptions, "Updated comment of the pullrequest", "Pullrequest to update comments to")
}

func updateValidArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	commentIDs, err := GetPullRequestCommentIDs(cmd.Context(), cmd, args, toComplete)
	if err != nil {
		return []string{}, cobra.ShellCompDirectiveNoFileComp
	}
	return commentIDs, cobra.ShellCompDirectiveNoFileComp
}

func updateProcess(cmd *cobra.Command, args []string) (err error) {
	profile, err := profile.GetProfileFromCommand(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get profile: %w", err)
	}

	repository, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get repository: %w", err)
	}

	payload := CommentUpdator{
		Content: ContentUpdator{Raw: updateOptions.Comment},
	}

	if updateOptions.File != "" {
		payload.Anchor = &common.FileAnchor{
			Path: updateOptions.File,
		}
		if updateOptions.From > 0 {
			payload.Anchor.From = uint64(updateOptions.From)
		}
		if updateOptions.To > 0 {
			payload.Anchor.To = uint64(updateOptions.To)
		}
	} else if updateOptions.From > 0 || updateOptions.To > 0 {
		return errors.New("cannot specify from/to without a file")
	}

	if cmd.Flag("pending").Changed {
		payload.Pending = &updateOptions.Pending
	}

	if updateOptions.ParentID > 0 {
		payload.Parent = &ParentReference{ID: updateOptions.ParentID}
	}

	lgr.Printf("[DEBUG] updating pullrequest comment")
	if !common.WhatIf(cmd, "Updating comment %s for pullrequest %s", args[0], updateOptions.PullRequestID.Value) {
		return nil
	}
	var comment Comment

	err = profile.Put(
		cmd.Context(),
		cmd,
		repository.GetPath("pullrequests", updateOptions.PullRequestID.Value, "comments", args[0]),
		payload,
		&comment,
	)
	if err != nil {
		return fmt.Errorf("failed to update comment for pullrequest %s: %w", updateOptions.PullRequestID.Value, err)
	}
	if err := profile.Print(cmd.Context(), cmd, comment); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}
