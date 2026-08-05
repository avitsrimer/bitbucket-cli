package comment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gildas/bitbucket-cli/cmd/common"
	"github.com/gildas/bitbucket-cli/cmd/profile"
	prcommon "github.com/gildas/bitbucket-cli/cmd/pullrequest/common"
	"github.com/gildas/bitbucket-cli/cmd/repository"
	"github.com/gildas/bitbucket-cli/cmd/user"
	"github.com/gildas/go-core"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

// commentEditOptions holds the flags shared by the create and update commands
type commentEditOptions struct {
	PullRequestID *common.EnumFlag
	Comment       string
	File          string
	From          int
	To            int
	ParentID      int64
	Pending       bool
}

// registerCommentEditFlags registers the flags shared by the create and update commands
func registerCommentEditFlags(cmd *cobra.Command, options *commentEditOptions, commentHelp, pullrequestHelp string) {
	options.PullRequestID = common.NewEnumFlagWithFunc(cmd, "", prcommon.GetPullRequestIDs)
	cmd.Flags().Var(options.PullRequestID, "pullrequest", pullrequestHelp)
	cmd.Flags().StringVar(&options.Comment, "comment", "", commentHelp)
	cmd.Flags().StringVar(&options.File, "file", "", "File to comment on")
	cmd.Flags().IntVar(&options.From, "line", 0, "From line to comment on. Cannot be used with --to")
	cmd.Flags().IntVar(&options.From, "from", 0, "From line to comment on. Cannot be used with --line")
	cmd.Flags().IntVar(&options.To, "to", 0, "To line to comment on. Cannot be used with --line")
	cmd.Flags().Int64Var(&options.ParentID, "parent", 0, "Parent comment ID to reply to")
	cmd.Flags().BoolVar(&options.Pending, "pending", false, "Mark the comment as pending")
	cmd.MarkFlagsMutuallyExclusive("line", "from")
	cmd.MarkFlagsMutuallyExclusive("line", "to")
	_ = cmd.MarkFlagRequired("pullrequest")
	_ = cmd.MarkFlagRequired("comment")
	_ = cmd.RegisterFlagCompletionFunc(options.PullRequestID.CompletionFunc("pullrequest"))
}

type Comment struct {
	Type        string                `json:"type"                 mapstructure:"type"`
	ID          int                   `json:"id"                   mapstructure:"id"`
	Content     common.RenderedText   `json:"content"              mapstructure:"content"`
	User        user.User             `json:"user"                 mapstructure:"user"`
	Anchor      *common.FileAnchor    `json:"inline,omitempty"     mapstructure:"inline"`
	Parent      *Comment              `json:"parent,omitempty"     mapstructure:"parent"`
	CreatedOn   time.Time             `json:"created_on"           mapstructure:"created_on"`
	UpdatedOn   time.Time             `json:"updated_on"           mapstructure:"updated_on"`
	IsDeleted   bool                  `json:"deleted"              mapstructure:"deleted"`
	IsPending   bool                  `json:"pending"              mapstructure:"pending"`
	Resolution  *Resolution           `json:"resolution,omitempty" mapstructure:"resolution"`
	PullRequest *PullRequestReference `json:"pullrequest"          mapstructure:"pullrequest"`
	Links       common.Links          `json:"links"                mapstructure:"links"`
}

type Resolution struct {
	Type      string    `json:"type"       mapstructure:"type"`
	User      user.User `json:"user"       mapstructure:"user"`
	CreatedOn time.Time `json:"created_on" mapstructure:"created_on"`
}

type PullRequestReference struct {
	Type  string       `json:"type"  mapstructure:"type"`
	ID    int          `json:"id"    mapstructure:"id"`
	Title string       `json:"title" mapstructure:"title"`
	Links common.Links `json:"links" mapstructure:"links"`
}

type ParentReference struct {
	ID int64 `json:"id" mapstructure:"id"`
}

// Command represents this folder's command
var Command = &cobra.Command{
	Use:   "comment",
	Short: "Manage comments",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Comment requires a subcommand:")
		for _, command := range cmd.Commands() {
			fmt.Println(command.Name())
		}
	},
}

var columns = common.Columns[Comment]{
	{Name: "id", DefaultSorter: true, Compare: func(a, b Comment) bool {
		return a.ID < b.ID
	}},
	{Name: "content", DefaultSorter: false, Compare: func(a, b Comment) bool {
		return strings.ToLower(a.Content.Raw) < strings.ToLower(b.Content.Raw)
	}},
	{Name: "user", DefaultSorter: false, Compare: func(a, b Comment) bool {
		return strings.ToLower(a.User.Name) < strings.ToLower(b.User.Name)
	}},
	{Name: "file", DefaultSorter: false, Compare: func(a, b Comment) bool {
		if a.Anchor != nil && b.Anchor != nil {
			return strings.ToLower(a.Anchor.String()) < strings.ToLower(b.Anchor.String())
		}
		return a.Anchor != nil
	}},
	{Name: "created_on", DefaultSorter: false, Compare: func(a, b Comment) bool {
		return a.CreatedOn.Before(b.CreatedOn)
	}},
	{Name: "updated_on", DefaultSorter: false, Compare: func(a, b Comment) bool {
		return a.UpdatedOn.Before(b.UpdatedOn)
	}},
	{Name: "deleted", DefaultSorter: false, Compare: func(a, b Comment) bool {
		return a.IsDeleted == b.IsDeleted
	}},
	{Name: "pending", DefaultSorter: false, Compare: func(a, b Comment) bool {
		return a.IsPending == b.IsPending
	}},
	{Name: "resolution", DefaultSorter: false, Compare: func(a, b Comment) bool {
		return (a.Resolution != nil) && (b.Resolution == nil)
	}},
	{Name: "pullrequest", DefaultSorter: false, Compare: func(a, b Comment) bool {
		if a.PullRequest != nil && b.PullRequest != nil {
			return a.PullRequest.ID < b.PullRequest.ID
		}
		return a.PullRequest != nil
	}},
}

// GetHeaders gets the header for a table
//
// implements common.Tableable
func (comment Comment) GetHeaders(cmd *cobra.Command) []string {
	if cmd != nil && cmd.Flag("columns") != nil && cmd.Flag("columns").Changed {
		if columns, err := cmd.Flags().GetStringSlice("columns"); err == nil {
			return core.Map(columns, func(column string) string { return strings.ReplaceAll(column, "_", " ") })
		}
	}
	return []string{"ID", "Created On", "Updated On", "File", "User", "Content"}
}

// GetRow gets the row for a table
//
// implements common.Tableable
func (comment Comment) GetRow(headers []string) []string {
	var row []string

	for _, header := range headers {
		switch strings.ToLower(header) {
		case "id":
			row = append(row, strconv.Itoa(comment.ID))
		case "created on", "created_on", "created-on", "created":
			row = append(row, comment.CreatedOn.Format("2006-01-02 15:04:05"))
		case "updated on", "updated_on", "updated-on", "updated":
			if !comment.UpdatedOn.IsZero() {
				row = append(row, comment.UpdatedOn.Format("2006-01-02 15:04:05"))
			} else {
				row = append(row, "N/A")
			}
		case "file":
			if comment.Anchor != nil {
				row = append(row, comment.Anchor.String())
			} else {
				row = append(row, "N/A")
			}
		case "user":
			row = append(row, comment.User.Name)
		case "content":
			row = append(row, comment.Content.Raw)
		case "deleted":
			row = append(row, strconv.FormatBool(comment.IsDeleted))
		case "pending":
			row = append(row, strconv.FormatBool(comment.IsPending))
		case "resolution":
			if comment.Resolution != nil {
				switch {
				case comment.Resolution.User.Name != "" && !comment.Resolution.CreatedOn.IsZero():
					row = append(row, fmt.Sprintf("resolved by %s on %s", comment.Resolution.User.Name, comment.Resolution.CreatedOn.Format("2006-01-02 15:04:05")))
				case comment.Resolution.User.Name != "":
					row = append(row, "resolved by "+comment.Resolution.User.Name)
				case !comment.Resolution.CreatedOn.IsZero():
					row = append(row, "resolved on "+comment.Resolution.CreatedOn.Format("2006-01-02 15:04:05"))
				default:
					row = append(row, "resolved")
				}
			} else {
				row = append(row, "unresolved")
			}
		case "pullrequest":
			if comment.PullRequest != nil {
				row = append(row, fmt.Sprintf("%s (%d)", comment.PullRequest.Title, comment.PullRequest.ID))
			} else {
				row = append(row, " ")
			}
		}
	}
	return row
}

// Validate validates a Comment
func (comment *Comment) Validate() error {
	return nil
}

// String gets a string representation of this pullrequest
//
// implements fmt.Stringer
func (comment Comment) String() string {
	return comment.Content.Raw
}

// MarshalJSON implements the json.Marshaler interface.
func (resolution Resolution) MarshalJSON() (data []byte, err error) {
	type surrogate Resolution

	var createdOn *string
	if !resolution.CreatedOn.IsZero() {
		formatted := resolution.CreatedOn.Format(time.RFC3339)
		createdOn = &formatted
	}

	data, err = json.Marshal(struct {
		surrogate
		CreatedOn *string `json:"created_on,omitempty"`
	}{
		surrogate: surrogate(resolution),
		CreatedOn: createdOn,
	})
	if err != nil {
		return nil, fmt.Errorf("cannot marshal resolution to json: %w", err)
	}
	return data, nil
}

// MarshalJSON implements the json.Marshaler interface.
func (comment Comment) MarshalJSON() (data []byte, err error) {
	type surrogate Comment

	data, err = json.Marshal(struct {
		surrogate
		CreatedOn string `json:"created_on"`
		UpdatedOn string `json:"updated_on"`
	}{
		surrogate: surrogate(comment),
		CreatedOn: comment.CreatedOn.Format(time.RFC3339),
		UpdatedOn: comment.UpdatedOn.Format(time.RFC3339),
	})
	if err != nil {
		return nil, fmt.Errorf("cannot marshal comment to json: %w", err)
	}
	return data, nil
}

// GetPullRequestCommentIDs gets the IDs of the comments for a pullrequest
func GetPullRequestCommentIDs(context context.Context, cmd *cobra.Command, args []string, toComplete string) (ids []string, err error) {
	repository, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return nil, fmt.Errorf("cannot get repository: %w", err)
	}

	if cmd.Flag("pullrequest") == nil {
		return nil, errors.New("flag --pullrequest is required")
	}
	pullRequestID := cmd.Flag("pullrequest").Value.String()

	comments, err := profile.GetAll[Comment](context, cmd, repository.GetPath(fmt.Sprintf("pullrequests/%s/comments", pullRequestID)))
	if err != nil {
		lgr.Printf("[ERROR] failed to get pullrequests: %v", err)
		return nil, err
	}
	ids = core.Map(comments, func(comment Comment) string { return strconv.Itoa(comment.ID) })
	return common.FilterValidArgs(ids, args, toComplete), nil
}
