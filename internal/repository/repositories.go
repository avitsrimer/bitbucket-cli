package repository

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/workspace"
	"github.com/gildas/go-core"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

type Repositories []Repository

// GetHeaders gets the header for a table
//
// implements common.Tableables
func (repositories Repositories) GetHeaders(cmd *cobra.Command) []string {
	return Repository{}.GetHeaders(cmd)
}

// GetRowAt gets the row for a table
//
// implements common.Tableables
func (repositories Repositories) GetRowAt(index int, headers []string) []string {
	if index < 0 || index >= len(repositories) {
		return []string{}
	}
	return repositories[index].GetRow(headers)
}

// Size gets the number of elements
//
// implements common.Tableables
func (repositories Repositories) Size() int {
	return len(repositories)
}

// GetRepositories gets the repositories of the current workspace.
func GetRepositories(ctx context.Context, cmd *cobra.Command) ([]Repository, error) {
	return GetRepositoriesWithQuery(ctx, cmd, url.Values{})
}

// GetRepositoriesWithQuery gets the repositories of the current workspace matching query,
// honoring cmd's own --page-length and --limit flags (see profile.GetAll).
func GetRepositoriesWithQuery(ctx context.Context, cmd *cobra.Command, query url.Values) ([]Repository, error) {
	ws, err := workspace.GetWorkspace(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("cannot get workspace: %w", err)
	}

	uriPath := "/repositories/" + ws.Slug
	if len(query) > 0 {
		uriPath += "?" + query.Encode()
	}

	lgr.Printf("[DEBUG] getting repositories in workspace %s with query %s", ws.Slug, query.Encode())
	repositories, err := profile.GetAll[Repository](ctx, cmd, uriPath)
	if err != nil {
		return nil, fmt.Errorf("cannot get repositories: %w", err)
	}
	lgr.Printf("[DEBUG] found %d repositories in workspace %s", len(repositories), ws.Slug)
	return repositories, nil
}

// GetRepositorySlugs gets the slugs of all repositories in the current workspace, sorted
// case-insensitively. It backs shell completion for a repository slug argument.
//
// This uses profile.GetAllUnbounded, not profile.GetAll: cmd here is frequently the very command
// whose own --limit flag is meant to bound a different, later query, and this enumeration must
// resolve every repository regardless of it.
func GetRepositorySlugs(ctx context.Context, cmd *cobra.Command) (slugs []string, err error) {
	ws, err := workspace.GetWorkspace(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("cannot get workspace: %w", err)
	}

	lgr.Printf("[DEBUG] getting all repositories in workspace %s", ws.Slug)
	repositories, err := profile.GetAllUnbounded[Repository](ctx, cmd, "/repositories/"+ws.Slug)
	if err != nil {
		return nil, err
	}
	lgr.Printf("[DEBUG] found %d repositories", len(repositories))
	core.Sort(repositories, func(a, b Repository) bool {
		return strings.ToLower(a.Slug) < strings.ToLower(b.Slug)
	})
	return core.Map(repositories, func(repository Repository) string { return repository.Slug }), nil
}
