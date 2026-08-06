package branch

import (
	"context"
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

type Branches []Branch

// GetHeaders gets the header for a table
//
// implements common.Tableables
func (branches Branches) GetHeaders(cmd *cobra.Command) []string {
	return Branch{}.GetHeaders(cmd)
}

// GetRowAt gets the row for a table
//
// implements common.Tableables
func (branches Branches) GetRowAt(index int, headers []string) []string {
	if index < 0 || index >= len(branches) {
		return []string{}
	}
	return branches[index].GetRow(headers)
}

// Size gets the number of elements
//
// implements common.Tableables
func (branches Branches) Size() int {
	return len(branches)
}

// branchesQuery builds the "refs/branches" path for cmd's optional --query flag, or the
// unfiltered path when it is not set.
func branchesQuery(ctx context.Context, cmd *cobra.Command) (string, error) {
	repository, err := repository.GetRepository(ctx, cmd)
	if err != nil {
		return "", fmt.Errorf("cannot get repository: %w", err)
	}

	uripath := repository.GetPath("refs/branches")
	if cmd != nil && cmd.Flag("query") != nil && cmd.Flag("query").Changed {
		query, err := cmd.Flags().GetString("query")
		if err != nil {
			return "", fmt.Errorf("cannot read query flag: %w", err)
		}
		uripath = fmt.Sprintf("%s?q=%s", uripath, url.QueryEscape(query))
	}
	return uripath, nil
}

// GetBranches gets the branches of a repository, honoring cmd's own --page-length and --limit
// flags (see profile.GetAll).
func GetBranches(context context.Context, cmd *cobra.Command) (branches []Branch, err error) {
	uripath, err := branchesQuery(context, cmd)
	if err != nil {
		return []Branch{}, err
	}
	return profile.GetAll[Branch](context, cmd, uripath)
}

// GetBranchNames gets the branch names of a repository, sorted case-insensitively. It backs shell
// completion for a branch name argument/flag.
//
// This uses profile.GetAllUnbounded, not profile.GetAll (via GetBranches): cmd here is frequently
// the very command whose own --limit flag is meant to bound a different, unrelated output query,
// and this enumeration must resolve every branch regardless of it.
func GetBranchNames(context context.Context, cmd *cobra.Command, args []string, toComplete string) (names []string, err error) {
	lgr.Printf("[DEBUG] getting branches for profile %v", profile.Current)
	uripath, err := branchesQuery(context, cmd)
	if err != nil {
		cobra.CompErrorln(err.Error())
		return []string{}, err
	}
	branches, err := profile.GetAllUnbounded[Branch](context, cmd, uripath)
	if err != nil {
		cobra.CompErrorln(err.Error())
		return []string{}, err
	}
	names = core.Map(branches, func(branch Branch) string { return branch.Name })
	core.Sort(names, func(a, b string) bool { return strings.ToLower(a) < strings.ToLower(b) })
	return common.FilterValidArgs(names, args, toComplete), nil
}
