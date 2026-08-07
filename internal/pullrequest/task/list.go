package task

import (
	"fmt"
	"net/url"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	prcommon "github.com/avitsrimer/bitbucket-cli/internal/pullrequest/common"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:               "list [flags] <pullrequest-id>",
	Short:             "list all pullrequest tasks of the pullrequest identified by <pullrequest-id>.",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: prcommon.PullRequestIDValidArgs,
	RunE:              listProcess,
}

var listOptions struct {
	Query string
}

func init() {
	Command.AddCommand(listCmd)

	listCmd.Flags().StringVar(&listOptions.Query, "query", "", "Query string to filter tasks")
	common.RegisterListFlags(listCmd, columns, "tasks")
}

func listProcess(cmd *cobra.Command, args []string) (err error) {
	ctx := cmd.Context()

	pullRequestID := args[0]
	if validateErr := common.ValidatePathIdentifier("pullrequest-id", pullRequestID); validateErr != nil {
		return fmt.Errorf("cannot list tasks: %w", validateErr)
	}

	repository, err := repository.GetRepository(ctx, cmd)
	if err != nil {
		return fmt.Errorf("cannot get repository: %w", err)
	}

	lgr.Printf("[DEBUG] listing pullrequest tasks for pullrequest %s", pullRequestID)
	if !common.WhatIf(cmd, "Listing pullrequest tasks for pullrequest "+pullRequestID) {
		return nil
	}

	uripath := repository.GetPath(fmt.Sprintf("pullrequests/%s/tasks", pullRequestID))

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
		common.Sort(tasks, columns.SortBy(sortValue))
	}
	if err := profile.Current.Print(ctx, cmd, Tasks(tasks)); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}
