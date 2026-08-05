package task

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
	Short: "list all pullrequest tasks",
	Args:  cobra.NoArgs,
	RunE:  listProcess,
}

var listOptions struct {
	Query         string
	PullRequestID *common.EnumFlag
	Columns       *common.EnumSliceFlag
	SortBy        *common.EnumFlag
	PageLength    int
}

func init() {
	Command.AddCommand(listCmd)

	listOptions.PullRequestID = common.NewEnumFlagWithFunc(listCmd, "", prcommon.GetPullRequestIDs)
	listOptions.Columns = common.NewEnumSliceFlagWithAllAllowed(columns.Columns()...)
	listOptions.SortBy = common.NewEnumFlag(columns.Sorters()...)
	listCmd.Flags().Var(listOptions.PullRequestID, "pullrequest", "pullrequest to list tasks from")
	listCmd.Flags().StringVar(&listOptions.Query, "query", "", "Query string to filter tasks")
	listCmd.Flags().Var(listOptions.Columns, "columns", "Comma-separated list of columns to display")
	listCmd.Flags().Var(listOptions.SortBy, "sort", "Column to sort by")
	listCmd.Flags().IntVar(&listOptions.PageLength, "page-length", 0, "Number of items per page to retrieve from Bitbucket. Default is the profile's default page length")
	_ = listCmd.MarkFlagRequired("pullrequest")
	_ = listCmd.RegisterFlagCompletionFunc(listOptions.PullRequestID.CompletionFunc("pullrequest"))
	_ = listCmd.RegisterFlagCompletionFunc(listOptions.Columns.CompletionFunc("columns"))
	_ = listCmd.RegisterFlagCompletionFunc(listOptions.SortBy.CompletionFunc("sort"))
}

func listProcess(cmd *cobra.Command, args []string) (err error) {
	ctx := cmd.Context()

	repository, err := repository.GetRepository(ctx, cmd)
	if err != nil {
		return fmt.Errorf("cannot get repository: %w", err)
	}

	lgr.Printf("[DEBUG] listing pullrequest tasks for pullrequest %s", listOptions.PullRequestID.Value)
	if !common.WhatIf(cmd, "Listing pullrequest tasks for pullrequest "+listOptions.PullRequestID.Value) {
		return nil
	}

	uripath := repository.GetPath(fmt.Sprintf("pullrequests/%s/tasks", listOptions.PullRequestID.Value))

	if listOptions.Query != "" {
		uripath = fmt.Sprintf("%s?q=%s", uripath, url.QueryEscape(listOptions.Query))
	}

	tasks, err := profile.GetAll[Task](ctx, cmd, uripath)
	if err != nil {
		return err
	}
	if len(tasks) == 0 {
		lgr.Printf("[DEBUG] no task found")
		return nil
	}
	core.Sort(tasks, columns.SortBy(listOptions.SortBy.Value))
	if err := profile.Current.Print(ctx, cmd, Tasks(tasks)); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}
