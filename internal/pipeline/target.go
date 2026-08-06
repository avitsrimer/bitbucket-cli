package pipeline

import (
	"github.com/avitsrimer/bitbucket-cli/internal/commit"
	"github.com/avitsrimer/bitbucket-cli/internal/common"
)

// pullRequestReference is the shape of the "pullrequest" object nested inside a
// pull-request-target payload; only the id is kept (see Target's doc comment).
type pullRequestReference struct {
	Type string `json:"type"`
	ID   uint64 `json:"id"`
}

// Target represents the target of a pipeline: a branch/tag reference, a bare commit reference, or
// a pull-request reference. BitBucket sends one shape or another depending on "type"; rather than
// a Target interface with a registry and three separate structs, every field any shape can carry
// is flattened into this one struct, with fields the current "type" doesn't use left at their zero
// value. PullRequest is a plain nested field rather than a flattened PullRequestID: the wire shape
// (a nested {"pullrequest":{"type":"pullrequest","id":N}} object) comes for free from encoding/
// json's normal struct tags, with no custom Marshal/Unmarshal needed to fold it in or out.
type Target struct {
	Type        string                  `json:"type"`
	RefType     string                  `json:"ref_type,omitempty"`
	RefName     string                  `json:"ref_name,omitempty"`
	Selector    *common.Selector        `json:"selector,omitempty"`
	Commit      *commit.CommitReference `json:"commit,omitempty"`
	Source      string                  `json:"source,omitempty"`
	Destination string                  `json:"destination,omitempty"`
	PullRequest *pullRequestReference   `json:"pullrequest,omitempty"`
}

// GetDestination returns the target's destination branch/tag name: RefName for a plain reference
// target, Destination for a pull-request target, or "" for a bare commit target.
func (target Target) GetDestination() string {
	if target.RefName != "" {
		return target.RefName
	}
	return target.Destination
}
