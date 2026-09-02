package pullrequest

import (
	"slices"
	"testing"
	"time"

	"github.com/avitsrimer/bitbucket-cli/internal/commit"
	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/avitsrimer/bitbucket-cli/internal/user"
	"github.com/spf13/cobra"
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
		Draft:        true,
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

// TestPullRequestColumnsSortByDraftOrdersNonDraftsFirst proves the "draft" comparator orders
// non-draft pull requests before drafts. Like every other low-cardinality comparator in the table
// (state, comments, tasks, participants) it declares no tie-break, so the order within each group
// is whatever common.Sort's unstable sort produces.
func TestPullRequestColumnsSortByDraftOrdersNonDraftsFirst(t *testing.T) {
	pullrequests := []PullRequest{{ID: 1, Draft: true}, {ID: 2, Draft: false}}

	common.Sort(pullrequests, columns.SortBy("draft"))

	if pullrequests[0].Draft || !pullrequests[1].Draft {
		t.Fatalf("order = %+v, want the non-draft first", pullrequests)
	}
}

// TestPullRequestGetHeadersOnRealCommands asserts the default column sets against the real
// package-level commands, not hand-built stand-ins: GetHeaders branches on cmd.Name(), so renaming
// a command's Use would otherwise drop draft (and, on get, description) from its default table with
// the rest of the suite still green. update and create carry draft so the row each prints -- the
// server's own response to the write -- confirms the resulting draft state; neither registers
// --columns, so the default is the only way that column can appear there. merge also prints a
// single row without registering --columns, but its response is always a non-draft, so it has no
// draft default and is not asserted here.
func TestPullRequestGetHeadersOnRealCommands(t *testing.T) {
	tests := []struct {
		name string
		cmd  *cobra.Command
		want []string
	}{
		{name: "get", cmd: getCmd, want: []string{"ID", "Title", "source", "destination", "state", "description", "draft"}},
		{name: "update", cmd: updateCmd, want: []string{"ID", "Title", "source", "destination", "state", "draft"}},
		{name: "create", cmd: createCmd, want: []string{"ID", "Title", "source", "destination", "state", "draft"}},
		{name: "list", cmd: listCmd, want: []string{"ID", "Title", "source", "destination", "state"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (PullRequest{}).GetHeaders(tt.cmd); !slices.Equal(got, tt.want) {
				t.Errorf("%s default headers = %v, want %v", tt.cmd.Name(), got, tt.want)
			}
		})
	}
}
