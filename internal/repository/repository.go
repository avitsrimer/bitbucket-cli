package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/project"
	"github.com/avitsrimer/bitbucket-cli/internal/remote"
	"github.com/avitsrimer/bitbucket-cli/internal/user"
	"github.com/avitsrimer/bitbucket-cli/internal/workspace"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

type Repository struct {
	ID                   common.UUID          `json:"uuid"`
	Name                 string               `json:"name,omitempty"`
	FullName             string               `json:"full_name,omitempty"`
	Slug                 string               `json:"slug,omitempty"`
	Owner                user.User            `json:"owner"`
	Workspace            *workspace.Workspace `json:"workspace,omitempty"`
	Project              project.Project      `json:"project"`
	HasIssues            bool                 `json:"has_issues"`
	HasWiki              bool                 `json:"has_wiki"`
	IsPrivate            bool                 `json:"is_private"`
	ForkPolicy           string               `json:"fork_policy,omitempty"`
	Size                 int64                `json:"size,omitempty"`
	Language             string               `json:"language,omitempty"`
	MainBranch           string               `json:"-"`
	DefaultMergeStrategy string               `json:"-"`
	BranchingModel       string               `json:"-"`
	Parent               *Repository          `json:"parent,omitempty"`
	Links                common.Links         `json:"links"`
	CreatedOn            time.Time            `json:"created_on"`
	UpdatedOn            time.Time            `json:"updated_on"`
}

type branch struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

// Command represents this folder's command
var Command = &cobra.Command{
	Use:     "repo",
	Aliases: []string{"repository"},
	Short:   "Manage repositories",
	Run:     common.SubcommandRequired("Repository"),
}

var columns = common.Columns[Repository]{
	{Name: "name", DefaultSorter: true, Compare: func(a, b Repository) bool {
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	}},
	{Name: "full_name", DefaultSorter: false, Compare: func(a, b Repository) bool {
		return strings.ToLower(a.FullName) < strings.ToLower(b.FullName)
	}},
	{Name: "slug", DefaultSorter: false, Compare: func(a, b Repository) bool {
		return strings.ToLower(a.Slug) < strings.ToLower(b.Slug)
	}},
	{Name: "owner", DefaultSorter: false, Compare: func(a, b Repository) bool {
		return strings.ToLower(a.Owner.Name) < strings.ToLower(b.Owner.Name)
	}},
	{Name: "workspace", DefaultSorter: false, Compare: func(a, b Repository) bool {
		return strings.ToLower(a.workspaceName()) < strings.ToLower(b.workspaceName())
	}},
	{Name: "project", DefaultSorter: false, Compare: func(a, b Repository) bool {
		return strings.ToLower(a.Project.Name) < strings.ToLower(b.Project.Name)
	}},
	{Name: "main_branch", DefaultSorter: false, Compare: func(a, b Repository) bool {
		return strings.ToLower(a.MainBranch) < strings.ToLower(b.MainBranch)
	}},
	{Name: "has_issues", DefaultSorter: false, Compare: func(a, b Repository) bool {
		return !a.HasIssues && b.HasIssues
	}},
	{Name: "has_wiki", DefaultSorter: false, Compare: func(a, b Repository) bool {
		return !a.HasWiki && b.HasWiki
	}},
	{Name: "is_private", DefaultSorter: false, Compare: func(a, b Repository) bool {
		return !a.IsPrivate && b.IsPrivate
	}},
	{Name: "fork_policy", DefaultSorter: false, Compare: func(a, b Repository) bool {
		return strings.ToLower(a.ForkPolicy) < strings.ToLower(b.ForkPolicy)
	}},
	{Name: "size", DefaultSorter: false, Compare: func(a, b Repository) bool {
		return a.Size < b.Size
	}},
	{Name: "language", DefaultSorter: false, Compare: func(a, b Repository) bool {
		return strings.ToLower(a.Language) < strings.ToLower(b.Language)
	}},
	{Name: "default_merge_strategy", DefaultSorter: false, Compare: func(a, b Repository) bool {
		return strings.ToLower(a.DefaultMergeStrategy) < strings.ToLower(b.DefaultMergeStrategy)
	}},
	{Name: "branching_model", DefaultSorter: false, Compare: func(a, b Repository) bool {
		return strings.ToLower(a.BranchingModel) < strings.ToLower(b.BranchingModel)
	}},
	{Name: "parent", DefaultSorter: false, Compare: func(a, b Repository) bool {
		if a.Parent == nil || b.Parent == nil {
			return a.Parent == nil && b.Parent != nil
		}
		return strings.ToLower(a.Parent.FullName) < strings.ToLower(b.Parent.FullName)
	}},
	{Name: "created_on", DefaultSorter: false, Compare: func(a, b Repository) bool {
		return a.CreatedOn.Before(b.CreatedOn)
	}},
	{Name: "updated_on", DefaultSorter: false, Compare: func(a, b Repository) bool {
		return a.UpdatedOn.Before(b.UpdatedOn)
	}},
}

var RepositoryCache = common.NewCache[Repository]()

// GetType gets the type of this repository
//
// implements core.TypeCarrier
func (repository Repository) GetType() string {
	return "repository"
}

// GetHeaders gets the header for a table
//
// implements common.Tableable
func (repository Repository) GetHeaders(cmd *cobra.Command) []string {
	return common.HeadersFromFlag(cmd, "Name", "Full Name", "Slug", "Workspace")
}

// GetRow gets the row for a table
//
// implements common.Tableable
func (repository Repository) GetRow(headers []string) []string {
	row := make([]string, 0, len(headers))

	for _, header := range headers {
		row = append(row, repository.getCell(common.NormalizeColumnKey(header)))
	}
	return row
}

// getCell renders the single value for a normalized column key, or " " when key does not match
// any declared column.
func (repository Repository) getCell(key string) string {
	switch key {
	case "name":
		return repository.Name
	case "full_name":
		return repository.FullName
	case "slug":
		return repository.Slug
	case "owner":
		return repository.Owner.Name
	case "workspace":
		return repository.workspaceName()
	case "project":
		return repository.Project.Name
	case "main_branch":
		return repository.MainBranch
	case "has_issues":
		return strconv.FormatBool(repository.HasIssues)
	case "has_wiki":
		return strconv.FormatBool(repository.HasWiki)
	case "is_private":
		return strconv.FormatBool(repository.IsPrivate)
	case "fork_policy":
		return repository.ForkPolicy
	case "size":
		return strconv.FormatInt(repository.Size, 10)
	case "language":
		return repository.Language
	case "default_merge_strategy":
		return repository.DefaultMergeStrategy
	case "branching_model":
		return repository.BranchingModel
	case "parent":
		return repository.parentFullName()
	case "created_on":
		return common.TimeCell(repository.CreatedOn)
	case "updated_on":
		return common.TimeCell(repository.UpdatedOn)
	default:
		return common.EmptyCell
	}
}

// workspaceName returns the repository's workspace name, or common.EmptyCell when Workspace is nil.
func (repository Repository) workspaceName() string {
	if repository.Workspace == nil {
		return common.EmptyCell
	}
	return repository.Workspace.Name
}

// parentFullName returns the repository's parent's full name, or common.EmptyCell when Parent is nil.
func (repository Repository) parentFullName() string {
	if repository.Parent == nil {
		return common.EmptyCell
	}
	return repository.Parent.FullName
}

// GetPath gets the API path of the repository
//
// The workspace segment comes from workspaceSlugFromFields, never from repository.Workspace.Slug
// directly: BitBucket omits "workspace" on a trimmed nested Repository payload (a pull request's
// source/destination repository, a commit's repository, ...), and GetPath has no *cobra.Command
// to fall back to the --workspace flag/git-remote/profile-default resolution GetWorkspaceSlug
// uses for that case, nor does it return an error existing callers could check. Falling back to
// FullName's workspace segment (itself no API call) instead of dereferencing a possibly-nil
// Workspace keeps this from panicking; when neither source has it, the workspace segment is
// simply empty, which 404s cleanly against the server instead of crashing the CLI.
func (repository Repository) GetPath(paths ...string) string {
	return path.Join(append([]string{"/repositories", repository.workspaceSlugFromFields(), repository.Slug}, paths...)...)
}

// String returns the string representation of the repository
//
// implements fmt.Stringer
func (repository Repository) String() string {
	if repository.Slug != "" {
		return repository.Slug
	}
	return repository.Name
}

// GetRepositoryName gets the name of the repository from the command line or from the git config
func GetRepositoryName(context context.Context, cmd *cobra.Command) (repositoryName string, err error) {
	if cmd.Flag("repository") != nil {
		if repositoryName = cmd.Flag("repository").Value.String(); repositoryName != "" {
			return
		}
	}
	if remote, err := remote.GetRemote(context, cmd); err == nil {
		return remote.RepositoryName(), nil
	}
	return "", errors.New("argument repository is missing")
}

// GetRepository gets a repository by its slug
func GetRepository(ctx context.Context, cmd *cobra.Command) (repository *Repository, err error) {
	name, err := GetRepositoryName(ctx, cmd)
	if err != nil {
		return nil, err
	}
	return GetRepositoryBySlugOrID(ctx, cmd, name)
}

// GetRepositoryBySlugOrID gets a repository by its slug name
//
// If the slug is in the format "workspace/repository", the workspace segment is used directly as
// the workspace slug.
//
// Otherwise, the workspace slug is determined by the --workspace flag, the git config, or the
// default workspace in the profile (see workspace.GetWorkspaceName). Neither branch fetches a
// Workspace object: the caller already supplied the workspace as a string, and a live
// GET /workspaces/{slug} would demand the read:workspace scope for a value nothing here needs to
// read back.
func GetRepositoryBySlugOrID(ctx context.Context, cmd *cobra.Command, slugOrID string) (repository *Repository, err error) {
	var workspaceSlug string

	if components := strings.Split(slugOrID, "/"); len(components) == 2 {
		lgr.Printf("[DEBUG] repository slug %s contains a workspace, extracting workspace and repository name", slugOrID)
		workspaceSlug = components[0]
		slugOrID = components[1]
	} else {
		lgr.Printf("[DEBUG] repository slug %s does not contain a workspace, using git config or default workspace", slugOrID)
		workspaceSlug, err = workspace.GetWorkspaceName(ctx, cmd)
		if err != nil {
			return nil, fmt.Errorf("cannot get workspace: %w", err)
		}
	}
	// An empty component here (a "workspace/repository" argument shaped like "/foo" or "foo/")
	// must be rejected explicitly: resolveRequestURL's JoinPath collapses "/repositories//foo"'s
	// double slash into "/repositories/foo", silently retargeting the request onto the *list
	// repositories in workspace "foo"* endpoint instead of erroring.
	if workspaceSlug == "" {
		return nil, errors.New("argument workspace is missing")
	}
	if slugOrID == "" {
		return nil, errors.New("argument repository is missing")
	}

	// In case we got a real UUID for either segment, get its canonical Bitbucket form (adds
	// braces): both segments are equally likely to be a bare UUID (a "workspace/repository"
	// argument can name either one that way), so both are normalized the same way a direct
	// object fetch (ParseUUID -> UUID.String()) always did.
	if parsedWorkspace, uuidErr := common.ParseUUID(workspaceSlug); uuidErr == nil {
		workspaceSlug = parsedWorkspace.String()
	}
	if parsedID, uuidErr := common.ParseUUID(slugOrID); uuidErr == nil {
		slugOrID = parsedID.String()
	}

	if repository, err = RepositoryCache.Get(fmt.Sprintf("%s/%s", workspaceSlug, slugOrID)); err == nil {
		lgr.Printf("[DEBUG] repository %s/%s found in cache", workspaceSlug, slugOrID)
		return repository, nil
	}

	lgr.Printf("[DEBUG] getting repository %s in workspace %s", slugOrID, workspaceSlug)
	profile, err := profile.GetProfileFromCommand(cmd.Context(), cmd)
	if err != nil {
		return nil, fmt.Errorf("cannot get profile: %w", err)
	}

	// workspaceSlug and slugOrID are caller/user-supplied identifiers (a "workspace/repository"
	// argument, a git remote, or a profile default), so both are escaped before being
	// interpolated into the request path: resolveRequestURL splits on "?", so an unescaped
	// segment containing one would silently truncate the path and turn the remainder into a
	// query string.
	err = profile.Get(
		ctx,
		fmt.Sprintf("/repositories/%s/%s", url.PathEscape(workspaceSlug), url.PathEscape(slugOrID)),
		&repository,
	)
	if err != nil {
		return repository, fmt.Errorf("cannot get resource: %w", err)
	}
	if repository == nil {
		return nil, fmt.Errorf("received an empty response for repository %s/%s", workspaceSlug, slugOrID)
	}
	_ = RepositoryCache.Set(fmt.Sprintf("%s/%s", workspaceSlug, slugOrID), *repository)
	return repository, nil
}

// GetEffectiveDefaultReviewers gets the effective default reviewers for a repository
//
// The workspace and repository slug segments of the request path are taken from FullName
// ("{workspace_slug}/{repo_slug}", the pairing BitBucket always sends and Validate requires)
// whenever it splits cleanly. repository.Slug alone cannot be trusted here: BitBucket omits
// "slug" on a pullrequest's source/destination repository, so Validate backfills Slug = Name,
// which 404s building this path for any repository whose display name differs from its slug
// ("My Repo" vs "my-repo"). When FullName isn't in the "{workspace}/{repo}" shape (e.g. a
// Repository built by hand rather than decoded from the API), GetWorkspaceSlug supplies the
// workspace segment instead, with no API call.
func (repository Repository) GetEffectiveDefaultReviewers(ctx context.Context, cmd *cobra.Command) (reviewers []project.Reviewer, err error) {
	workspaceSlug, repositorySlug, err := repository.effectiveReviewersPathSegments(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("cannot get workspace of repository %s: %w", repository.Slug, err)
	}
	lgr.Printf("[DEBUG] getting effective default reviewers of repository %s/%s", workspaceSlug, repositorySlug)
	return profile.GetAll[project.Reviewer](ctx, cmd, path.Join("/repositories", workspaceSlug, repositorySlug, "effective-default-reviewers"))
}

// effectiveReviewersPathSegments resolves the workspace slug and repository slug used to build
// GetEffectiveDefaultReviewers' request path. See that method's comment for why FullName is
// preferred over repository.Slug/GetWorkspaceSlug.
func (repository Repository) effectiveReviewersPathSegments(ctx context.Context, cmd *cobra.Command) (workspaceSlug, repositorySlug string, err error) {
	if fullNameWorkspace, fullNameRepository, ok := splitFullName(repository.FullName); ok {
		return fullNameWorkspace, fullNameRepository, nil
	}
	workspaceSlug, err = repository.GetWorkspaceSlug(ctx, cmd)
	if err != nil {
		return "", "", fmt.Errorf("cannot get workspace: %w", err)
	}
	return workspaceSlug, repository.Slug, nil
}

// splitFullName splits fullName ("{workspace_slug}/{repository_slug}", the shape BitBucket
// always sends) into its two segments, reporting ok=false when it does not split cleanly into
// exactly two non-empty parts.
func splitFullName(fullName string) (workspaceSlug, repositorySlug string, ok bool) {
	components := strings.SplitN(fullName, "/", 2)
	if len(components) != 2 || components[0] == "" || components[1] == "" {
		return "", "", false
	}
	return components[0], components[1], true
}

// workspaceSlugFromFields resolves the workspace slug using only data already present on
// repository -- repository.Workspace.Slug when populated (the API's own response for this
// repository already carried it -- no extra request), or the workspace segment of FullName when
// it splits cleanly into "{workspace}/{repo}" -- touching the network never. Returns "" when
// neither source has it, leaving the network fallback (the --workspace flag/git-remote/
// profile-default workspace, see workspace.GetWorkspaceName) to callers that have a context and
// command to resolve it with; GetPath does not, so it stops here.
func (repository Repository) workspaceSlugFromFields() string {
	if repository.Workspace != nil && repository.Workspace.Slug != "" {
		return repository.Workspace.Slug
	}
	if workspaceSlug, _, ok := splitFullName(repository.FullName); ok {
		return workspaceSlug
	}
	return ""
}

// GetWorkspaceSlug gets the workspace slug of the repository without ever fetching a Workspace
// object: repository.Workspace.Slug when it is already populated, the workspace segment of
// FullName when it splits cleanly into "{workspace}/{repo}" (see workspaceSlugFromFields for
// both), or otherwise the --workspace flag/git-remote/profile-default workspace slug (see
// workspace.GetWorkspaceName). None of these read paths call the BitBucket API, so none of them
// need the read:workspace scope.
func (repository Repository) GetWorkspaceSlug(ctx context.Context, cmd *cobra.Command) (string, error) {
	if workspaceSlug := repository.workspaceSlugFromFields(); workspaceSlug != "" {
		lgr.Printf("[DEBUG] getting workspace of repository %s/%s from known fields", repository.FullName, repository.Slug)
		return workspaceSlug, nil
	}

	workspaceSlug, err := workspace.GetWorkspaceName(ctx, cmd)
	if err != nil {
		return "", fmt.Errorf("cannot get workspace: %w", err)
	}
	return workspaceSlug, nil
}

// Validate validates a Repository
func (repository *Repository) Validate() error {
	var errs []error

	if repository.ID.IsNil() {
		errs = append(errs, errors.New("argument uuid is missing"))
	}
	if repository.Name == "" {
		errs = append(errs, errors.New("argument name is missing"))
	}
	if repository.FullName == "" {
		errs = append(errs, errors.New("argument full_name is missing"))
	}
	if repository.Slug == "" {
		repository.Slug = repository.Name
	}

	return errors.Join(errs...)
}

// MarshalJSON implements the json.Marshaler interface.
//
// Implements json.Marshaler
func (repository Repository) MarshalJSON() (data []byte, err error) {
	type surrogate Repository
	var owner *user.User
	var proj *project.Project
	var br *branch
	var createdOn string
	var updatedOn string
	var hasIssues *bool
	var hasWiki *bool
	var isPrivate *bool

	if !repository.Owner.ID.IsNil() {
		owner = &repository.Owner
	}
	if !repository.Project.ID.IsNil() {
		proj = &repository.Project
	}
	if repository.MainBranch != "" {
		br = &branch{Type: "branch", Name: repository.MainBranch}
	}
	if !repository.CreatedOn.IsZero() {
		createdOn = repository.CreatedOn.Format(common.JSONTimeFormat)
	}
	if !repository.UpdatedOn.IsZero() {
		updatedOn = repository.UpdatedOn.Format(common.JSONTimeFormat)
	}
	if repository.HasIssues {
		hasIssues = &repository.HasIssues
	}
	if repository.HasWiki {
		hasWiki = &repository.HasWiki
	}
	if repository.IsPrivate {
		isPrivate = &repository.IsPrivate
	}
	if repository.Slug == repository.Name {
		repository.Slug = ""
	}

	data, err = json.Marshal(struct {
		Type string `json:"type"`
		surrogate
		Owner      *user.User       `json:"owner,omitempty"`
		Project    *project.Project `json:"project,omitempty"`
		MainBranch *branch          `json:"mainbranch,omitempty"`
		CreatedOn  string           `json:"created_on,omitempty"`
		UpdatedOn  string           `json:"updated_on,omitempty"`
		HasIssues  *bool            `json:"has_issues,omitempty"`
		HasWiki    *bool            `json:"has_wiki,omitempty"`
		IsPrivate  *bool            `json:"is_private,omitempty"`
	}{
		Type:       repository.GetType(),
		surrogate:  surrogate(repository),
		Owner:      owner,
		Project:    proj,
		MainBranch: br,
		CreatedOn:  createdOn,
		UpdatedOn:  updatedOn,
		HasIssues:  hasIssues,
		HasWiki:    hasWiki,
		IsPrivate:  isPrivate,
	})
	if err != nil {
		return nil, fmt.Errorf("cannot marshal repository to json: %w", err)
	}
	return data, nil
}

// UnmarshalJSON implements the json.Unmarshaler interface.
//
// Implements json.Unmarshaler
func (repository *Repository) UnmarshalJSON(data []byte) (err error) {
	type surrogate Repository
	var inner struct {
		Type string `json:"type"`
		surrogate
		MainBranch branch `json:"mainbranch"`
	}
	if err = json.Unmarshal(data, &inner); err != nil {
		return fmt.Errorf("cannot unmarshal repository: %w", err)
	}
	if inner.Type != repository.GetType() {
		return fmt.Errorf("cannot unmarshal repository: invalid type %s, expected %s", inner.Type, repository.GetType())
	}
	*repository = Repository(inner.surrogate)
	repository.MainBranch = inner.MainBranch.Name
	if err := repository.Validate(); err != nil {
		return fmt.Errorf("cannot unmarshal repository: %w", err)
	}
	return nil
}
