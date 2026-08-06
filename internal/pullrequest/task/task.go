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
	prcommon "github.com/avitsrimer/bitbucket-cli/internal/pullrequest/common"
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
	Run:   common.SubcommandRequired("Task"),
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
		return !a.IsPending && b.IsPending
	}},
}

// GetHeaders returns the headers of the columns to display
//
// implements common.Tableables
func (task Task) GetHeaders(cmd *cobra.Command) []string {
	return common.HeadersFromFlag(cmd, "id", "state", "creator", "created_on", "updated_on", "resolved_on", "resolved_by", "content")
}

// GetRow returns the row to display for this task
//
// implements common.Tableables
//
// header is matched via common.NormalizeColumnKey, so a user-supplied --columns value like
// "resolved by" or "resolved-on" resolves the same underscore-separated key as the GetHeaders
// default ("resolved_by", "resolved_on").
func (task Task) GetRow(headers []string) []string {
	simple := map[string]string{
		"id":         strconv.Itoa(task.ID),
		"content":    task.Content.Raw,
		"creator":    task.Creator.String(),
		"created_on": common.TimeCell(task.CreatedOn),
		"updated_on": common.TimeCell(task.UpdatedOn),
		"state":      task.State,
		"pending":    strconv.FormatBool(task.IsPending),
	}

	row := make([]string, 0, len(headers))
	for _, header := range headers {
		switch key := common.NormalizeColumnKey(header); key {
		case "resolved_on":
			if task.ResolvedOn != nil {
				row = append(row, common.TimeCell(*task.ResolvedOn))
			} else {
				row = append(row, common.EmptyCell)
			}
		case "resolved_by":
			if task.ResolvedBy != nil {
				row = append(row, task.ResolvedBy.String())
			} else {
				row = append(row, common.EmptyCell)
			}
		default:
			if value, found := simple[key]; found {
				row = append(row, value)
			} else {
				row = append(row, common.EmptyCell)
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

// pullRequestAndTaskIDValidArgs is the ValidArgsFunction shared by every task subcommand that
// takes exactly <pullrequest-id> <task-id> as its two positionals (get, update): arg 0
// completes open pullrequest ids, arg 1 completes the task ids of the pullrequest named in
// arg 0.
func pullRequestAndTaskIDValidArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	switch len(args) {
	case 0:
		ids, err := prcommon.GetPullRequestIDs(cmd.Context(), cmd, args, toComplete)
		if err != nil {
			return []string{}, cobra.ShellCompDirectiveNoFileComp
		}
		return common.FilterValidArgs(ids, args, toComplete), cobra.ShellCompDirectiveNoFileComp
	case 1:
		taskIDs, err := GetPullRequestTaskIDs(cmd.Context(), cmd, args[0])
		if err != nil {
			return []string{}, cobra.ShellCompDirectiveNoFileComp
		}
		return common.FilterValidArgs(taskIDs, args[1:], toComplete), cobra.ShellCompDirectiveNoFileComp
	default:
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
}
