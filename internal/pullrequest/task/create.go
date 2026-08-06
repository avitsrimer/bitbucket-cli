package task

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/pullrequest/comment"
	prcommon "github.com/avitsrimer/bitbucket-cli/internal/pullrequest/common"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

type TaskCreator struct {
	Content   ContentCreator           `json:"content"`
	Comment   *comment.ParentReference `json:"comment,omitempty"`
	IsPending bool                     `json:"pending"`
}

type ContentCreator struct {
	Raw string `json:"raw"`
}

var createCmd = &cobra.Command{
	Use:               "create [flags] <pullrequest-id>",
	Aliases:           []string{"add", "new"},
	Short:             "create a pullrequest task on the pullrequest identified by <pullrequest-id>.",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: createValidArgs,
	RunE:              createProcess,
}

var createOptions struct {
	Content   string
	CommentID *common.EnumFlag
	Pending   bool
}

func init() {
	Command.AddCommand(createCmd)

	// GetPullRequestCommentIDs needs the pullrequest-id, which is now args[0] rather than a
	// flag; cobra passes the positionals typed so far into a flag's completion func, so this
	// closure reads it from there instead of from a --pullrequest flag.
	createOptions.CommentID = common.NewEnumFlagWithFunc("", func(ctx context.Context, cmd *cobra.Command, args []string, _ string) ([]string, error) {
		if len(args) == 0 {
			return []string{}, nil
		}
		return comment.GetPullRequestCommentIDs(ctx, cmd, args[0])
	})
	createCmd.Flags().StringVar(&createOptions.Content, "content", "", "Content of the task")
	createCmd.Flags().Var(createOptions.CommentID, "comment", "Comment ID to create task on")
	createCmd.Flags().BoolVar(&createOptions.Pending, "pending", false, "Mark the task as pending")
	_ = createCmd.MarkFlagRequired("content")
	_ = createCmd.RegisterFlagCompletionFunc(createOptions.CommentID.CompletionFunc("comment"))
}

func createValidArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	ids, err := prcommon.GetPullRequestIDs(cmd.Context(), cmd, args, toComplete)
	if err != nil {
		return []string{}, cobra.ShellCompDirectiveNoFileComp
	}
	return common.FilterValidArgs(ids, args, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func createProcess(cmd *cobra.Command, args []string) error {
	pullRequestID := args[0]
	if err := common.ValidatePathIdentifier("pullrequest-id", pullRequestID); err != nil {
		return fmt.Errorf("cannot create task: %w", err)
	}
	if strings.TrimSpace(createOptions.Content) == "" {
		return errors.New("task content is empty")
	}

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

	if err = prcommon.ExistsPullRequest(cmd.Context(), cmd, repository, pullRequestID); err != nil {
		return fmt.Errorf("cannot create task: %w", err)
	}

	uripath := repository.GetPath("pullrequests", pullRequestID, "tasks")

	lgr.Printf("[DEBUG] creating pullrequest task on pullrequest %s", pullRequestID)
	if !common.WhatIfPayload(cmd, uripath, task, "Creating pullrequest task on pullrequest "+pullRequestID) {
		return nil
	}

	var created Task

	err = profile.Post(
		cmd.Context(),
		uripath,
		task,
		&created,
	)
	if err != nil {
		return fmt.Errorf("failed to create pull request task on pull request %s: %w", pullRequestID, err)
	}
	if err := profile.Print(cmd.Context(), cmd, created); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}
