package task

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/pullrequest/comment"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/avitsrimer/bitbucket-cli/internal/user"
	"github.com/gildas/go-core"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

// Task represents a pull request task
type Task struct {
	ID         int                 `json:"id"`
	Content    common.RenderedText `json:"content"`
	Creator    user.User           `json:"creator"`
	IsPending  bool                `json:"pending"`
	State      string              `json:"state"`
	Comment    *comment.Comment    `json:"comment,omitempty"`
	ResolvedBy *user.User          `json:"resolved_by,omitempty"`
	CreatedOn  time.Time           `json:"created_on"`
	UpdatedOn  time.Time           `json:"updated_on"`
	ResolvedOn *time.Time          `json:"resolved_on,omitempty"`
}

// Command represents this folder's command
var Command = &cobra.Command{
	Use:   "task",
	Short: "Manage tasks",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Task requires a subcommand:")
		for _, command := range cmd.Commands() {
			fmt.Println(command.Name())
		}
	},
}

var columns = common.Columns[Task]{
	{Name: "id", DefaultSorter: true, Compare: func(a, b Task) bool {
		return a.ID < b.ID
	}},
	{Name: "content", DefaultSorter: false, Compare: func(a, b Task) bool {
		return strings.ToLower(a.Content.Raw) < strings.ToLower(b.Content.Raw)
	}},
	{Name: "creator", DefaultSorter: false, Compare: func(a, b Task) bool {
		return strings.ToLower(a.Creator.Name) < strings.ToLower(b.Creator.Name)
	}},
	{Name: "created_on", DefaultSorter: false, Compare: func(a, b Task) bool {
		return a.CreatedOn.Before(b.CreatedOn)
	}},
	{Name: "updated_on", DefaultSorter: false, Compare: func(a, b Task) bool {
		return a.UpdatedOn.Before(b.UpdatedOn)
	}},
	{Name: "resolved_on", DefaultSorter: false, Compare: func(a, b Task) bool {
		if a.ResolvedOn == nil {
			return false
		}
		if b.ResolvedOn == nil {
			return true
		}
		return a.ResolvedOn.Before(*b.ResolvedOn)
	}},
	{Name: "state", DefaultSorter: false, Compare: func(a, b Task) bool {
		return strings.ToLower(a.State) < strings.ToLower(b.State)
	}},
	{Name: "resolved_by", DefaultSorter: false, Compare: func(a, b Task) bool {
		if a.ResolvedBy == nil {
			return false
		}
		if b.ResolvedBy == nil {
			return true
		}
		return strings.ToLower(a.ResolvedBy.Name) < strings.ToLower(b.ResolvedBy.Name)
	}},
	{Name: "pending", DefaultSorter: false, Compare: func(a, b Task) bool {
		return a.IsPending == b.IsPending
	}},
}

// GetHeaders returns the headers of the columns to display
//
// implements common.Tableables
func (task Task) GetHeaders(cmd *cobra.Command) []string {
	if cmd != nil && cmd.Flag("columns") != nil && cmd.Flag("columns").Changed {
		if columns, err := cmd.Flags().GetStringSlice("columns"); err == nil {
			return core.Map(columns, func(column string) string { return strings.ReplaceAll(column, "_", " ") })
		}
	}
	return []string{"id", "state", "creator", "created_on", "updated_on", "resolved_on", "resolved_by", "content"}
}

// GetRow returns the row to display for this task
//
// implements common.Tableables
//
// headers is normalized (lowercased, spaces treated the same as underscores) before matching,
// the same way every other GetRow in this codebase does: GetHeaders maps a user-supplied
// --columns value like "resolved_by" through strings.ReplaceAll(column, "_", " ") into "resolved
// by", so matching against the raw, case-sensitive, underscore spelling here would otherwise
// leave every multi-word column blank.
func (task Task) GetRow(headers []string) []string {
	simple := map[string]string{
		"id":         strconv.Itoa(task.ID),
		"content":    task.Content.Raw,
		"creator":    task.Creator.String(),
		"created_on": task.CreatedOn.Format(time.RFC3339),
		"updated_on": task.UpdatedOn.Format(time.RFC3339),
		"state":      task.State,
		"pending":    strconv.FormatBool(task.IsPending),
	}

	row := make([]string, 0, len(headers))
	for _, header := range headers {
		switch key := strings.ReplaceAll(strings.ToLower(header), " ", "_"); key {
		case "resolved_on":
			if task.ResolvedOn != nil {
				row = append(row, task.ResolvedOn.Format(time.RFC3339))
			} else {
				row = append(row, "")
			}
		case "resolved_by":
			if task.ResolvedBy != nil {
				row = append(row, task.ResolvedBy.String())
			} else {
				row = append(row, "")
			}
		default:
			if value, found := simple[key]; found {
				row = append(row, value)
			} else {
				row = append(row, "")
			}
		}
	}
	return row
}

// MarshalJSON implements the json.Marshaller interface
func (task Task) MarshalJSON() ([]byte, error) {
	type surrogate Task

	data, err := json.Marshal(struct {
		surrogate
		CreatedOn  core.Time  `json:"created_on"`
		UpdatedOn  core.Time  `json:"updated_on"`
		ResolvedOn *core.Time `json:"resolved_on,omitempty"`
	}{
		surrogate:  surrogate(task),
		CreatedOn:  core.Time(task.CreatedOn),
		UpdatedOn:  core.Time(task.UpdatedOn),
		ResolvedOn: (*core.Time)(task.ResolvedOn),
	})
	if err != nil {
		return nil, fmt.Errorf("cannot marshal json: %w", err)
	}
	return data, nil
}

// GetPullRequestTaskIDs gets the IDs of the tasks for a pullrequest
func GetPullRequestTaskIDs(ctx context.Context, cmd *cobra.Command, pullRequestID string) (ids []string, err error) {
	repository, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return nil, fmt.Errorf("cannot get repository: %w", err)
	}

	tasks, err := profile.GetAll[Task](ctx, cmd, repository.GetPath(fmt.Sprintf("pullrequests/%s/tasks", pullRequestID)))
	if err != nil {
		lgr.Printf("[ERROR] failed to get pullrequests: %v", err)
		return nil, err
	}
	return core.Map(tasks, func(task Task) string {
		return strconv.Itoa(task.ID)
	}), nil
}
