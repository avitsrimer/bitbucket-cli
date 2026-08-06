package branch

import (
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/commit"
)

func TestBranchGetHeadersDefault(t *testing.T) {
	want := []string{"Name"}
	got := Branch{}.GetHeaders(nil)
	if len(got) != len(want) {
		t.Fatalf("GetHeaders() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("GetHeaders()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestBranchGetRowCoversEveryColumn iterates columns.Columns() (the single source of truth, not a
// hand-written header list) and requires every declared column to produce its real value instead
// of falling through to GetRow's default " " arm.
func TestBranchGetRowCoversEveryColumn(t *testing.T) {
	target := Branch{
		Name:                 "main",
		Target:               commit.Commit{Hash: "aaaaaaa"},
		MergeStrategies:      []string{"merge_commit", "squash"},
		DefaultMergeStrategy: "merge_commit",
	}

	for _, name := range columns.Columns() {
		row := target.GetRow([]string{name})
		if len(row) != 1 {
			t.Fatalf("GetRow([%q]) = %v, want exactly one cell", name, row)
		}
		if row[0] == " " {
			t.Errorf("column %q hit GetRow's default arm instead of a real case", name)
		}
	}
}

func TestBranchGetRowUnknownColumnFillsPlaceholder(t *testing.T) {
	target := Branch{Name: "main"}

	row := target.GetRow([]string{"name", "not-a-real-column"})
	if len(row) != 2 {
		t.Fatalf("GetRow() = %v, want exactly one cell per header", row)
	}
	if row[0] != "main" || row[1] != " " {
		t.Errorf("GetRow() = %v, want [\"main\", \" \"]", row)
	}
}

func TestBranchGetRowAcceptsHyphenAndSpaceColumnSpellings(t *testing.T) {
	target := Branch{DefaultMergeStrategy: "squash"}

	for _, spelling := range []string{"default merge strategy", "default-merge-strategy", "default_merge_strategy"} {
		row := target.GetRow([]string{spelling})
		if len(row) != 1 || row[0] != "squash" {
			t.Errorf("GetRow([%q]) = %v, want [\"squash\"]", spelling, row)
		}
	}
}
