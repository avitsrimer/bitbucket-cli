package pullrequest_test

import (
	"testing"
	"time"

	"github.com/avitsrimer/bitbucket-cli/internal/commit"
	"github.com/avitsrimer/bitbucket-cli/internal/pullrequest"
	"github.com/avitsrimer/bitbucket-cli/internal/user"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPullRequestGetHeadersDefault(t *testing.T) {
	target := pullrequest.PullRequest{}
	assert.Equal(t, []string{"ID", "Title", "source", "destination", "state"}, target.GetHeaders(nil))
}

// TestPullRequestGetHeadersGetIncludesDescription reproduces the FINAL CRITICAL GATE's priority-4
// finding: GetHeaders is shared by `pullrequest list` and `pullrequest get`, and dropping
// Description from the shared default column set (to keep the list view readable) left `get`
// with no default table/csv/tsv path to a PR's body at all. `get`'s own command (Use starting
// with "get") must include description in its defaults; `list`'s (or any other/nil command) must
// not.
func TestPullRequestGetHeadersGetIncludesDescription(t *testing.T) {
	target := pullrequest.PullRequest{}

	getCmd := &cobra.Command{Use: "get [flags] <pullrequest-id>"}
	assert.Equal(t, []string{"ID", "Title", "source", "destination", "state", "description"}, target.GetHeaders(getCmd))

	listCmd := &cobra.Command{Use: "list"}
	assert.Equal(t, []string{"ID", "Title", "source", "destination", "state"}, target.GetHeaders(listCmd))
}

func TestPullRequestGetRow(t *testing.T) {
	createdOn := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	updatedOn := time.Date(2024, 6, 7, 8, 9, 10, 0, time.UTC)
	target := pullrequest.PullRequest{
		ID:           42,
		Title:        "Add feature",
		Description:  "some description",
		State:        "OPEN",
		Author:       user.User{Name: "Jane Doe"},
		ClosedBy:     user.User{Name: "John Doe"},
		Reason:       "declined by reviewer",
		CommentCount: 3,
		TaskCount:    1,
		CreatedOn:    createdOn,
		UpdatedOn:    updatedOn,
		Source:       pullrequest.Endpoint{Branch: pullrequest.Branch{Name: "feature"}},
		Destination:  pullrequest.Endpoint{Branch: pullrequest.Branch{Name: "master"}},
	}

	headers := []string{
		"id", "title", "description", "source", "destination", "state",
		"author", "closed by", "reason", "comments", "tasks", "created_on", "updated_on",
	}
	row := target.GetRow(headers)

	assert.Equal(t, []string{
		"42", "Add feature", "some description", "feature", "master", "OPEN",
		"Jane Doe", "John Doe", "declined by reviewer", "3", "1",
		"2024-01-02 03:04:05", "2024-06-07 08:09:10",
	}, row)
}

// TestPullRequestGetRowParticipants proves the "participants" column renders one
// "nickname:state" pair per participant (state readable per reviewer, the whole point of FR-9),
// falls back to the display name when a participant has no nickname, and reports "pending" for a
// participant who has not yet reviewed (an empty State from the API).
func TestPullRequestGetRowParticipants(t *testing.T) {
	target := pullrequest.PullRequest{
		Participants: []user.Participant{
			{User: user.User{Nickname: "jane_doe"}, State: "approved"},
			{User: user.User{Nickname: "john_doe"}, State: "changes_requested"},
			{User: user.User{Name: "No Nickname"}, State: ""},
		},
	}

	row := target.GetRow([]string{"participants"})

	assert.Equal(t, []string{"jane_doe:approved, john_doe:changes_requested, No Nickname:pending"}, row)
}

// TestPullRequestGetRowParticipantsEmpty proves an empty/nil participant slice renders the shared
// common.EmptyCell filler rather than an empty string, matching every other empty-optional column.
func TestPullRequestGetRowParticipantsEmpty(t *testing.T) {
	target := pullrequest.PullRequest{}

	row := target.GetRow([]string{"participants"})

	assert.Equal(t, []string{" "}, row)
}

// TestPullRequestGetRowCoversEveryColumn iterates every column name GetHeaders can produce via
// --columns (the participants column above already covers the one entry needing a non-trivial
// fixture) and requires each declared column to produce its real value instead of falling through
// to GetRow's default " " arm for a populated field.
func TestPullRequestGetRowCoversEveryColumn(t *testing.T) {
	createdOn := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	target := pullrequest.PullRequest{
		ID:           42,
		Title:        "Add feature",
		Description:  "some description",
		State:        "OPEN",
		Author:       user.User{Name: "Jane Doe"},
		ClosedBy:     user.User{Name: "John Doe"},
		Reason:       "declined by reviewer",
		CommentCount: 3,
		TaskCount:    1,
		Participants: []user.Participant{{User: user.User{Nickname: "jane_doe"}, State: "approved"}},
		CreatedOn:    createdOn,
		UpdatedOn:    createdOn,
		Source:       pullrequest.Endpoint{Branch: pullrequest.Branch{Name: "feature"}},
		Destination:  pullrequest.Endpoint{Branch: pullrequest.Branch{Name: "master"}},
		MergeCommit:  &commit.CommitReference{Hash: "abcdef0123456789"},
	}

	for _, name := range []string{
		"id", "title", "description", "source", "destination", "state", "author", "closed_by",
		"commit", "reason", "comments", "tasks", "participants", "created_on", "updated_on",
	} {
		row := target.GetRow([]string{name})
		require.Len(t, row, 1)
		assert.NotEqual(t, " ", row[0], "column %q hit GetRow's default arm instead of a real case", name)
	}
}

func TestPullRequestGetRowWithoutUpdatedOnOrMergeCommit(t *testing.T) {
	target := pullrequest.PullRequest{}

	row := target.GetRow([]string{"commit", "updated_on"})

	assert.Equal(t, []string{" ", " "}, row)
}

func TestPullRequestGetRowWithMergeCommit(t *testing.T) {
	target := pullrequest.PullRequest{
		MergeCommit: &commit.CommitReference{Hash: "abcdef0123456789"},
	}

	row := target.GetRow([]string{"commit"})

	assert.Equal(t, []string{"abcdef0"}, row)
}

// TestPullRequestGetRowWithShortMergeCommitHashDoesNotPanic verifies that GetRow does not panic on
// a merge_commit hash shorter than 7 characters (e.g. from a non-standard SCM, or truncated
// test/mock data): it renders the hash via CommitReference.GetShortHash's length-guarded behavior
// instead of slicing Hash[:7] directly.
func TestPullRequestGetRowWithShortMergeCommitHashDoesNotPanic(t *testing.T) {
	target := pullrequest.PullRequest{
		MergeCommit: &commit.CommitReference{Hash: "abc"},
	}

	var row []string
	assert.NotPanics(t, func() {
		row = target.GetRow([]string{"commit"})
	})
	assert.Equal(t, []string{"abc"}, row)
}

func TestPullRequestString(t *testing.T) {
	target := pullrequest.PullRequest{Title: "Add feature"}
	assert.Equal(t, "Add feature", target.String())
}

func TestGetPullRequestIDFromArgsWithValidArg(t *testing.T) {
	id, err := pullrequest.GetPullRequestIDFromArgs(t.Context(), nil, nil, []string{"42"})
	require.NoError(t, err)
	assert.Equal(t, "42", id)
}

func TestGetPullRequestIDFromArgsWithInvalidArg(t *testing.T) {
	_, err := pullrequest.GetPullRequestIDFromArgs(t.Context(), nil, nil, []string{"not-a-number"})
	assert.ErrorContains(t, err, "argument pullrequest-id is invalid")
}

func TestPullRequestsTableables(t *testing.T) {
	target := pullrequest.PullRequests{
		{ID: 1, Title: "First"},
		{ID: 2, Title: "Second"},
	}

	assert.Equal(t, 2, target.Size())
	assert.Equal(t, []string{"ID", "Title", "source", "destination", "state"}, target.GetHeaders(nil))
	assert.Equal(t, []string{"1", "First"}, target.GetRowAt(0, []string{"id", "title"}))
	assert.Equal(t, []string{"2", "Second"}, target.GetRowAt(1, []string{"id", "title"}))
	assert.Empty(t, target.GetRowAt(-1, []string{"id"}))
	assert.Empty(t, target.GetRowAt(5, []string{"id"}))
}
