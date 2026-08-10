package pullrequest

import (
	"github.com/avitsrimer/bitbucket-cli/internal/commit"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
)

type Endpoint struct {
	Branch     Branch                  `json:"branch"`
	Commit     *commit.CommitReference `json:"commit,omitempty"`
	Repository *repository.Repository  `json:"repository,omitempty"`
}

// repositoryFullName returns the endpoint's repository full name ("workspace/repo-slug"), or an
// empty string when the payload carried no repository. The field is optional in the API, so both
// the "repository" column's renderer and its sorter go through this instead of dereferencing.
func (endpoint Endpoint) repositoryFullName() string {
	if endpoint.Repository == nil {
		return ""
	}
	return endpoint.Repository.FullName
}

// repositoryName returns the endpoint's repository Name, or "" when the endpoint carries no
// repository (e.g. a source/destination whose repository was deleted after the pull request was
// opened). Companion of repositoryFullName -- both nil-safe accessors live here, in the same shape.
func (endpoint Endpoint) repositoryName() string {
	if endpoint.Repository == nil {
		return ""
	}
	return endpoint.Repository.Name
}
