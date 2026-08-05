package task

import (
	"fmt"
	"strconv"

	"github.com/gildas/bitbucket-cli/cmd/common"
	"github.com/gildas/bitbucket-cli/cmd/profile"
	"github.com/gildas/bitbucket-cli/cmd/pullrequest/comment"
	prcommon "github.com/gildas/bitbucket-cli/cmd/pullrequest/common"
	"github.com/gildas/bitbucket-cli/cmd/repository"
	"github.com/gildas/go-flags"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

type TaskCreator struct {
	Content   ContentCreator           `json:"content"           mapstructure:"content"`
	Comment   *comment.ParentReference `json:"comment,omitempty" mapstructure:"comment"`
	IsPending bool                     `json:"pending"           mapstructure:"pending"`
}

type ContentCreator struct {
	Raw string `json:"raw" mapstructure:"raw"`
}

var createCmd = &cobra.Command{
	Use:     "create",
	Aliases: []string{"add", "new"},
	Short:   "create a pullrequest task",
	Args:    cobra.NoArgs,
	RunE:    createProcess,
}

var createOptions struct {
	PullRequestID *flags.EnumFlag
	Content       string
	CommentID     *flags.EnumFlag
	Pending       bool
}

func init() {
	Command.AddCommand(createCmd)

	createOptions.PullRequestID = flags.NewEnumFlagWithFunc(createCmd, "", prcommon.GetPullRequestIDs)
	createOptions.CommentID = flags.NewEnumFlagWithFunc(createCmd, "", comment.GetPullRequestCommentIDs)
	createCmd.Flags().Var(createOptions.PullRequestID, "pullrequest", "Pullrequest to create tasks to")
	createCmd.Flags().StringVar(&createOptions.Content, "content", "", "Content of the task")
	createCmd.Flags().Var(createOptions.CommentID, "comment", "Comment ID to create task on")
	createCmd.Flags().BoolVar(&createOptions.Pending, "pending", false, "Mark the task as pending")
	_ = createCmd.MarkFlagRequired("pullrequest")
	_ = createCmd.MarkFlagRequired("content")
	_ = createCmd.RegisterFlagCompletionFunc(createOptions.PullRequestID.CompletionFunc("pullrequest"))
	_ = createCmd.RegisterFlagCompletionFunc(createOptions.CommentID.CompletionFunc("comment"))
}

func createProcess(cmd *cobra.Command, args []string) error {
	profile, err := profile.GetProfileFromCommand(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get profile: %w", err)
	}

	repository, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get repository: %w", err)
	}

	task := TaskCreator{
		Content: ContentCreator{
			Raw: createOptions.Content,
		},
		IsPending: createOptions.Pending,
	}
	if createOptions.CommentID.Value != "" {
		commentID, parseErr := strconv.ParseInt(createOptions.CommentID.Value, 10, 64)
		if parseErr != nil {
			return fmt.Errorf("failed to parse comment ID %s: %w", createOptions.CommentID.Value, parseErr)
		}
		task.Comment = &comment.ParentReference{
			ID: commentID,
		}
	}

	lgr.Printf("[DEBUG] creating pullrequest task on pullrequest %s", createOptions.PullRequestID.Value)
	if !common.WhatIf(cmd, "Creating pullrequest task on pullrequest "+createOptions.PullRequestID.Value) {
		return nil
	}

	var created Task

	err = profile.Post(
		cmd.Context(),
		cmd,
		repository.GetPath("pullrequests", createOptions.PullRequestID.Value, "tasks"),
		task,
		&created,
	)
	if err != nil {
		return fmt.Errorf("failed to create pull request task on pull request %s: %w", createOptions.PullRequestID.Value, err)
	}
	if err := profile.Print(cmd.Context(), cmd, created); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}
