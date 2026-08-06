package comment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	prcommon "github.com/avitsrimer/bitbucket-cli/internal/pullrequest/common"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/avitsrimer/bitbucket-cli/internal/user"
	"github.com/gildas/go-core"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

// CommentPayload is the request body for creating or updating a pull request comment.
type CommentPayload struct {
	Content CommentContent     `json:"content"`
	Anchor  *common.FileAnchor `json:"inline,omitempty"`
	Parent  *ParentReference   `json:"parent,omitempty"`
	Pending *bool              `json:"pending,omitempty"`
}

// CommentContent is the "content" field of CommentPayload.
type CommentContent struct {
	Raw string `json:"raw"`
}

// commentEditOptions holds the flags shared by the create and update commands
type commentEditOptions struct {
	Comment     string
	CommentFile string
	File        string
	From        int
	To          int
	ParentID    int64
	Pending     bool
}

// payload builds the request body for a create/update comment request from o, resolving the
// comment body from --comment or --comment-file/stdin, the --file/--line/--from/--to file anchor,
// and the --parent/--pending flags. Returns an error when the resolved comment body is empty or
// when --line/--from/--to was given without --file.
func (o commentEditOptions) payload(cmd *cobra.Command) (CommentPayload, error) {
	commentBody, err := o.resolveComment(cmd)
	if err != nil {
		return CommentPayload{}, err
	}
	if strings.TrimSpace(commentBody) == "" {
		return CommentPayload{}, errors.New("comment body is empty")
	}

	payload := CommentPayload{
		Content: CommentContent{Raw: commentBody},
	}

	if o.ParentID > 0 {
		payload.Parent = &ParentReference{ID: o.ParentID}
	}

	if o.File != "" {
		payload.Anchor = &common.FileAnchor{
			Path: o.File,
		}
		if o.From > 0 {
			payload.Anchor.From = uint64(o.From)
		}
		if o.To > 0 {
			payload.Anchor.To = uint64(o.To)
		}
	} else if o.From > 0 || o.To > 0 {
		return CommentPayload{}, errors.New("cannot specify from/to without a file")
	}

	if cmd.Flag("pending").Changed {
		payload.Pending = &o.Pending
	}

	return payload, nil
}

// resolveComment returns the comment body to send: o.CommentFile's content (or cmd's stdin, via
// "-") when o.CommentFile is set, otherwise o.Comment verbatim. registerCommentEditFlags'
// MarkFlagsMutuallyExclusive guarantees at most one of --comment/--comment-file was given, and its
// MarkFlagsOneRequired guarantees at least one was.
func (o commentEditOptions) resolveComment(cmd *cobra.Command) (string, error) {
	if o.CommentFile == "" {
		return o.Comment, nil
	}
	body, err := common.ReadBodyFromFileOrStdin(cmd, o.CommentFile)
	if err != nil {
		return "", fmt.Errorf("cannot read comment body: %w", err)
	}
	return body, nil
}

// registerCommentEditFlags registers the flags shared by the create and update commands
func registerCommentEditFlags(cmd *cobra.Command, options *commentEditOptions, commentHelp string) {
	cmd.Flags().StringVar(&options.Comment, "comment", "", commentHelp)
	cmd.Flags().StringVar(&options.CommentFile, "comment-file", "", "Read the comment body from <path>, or - to read it from stdin. Mutually exclusive with --comment.")
	cmd.Flags().StringVar(&options.File, "file", "", "File to comment on")
	cmd.Flags().IntVar(&options.From, "line", 0, "Line to comment on, same as --from. Cannot be used with --to")
	cmd.Flags().IntVar(&options.From, "from", 0, "From line to comment on. Cannot be used with --line")
	cmd.Flags().IntVar(&options.To, "to", 0, "To line to comment on. Cannot be used with --line")
	cmd.Flags().Int64Var(&options.ParentID, "parent", 0, "Parent comment ID to reply to")
	cmd.Flags().BoolVar(&options.Pending, "pending", false, "Mark the comment as pending")
	cmd.MarkFlagsMutuallyExclusive("line", "from")
	cmd.MarkFlagsMutuallyExclusive("line", "to")
	cmd.MarkFlagsMutuallyExclusive("comment", "comment-file")
	cmd.MarkFlagsOneRequired("comment", "comment-file")
	_ = cmd.MarkFlagFilename("comment-file")
}

// diffstatEntry is the shape of one entry of a pull request's diffstat (GET
// pullrequests/{id}/diffstat) that validateFileAnchor needs: the old/new file each entry touched.
// old is absent for a pure addition, new for a pure deletion.
type diffstatEntry struct {
	Old *diffstatFile `json:"old"`
	New *diffstatFile `json:"new"`
}

// diffstatFile is the "old"/"new" side of a diffstatEntry.
type diffstatFile struct {
	Path string `json:"path"`
}

// validateFileAnchor confirms anchor.Path names a file actually changed by the pull request
// identified by pullRequestID, via a GET of its diffstat, run on every --file invocation
// (--dry-run or not) as part of comment create/update's preflight -- deliberately stricter than
// what the write endpoint itself enforces: --file is a diff anchor path inside the pull request,
// not a local file, so this checks the pull request's actual diffstat rather than os.Stat-ing
// anything on disk.
func validateFileAnchor(ctx context.Context, cmd *cobra.Command, repo *repository.Repository, pullRequestID string, anchor *common.FileAnchor) error {
	if anchor == nil {
		return nil
	}
	entries, err := profile.GetAllUnbounded[diffstatEntry](ctx, cmd, repo.GetPath("pullrequests", pullRequestID, "diffstat"))
	if err != nil {
		return fmt.Errorf("cannot get diffstat of pullrequest %s: %w", pullRequestID, err)
	}
	for _, entry := range entries {
		if entry.Old != nil && entry.Old.Path == anchor.Path {
			return nil
		}
		if entry.New != nil && entry.New.Path == anchor.Path {
			return nil
		}
	}
	return fmt.Errorf("file %q is not part of the diff of pullrequest %s", anchor.Path, pullRequestID)
}

// existsComment validates via a GET that commentID names an existing comment on the pull request
// identified by pullRequestID, returning the same error a write against that id would produce.
// This validates the parent pull request's existence too, so update/reopen/resolve/delete each
// need only this one check.
func existsComment(ctx context.Context, cmd *cobra.Command, repo *repository.Repository, pullRequestID, commentID string) error {
	currentProfile, err := profile.GetProfileFromCommand(ctx, cmd)
	if err != nil {
		return fmt.Errorf("cannot get profile: %w", err)
	}
	if err := currentProfile.Get(ctx, repo.GetPath("pullrequests", pullRequestID, "comments", commentID), nil); err != nil {
		return fmt.Errorf("failed to get comment %s of pullrequest %s: %w", commentID, pullRequestID, err)
	}
	return nil
}

type Comment struct {
	Type        string                `json:"type"`
	ID          int                   `json:"id"`
	Content     common.RenderedText   `json:"content"`
	User        user.User             `json:"user"`
	Anchor      *common.FileAnchor    `json:"inline,omitempty"`
	Parent      *Comment              `json:"parent,omitempty"`
	CreatedOn   time.Time             `json:"created_on"`
	UpdatedOn   time.Time             `json:"updated_on"`
	IsDeleted   bool                  `json:"deleted"`
	IsPending   bool                  `json:"pending"`
	Resolution  *Resolution           `json:"resolution,omitempty"`
	PullRequest *PullRequestReference `json:"pullrequest"`
	Links       common.Links          `json:"links"`
}

type Resolution struct {
	Type      string    `json:"type"`
	User      user.User `json:"user"`
	CreatedOn time.Time `json:"created_on"`
}

type PullRequestReference struct {
	Type  string       `json:"type"`
	ID    int          `json:"id"`
	Title string       `json:"title"`
	Links common.Links `json:"links"`
}

type ParentReference struct {
	ID int64 `json:"id"`
}

// Command represents this folder's command
var Command = &cobra.Command{
	Use:   "comment",
	Short: "Manage comments",
	Run:   common.SubcommandRequired("Comment"),
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
		return !a.IsDeleted && b.IsDeleted
	}},
	{Name: "pending", DefaultSorter: false, Compare: func(a, b Comment) bool {
		return !a.IsPending && b.IsPending
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
	return common.HeadersFromFlag(cmd, "ID", "Created On", "Updated On", "File", "User", "Content")
}

// GetRow gets the row for a table
//
// implements common.Tableable
func (comment Comment) GetRow(headers []string) []string {
	var row []string

	for _, header := range headers {
		switch common.NormalizeColumnKey(header) {
		case "id":
			row = append(row, strconv.Itoa(comment.ID))
		case "created_on", "created":
			row = append(row, common.TimeCell(comment.CreatedOn))
		case "updated_on", "updated":
			row = append(row, common.TimeCell(comment.UpdatedOn))
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
					row = append(row, fmt.Sprintf("resolved by %s on %s", comment.Resolution.User.Name, comment.Resolution.CreatedOn.Format(common.TableTimeFormat)))
				case comment.Resolution.User.Name != "":
					row = append(row, "resolved by "+comment.Resolution.User.Name)
				case !comment.Resolution.CreatedOn.IsZero():
					row = append(row, "resolved on "+comment.Resolution.CreatedOn.Format(common.TableTimeFormat))
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
				row = append(row, common.EmptyCell)
			}
		default:
			row = append(row, common.EmptyCell)
		}
	}
	return row
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

	data, err = json.Marshal(struct {
		surrogate
		CreatedOn *string `json:"created_on,omitempty"`
	}{
		surrogate: surrogate(resolution),
		CreatedOn: common.FormatOptionalTime(resolution.CreatedOn),
	})
	if err != nil {
		return nil, fmt.Errorf("cannot marshal resolution to json: %w", err)
	}
	return data, nil
}

// MarshalJSON implements the json.Marshaler interface.
// CreatedOn/UpdatedOn are only formatted (and only included at all, via omitempty) when non-zero,
// matching Resolution.MarshalJSON just above: a year-1 "0001-01-01T00:00:00Z" in machine-readable
// output has no meaning to a caller scripting against it.
func (comment Comment) MarshalJSON() (data []byte, err error) {
	type surrogate Comment

	data, err = json.Marshal(struct {
		surrogate
		CreatedOn *string `json:"created_on,omitempty"`
		UpdatedOn *string `json:"updated_on,omitempty"`
	}{
		surrogate: surrogate(comment),
		CreatedOn: common.FormatOptionalTime(comment.CreatedOn),
		UpdatedOn: common.FormatOptionalTime(comment.UpdatedOn),
	})
	if err != nil {
		return nil, fmt.Errorf("cannot marshal comment to json: %w", err)
	}
	return data, nil
}

// GetPullRequestCommentIDs gets the IDs of the comments for the pullrequest identified by
// pullRequestID
func GetPullRequestCommentIDs(ctx context.Context, cmd *cobra.Command, pullRequestID string) (ids []string, err error) {
	repository, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return nil, fmt.Errorf("cannot get repository: %w", err)
	}

	comments, err := profile.GetAll[Comment](ctx, cmd, repository.GetPath(fmt.Sprintf("pullrequests/%s/comments", pullRequestID)))
	if err != nil {
		lgr.Printf("[ERROR] failed to get pullrequests: %v", err)
		return nil, err
	}
	ids = core.Map(comments, func(comment Comment) string { return strconv.Itoa(comment.ID) })
	return ids, nil
}

// pullRequestAndCommentIDValidArgs is the ValidArgsFunction shared by every comment subcommand
// that takes exactly <pullrequest-id> <comment-id> as its two positionals (get, update, reopen,
// resolve): arg 0 completes open pullrequest ids, arg 1 completes the comment ids of the
// pullrequest named in arg 0.
func pullRequestAndCommentIDValidArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	switch len(args) {
	case 0:
		ids, err := prcommon.GetPullRequestIDs(cmd.Context(), cmd, args, toComplete)
		if err != nil {
			return []string{}, cobra.ShellCompDirectiveNoFileComp
		}
		return common.FilterValidArgs(ids, args, toComplete), cobra.ShellCompDirectiveNoFileComp
	case 1:
		commentIDs, err := GetPullRequestCommentIDs(cmd.Context(), cmd, args[0])
		if err != nil {
			return []string{}, cobra.ShellCompDirectiveNoFileComp
		}
		return common.FilterValidArgs(commentIDs, args[1:], toComplete), cobra.ShellCompDirectiveNoFileComp
	default:
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
}
