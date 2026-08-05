package commit

import (
	"encoding/json"

	"github.com/gildas/bitbucket-cli/cmd/common"
	"github.com/gildas/go-errors"
)

type CommitReference struct {
	Hash  string       `json:"hash"  mapstructure:"hash"`
	Links common.Links `json:"links" mapstructure:"links"`
}

// String gets a string representation of this commit
//
// implements fmt.Stringer
func (reference CommitReference) String() string {
	return reference.Hash
}

// MarshalJSON implements the json.Marshaler interface.
func (reference CommitReference) MarshalJSON() (data []byte, err error) {
	type surrogate CommitReference
	var links *common.Links

	if !reference.Links.IsEmpty() {
		links = &reference.Links
	}

	data, err = json.Marshal(struct {
		Type string `json:"type"`
		surrogate
		Links *common.Links `json:"links,omitempty"`
	}{
		Type:      "commit",
		surrogate: surrogate(reference),
		Links:     links,
	})
	return data, errors.JSONMarshalError.Wrap(err)
}
