package pipeline

import (
	"encoding/json"
	"fmt"

	"github.com/avitsrimer/bitbucket-cli/internal/commit"
	"github.com/avitsrimer/bitbucket-cli/internal/common"
)

// Target represents the target of a pipeline: a branch/tag reference, a bare commit reference, or
// a pull-request reference. BitBucket sends one shape or another depending on "type"; rather than
// a Target interface with a registry and three separate structs, every field any shape can carry
// is flattened into this one struct, with fields the current "type" doesn't use left at their zero
// value.
type Target struct {
	Type          string                  `json:"type"`
	RefType       string                  `json:"ref_type,omitempty"`
	RefName       string                  `json:"ref_name,omitempty"`
	Selector      *common.Selector        `json:"selector,omitempty"`
	Commit        *commit.CommitReference `json:"commit,omitempty"`
	Source        string                  `json:"source,omitempty"`
	Destination   string                  `json:"destination,omitempty"`
	PullRequestID uint64                  `json:"-"`
}

// pullRequestReference is the minimal shape of the "pullrequest" object nested inside a
// pull-request-target payload; only the id is kept (see Target's doc comment).
type pullRequestReference struct {
	Type string `json:"type"`
	ID   uint64 `json:"id"`
}

// GetDestination returns the target's destination branch/tag name: RefName for a plain reference
// target, Destination for a pull-request target, or "" for a bare commit target.
func (target Target) GetDestination() string {
	if target.RefName != "" {
		return target.RefName
	}
	return target.Destination
}

// MarshalJSON implements the json.Marshaler interface.
func (target Target) MarshalJSON() ([]byte, error) {
	type surrogate Target

	var pullRequest *pullRequestReference
	if target.PullRequestID != 0 {
		pullRequest = &pullRequestReference{Type: "pullrequest", ID: target.PullRequestID}
	}

	data, err := json.Marshal(struct {
		surrogate
		PullRequest *pullRequestReference `json:"pullrequest,omitempty"`
	}{
		surrogate:   surrogate(target),
		PullRequest: pullRequest,
	})
	if err != nil {
		return nil, fmt.Errorf("cannot marshal pipeline target to json: %w", err)
	}
	return data, nil
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (target *Target) UnmarshalJSON(data []byte) error {
	type surrogate Target
	var inner struct {
		surrogate
		PullRequest *pullRequestReference `json:"pullrequest,omitempty"`
	}
	if err := json.Unmarshal(data, &inner); err != nil {
		return fmt.Errorf("cannot unmarshal pipeline target: %w", err)
	}
	*target = Target(inner.surrogate)
	if inner.PullRequest != nil {
		target.PullRequestID = inner.PullRequest.ID
	}
	return nil
}
