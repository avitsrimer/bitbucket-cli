package user

import (
	"strconv"
	"time"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/spf13/cobra"
)

type Participant struct {
	Type           string    `json:"type"`
	User           User      `json:"user"`
	Role           string    `json:"role"`
	Approved       bool      `json:"approved"`
	State          string    `json:"state"`
	ParticipatedOn time.Time `json:"participated_on"`
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
