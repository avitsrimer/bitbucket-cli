package pullrequest

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/gildas/go-core"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "list all pullrequests",
	Args:  cobra.NoArgs,
	RunE:  listProcess,
}

var listOptions struct {
	Commit string
	State  *common.EnumFlag
	Query  string
}

func init() {
	Command.AddCommand(listCmd)

	listOptions.State = common.NewEnumFlag("all", "declined", "merged", "+open", "superseded")
	listCmd.Flags().StringVar(&listOptions.Commit, "commit", "", "List pull requests by commit hash")
	listCmd.Flags().Var(listOptions.State, "state", "Pull request state to fetch. Defaults to \"open\"")
	listCmd.Flags().StringVar(&listOptions.Query, "query", "", "Query string to filter pull requests")
	common.RegisterListFlags(listCmd, columns, "pull requests")
	listCmd.MarkFlagsMutuallyExclusive("commit", "state")
	listCmd.MarkFlagsMutuallyExclusive("commit", "query")
	_ = listCmd.RegisterFlagCompletionFunc(listOptions.State.CompletionFunc("state"))
}

func listProcess(cmd *cobra.Command, args []string) (err error) {
	repository, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get repository: %w", err)
	}

	var uripath string

	switch {
	case listOptions.Commit != "":
		uripath = repository.GetPath("commit", listOptions.Commit, "pullrequests")
	case listOptions.Query != "":
		uripath = repository.GetPath(fmt.Sprintf("pullrequests?state=%s&q=%s", url.QueryEscape(strings.ToUpper(listOptions.State.String())), url.QueryEscape(listOptions.Query)))
	default:
		uripath = repository.GetPath("pullrequests?state=" + url.QueryEscape(strings.ToUpper(listOptions.State.String())))
	}

	lgr.Printf("[DEBUG] listing %s pull requests for repository: %s", listOptions.State, repository)
	if !common.WhatIf(cmd, fmt.Sprintf("Showing %s pull requests for repository: %s", listOptions.State, repository)) {
		return nil
	}

	pullrequests, err := profile.GetAll[PullRequest](cmd.Context(), cmd, uripath)
	if err != nil {
		return err
	}
	if len(pullrequests) == 0 {
		lgr.Printf("[DEBUG] no pullrequest found")
		return err
	}
	if sortValue := common.SortFlagValue(cmd); sortValue != "" {
		core.Sort(pullrequests, columns.SortBy(sortValue))
	}
	if err := profile.Current.Print(cmd.Context(), cmd, PullRequests(pullrequests)); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}
