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
}

func init() {
	Command.AddCommand(listCmd)

	listOptions.PullRequestID = common.NewEnumFlagWithFunc("", prcommon.GetPullRequestIDs)
	listCmd.Flags().Var(listOptions.PullRequestID, "pullrequest", "pullrequest to list tasks from")
	listCmd.Flags().StringVar(&listOptions.Query, "query", "", "Query string to filter tasks")
	common.RegisterListFlags(listCmd, columns, "tasks")
	_ = listCmd.MarkFlagRequired("pullrequest")
	_ = listCmd.RegisterFlagCompletionFunc(listOptions.PullRequestID.CompletionFunc("pullrequest"))
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
	if sortValue := common.SortFlagValue(cmd); sortValue != "" {
		core.Sort(tasks, columns.SortBy(sortValue))
	}
	if err := profile.Current.Print(ctx, cmd, Tasks(tasks)); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}
