package pullrequest

import (
	"testing"
	"time"

	"github.com/avitsrimer/bitbucket-cli/internal/commit"
	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/avitsrimer/bitbucket-cli/internal/user"
)

// TestPullRequestGetRowCoversEveryColumn requires every column the shared columns table declares --
// i.e. every name reachable via --columns/--sort -- to produce its real value from a fully populated
// pull request instead of falling through to GetRow's default common.EmptyCell arm.
//
// It iterates columns.Columns() rather than a hand-written name list (which is why it lives in the
// package rather than beside the other GetRow tests in pullrequest_row_test.go): a list would have to
// be remembered on every new column, and forgetting it ships an uncovered column with the test still
// green.
func TestPullRequestGetRowCoversEveryColumn(t *testing.T) {
	createdOn := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	target := PullRequest{
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
		Source:       Endpoint{Branch: Branch{Name: "feature"}},
		Destination: Endpoint{
			Branch:     Branch{Name: "master"},
			Repository: &repository.Repository{FullName: "acme/widgets"},
		},
		MergeCommit: &commit.CommitReference{Hash: "abcdef0123456789"},
	}

	names := columns.Columns()
	if len(names) == 0 {
		t.Fatal("columns table declares no column at all")
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			row := target.GetRow([]string{name})
			if len(row) != 1 {
				t.Fatalf("GetRow([%q]) = %v, want exactly one cell", name, row)
			}
			if row[0] == common.EmptyCell {
				t.Errorf("column %q hit GetRow's default arm instead of a real case", name)
			}
		})
	}
}
