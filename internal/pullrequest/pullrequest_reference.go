package pullrequest

import (
	"encoding/json"
	"fmt"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
)

// PullRequestReference describes a reference to a PullRequest
type PullRequestReference struct {
	ID       uint64       `json:"id"`
	Title    string       `json:"title,omitempty"`
	IsDraft  bool         `json:"draft,omitempty"`
	IsQueued bool         `json:"queued,omitempty"`
	Links    common.Links `json:"links"`
}

// GetType returns the type of the PullRequestReference.
//
// implements core.TypeCarrier
func (reference PullRequestReference) GetType() string {
	return "pullrequest"
}

// MarshalJSON marshals the PullRequestReference to JSON
//
// implements json.Marshaler
func (reference PullRequestReference) MarshalJSON() ([]byte, error) {
	type surrogate PullRequestReference
	var links *common.Links

	if !reference.Links.IsEmpty() {
		links = &reference.Links
	}

	data, err := json.Marshal(struct {
		Type string `json:"type"`
		surrogate
		Links *common.Links `json:"links,omitempty"`
	}{
		Type:      reference.GetType(),
		surrogate: surrogate(reference),
		Links:     links,
	})
	if err != nil {
		return nil, fmt.Errorf("cannot marshal json: %w", err)
	}
	return data, nil
}
