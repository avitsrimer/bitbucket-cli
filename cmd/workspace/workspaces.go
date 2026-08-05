package workspace

import (
	"context"
	"net/url"
	"strings"

	"github.com/gildas/bitbucket-cli/cmd/profile"
	"github.com/gildas/go-core"
	"github.com/gildas/go-logger"
	"github.com/spf13/cobra"
)

// GetWorkspaces gets the workspaces for the current user
func GetWorkspaces(ctx context.Context, cmd *cobra.Command) ([]Workspace, error) {
	return GetWorkspacesWithQuery(ctx, cmd, url.Values{})
}

// GetWorkspacesWithQuery gets the workspaces for the current user with a query
func GetWorkspacesWithQuery(ctx context.Context, cmd *cobra.Command, query url.Values) ([]Workspace, error) {
	log := logger.Must(logger.FromContext(ctx)).Child("workspace", "slugs")

	uripath := "/user/workspaces"
	if len(query) > 0 {
		uripath += "?" + query.Encode()
	}

	log.Debugf("Getting all workspaces with query %s", query)
	workspaces, err := profile.GetAll[Workspace](ctx, cmd, uripath)
	if err != nil {
		return nil, err
	}
	log.Debugf("Found %d workspaces", len(workspaces))
	core.Sort(workspaces, func(a, b Workspace) bool {
		return strings.Compare(strings.ToLower(a.Slug), strings.ToLower(b.Slug)) == -1
	})
	return workspaces, nil
}

// GetWorkspaceSlugs gets the slugs of all workspaces
func GetWorkspaceSlugs(ctx context.Context, cmd *cobra.Command) (slugs []string, err error) {
	workspaces, err := GetWorkspaces(ctx, cmd)
	if err != nil {
		return
	}
	return core.Map(workspaces, func(workspace Workspace) string { return workspace.Slug }), nil
}

// GetWorkspaceAllowedSlugs gets the slugs of all workspaces to use with enum flag completion
func GetWorkspaceAllowedSlugs(ctx context.Context, cmd *cobra.Command, args []string, toComplete string) (slugs []string, err error) {
	return GetWorkspaceSlugs(ctx, cmd)
}
