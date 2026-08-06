package commit_test

import (
	"testing"
	"time"

	"github.com/avitsrimer/bitbucket-cli/internal/commit"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/avitsrimer/bitbucket-cli/internal/user"
)

func TestCommitGetHeadersDefault(t *testing.T) {
	want := []string{"Hash", "Date", "Author", "Message"}
	got := commit.Commit{}.GetHeaders(nil)
	if len(got) != len(want) {
		t.Fatalf("GetHeaders() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("GetHeaders()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestCommitGetRowCoversEveryColumn iterates GetColumnDefinitions().Columns() (the single source
// of truth, not a hand-written header list) and requires every declared column to produce its
// real value instead of falling through to GetRow's default " " arm.
func TestCommitGetRowCoversEveryColumn(t *testing.T) {
	target := commit.Commit{
		Hash:       "0265607aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Author:     user.Author{User: user.User{Name: "Jane Doe"}},
		Message:    "Add feature",
		Date:       time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Repository: repository.Repository{Name: "widgets"},
	}

	for _, name := range target.GetColumnDefinitions().Columns() {
		row := target.GetRow([]string{name})
		if len(row) != 1 {
			t.Fatalf("GetRow([%q]) = %v, want exactly one cell", name, row)
		}
		if row[0] == " " {
			t.Errorf("column %q hit GetRow's default arm instead of a real case", name)
		}
	}
}

func TestCommitGetRowUnknownColumnFillsPlaceholder(t *testing.T) {
	target := commit.Commit{Hash: "0265607aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}

	row := target.GetRow([]string{"hash", "not-a-real-column"})
	if len(row) != 2 {
		t.Fatalf("GetRow() = %v, want exactly one cell per header", row)
	}
	if row[0] != "0265607" || row[1] != " " {
		t.Errorf("GetRow() = %v, want [\"0265607\", \" \"]", row)
	}
}
