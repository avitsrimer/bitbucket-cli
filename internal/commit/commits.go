package commit

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/gildas/go-core"
	"github.com/spf13/cobra"
)

type Commits []Commit

// GetHeaders gets the header for a table
//
// implements common.Tableables
func (commits Commits) GetHeaders(cmd *cobra.Command) []string {
	return Commit{}.GetHeaders(cmd)
}

// GetRowAt gets the row for a table
//
// implements common.Tableables
func (commits Commits) GetRowAt(index int, headers []string) []string {
	if index < 0 || index >= len(commits) {
		return []string{}
	}
	return commits[index].GetRow(headers)
}

// Size gets the number of elements
//
// implements common.Tableables
func (commits Commits) Size() int {
	return len(commits)
}

// GetCommits gets the commits of the repository, honoring cmd's own --page-length and --limit
// flags (see profile.GetAll), optionally filtered by cmd's --query, --include, and --exclude
// flags.
func GetCommits(ctx context.Context, cmd *cobra.Command) ([]Commit, error) {
	repo, err := repository.GetRepository(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("cannot get repository: %w", err)
	}
	uripath := repo.GetPath("commits") + commitsQuery(cmd)
	commits, err := profile.GetAll[Commit](ctx, cmd, uripath)
	if err != nil {
		return nil, fmt.Errorf("cannot get commits: %w", err)
	}
	return commits, nil
}

// GetCommitsWithPrefix gets the commits of the repository whose hash starts with prefix, or every
// commit when prefix is empty. It backs shell completion for a commit hash argument and therefore
// always uses profile.GetAllUnbounded (never GetAll -- the --limit truncation fix), regardless of
// cmd's own --limit flag, which here belongs to a different, unrelated output query.
func GetCommitsWithPrefix(ctx context.Context, cmd *cobra.Command, prefix string) ([]Commit, error) {
	repo, err := repository.GetRepository(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("cannot get repository: %w", err)
	}
	uripath := repo.GetPath("commits")
	if prefix != "" {
		query := url.Values{}
		query.Set("q", fmt.Sprintf("hash~%q", prefix))
		uripath = uripath + "?" + query.Encode()
	}
	commits, err := profile.GetAllUnbounded[Commit](ctx, cmd, uripath)
	if err != nil {
		return nil, fmt.Errorf("cannot get commits: %w", err)
	}
	return commits, nil
}

// GetCommitHashes gets the commit hashes of the repository, sorted case-insensitively. It backs
// shell completion for a commit hash argument.
func GetCommitHashes(ctx context.Context, cmd *cobra.Command, args []string, toComplete string) (hashes []string, err error) {
	commits, err := GetCommitsWithPrefix(ctx, cmd, toComplete)
	if err != nil {
		cobra.CompErrorln(err.Error())
		return []string{}, err
	}
	hashes = core.Map(commits, func(commit Commit) string { return commit.Hash })
	core.Sort(hashes, func(a, b string) bool { return strings.ToLower(a) < strings.ToLower(b) })
	return common.FilterValidArgs(hashes, args, toComplete), nil
}

// commitsQuery builds the "?q=...&include=...&exclude=..." query string for cmd's --query,
// --include, and --exclude flags (all optional), or "" when none of them are set.
func commitsQuery(cmd *cobra.Command) string {
	query := url.Values{}
	addStringQueryFilter(cmd, query, "query", "q")
	addStringSliceQueryFilter(cmd, query, "include")
	addStringSliceQueryFilter(cmd, query, "exclude")
	if len(query) == 0 {
		return ""
	}
	return "?" + query.Encode()
}

// addStringQueryFilter adds query[queryKey] = cmd's flagName value, when cmd has flagName
// registered, Changed, and non-empty.
func addStringQueryFilter(cmd *cobra.Command, query url.Values, flagName, queryKey string) {
	if cmd == nil {
		return
	}
	flag := cmd.Flag(flagName)
	if flag == nil || !flag.Changed {
		return
	}
	if value, err := cmd.Flags().GetString(flagName); err == nil && value != "" {
		query.Add(queryKey, value)
	}
}

// addStringSliceQueryFilter adds one query[name] entry per value of cmd's --name flag, when cmd
// has that flag registered and Changed.
func addStringSliceQueryFilter(cmd *cobra.Command, query url.Values, name string) {
	if cmd == nil {
		return
	}
	flag := cmd.Flag(name)
	if flag == nil || !flag.Changed {
		return
	}
	values, err := cmd.Flags().GetStringSlice(name)
	if err != nil {
		return
	}
	for _, value := range values {
		query.Add(name, value)
	}
}
