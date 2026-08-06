package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/remote"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

type Workspace struct {
	ID            common.UUID  `json:"uuid"`
	Name          string       `json:"name"`
	Slug          string       `json:"slug"`
	Administrator bool         `json:"administrator"`
	Links         common.Links `json:"links"`
}

var WorkspaceCache = common.NewCache[Workspace]()

// Command represents this folder's command
var Command = &cobra.Command{
	Use:   "workspace",
	Short: "Manage workspaces",
	Run:   common.SubcommandRequired("Workspace"),
}

var columns = common.Columns[Workspace]{
	{Name: "id", DefaultSorter: false, Compare: func(a, b Workspace) bool {
		return strings.ToLower(a.ID.String()) < strings.ToLower(b.ID.String())
	}},
	{Name: "name", DefaultSorter: true, Compare: func(a, b Workspace) bool {
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	}},
	{Name: "slug", DefaultSorter: false, Compare: func(a, b Workspace) bool {
		return strings.ToLower(a.Slug) < strings.ToLower(b.Slug)
	}},
}

// GetType gets the type of the workspace
//
// implements core.TypeCarrier
func (workspace Workspace) GetType() string {
	return "workspace"
}

// GetHeaders gets the header for a table
//
// implements common.Tableable
func (workspace Workspace) GetHeaders(cmd *cobra.Command) []string {
	return common.HeadersFromFlag(cmd, "ID", "Name", "Slug")
}

// GetRow gets the row for a table
//
// implements common.Tableable
func (workspace Workspace) GetRow(headers []string) []string {
	var row []string

	for _, header := range headers {
		switch common.NormalizeColumnKey(header) {
		case "id":
			row = append(row, workspace.ID.String())
		case "name":
			row = append(row, workspace.Name)
		case "slug":
			row = append(row, workspace.Slug)
		default:
			row = append(row, " ")
		}
	}
	return row
}

// String returns the string representation of the workspace
//
// implements fmt.Stringer
func (workspace Workspace) String() string {
	if workspace.Slug != "" {
		return workspace.Slug
	}
	return workspace.Name
}

// GetWorkspaceName gets the workspace slug the user already supplied, with no API call: the
// --workspace flag, the git remote, or the profile's default workspace, in that order. Callers
// that only need the slug (to build a request path, for example) should call this instead of
// GetWorkspace: GetWorkspace additionally fetches the Workspace object via GET /workspaces/{slug},
// which requires the read:workspace scope that a repository-scoped or narrowly-scoped token
// typically lacks, for a value the caller already has as a string.
//
// The workspace is determined by the following order:
//  1. The workspace flag in the command
//  2. The git config
//  3. The default workspace in the profile
func GetWorkspaceName(context context.Context, cmd *cobra.Command) (workspaceName string, err error) {
	if cmd.Flag("workspace") != nil {
		if workspaceName = cmd.Flag("workspace").Value.String(); workspaceName != "" {
			lgr.Printf("[DEBUG] workspace name found in command flag: %s", workspaceName)
			return
		}
	}
	if remote, err := remote.GetRemote(context, cmd); err == nil {
		lgr.Printf("[DEBUG] workspace name found in git config: %s, from remote: %s", remote.WorkspaceName(), remote.URL)
		return remote.WorkspaceName(), nil
	}
	if profile.Current != nil && profile.Current.DefaultWorkspace != "" {
		lgr.Printf("[DEBUG] workspace name found in profile: %s", profile.Current.DefaultWorkspace)
		return profile.Current.DefaultWorkspace, nil
	}
	return "", errors.New("argument workspace is missing")
}

// GetWorkspace gets the current workspace as a full Workspace object, fetching it via
// GET /workspaces/{slug} (see GetWorkspaceBySlugOrID) when it is not already cached. Callers that
// only need the workspace slug, not its other fields (ID, Name), should call GetWorkspaceName
// instead: it never reaches the API, so it works with tokens that lack read:workspace.
//
// The workspace is determined by the following order:
// 1. The workspace flag in the command
// 2. The git config
// 3. The default workspace in the profile
func GetWorkspace(ctx context.Context, cmd *cobra.Command) (workspace *Workspace, err error) {
	workspaceName, err := GetWorkspaceName(ctx, cmd)
	if err != nil {
		return nil, err
	}
	return GetWorkspaceBySlugOrID(ctx, cmd, workspaceName)
}

// GetWorkspaceBySlugOrID gets the workspace by its slug name or ID
func GetWorkspaceBySlugOrID(ctx context.Context, cmd *cobra.Command, slugOrID string) (workspace *Workspace, err error) {
	if slugOrID == "" {
		return nil, errors.New("argument workspace slug or ID is missing")
	}

	if workspace, err = WorkspaceCache.Get(slugOrID); err == nil {
		lgr.Printf("[DEBUG] workspace %s found in cache", slugOrID)
		return workspace, nil
	}

	currentProfile, err := profile.GetProfileFromCommand(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("cannot get profile: %w", err)
	}

	lgr.Printf("[DEBUG] retrieving workspace %s", slugOrID)

	// In case we got a real UUID, get the Bitbucket UUID
	if parsedID, uuidErr := common.ParseUUID(slugOrID); uuidErr == nil {
		slugOrID = parsedID.String()
	}

	// slugOrID is a caller/user-supplied identifier (a --workspace value, a git remote's
	// workspace segment, or a profile default), so it is escaped before being interpolated into
	// the request path: resolveRequestURL splits on "?", so an unescaped slug containing one
	// would silently truncate the path and turn the remainder into a query string.
	err = currentProfile.Get(
		ctx,
		"/workspaces/"+url.PathEscape(slugOrID),
		&workspace,
	)
	if err != nil {
		return workspace, fmt.Errorf("failed to get workspace %s: %w", slugOrID, err)
	}
	if workspace == nil {
		return nil, fmt.Errorf("received an empty response for workspace %s", slugOrID)
	}
	_ = WorkspaceCache.Set(slugOrID, *workspace)
	return workspace, nil
}

// GetMembers gets the members of the workspace identified by workspaceSlug. workspaceSlug is a
// caller/user-supplied identifier, so it is escaped before being interpolated into the request
// path (see GetWorkspaceBySlugOrID for why).
func GetMembers(ctx context.Context, cmd *cobra.Command, workspaceSlug string) ([]Member, error) {
	members, err := profile.GetAll[Member](
		ctx,
		cmd,
		fmt.Sprintf("/workspaces/%s/members", url.PathEscape(workspaceSlug)),
	)
	if err != nil {
		return nil, fmt.Errorf("cannot get members of workspace %s: %w", workspaceSlug, err)
	}
	return members, nil
}

// MarshalJSON marshals the workspace to JSON
//
// implements json.Marshaler
func (workspace Workspace) MarshalJSON() ([]byte, error) {
	type surrogate Workspace

	data, err := json.Marshal(struct {
		Type string `json:"type"`
		surrogate
	}{
		Type:      workspace.GetType(),
		surrogate: surrogate(workspace),
	})
	if err != nil {
		return nil, fmt.Errorf("cannot marshal workspace to json: %w", err)
	}
	return data, nil
}

// UnmarshalJSON unmarshals the workspace from JSON
//
// implements json.Unmarshaler
func (workspace *Workspace) UnmarshalJSON(data []byte) error {
	type surrogate Workspace

	var typeholder struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &typeholder); err != nil {
		return fmt.Errorf("cannot unmarshal workspace: %w", err)
	}
	switch typeholder.Type {
	case "workspace_access":
		var inner struct {
			Type          string `json:"type"`
			Administrator bool   `json:"administrator"`
			Workspace     struct {
				Type string `json:"type"`
				surrogate
			} `json:"workspace"`
		}
		if err := json.Unmarshal(data, &inner); err != nil {
			return fmt.Errorf("cannot unmarshal workspace: %w", err)
		}
		if inner.Workspace.Type != "workspace_base" {
			return fmt.Errorf("cannot unmarshal workspace: invalid type %s, expected %s", inner.Workspace.Type, "workspace_base")
		}

		*workspace = Workspace(inner.Workspace.surrogate)
		workspace.Administrator = inner.Administrator
	case "workspace":
		var inner struct {
			Type string `json:"type"`
			surrogate
		}
		if err := json.Unmarshal(data, &inner); err != nil {
			return fmt.Errorf("cannot unmarshal workspace: %w", err)
		}
		if inner.Type != workspace.GetType() {
			return fmt.Errorf("cannot unmarshal workspace: invalid type %s, expected %s", inner.Type, workspace.GetType())
		}

		*workspace = Workspace(inner.surrogate)
	default:
		return fmt.Errorf("cannot unmarshal workspace: invalid type %s, expected %s", typeholder.Type, Workspace{}.GetType()+", "+"workspace_access")
	}

	return nil
}
