package branch

import (
	"encoding/json"
	"fmt"

	"github.com/avitsrimer/bitbucket-cli/internal/commit"
	"github.com/avitsrimer/bitbucket-cli/internal/common"
)

type Branch struct {
	Name                 string        `json:"name"`
	Target               commit.Commit `json:"target"`
	Links                common.Links  `json:"links"`
	MergeStrategies      []string      `json:"merge_strategies,omitempty"`
	DefaultMergeStrategy string        `json:"default_merge_strategy,omitempty"`
}

// GetType returns the branch type
func (branch Branch) GetType() string {
	return "branch"
}

// String gets a string representation of this Branch
//
// implements fmt.Stringer
func (branch Branch) String() string {
	return branch.Name
}

// MarshalJSON custom JSON marshaling for Branch
//
// implements json.Marshaler
func (branch Branch) MarshalJSON() ([]byte, error) {
	type surrogate Branch
	data, err := json.Marshal(struct {
		Type string `json:"type"`
		surrogate
	}{
		Type:      branch.GetType(),
		surrogate: surrogate(branch),
	})
	if err != nil {
		return nil, fmt.Errorf("cannot marshal json: %w", err)
	}
	return data, nil
}

// UnmarshalJSON custom JSON unmarshalling for Branch
//
// implements json.Unmarshaler
func (branch *Branch) UnmarshalJSON(data []byte) error {
	type surrogate Branch
	var inner struct {
		Type string `json:"type"`
		surrogate
	}

	if err := json.Unmarshal(data, &inner); err != nil {
		return fmt.Errorf("cannot unmarshal json: %w", err)
	}
	if inner.Type != branch.GetType() {
		return fmt.Errorf("invalid type %s, expected %s", inner.Type, branch.GetType())
	}
	*branch = Branch(inner.surrogate)
	return nil
}
