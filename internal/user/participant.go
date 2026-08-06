package user

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type Participant struct {
	Type           string    `json:"type"`
	User           User      `json:"user"`
	Role           string    `json:"role"`
	Approved       bool      `json:"approved"`
	State          string    `json:"state"`
	ParticipatedOn time.Time `json:"participated_on"`
}

// MarshalJSON implements the json.Marshaler interface.
//
// ParticipatedOn is only formatted (and only included at all, via omitempty) when non-zero:
// BitBucket returns no participated_on at all for a reviewer who has not yet acted, which decodes
// here as the zero time.Time, and a year-1 "0001-01-01T00:00:00Z" in machine-readable output has
// no meaning to a caller scripting against it. Matches the same pattern already used by
// Comment/Resolution/ActivityUpdate for their own optional timestamps.
func (participant Participant) MarshalJSON() (data []byte, err error) {
	type surrogate Participant

	data, err = json.Marshal(struct {
		surrogate
		ParticipatedOn *string `json:"participated_on,omitempty"`
	}{
		surrogate:      surrogate(participant),
		ParticipatedOn: common.FormatOptionalTime(participant.ParticipatedOn),
	})
	if err != nil {
		return nil, fmt.Errorf("cannot marshal participant to json: %w", err)
	}
	return data, nil
}

// MarshalYAML implements the yaml.Marshaler interface.
//
// ParticipatedOn is only included when non-zero, mirroring MarshalJSON's omission of the same
// year-1 "0001-01-01T00:00:00Z" timestamp for a reviewer who has not yet acted: yaml.Marshal never
// consults a type's json.Marshaler, so without this a `-o yaml` reviewer list would still leak the
// zero value MarshalJSON already hides from `-o json`.
func (participant Participant) MarshalYAML() (any, error) {
	type surrogate Participant

	var node yaml.Node
	if err := node.Encode(surrogate(participant)); err != nil {
		return nil, fmt.Errorf("cannot encode participant: %w", err)
	}
	if participant.ParticipatedOn.IsZero() {
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value == "participatedon" {
				node.Content = append(node.Content[:i], node.Content[i+2:]...)
				break
			}
		}
	}
	return &node, nil
}

// GetHeaders gets the header for a table
//
// implements common.Tableable
//
// cmd's --columns flag is intentionally not consulted: a Participant is printed only as the
// single result of a pullrequest approve/unapprove/decline/request-changes/remove-request-changes
// action, none of which register a --columns flag, so there is never a value to read here.
func (participant Participant) GetHeaders(cmd *cobra.Command) []string {
	return []string{"ID", "Name", "participated on", "approved", "state"}
}

// GetRow gets the row for a table
//
// implements common.Tableable
func (participant Participant) GetRow(headers []string) []string {
	var row []string

	for _, header := range headers {
		switch common.NormalizeColumnKey(header) {
		case "id":
			row = append(row, participant.User.ID.String())
		case "name":
			row = append(row, participant.User.Name)
		case "participated_on":
			row = append(row, common.TimeCell(participant.ParticipatedOn))
		case "approved":
			row = append(row, strconv.FormatBool(participant.Approved))
		case "state":
			row = append(row, participant.State)
		default:
			row = append(row, common.EmptyCell)
		}
	}
	return row
}
