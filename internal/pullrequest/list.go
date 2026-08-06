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
	Query  string
}

// listDefaultState is the state --state fetches when the flag is never explicitly set on the
// command line.
const listDefaultState = "open"

func init() {
	Command.AddCommand(listCmd)

	listCmd.Flags().StringVar(&listOptions.Commit, "commit", "", "List pull requests by commit hash")
	stateFlag := common.NewEnumSliceFlagWithAllAllowed("declined", "merged", "open", "superseded")
	listCmd.Flags().Var(stateFlag, "state", "Pull request state(s) to fetch (repeatable, or \"all\"). Defaults to \""+listDefaultState+"\"")
	listCmd.Flags().StringVar(&listOptions.Query, "query", "", "Query string to filter pull requests")
	listCmd.Flags().String("source", "", "Filter by source branch name")
	listCmd.Flags().String("destination", "", "Filter by destination branch name")
	common.RegisterListFlags(listCmd, columns, "pull requests")
	listCmd.MarkFlagsMutuallyExclusive("commit", "state")
	listCmd.MarkFlagsMutuallyExclusive("commit", "query")
	listCmd.MarkFlagsMutuallyExclusive("commit", "source")
	listCmd.MarkFlagsMutuallyExclusive("commit", "destination")
	_ = listCmd.RegisterFlagCompletionFunc(stateFlag.CompletionFunc("state"))
}

func listProcess(cmd *cobra.Command, args []string) (err error) {
	repository, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get repository: %w", err)
	}

	states := listStates(cmd)

	var uripath string

	switch {
	case listOptions.Commit != "":
		uripath = repository.GetPath("commit", listOptions.Commit, "pullrequests")
	default:
		query := url.Values{}
		for _, state := range states {
			query.Add("state", strings.ToUpper(state))
		}
		if filter := listQueryFilter(cmd); filter != "" {
			query.Set("q", filter)
		}
		uripath = repository.GetPath("pullrequests") + "?" + query.Encode()
	}

	lgr.Printf("[DEBUG] listing %s pull requests for repository: %s", strings.Join(states, ","), repository)
	if !common.WhatIf(cmd, fmt.Sprintf("Showing %s pull requests for repository: %s", strings.Join(states, ","), repository)) {
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

// listStates resolves cmd's own --state flag, read directly off cmd rather than a package-level
// binding (the flag's repeatable EnumSliceFlag value does not round-trip through
// cmd.Flags().GetStringSlice, whose CSV-based parsing does not understand EnumSliceFlag.String's
// bracketed representation). Defaults to listDefaultState when the flag was never registered on
// cmd or never set explicitly, "all" resolves to every allowed state via
// common.NewEnumSliceFlagWithAllAllowed.
func listStates(cmd *cobra.Command) []string {
	flag := cmd.Flags().Lookup("state")
	if flag == nil || !flag.Changed {
		return []string{listDefaultState}
	}
	if states, ok := flag.Value.(*common.EnumSliceFlag); ok {
		return states.GetSlice()
	}
	return []string{listDefaultState}
}

// listQueryFilter builds the "q=" filter from listOptions.Query and cmd's --source/--destination
// branch flags, ANDing every non-empty piece together so all three compose. Branch names are
// double-quoted with embedded double quotes/backslashes escaped for Bitbucket's query syntax;
// listOptions.Query is passed through verbatim since it is already a raw Bitbucket query
// expression.
func listQueryFilter(cmd *cobra.Command) string {
	var clauses []string
	if listOptions.Query != "" {
		clauses = append(clauses, listOptions.Query)
	}
	if source := common.StringFlagValue(cmd, "source"); source != "" {
		clauses = append(clauses, "source.branch.name="+quoteBranchFilter(source))
	}
	if destination := common.StringFlagValue(cmd, "destination"); destination != "" {
		clauses = append(clauses, "destination.branch.name="+quoteBranchFilter(destination))
	}
	return strings.Join(clauses, " AND ")
}

// quoteBranchFilter double-quotes value for a Bitbucket q= filter clause, backslash-escaping any
// embedded backslash or double quote.
func quoteBranchFilter(value string) string {
	escaped := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value)
	return `"` + escaped + `"`
}
