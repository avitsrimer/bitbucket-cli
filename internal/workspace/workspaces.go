package workspace

import (
	"context"
	"strings"

	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/gildas/go-core"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

type Workspaces []Workspace

// GetHeaders gets the header for a table
//
// implements common.Tableables
func (workspaces Workspaces) GetHeaders(cmd *cobra.Command) []string {
	return Workspace{}.GetHeaders(cmd)
}

// GetRowAt gets the row for a table
//
// implements common.Tableables
func (workspaces Workspaces) GetRowAt(index int, headers []string) []string {
	if index < 0 || index >= len(workspaces) {
		return []string{}
	}
	return workspaces[index].GetRow(headers)
}

// Size gets the number of elements
//
// implements common.Tableables
func (workspaces Workspaces) Size() int {
	return len(workspaces)
}

// GetWorkspaceAllowedSlugs gets the slugs of all workspaces for the current user, sorted
// case-insensitively. It backs the root command's --workspace flag: dynamic allowed-value
// validation and shell completion (see common.AllowedFunc).
//
// This uses profile.GetAllUnbounded, not profile.GetAll: cmd here is frequently the command whose
// own --limit flag is meant to bound a different, unrelated output query, and this enumeration
// must resolve every workspace regardless of it.
func GetWorkspaceAllowedSlugs(ctx context.Context, cmd *cobra.Command, _ []string, _ string) ([]string, error) {
	lgr.Printf("[DEBUG] getting all workspaces")
	workspaces, err := profile.GetAllUnbounded[Workspace](ctx, cmd, "/user/workspaces")
	if err != nil {
		return nil, err
	}
	lgr.Printf("[DEBUG] found %d workspaces", len(workspaces))
	core.Sort(workspaces, func(a, b Workspace) bool {
		return strings.ToLower(a.Slug) < strings.ToLower(b.Slug)
	})
	return core.Map(workspaces, func(workspace Workspace) string { return workspace.Slug }), nil
}
