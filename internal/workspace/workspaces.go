package workspace

import (
	"context"
	"strings"

	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/gildas/go-core"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

// GetWorkspaceAllowedSlugs gets the slugs of all workspaces for the current user, sorted
// case-insensitively. It backs the root command's --workspace flag: dynamic allowed-value
// validation and shell completion (see common.AllowedFunc).
func GetWorkspaceAllowedSlugs(ctx context.Context, cmd *cobra.Command, _ []string, _ string) ([]string, error) {
	lgr.Printf("[DEBUG] getting all workspaces")
	workspaces, err := profile.GetAll[Workspace](ctx, cmd, "/user/workspaces")
	if err != nil {
		return nil, err
	}
	lgr.Printf("[DEBUG] found %d workspaces", len(workspaces))
	core.Sort(workspaces, func(a, b Workspace) bool {
		return strings.ToLower(a.Slug) < strings.ToLower(b.Slug)
	})
	return core.Map(workspaces, func(workspace Workspace) string { return workspace.Slug }), nil
}
