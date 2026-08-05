package task

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gildas/bitbucket-cli/cmd/common"
	"github.com/gildas/bitbucket-cli/cmd/profile"
	"github.com/gildas/bitbucket-cli/cmd/pullrequest/comment"
	"github.com/gildas/bitbucket-cli/cmd/repository"
	"github.com/gildas/bitbucket-cli/cmd/user"
	"github.com/gildas/go-core"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

// Task represents a pull request task
type Task struct {
	ID         int                 `json:"id"                    mapstructure:"id"`
	Content    common.RenderedText `json:"content"               mapstructure:"content"`
	Creator    user.User           `json:"creator"               mapstructure:"creator"`
	IsPending  bool                `json:"pending"               mapstructure:"pending"`
	State      string              `json:"state"                 mapstructure:"state"`
	Comment    *comment.Comment    `json:"comment,omitempty"     mapstructure:"comment"`
	ResolvedBy *user.User          `json:"resolved_by,omitempty" mapstructure:"resolved_by"`
	CreatedOn  time.Time           `json:"created_on"            mapstructure:"created_on"`
	UpdatedOn  time.Time           `json:"updated_on"            mapstructure:"updated_on"`
	ResolvedOn *time.Time          `json:"resolved_on,omitempty" mapstructure:"resolved_on"`
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
func (task Task) GetRow(headers []string) []string {
	var row []string

	for _, header := range headers {
		switch header {
		case "id":
			row = append(row, strconv.Itoa(task.ID))
		case "content":
			row = append(row, task.Content.Raw)
		case "creator":
			row = append(row, task.Creator.String())
		case "created_on":
			row = append(row, task.CreatedOn.Format(time.RFC3339))
		case "updated_on":
			row = append(row, task.UpdatedOn.Format(time.RFC3339))
		case "resolved_on":
			if task.ResolvedOn != nil {
				row = append(row, task.ResolvedOn.Format(time.RFC3339))
			} else {
				row = append(row, "")
			}
		case "state":
			row = append(row, task.State)
		case "resolved_by":
			if task.ResolvedBy != nil {
				row = append(row, task.ResolvedBy.String())
			} else {
				row = append(row, "")
			}
		case "pending":
			row = append(row, strconv.FormatBool(task.IsPending))
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
