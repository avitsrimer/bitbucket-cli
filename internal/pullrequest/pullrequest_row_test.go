package pullrequest_test

import (
	"testing"
	"time"

	"github.com/avitsrimer/bitbucket-cli/internal/commit"
	"github.com/avitsrimer/bitbucket-cli/internal/pullrequest"
	"github.com/avitsrimer/bitbucket-cli/internal/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPullRequestGetHeadersDefault(t *testing.T) {
	target := pullrequest.PullRequest{}
	assert.Equal(t, []string{"ID", "Title", "Description", "source", "destination", "state"}, target.GetHeaders(nil))
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
	assert.Equal(t, []string{"ID", "Title", "Description", "source", "destination", "state"}, target.GetHeaders(nil))
	assert.Equal(t, []string{"1", "First"}, target.GetRowAt(0, []string{"id", "title"}))
	assert.Equal(t, []string{"2", "Second"}, target.GetRowAt(1, []string{"id", "title"}))
	assert.Empty(t, target.GetRowAt(-1, []string{"id"}))
	assert.Empty(t, target.GetRowAt(5, []string{"id"}))
}
