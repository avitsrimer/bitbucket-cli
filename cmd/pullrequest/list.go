package pullrequest

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/gildas/bitbucket-cli/cmd/common"
	"github.com/gildas/bitbucket-cli/cmd/profile"
	"github.com/gildas/bitbucket-cli/cmd/repository"
	"github.com/gildas/go-core"
	"github.com/gildas/go-flags"
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
	Commit     string
	State      *flags.EnumFlag
	Query      string
	Columns    *flags.EnumSliceFlag
	SortBy     *flags.EnumFlag
	PageLength int
}

func init() {
	Command.AddCommand(listCmd)

	listOptions.State = flags.NewEnumFlag("all", "declined", "merged", "+open", "superseded")
	listOptions.Columns = flags.NewEnumSliceFlagWithAllAllowed(columns.Columns()...)
	listOptions.SortBy = flags.NewEnumFlag(columns.Sorters()...)
	listCmd.Flags().StringVar(&listOptions.Commit, "commit", "", "List pull requests by commit hash")
	listCmd.Flags().Var(listOptions.State, "state", "Pull request state to fetch. Defaults to \"open\"")
	listCmd.Flags().StringVar(&listOptions.Query, "query", "", "Query string to filter pull requests")
	listCmd.Flags().Var(listOptions.Columns, "columns", "Comma-separated list of columns to display")
	listCmd.Flags().Var(listOptions.SortBy, "sort", "Column to sort by")
	listCmd.Flags().IntVar(&listOptions.PageLength, "page-length", 0, "Number of items per page to retrieve from Bitbucket. Default is the profile's default page length")
	listCmd.MarkFlagsMutuallyExclusive("commit", "state")
	listCmd.MarkFlagsMutuallyExclusive("commit", "query")
	_ = listCmd.RegisterFlagCompletionFunc(listOptions.State.CompletionFunc("state"))
	_ = listCmd.RegisterFlagCompletionFunc(listOptions.Columns.CompletionFunc("columns"))
	_ = listCmd.RegisterFlagCompletionFunc(listOptions.SortBy.CompletionFunc("sort"))
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
	core.Sort(pullrequests, columns.SortBy(listOptions.SortBy.Value))
	if err := profile.Current.Print(cmd.Context(), cmd, PullRequests(pullrequests)); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}
