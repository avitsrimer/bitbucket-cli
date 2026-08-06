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
	"github.com/avitsrimer/bitbucket-cli/internal/project"
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
	Type              string                  `json:"type"`
	ID                uint64                  `json:"id"`
	Title             string                  `json:"title"`
	Description       string                  `json:"description"`
	Summary           common.RenderedText     `json:"summary"`
	State             string                  `json:"state"`
	MergeCommit       *commit.CommitReference `json:"merge_commit,omitempty"`
	CloseSourceBranch bool                    `json:"close_source_branch"`
	ClosedBy          user.User               `json:"closed_by"`
	Author            user.User               `json:"author"`
	Reviewers         []user.User             `json:"reviewers,omitempty"`
	Reason            string                  `json:"reason"`
	Destination       Endpoint                `json:"destination"`
	Source            Endpoint                `json:"source"`
	Links             common.Links            `json:"links"`
	CommentCount      uint64                  `json:"comment_count"`
	TaskCount         uint64                  `json:"task_count"`
	CreatedOn         time.Time               `json:"created_on"`
	UpdatedOn         time.Time               `json:"updated_on"`
}

// Command represents this folder's command
var Command = &cobra.Command{
	Use:     "pullrequest",
	Aliases: []string{"pr", "pull-request"},
	Short:   "Manage pull requests",
	Run:     common.SubcommandRequired("Pullrequest"),
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
				row = append(row, pullrequest.MergeCommit.GetShortHash())
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
			row = append(row, pullrequest.CreatedOn.Format(common.TableTimeFormat))
		case "updated on", "updated_on", "updated-on":
			if !pullrequest.UpdatedOn.IsZero() {
				row = append(row, pullrequest.UpdatedOn.Format(common.TableTimeFormat))
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

// GetPullRequestIDFromArgs gets the pullrequest ID from the command arguments or, if not provided, from the only open pull request
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
			return "", fmt.Errorf("too many open pullrequests, specify one: %s", strings.Join(pullRequestIDs, ", "))
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
	pullrequestWorkspace, err := workspace.GetWorkspace(ctx, cmd)
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

// reviewerCompletionFunc adapts GetReviewerNicknames to cobra's shell-completion function
// signature for the --reviewer/--add-reviewer/--remove-reviewer flags.
//
// These flags are plain string slices, not common.EnumSliceFlag: the reviewer identifier a user
// may pass is not limited to a workspace member's nickname -- it can be an Account ID, a UUID, a
// display name, the `all` sentinel (every workspace member, see expandAllReviewers), or the
// documented `default` sentinel. resolveExplicitReviewers, resolveCreateDefaultReviewers,
// resolveDefaultReviewers, and addRequestedReviewers validate and resolve the value at request
// time instead: a value that cannot be resolved to a workspace member or a real user is a hard
// error (subject to the profile's ShouldStopOnError/ShouldWarnOnError/ShouldIgnoreErrors
// tolerance), aborting before any POST/PUT is sent. GetReviewerNicknames' member list is used
// here purely as a shell-completion aid, never to reject an otherwise valid value at flag-parse
// time.
func reviewerCompletionFunc(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	nicknames, err := GetReviewerNicknames(cmd.Context(), cmd, args, toComplete)
	if err != nil {
		return []string{}, cobra.ShellCompDirectiveError
	}
	return nicknames, cobra.ShellCompDirectiveNoFileComp
}

// expandAllReviewers implements the "all" sentinel for --reviewer/--add-reviewer: when the caller
// passed exactly "all" and nothing else, every workspace member's nickname is substituted in its
// place, then matched against the workspace like any other reviewer value. Any other combination
// of values (e.g. "all,bob", or "all" alongside other flags) is left untouched, so a workspace
// with no member literally named "all" failing to resolve it is expected, not a bug.
func expandAllReviewers(values []string, members []workspace.Member) []string {
	if len(values) == 1 && values[0] == "all" {
		return core.Map(members, func(member workspace.Member) string { return member.User.Nickname })
	}
	return values
}

// matchesMember reports whether member is identified by id: a value that parses as a UUID is
// compared against the member's ID, otherwise id is compared case-insensitively against the
// member's Account ID, nickname, or display name.
func matchesMember(member workspace.Member, id string) bool {
	if parsedID, uuidErr := common.ParseUUID(id); uuidErr == nil {
		return member.User.ID == parsedID
	}
	return member.User.AccountID == id || strings.EqualFold(member.User.Nickname, id) || strings.EqualFold(member.User.Name, id)
}

// effectiveDefaultReviewers resolves the effective default reviewers of repo (repository or
// project settings), excluding the current user when known.
func effectiveDefaultReviewers(ctx context.Context, cmd *cobra.Command, repo *repository.Repository) ([]project.Reviewer, error) {
	lgr.Printf("[DEBUG] finding current user")
	me, errMe := user.GetMe(ctx, cmd)
	if errMe != nil {
		// RAT (repo scoped tokens) do not have access to that API endpoint usually
		lgr.Printf("[WARN] failed to get current user, this may be a RAT client. Error: %s", errMe.Error())
	} else {
		lgr.Printf("[DEBUG] current user: %s (%s)", me.Username, me.ID)
	}

	lgr.Printf("[DEBUG] getting effective default reviewers of repository %s", repo)
	reviewers, err := repo.GetEffectiveDefaultReviewers(ctx, cmd)
	if err != nil {
		lgr.Printf("[ERROR] failed to get default reviewers: %v", err)
		return nil, errors.Join(fmt.Errorf("failed to get the default reviewers: %w", err), errMe)
	}
	lgr.Printf("[DEBUG] found %d default reviewers", len(reviewers))

	if me != nil {
		// removing the current user from the reviewers, since they cannot review their own pullrequest
		reviewers = core.Filter(reviewers, func(reviewer project.Reviewer) bool { return reviewer.User.ID != me.ID })
		lgr.Printf("[DEBUG] filtered reviewers to remove current user: %d reviewers remaining", len(reviewers))
	}
	return reviewers, nil
}

// tolerateReviewerErrors decides, given prof's ShouldWarnOnError/ShouldIgnoreErrors tolerance,
// whether errs (aggregated reviewer resolution failures) should be returned as a hard error,
// printed to stderr as a warning, or silently logged and ignored. summary describes the failed
// action in lowercase (e.g. "resolve these reviewers") for both the stderr and log messages. It
// returns nil whenever the profile's tolerance absorbs errs, or the joined error otherwise.
func tolerateReviewerErrors(cmd *cobra.Command, prof *profile.Profile, errs []error, summary string) error {
	joined := errors.Join(errs...)
	if joined == nil {
		return nil
	}
	if prof.ShouldWarnOnError(cmd) {
		fmt.Fprintf(os.Stderr, "Failed to %s: %s\n", summary, joined)
		return nil
	}
	if prof.ShouldIgnoreErrors(cmd) {
		lgr.Printf("[WARN] failed to %s, but ignoring errors: %s", summary, joined)
		return nil
	}
	return joined
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
		CreatedOn: pullrequest.CreatedOn.Format(common.JSONTimeFormat),
		UpdatedOn: pullrequest.UpdatedOn.Format(common.JSONTimeFormat),
	})
	if err != nil {
		return nil, fmt.Errorf("cannot marshal pullrequest to json: %w", err)
	}
	return data, nil
}
