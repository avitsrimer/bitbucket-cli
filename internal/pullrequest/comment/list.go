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
	Columns       *common.EnumSliceFlag
	SortBy        *common.EnumFlag
	PageLength    int
	Limit         int
}

func init() {
	Command.AddCommand(listCmd)

	listOptions.PullRequestID = common.NewEnumFlagWithFunc(listCmd, "", prcommon.GetPullRequestIDs)
	listOptions.Columns = common.NewEnumSliceFlagWithAllAllowed(columns.Columns()...)
	listOptions.SortBy = common.NewEnumFlag(columns.Sorters()...)
	listCmd.Flags().Var(listOptions.PullRequestID, "pullrequest", "pullrequest to list comments from")
	listCmd.Flags().StringVar(&listOptions.Query, "query", "", "Query string to filter comments")
	listCmd.Flags().Var(listOptions.Columns, "columns", "Comma-separated list of columns to display")
	listCmd.Flags().Var(listOptions.SortBy, "sort", "Column to sort by")
	listCmd.Flags().IntVar(&listOptions.PageLength, "page-length", 0, "Number of items per page to retrieve from Bitbucket. Default is the profile's default page length")
	listCmd.Flags().IntVar(&listOptions.Limit, "limit", 0, "Maximum total number of comments to retrieve. Default is to retrieve all of them")
	_ = listCmd.MarkFlagRequired("pullrequest")
	_ = listCmd.RegisterFlagCompletionFunc(listOptions.PullRequestID.CompletionFunc("pullrequest"))
	_ = listCmd.RegisterFlagCompletionFunc(listOptions.Columns.CompletionFunc("columns"))
	_ = listCmd.RegisterFlagCompletionFunc(listOptions.SortBy.CompletionFunc("sort"))
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
	core.Sort(comments, columns.SortBy(listOptions.SortBy.Value))
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
