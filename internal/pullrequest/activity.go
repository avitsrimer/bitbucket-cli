package pullrequest

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/avitsrimer/bitbucket-cli/internal/commit"
	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/pullrequest/comment"
	"github.com/avitsrimer/bitbucket-cli/internal/user"
	"github.com/gildas/go-core"
	"github.com/spf13/cobra"
)

// Activity describes an activity on a PullRequest, which can be an approval, a comment or an update
type Activity struct {
	PullRequest PullRequestReference `json:"pull_request"`
	Approval    *ActivityApproval    `json:"approval,omitempty"`
	Comment     *comment.Comment     `json:"comment,omitempty"`
	Update      *ActivityUpdate      `json:"update,omitempty"`
}

// ActivityApproval describes an approval activity on a PullRequest
type ActivityApproval struct {
	Date        time.Time             `json:"date"`
	User        user.User             `json:"user"`
	PullRequest *PullRequestReference `json:"pullrequest"`
}

// ActivityUpdate describes an update activity on a PullRequest
type ActivityUpdate struct {
	Date              time.Time           `json:"date"`
	Type              string              `json:"type"`
	ID                uint64              `json:"id"`
	Title             string              `json:"title"`
	Description       string              `json:"description"`
	Summary           common.RenderedText `json:"summary"`
	State             string              `json:"state"`
	MergeCommit       *commit.Commit      `json:"merge_commit,omitempty"`
	CloseSourceBranch bool                `json:"close_source_branch"`
	ClosedBy          user.User           `json:"closed_by"`
	Author            user.User           `json:"author"`
	Reason            string              `json:"reason"`
	Destination       Endpoint            `json:"destination"`
	Source            Endpoint            `json:"source"`
	Links             common.Links        `json:"links"`
	CommentCount      uint64              `json:"comment_count"`
	TaskCount         uint64              `json:"task_count"`
	CreatedOn         time.Time           `json:"created_on"`
	UpdatedOn         time.Time           `json:"updated_on"`
}

var activityColumns = common.Columns[Activity]{
	{Name: "pull_request", DefaultSorter: true, Compare: func(a, b Activity) bool {
		return a.PullRequest.ID < b.PullRequest.ID
	}},
	{Name: "date", DefaultSorter: false, Compare: func(a, b Activity) bool {
		if a.Approval != nil && b.Approval != nil {
			return a.Approval.Date.Before(b.Approval.Date)
		} else if a.Update != nil && b.Update != nil {
			return a.Update.Date.Before(b.Update.Date)
		}
		return false
	}},
	{Name: "approved", DefaultSorter: false, Compare: func(a, b Activity) bool {
		if a.Approval != nil && b.Approval != nil {
			return a.Approval.User.Name < b.Approval.User.Name
		}
		return false
	}},
	{Name: "description", DefaultSorter: false, Compare: func(a, b Activity) bool {
		if a.Update != nil && b.Update != nil {
			return strings.ToLower(a.Update.Description) < strings.ToLower(b.Update.Description)
		}
		return false
	}},
	{Name: "state", DefaultSorter: false, Compare: func(a, b Activity) bool {
		if a.Update != nil && b.Update != nil {
			return strings.ToLower(a.Update.State) < strings.ToLower(b.Update.State)
		}
		return false
	}},
	{Name: "author", DefaultSorter: false, Compare: func(a, b Activity) bool {
		if a.Update != nil && b.Update != nil {
			return strings.ToLower(a.Update.Author.Name) < strings.ToLower(b.Update.Author.Name)
		}
		return false
	}},
	{Name: "closed_by", DefaultSorter: false, Compare: func(a, b Activity) bool {
		if a.Update != nil && b.Update != nil {
			return strings.ToLower(a.Update.ClosedBy.Name) < strings.ToLower(b.Update.ClosedBy.Name)
		}
		return false
	}},
	{Name: "reason", DefaultSorter: false, Compare: func(a, b Activity) bool {
		if a.Update != nil && b.Update != nil {
			return strings.ToLower(a.Update.Reason) < strings.ToLower(b.Update.Reason)
		}
		return false
	}},
	{Name: "user", DefaultSorter: false, Compare: func(a, b Activity) bool {
		if a.Approval != nil && b.Approval != nil {
			return strings.ToLower(a.Approval.User.Name) < strings.ToLower(b.Approval.User.Name)
		} else if a.Update != nil && b.Update != nil {
			return strings.ToLower(a.Update.Author.Name) < strings.ToLower(b.Update.Author.Name)
		}
		return false
	}},
	{Name: "destination", DefaultSorter: false, Compare: func(a, b Activity) bool {
		if a.Update != nil && b.Update != nil && a.Update.Destination.Repository != nil && b.Update.Destination.Repository != nil {
			return strings.ToLower(a.Update.Destination.Repository.Name) < strings.ToLower(b.Update.Destination.Repository.Name)
		}
		return false
	}},
	{Name: "source", DefaultSorter: false, Compare: func(a, b Activity) bool {
		if a.Update != nil && b.Update != nil && a.Update.Source.Repository != nil && b.Update.Source.Repository != nil {
			return strings.ToLower(a.Update.Source.Repository.Name) < strings.ToLower(b.Update.Source.Repository.Name)
		}
		return false
	}},
	{Name: "created_on", DefaultSorter: false, Compare: func(a, b Activity) bool {
		if a.Update != nil && b.Update != nil {
			return a.Update.CreatedOn.Before(b.Update.CreatedOn)
		}
		return false
	}},
	{Name: "updated_on", DefaultSorter: false, Compare: func(a, b Activity) bool {
		if a.Update != nil && b.Update != nil && !a.Update.UpdatedOn.IsZero() && !b.Update.UpdatedOn.IsZero() {
			return a.Update.UpdatedOn.Before(b.Update.UpdatedOn)
		}
		return false
	}},
}

// GetHeaders gets the header for a table
//
// implements common.Tableable
func (activity Activity) GetHeaders(cmd *cobra.Command) []string {
	if cmd != nil && cmd.Flag("columns") != nil && cmd.Flag("columns").Changed {
		if columns, err := cmd.Flags().GetStringSlice("columns"); err == nil {
			return core.Map(columns, func(column string) string { return strings.ReplaceAll(column, "_", " ") })
		}
	}
	return []string{"Date", "Approved", "State", "User"}
}

// GetRow gets the row for a table
//
// implements common.Tableable
func (activity Activity) GetRow(headers []string) []string {
	var activityDate time.Time
	var approval bool
	var state string
	var actor user.User

	switch {
	case activity.Approval != nil:
		activityDate = activity.Approval.Date
		approval = true
		actor = activity.Approval.User
		state = "N/A"
	case activity.Update != nil:
		activityDate = activity.Update.Date
		state = activity.Update.State
		actor = activity.Update.Author
	}

	row := make([]string, 0, len(headers))
	for _, header := range headers {
		switch strings.ToLower(header) {
		case "date":
			row = append(row, activityDate.Format("2006-01-02 15:04:05"))
		case "approved":
			row = append(row, strconv.FormatBool(approval))
		case "description":
			row = append(row, activity.updateField(func(update *ActivityUpdate) string { return update.Description }))
		case "state":
			row = append(row, state)
		case "author":
			row = append(row, activity.updateField(func(update *ActivityUpdate) string { return update.Author.Name }))
		case "closed by":
			row = append(row, activity.updateField(func(update *ActivityUpdate) string { return update.ClosedBy.Name }))
		case "reason":
			row = append(row, activity.updateField(func(update *ActivityUpdate) string { return update.Reason }))
		case "user":
			row = append(row, actor.Name)
		case "destination":
			row = append(row, activity.updateField(func(update *ActivityUpdate) string {
				if update.Destination.Repository == nil {
					return " "
				}
				return update.Destination.Repository.Name
			}))
		case "source":
			row = append(row, activity.updateField(func(update *ActivityUpdate) string {
				if update.Source.Repository == nil {
					return " "
				}
				return update.Source.Repository.Name
			}))
		case "created on", "created_on", "created-on", "created":
			row = append(row, activity.updateField(func(update *ActivityUpdate) string { return update.CreatedOn.Format("2006-01-02 15:04:05") }))
		case "updated on", "updated_on", "updated-on", "updated":
			row = append(row, activity.updateField(func(update *ActivityUpdate) string {
				if update.UpdatedOn.IsZero() {
					return " "
				}
				return update.UpdatedOn.Format("2006-01-02 15:04:05")
			}))
		}
	}
	return row
}

// updateField returns " " when activity has no Update, otherwise the value returned by get
func (activity Activity) updateField(get func(*ActivityUpdate) string) string {
	if activity.Update == nil {
		return " "
	}
	return get(activity.Update)
}

// Validate validates a Comment
func (activity *Activity) Validate() error {
	if activity.Approval == nil && activity.Comment == nil && activity.Update == nil {
		return errors.New("argument approval, comment, or update is missing")
	}
	return nil
}

// String gets a string representation of this pullrequest
//
// implements fmt.Stringer
func (activity Activity) String() string {
	return activity.PullRequest.Title
}

// MarshalJSON implements the json.Marshaler interface.
//
// implements json.Marshaler
func (activity Activity) MarshalJSON() (data []byte, err error) {
	type surrogate Activity

	data, err = json.Marshal(struct {
		surrogate
	}{
		surrogate: surrogate(activity),
	})
	if err != nil {
		return nil, fmt.Errorf("cannot marshal activity to json: %w", err)
	}
	return data, nil
}

// UnmarshalJSON implements the json.Unmarshaler interface.
//
// implements json.Unmarshaler
func (activity *Activity) UnmarshalJSON(data []byte) (err error) {
	type surrogate Activity

	var surrogateActivity surrogate
	if err = json.Unmarshal(data, &surrogateActivity); err != nil {
		return fmt.Errorf("cannot unmarshal activity: %w", err)
	}

	*activity = Activity(surrogateActivity)
	if err := activity.Validate(); err != nil {
		return fmt.Errorf("cannot unmarshal activity: %w", err)
	}
	return nil
}
