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
	Participants      []user.Participant      `json:"participants,omitempty"`
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
	{Name: "participants", DefaultSorter: false, Compare: func(a, b PullRequest) bool {
		return len(a.Participants) < len(b.Participants)
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
//
// GetHeaders is shared by both `pullrequest list` and `pullrequest get`, which need different
// defaults: Description is deliberately not part of list's default column set (a full,
// potentially multi-paragraph pull request body adds little to a list view already showing
// Title/source/destination/state), but a single `pullrequest get` is precisely the one place the
// body belongs by default -- without this, there was no default table/csv/tsv path to a PR's
// description at all, only -o json/yaml or an explicit --columns description on `get`. The
// table renderer still ellipsizes it past profile.maxTableCellWidth like any other free-text
// column (see profile.freeTextColumnKeys); -o json/yaml/csv/tsv always show it complete.
// cmd.Name() distinguishes the two commands: getCmd's Use starts with "get", listCmd's with
// "list".
//
// participants is deliberately out of both defaults, on either command: a multi-reviewer PR's
// "nickname:state" summary (see formatParticipants) is exactly the kind of unbounded, list-shaped
// value the default column set otherwise avoids -- it stays reachable via an explicit
// `--columns participants` on either command, or unconditionally in -o json/yaml.
func (pullrequest PullRequest) GetHeaders(cmd *cobra.Command) []string {
	defaults := []string{"ID", "Title", "source", "destination", "state"}
	if cmd != nil && cmd.Name() == "get" {
		defaults = append(defaults, "description")
	}
	return common.HeadersFromFlag(cmd, defaults...)
}

// GetRow gets the row for a table
//
// implements common.Tableable
func (pullrequest PullRequest) GetRow(headers []string) []string {
	var row []string

	for _, header := range headers {
		switch common.NormalizeColumnKey(header) {
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
		case "closed_by":
			row = append(row, pullrequest.ClosedBy.Name)
		case "commit":
			if pullrequest.MergeCommit != nil {
				row = append(row, pullrequest.MergeCommit.GetShortHash())
			} else {
				row = append(row, common.EmptyCell)
			}
		case "reason":
			row = append(row, pullrequest.Reason)
		case "comments":
			row = append(row, strconv.FormatUint(pullrequest.CommentCount, 10))
		case "tasks":
			row = append(row, strconv.FormatUint(pullrequest.TaskCount, 10))
		case "participants":
			row = append(row, formatParticipants(pullrequest.Participants))
		case "created_on":
			row = append(row, common.TimeCell(pullrequest.CreatedOn))
		case "updated_on":
			row = append(row, common.TimeCell(pullrequest.UpdatedOn))
		default:
			row = append(row, common.EmptyCell)
		}
	}
	return row
}

// formatParticipants renders participants as a compact, comma-separated "nickname:state" summary
// for table/csv/tsv display, one entry per participant so a reviewer's approval state stays
// individually readable instead of being collapsed into a single count. A participant without a
// nickname falls back to their display name; a participant who has not yet reviewed reports an
// empty State from the API, rendered here as "pending" rather than an empty segment.
func formatParticipants(participants []user.Participant) string {
	if len(participants) == 0 {
		return common.EmptyCell
	}
	summaries := make([]string, 0, len(participants))
	for _, participant := range participants {
		name := participant.User.Nickname
		if name == "" {
			name = participant.User.Name
		}
		state := participant.State
		if state == "" {
			state = "pending"
		}
		summaries = append(summaries, name+":"+state)
	}
	return strings.Join(summaries, ", ")
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

// GetReviewerNicknames gets the reviewer nicknames for the current Workspace. The workspace slug
// comes from workspace.GetWorkspaceName (no API call); only GetMembers itself reaches the API,
// scoped to the slug the caller already supplied.
func GetReviewerNicknames(ctx context.Context, cmd *cobra.Command, args []string, toComplete string) (nicknames []string, err error) {
	if cmd == nil {
		fmt.Fprintln(os.Stderr, "cmd is nil")
		return []string{}, errors.New("argument cmd is missing")
	}

	lgr.Printf("[DEBUG] getting reviewer nicknames for profile %s", profile.Current)
	workspaceSlug, err := workspace.GetWorkspaceName(ctx, cmd)
	if err != nil {
		lgr.Printf("[ERROR] failed to get repository: %v", err)
		return []string{}, fmt.Errorf("cannot get workspace: %w", err)
	}
	lgr.Printf("[DEBUG] getting members of workspace %s", workspaceSlug)
	members, _ := workspace.GetMembers(ctx, cmd, workspaceSlug)
	nicknames = common.Map(members, func(member workspace.Member) string { return member.User.Nickname })
	common.Sort(nicknames, func(a, b string) bool { return strings.ToLower(a) < strings.ToLower(b) })
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
// passed exactly "all" and nothing else, every workspace member's Account UUID is substituted in
// its place, then matched against the workspace like any other reviewer value (matchesMember
// parses a UUID-shaped value and compares it against the member's ID directly, so this always
// matches regardless of whether the member has a nickname). Any other combination of values (e.g.
// "all,bob", or "all" alongside other flags) is left untouched, so a workspace with no member
// literally named "all" failing to resolve it is expected, not a bug.
//
// membersErr is the error (if any) GetMembers returned resolving members: when "all" was
// requested and the member list could not be resolved, there is nothing to expand it to, so that
// is returned as a hard error instead of silently proceeding as if the workspace had no members --
// which would otherwise create/update a pullrequest with zero reviewers at exit 0, a silent no-op
// that every other reviewer resolution failure avoids via the
// ShouldStopOnError/ShouldWarnOnError/ShouldIgnoreErrors tolerance.
func expandAllReviewers(values []string, members []workspace.Member, membersErr error) ([]string, error) {
	if len(values) != 1 || values[0] != "all" {
		return values, nil
	}
	if membersErr != nil {
		return nil, fmt.Errorf("cannot expand reviewer \"all\": failed to list workspace members: %w", membersErr)
	}
	return common.Map(members, func(member workspace.Member) string { return member.User.ID.String() }), nil
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
		return nil, errors.Join(fmt.Errorf("failed to get the default reviewers: %w", err), errMe)
	}
	lgr.Printf("[DEBUG] found %d default reviewers", len(reviewers))

	if me != nil {
		// removing the current user from the reviewers, since they cannot review their own pullrequest
		reviewers = common.Filter(reviewers, func(reviewer project.Reviewer) bool { return reviewer.User.ID != me.ID })
		lgr.Printf("[DEBUG] filtered reviewers to remove current user: %d reviewers remaining", len(reviewers))
	}
	return reviewers, nil
}

// MarshalJSON implements the json.Marshaler interface.
//
// CreatedOn/UpdatedOn are only formatted (and only included at all, via omitempty) when non-zero:
// a PullRequest built by hand (e.g. in a test, or a payload this codebase itself constructs)
// rather than decoded from a real API response otherwise emits time.Time's own zero-value
// marshaling, "0001-01-01T00:00:00Z", into machine-readable JSON/YAML output -- a year-1
// timestamp with no meaning to a caller scripting against it.
func (pullrequest PullRequest) MarshalJSON() (data []byte, err error) {
	type surrogate PullRequest

	data, err = json.Marshal(struct {
		surrogate
		CreatedOn *string `json:"created_on,omitempty"`
		UpdatedOn *string `json:"updated_on,omitempty"`
	}{
		surrogate: surrogate(pullrequest),
		CreatedOn: common.FormatOptionalTime(pullrequest.CreatedOn),
		UpdatedOn: common.FormatOptionalTime(pullrequest.UpdatedOn),
	})
	if err != nil {
		return nil, fmt.Errorf("cannot marshal pullrequest to json: %w", err)
	}
	return data, nil
}
