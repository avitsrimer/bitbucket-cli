package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/gildas/bitbucket-cli/cmd/common"
	"github.com/gildas/bitbucket-cli/cmd/profile"
	"github.com/gildas/bitbucket-cli/cmd/project"
	"github.com/gildas/bitbucket-cli/cmd/project/reviewer"
	"github.com/gildas/bitbucket-cli/cmd/remote"
	"github.com/gildas/bitbucket-cli/cmd/user"
	"github.com/gildas/bitbucket-cli/cmd/workspace"
	"github.com/gildas/go-errors"
	"github.com/gildas/go-logger"
	"github.com/spf13/cobra"
)

type Repository struct {
	ID                   common.UUID          `json:"uuid"                  mapstructure:"uuid"`
	Name                 string               `json:"name,omitempty"                  mapstructure:"name"`
	FullName             string               `json:"full_name,omitempty"             mapstructure:"full_name"`
	Slug                 string               `json:"slug,omitempty"                  mapstructure:"slug"`
	Owner                user.User            `json:"owner,omitempty"                 mapstructure:"owner"`
	Workspace            *workspace.Workspace `json:"workspace,omitempty"             mapstructure:"workspace"`
	Project              project.Project      `json:"project,omitempty"               mapstructure:"project"`
	HasIssues            bool                 `json:"has_issues"            mapstructure:"has_issues"`
	HasWiki              bool                 `json:"has_wiki"              mapstructure:"has_wiki"`
	IsPrivate            bool                 `json:"is_private"            mapstructure:"is_private"`
	ForkPolicy           string               `json:"fork_policy,omitempty" mapstructure:"fork_policy"`
	Size                 int64                `json:"size,omitempty"                  mapstructure:"size"`
	Language             string               `json:"language,omitempty"    mapstructure:"language"`
	MainBranch           string               `json:"-"                     mapstructure:"-"`
	DefaultMergeStrategy string               `json:"-"                     mapstructure:"-"`
	BranchingModel       string               `json:"-"                     mapstructure:"-"`
	Parent               *Repository          `json:"parent,omitempty"      mapstructure:"parent"`
	Links                common.Links         `json:"links"                 mapstructure:"links"`
	CreatedOn            time.Time            `json:"created_on"            mapstructure:"created_on"`
	UpdatedOn            time.Time            `json:"updated_on"            mapstructure:"updated_on"`
}

type branch struct {
	Type string `json:"type" mapstructure:"type"`
	Name string `json:"name" mapstructure:"name"`
}

var RepositoryCache = common.NewCache[Repository]()

// GetType gets the type of this repository
//
// implements core.TypeCarrier
func (repository Repository) GetType() string {
	return "repository"
}

// GetPath gets the API path of the repository
func (repository Repository) GetPath(paths ...string) string {
	return path.Join(append([]string{"/repositories", repository.Workspace.Slug, repository.Slug}, paths...)...)
}

// String returns the string representation of the repository
//
// implements fmt.Stringer
func (repository Repository) String() string {
	if len(repository.Slug) > 0 {
		return repository.Slug
	}
	return repository.Name
}

// GetRepositoryName gets the name of the repository from the command line or from the git config
func GetRepositoryName(context context.Context, cmd *cobra.Command) (repositoryName string, err error) {
	if cmd.Flag("repository") != nil {
		if repositoryName = cmd.Flag("repository").Value.String(); len(repositoryName) > 0 {
			return
		}
	}
	if remote, err := remote.GetRemote(context, cmd); err == nil {
		return remote.RepositoryName(), nil
	}
	return "", errors.ArgumentMissing.With("repository")
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
// If the slug is in the format "workspace/repository", the workspace is used to get the repository.
//
// Otherwise, the workspace is determined by the git config or the default workspace in the profile.
func GetRepositoryBySlugOrID(ctx context.Context, cmd *cobra.Command, slugOrID string) (repository *Repository, err error) {
	log := logger.Must(logger.FromContext(ctx)).Child("repository", "get_by_slug_or_id", "repository", slugOrID)
	var ws *workspace.Workspace

	if components := strings.Split(slugOrID, "/"); len(components) == 2 {
		log.Debugf("Repository slug %s contains a workspace, extracting workspace and repository name", slugOrID)
		slugOrID = components[1]
		ws, err = workspace.GetWorkspaceBySlugOrID(ctx, cmd, components[0])
	} else {
		log.Debugf("Repository slug %s does not contain a workspace, using git config or default workspace", slugOrID)
		ws, err = workspace.GetWorkspace(ctx, cmd)
	}
	if err != nil {
		return nil, err
	}

	// In case we got a real UUID, get the Bitbucket UUID
	if id, err := common.ParseUUID(slugOrID); err == nil {
		slugOrID = id.String()
	}

	if repository, err = RepositoryCache.Get(fmt.Sprintf("%s/%s", ws.Slug, slugOrID)); err == nil {
		log.Debugf("Repository %s/%s found in cache", ws.Slug, slugOrID)
		return
	}

	log.Infof("Getting repository %s in workspace %s", slugOrID, ws.Slug)
	profile, err := profile.GetProfileFromCommand(cmd.Context(), cmd)
	if err != nil {
		return nil, err
	}

	err = profile.Get(
		ctx,
		cmd,
		fmt.Sprintf("/repositories/%s/%s", ws.Slug, slugOrID),
		&repository,
	)
	if err == nil {
		_ = RepositoryCache.Set(*repository, fmt.Sprintf("%s/%s", ws.Slug, slugOrID))
	}
	return
}

// GetEffectiveDefaultReviewers gets the effective default reviewers for a repository
func (repository Repository) GetEffectiveDefaultReviewers(ctx context.Context, cmd *cobra.Command) (reviewers []reviewer.Reviewer, err error) {
	log := logger.Must(logger.FromContext(ctx)).Child("repository", "effective-default-reviewers")

	log.Infof("Getting effective default reviewers of repository %s/%s", repository.Workspace.Slug, repository.Slug)
	return profile.GetAll[reviewer.Reviewer](ctx, cmd, repository.GetPath("effective-default-reviewers"))
}

// GetWorkspace gets the workspace of the repository
func (repository Repository) GetWorkspace(ctx context.Context, cmd *cobra.Command) (*workspace.Workspace, error) {
	log := logger.Must(logger.FromContext(ctx)).Child("repository", "get_workspace")

	if repository.Workspace != nil && !repository.Workspace.ID.IsNil() && len(repository.Workspace.Slug) > 0 {
		log.Debugf("Getting workspace of repository %s/%s from cache", repository.Workspace.Slug, repository.Slug)
		return repository.Workspace, nil
	}

	if len(repository.FullName) > 0 {
		log.Debugf("Getting workspace of repository %s/%s from full name", repository.FullName, repository.Slug)
		components := strings.Split(repository.FullName, "/")
		if len(components) == 2 {
			return workspace.GetWorkspaceBySlugOrID(ctx, cmd, components[0])
		}
	}
	return workspace.GetWorkspace(ctx, cmd)
}

// Validate validates a Repository
func (repository *Repository) Validate() error {
	var merr errors.MultiError

	if repository.ID.IsNil() {
		merr.Append(errors.ArgumentMissing.With("uuid"))
	}
	if len(repository.Name) == 0 {
		merr.Append(errors.ArgumentMissing.With("name"))
	}
	if len(repository.FullName) == 0 {
		merr.Append(errors.ArgumentMissing.With("full_name"))
	}
	if len(repository.Slug) == 0 {
		repository.Slug = repository.Name
	}

	return merr.AsError()
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
	if len(repository.MainBranch) > 0 {
		br = &branch{Type: "branch", Name: repository.MainBranch}
	}
	if !repository.CreatedOn.IsZero() {
		createdOn = repository.CreatedOn.Format("2006-01-02T15:04:05.999999999-07:00")
	}
	if !repository.UpdatedOn.IsZero() {
		updatedOn = repository.UpdatedOn.Format("2006-01-02T15:04:05.999999999-07:00")
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
	return data, errors.JSONMarshalError.Wrap(err)
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
		return errors.JSONUnmarshalError.Wrap(err)
	}
	if inner.Type != repository.GetType() {
		return errors.JSONUnmarshalError.Wrap(errors.InvalidType.With(inner.Type, repository.GetType()))
	}
	*repository = Repository(inner.surrogate)
	repository.MainBranch = inner.MainBranch.Name
	return errors.JSONUnmarshalError.Wrap(repository.Validate())
}
