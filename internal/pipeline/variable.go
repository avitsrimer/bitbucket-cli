package pipeline

import (
	"encoding/json"
	"fmt"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
)

// Variable represents a pipeline variable
type Variable struct {
	ID      common.UUID `json:"uuid"`
	Key     string      `json:"key"`
	Value   string      `json:"value"`
	Secured bool        `json:"secured"`
}

// MarshalJSON implements the json.Marshaler interface.
func (variable Variable) MarshalJSON() ([]byte, error) {
	type surrogate Variable
	var id *common.UUID
	if !variable.ID.IsNil() {
		id = &variable.ID
	}

	data, err := json.Marshal(struct {
		ID *common.UUID `json:"uuid,omitempty"`
		surrogate
	}{
		ID:        id,
		surrogate: surrogate(variable),
	})
	if err != nil {
		return nil, fmt.Errorf("cannot marshal pipeline variable to json: %w", err)
	}
	return data, nil
}
