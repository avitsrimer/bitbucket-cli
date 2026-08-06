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

// TestCommitsTableables proves the Commits collection type's Tableables wiring: GetHeaders,
// GetRowAt (including its out-of-range guards), and Size. Every sibling package restored
// alongside commit already has an equivalent test for its own collection type; this one was
// missing.
func TestCommitsTableables(t *testing.T) {
	target := commit.Commits{
		{Hash: "aaaaaaa"},
		{Hash: "bbbbbbb"},
	}

	if target.Size() != 2 {
		t.Errorf("Size() = %d, want 2", target.Size())
	}
	if headers := target.GetHeaders(nil); len(headers) == 0 {
		t.Error("GetHeaders(nil) returned no headers, want the default column set")
	}
	if row := target.GetRowAt(0, []string{"hash"}); len(row) != 1 || row[0] != "aaaaaaa" {
		t.Errorf("GetRowAt(0, ...) = %v, want [\"aaaaaaa\"]", row)
	}
	if row := target.GetRowAt(1, []string{"hash"}); len(row) != 1 || row[0] != "bbbbbbb" {
		t.Errorf("GetRowAt(1, ...) = %v, want [\"bbbbbbb\"]", row)
	}
	if row := target.GetRowAt(-1, []string{"hash"}); len(row) != 0 {
		t.Errorf("GetRowAt(-1, ...) = %v, want empty", row)
	}
	if row := target.GetRowAt(5, []string{"hash"}); len(row) != 0 {
		t.Errorf("GetRowAt(5, ...) = %v, want empty", row)
	}
}
