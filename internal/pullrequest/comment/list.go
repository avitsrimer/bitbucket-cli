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
	Use:               "list [flags] <pullrequest-id>",
	Short:             "list all pullrequest comments of the pullrequest identified by <pullrequest-id>.",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: listValidArgs,
	RunE:              listProcess,
}

var listOptions struct {
	Query string
}

func init() {
	Command.AddCommand(listCmd)

	listCmd.Flags().StringVar(&listOptions.Query, "query", "", "Query string to filter comments")
	common.RegisterListFlags(listCmd, columns, "comments")
}

func listValidArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	ids, err := prcommon.GetPullRequestIDs(cmd.Context(), cmd, args, toComplete)
	if err != nil {
		return []string{}, cobra.ShellCompDirectiveNoFileComp
	}
	return common.FilterValidArgs(ids, args, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func listProcess(cmd *cobra.Command, args []string) (err error) {
	pullRequestID := args[0]
	if validateErr := common.ValidatePathIdentifier("pullrequest-id", pullRequestID); validateErr != nil {
		return fmt.Errorf("cannot list comments: %w", validateErr)
	}

	repository, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get repository: %w", err)
	}

	uripath := repository.GetPath(fmt.Sprintf("pullrequests/%s/comments", pullRequestID))

	if listOptions.Query != "" {
		uripath = fmt.Sprintf("%s?q=%s", uripath, url.QueryEscape(listOptions.Query))
	}

	lgr.Printf("[DEBUG] listing all comments from repository %s", repository)
	if !common.WhatIf(cmd, fmt.Sprintf("Showing comments for pullrequest %s in repository %s with profile %s", pullRequestID, repository, profile.Current)) {
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
