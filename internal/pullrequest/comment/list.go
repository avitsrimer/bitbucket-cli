package comment

import (
	"fmt"
	"net/url"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	prcommon "github.com/avitsrimer/bitbucket-cli/internal/pullrequest/common"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/gildas/go-core"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "list all pullrequest comments",
	Args:  cobra.NoArgs,
	RunE:  listProcess,
}

var listOptions struct {
	Query         string
	PullRequestID *common.EnumFlag
}

func init() {
	Command.AddCommand(listCmd)

	listOptions.PullRequestID = common.NewEnumFlagWithFunc("", prcommon.GetPullRequestIDs)
	listCmd.Flags().Var(listOptions.PullRequestID, "pullrequest", "pullrequest to list comments from")
	listCmd.Flags().StringVar(&listOptions.Query, "query", "", "Query string to filter comments")
	common.RegisterListFlags(listCmd, columns, "comments")
	_ = listCmd.MarkFlagRequired("pullrequest")
	_ = listCmd.RegisterFlagCompletionFunc(listOptions.PullRequestID.CompletionFunc("pullrequest"))
}

func listProcess(cmd *cobra.Command, args []string) (err error) {
	repository, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get repository: %w", err)
	}

	uripath := repository.GetPath(fmt.Sprintf("pullrequests/%s/comments", listOptions.PullRequestID.Value))

	if listOptions.Query != "" {
		uripath = fmt.Sprintf("%s?q=%s", uripath, url.QueryEscape(listOptions.Query))
	}

	lgr.Printf("[DEBUG] listing all comments from repository %s", repository)
	if !common.WhatIf(cmd, fmt.Sprintf("Showing comments for pullrequest %s in repository %s with profile %s", listOptions.PullRequestID.Value, repository, profile.Current)) {
		return nil
	}

	comments, err := profile.GetAll[Comment](cmd.Context(), cmd, uripath)
	if err != nil {
		return err
	}
	if len(comments) == 0 {
		lgr.Printf("[DEBUG] no comment found")
		return nil
	}
	if sortValue := common.SortFlagValue(cmd); sortValue != "" {
		core.Sort(comments, columns.SortBy(sortValue))
	}
	if err := profile.Current.Print(
		cmd.Context(),
		cmd,
		Comments(core.Filter(comments, func(comment Comment) bool {
			return comment.Content.Raw != ""
		})),
	); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}
