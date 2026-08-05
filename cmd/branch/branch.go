package branch

import (
	"encoding/json"

	"github.com/gildas/bitbucket-cli/cmd/commit"
	"github.com/gildas/bitbucket-cli/cmd/common"
	"github.com/gildas/go-errors"
)

type Branch struct {
	Name                 string        `json:"name"                             mapstructure:"name"`
	Target               commit.Commit `json:"target"                           mapstructure:"target"`
	Links                common.Links  `json:"links"                            mapstructure:"links"`
	MergeStrategies      []string      `json:"merge_strategies,omitempty"       mapstructure:"merge_strategies"`
	DefaultMergeStrategy string        `json:"default_merge_strategy,omitempty" mapstructure:"default_merge_strategy"`
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

// MarshalJSON custom JSON marshalling for Branch
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
	return data, errors.JSONMarshalError.Wrap(err)
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
		return errors.JSONUnmarshalError.WrapIfNotMe(err)
	}
	if inner.Type != branch.GetType() {
		return errors.JSONUnmarshalError.Wrap(errors.InvalidType.With(inner.Type, branch.GetType()))
	}
	*branch = Branch(inner.surrogate)
	return nil
}
