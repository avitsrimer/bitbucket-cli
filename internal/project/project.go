package project

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/user"
	"github.com/avitsrimer/bitbucket-cli/internal/workspace"
)

type Project struct {
	Type                           string              `json:"type"`
	ID                             common.UUID         `json:"uuid"`
	Name                           string              `json:"name"`
	Description                    string              `json:"description,omitempty"`
	Key                            string              `json:"key"`
	Owner                          user.User           `json:"owner"`
	Workspace                      workspace.Workspace `json:"workspace"`
	Links                          common.Links        `json:"links"`
	IsPrivate                      bool                `json:"is_private"`
	HasPubliclyVisibleRepositories bool                `json:"has_publicly_visible_repos"`
	CreatedOn                      time.Time           `json:"created_on"`
	UpdatedOn                      time.Time           `json:"updated_on"`
}

// String gets a string representation of this pullrequest
//
// implements fmt.Stringer
func (project Project) String() string {
	return project.Name
}

// MarshalJSON implements the json.Marshaler interface.
func (project Project) MarshalJSON() (data []byte, err error) {
	type surrogate Project
	var owner *user.User
	var wspace *workspace.Workspace
	var createdOn string
	var updatedOn string

	if !project.Owner.ID.IsNil() {
		owner = &project.Owner
	}
	if !project.Workspace.ID.IsNil() {
		wspace = &project.Workspace
	}
	if !project.CreatedOn.IsZero() {
		createdOn = project.CreatedOn.Format("2006-01-02T15:04:05.999999999-07:00")
	}
	if !project.UpdatedOn.IsZero() {
		updatedOn = project.UpdatedOn.Format("2006-01-02T15:04:05.999999999-07:00")
	}

	data, err = json.Marshal(struct {
		surrogate
		Owner     *user.User           `json:"owner,omitempty"`
		Workspace *workspace.Workspace `json:"workspace,omitempty"`
		CreatedOn string               `json:"created_on,omitempty"`
		UpdatedOn string               `json:"updated_on,omitempty"`
	}{
		surrogate: surrogate(project),
		Owner:     owner,
		Workspace: wspace,
		CreatedOn: createdOn,
		UpdatedOn: updatedOn,
	})
	if err != nil {
		return nil, fmt.Errorf("cannot marshal json: %w", err)
	}
	return data, nil
}
