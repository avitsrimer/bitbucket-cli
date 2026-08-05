package pullrequest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/avitsrimer/bitbucket-cli/internal/commit"
	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/pullrequest/activity"
	"github.com/avitsrimer/bitbucket-cli/internal/pullrequest/comment"
	prcommon "github.com/avitsrimer/bitbucket-cli/internal/pullrequest/common"
	"github.com/avitsrimer/bitbucket-cli/internal/pullrequest/task"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/avitsrimer/bitbucket-cli/internal/user"
	"github.com/avitsrimer/bitbucket-cli/internal/workspace"
	"github.com/gildas/go-core"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

type PullRequest struct {
	Type              string                  `json:"type"                   mapstructure:"type"`
	ID                uint64                  `json:"id"                     mapstructure:"id"`
	Title             string                  `json:"title"                  mapstructure:"title"`
	Description       string                  `json:"description"            mapstructure:"description"`
	Summary           common.RenderedText     `json:"summary"                mapstructure:"summary"`
	State             string                  `json:"state"                  mapstructure:"state"`
	MergeCommit       *commit.CommitReference `json:"merge_commit,omitempty" mapstructure:"merge_commit"`
	CloseSourceBranch bool                    `json:"close_source_branch"    mapstructure:"close_source_branch"`
	ClosedBy          user.User               `json:"closed_by"              mapstructure:"closed_by"`
	Author            user.User               `json:"author"                 mapstructure:"author"`
	Reviewers         []user.User             `json:"reviewers,omitempty"    mapstructure:"reviewers"`
	Reason            string                  `json:"reason"                 mapstructure:"reason"`
	Destination       Endpoint                `json:"destination"            mapstructure:"destination"`
	Source            Endpoint                `json:"source"                 mapstructure:"source"`
	Links             common.Links            `json:"links"                  mapstructure:"links"`
	CommentCount      uint64                  `json:"comment_count"          mapstructure:"comment_count"`
	TaskCount         uint64                  `json:"task_count"             mapstructure:"task_count"`
	CreatedOn         time.Time               `json:"created_on"             mapstructure:"created_on"`
	UpdatedOn         time.Time               `json:"updated_on"             mapstructure:"updated_on"`
}

// Command represents this folder's command
var Command = &cobra.Command{
	Use:     "pullrequest",
	Aliases: []string{"pr", "pull-request"},
	Short:   "Manage pull requests",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Pullrequest requires a subcommand:")
		for _, command := range cmd.Commands() {
			fmt.Println(command.Name())
		}
	},
}

var columns = common.Columns[PullRequest]{
	{Name: "id", DefaultSorter: true, Compare: func(a, b PullRequest) bool {
		return a.ID < b.ID
	}},
	{Name: "title", DefaultSorter: false, Compare: func(a, b PullRequest) bool {
		return strings.ToLower(a.Title) < strings.ToLower(b.Title)
	}},
	{Name: "description", DefaultSorter: false, Compare: func(a, b PullRequest) bool {
		return strings.ToLower(a.Description) < strings.ToLower(b.Description)
	}},
	{Name: "source", DefaultSorter: false, Compare: func(a, b PullRequest) bool {
		return strings.ToLower(a.Source.Branch.Name) < strings.ToLower(b.Source.Branch.Name)
	}},
	{Name: "destination", DefaultSorter: false, Compare: func(a, b PullRequest) bool {
		return strings.ToLower(a.Destination.Branch.Name) < strings.ToLower(b.Destination.Branch.Name)
	}},
	{Name: "state", DefaultSorter: false, Compare: func(a, b PullRequest) bool {
		return strings.ToLower(a.State) < strings.ToLower(b.State)
	}},
	{Name: "author", DefaultSorter: false, Compare: func(a, b PullRequest) bool {
		return strings.ToLower(a.Author.Name) < strings.ToLower(b.Author.Name)
	}},
	{Name: "closed_by", DefaultSorter: false, Compare: func(a, b PullRequest) bool {
		return strings.ToLower(a.ClosedBy.Name) < strings.ToLower(b.ClosedBy.Name)
	}},
	{Name: "commit", DefaultSorter: false, Compare: func(a, b PullRequest) bool {
		if a.MergeCommit != nil && b.MergeCommit != nil {
			return strings.ToLower(a.MergeCommit.Hash) < strings.ToLower(b.MergeCommit.Hash)
		}
		if a.MergeCommit != nil {
			return true
		}
		if b.MergeCommit != nil {
			return false
		}
		return false
	}},
	{Name: "reason", DefaultSorter: false, Compare: func(a, b PullRequest) bool {
		return strings.ToLower(a.Reason) < strings.ToLower(b.Reason)
	}},
	{Name: "comments", DefaultSorter: false, Compare: func(a, b PullRequest) bool {
		return a.CommentCount < b.CommentCount
	}},
	{Name: "tasks", DefaultSorter: false, Compare: func(a, b PullRequest) bool {
		return a.TaskCount < b.TaskCount
	}},
	{Name: "created_on", DefaultSorter: false, Compare: func(a, b PullRequest) bool {
		return a.CreatedOn.Before(b.CreatedOn)
	}},
	{Name: "updated_on", DefaultSorter: false, Compare: func(a, b PullRequest) bool {
		if a.UpdatedOn.IsZero() && b.UpdatedOn.IsZero() {
			return false
		}
		if a.UpdatedOn.IsZero() {
			return true
		}
		if b.UpdatedOn.IsZero() {
			return false
		}
		return a.UpdatedOn.Before(b.UpdatedOn)
	}},
}

func init() {
	Command.AddCommand(comment.Command)
	Command.AddCommand(activity.Command)
	Command.AddCommand(task.Command)
}

// GetHeaders gets the header for a table
//
// implements common.Tableable
func (pullrequest PullRequest) GetHeaders(cmd *cobra.Command) []string {
	if cmd != nil && cmd.Flag("columns") != nil && cmd.Flag("columns").Changed {
		if columns, err := cmd.Flags().GetStringSlice("columns"); err == nil {
			return core.Map(columns, func(column string) string { return strings.ReplaceAll(column, "_", " ") })
		}
	}
	return []string{"ID", "Title", "Description", "source", "destination", "state"}
}

// GetRow gets the row for a table
//
// implements common.Tableable
func (pullrequest PullRequest) GetRow(headers []string) []string {
	var row []string

	for _, header := range headers {
		switch strings.ToLower(header) {
		case "id":
			row = append(row, strconv.FormatUint(pullrequest.ID, 10))
		case "title":
			row = append(row, pullrequest.Title)
		case "description":
			row = append(row, pullrequest.Description)
		case "source":
			row = append(row, pullrequest.Source.Branch.Name)
		case "destination":
			row = append(row, pullrequest.Destination.Branch.Name)
		case "state":
			row = append(row, pullrequest.State)
		case "author":
			row = append(row, pullrequest.Author.Name)
		case "closed by":
			row = append(row, pullrequest.ClosedBy.Name)
		case "commit":
			if pullrequest.MergeCommit != nil {
				row = append(row, pullrequest.MergeCommit.Hash[:7])
			} else {
				row = append(row, " ")
			}
		case "reason":
			row = append(row, pullrequest.Reason)
		case "comments":
			row = append(row, strconv.FormatUint(pullrequest.CommentCount, 10))
		case "tasks":
			row = append(row, strconv.FormatUint(pullrequest.TaskCount, 10))
		case "created on", "created_on", "created-on":
			row = append(row, pullrequest.CreatedOn.Format("2006-01-02 15:04:05"))
		case "updated on", "updated_on", "updated-on":
			if !pullrequest.UpdatedOn.IsZero() {
				row = append(row, pullrequest.UpdatedOn.Format("2006-01-02 15:04:05"))
			} else {
				row = append(row, " ")
			}
		}
	}
	return row
}

// Validate validates a PullRequest
func (pullrequest *PullRequest) Validate() error {
	return nil
}

// String gets a string representation of this pullrequest
//
// implements fmt.Stringer
func (pullrequest PullRequest) String() string {
	return pullrequest.Title
}

// GetPullRequestIDFromArgs gets the pullrequest ID from the command arguments or, if not provided, from the only open pullrequestA
func GetPullRequestIDFromArgs(ctx context.Context, cmd *cobra.Command, repository *repository.Repository, args []string) (pullRequestID string, err error) {
	if len(args) == 0 {
		pullRequestIDs, err := prcommon.GetPullRequestIDsFromRepositoryWithState(cmd.Context(), cmd, repository, "OPEN")
		if err != nil {
			return "", fmt.Errorf("cannot list pull requests: %w", err)
		}
		if len(pullRequestIDs) == 0 {
			return "", fmt.Errorf("no open pullrequest found for repository %s", repository.FullName)
		}
		if len(pullRequestIDs) > 1 {
			return "", fmt.Errorf("too many pullrequests to merge: %s", strings.Join(pullRequestIDs, ", "))
		}
		return pullRequestIDs[0], nil
	}
	if _, err := strconv.Atoi(args[0]); err != nil {
		return "", fmt.Errorf("argument pullrequest-id is invalid (value: %s)", args[0])
	}
	return args[0], nil
}

// GetReviewerNicknames gets the reviewer nicknames for the current Workspace
func GetReviewerNicknames(ctx context.Context, cmd *cobra.Command, args []string, toComplete string) (nicknames []string, err error) {
	if cmd == nil {
		fmt.Fprintln(os.Stderr, "cmd is nil")
		return []string{}, errors.New("argument cmd is missing")
	}

	lgr.Printf("[DEBUG] getting reviewer nicknames for profile %s", profile.Current)
	pullrequestWorkspace, err := workspace.GetWorkspace(cmd.Context(), cmd)
	if err != nil {
		lgr.Printf("[ERROR] failed to get repository: %v", err)
		return []string{}, fmt.Errorf("cannot get workspace: %w", err)
	}
	lgr.Printf("[DEBUG] getting members of workspace %s", pullrequestWorkspace)
	members, _ := pullrequestWorkspace.GetMembers(ctx, cmd)
	nicknames = core.Map(members, func(member workspace.Member) string { return member.User.Nickname })
	core.Sort(nicknames, func(a, b string) bool { return strings.ToLower(a) < strings.ToLower(b) })
	return common.FilterValidArgs(nicknames, args, toComplete), nil
}

// MarshalJSON implements the json.Marshaler interface.
func (pullrequest PullRequest) MarshalJSON() (data []byte, err error) {
	type surrogate PullRequest

	data, err = json.Marshal(struct {
		surrogate
		CreatedOn string `json:"created_on"`
		UpdatedOn string `json:"updated_on"`
	}{
		surrogate: surrogate(pullrequest),
		CreatedOn: pullrequest.CreatedOn.Format("2006-01-02T15:04:05.999999999-07:00"),
		UpdatedOn: pullrequest.UpdatedOn.Format("2006-01-02T15:04:05.999999999-07:00"),
	})
	if err != nil {
		return nil, fmt.Errorf("cannot marshal pullrequest to json: %w", err)
	}
	return data, nil
}
