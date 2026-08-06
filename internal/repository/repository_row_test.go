package repository

import (
	"testing"
	"time"

	"github.com/avitsrimer/bitbucket-cli/internal/project"
	"github.com/avitsrimer/bitbucket-cli/internal/user"
	"github.com/avitsrimer/bitbucket-cli/internal/workspace"
)

func TestRepositoryGetHeadersDefault(t *testing.T) {
	want := []string{"Name", "Full Name", "Slug", "Workspace"}
	got := Repository{}.GetHeaders(nil)
	if len(got) != len(want) {
		t.Fatalf("GetHeaders() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("GetHeaders()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestRepositoryGetRowCoversEveryColumn iterates columns.Columns() (the single source of truth,
// not a hand-written header list) and requires every declared column to produce its real value
// instead of falling through to GetRow's default " " arm.
func TestRepositoryGetRowCoversEveryColumn(t *testing.T) {
	target := Repository{
		Name:                 "bb",
		FullName:             "acme/bb",
		Slug:                 "bb",
		Owner:                user.User{Name: "Jane Doe"},
		Workspace:            &workspace.Workspace{Name: "Acme Corp", Slug: "acme"},
		Project:              project.Project{Name: "Tools"},
		HasIssues:            true,
		HasWiki:              true,
		IsPrivate:            true,
		ForkPolicy:           "no_forks",
		Size:                 1024,
		Language:             "go",
		MainBranch:           "master",
		DefaultMergeStrategy: "squash",
		BranchingModel:       "git-flow",
		Parent:               &Repository{FullName: "acme/upstream"},
		CreatedOn:            time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedOn:            time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
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

func TestRepositoryGetRowUnknownColumnFillsPlaceholder(t *testing.T) {
	target := Repository{Slug: "bb"}

	row := target.GetRow([]string{"slug", "not-a-real-column"})
	if len(row) != 2 {
		t.Fatalf("GetRow() = %v, want exactly one cell per header", row)
	}
	if row[0] != "bb" || row[1] != " " {
		t.Errorf("GetRow() = %v, want [\"bb\", \" \"]", row)
	}
}

func TestRepositoryGetRowNilWorkspaceAndParentFillPlaceholder(t *testing.T) {
	target := Repository{Slug: "bb"}

	row := target.GetRow([]string{"workspace", "parent"})
	if len(row) != 2 || row[0] != " " || row[1] != " " {
		t.Errorf("GetRow() = %v, want [\" \", \" \"] for a nil Workspace and Parent", row)
	}
}

func TestRepositoryGetRowZeroUpdatedOnFillsPlaceholder(t *testing.T) {
	target := Repository{Slug: "bb", CreatedOn: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}

	row := target.GetRow([]string{"updated_on"})
	if len(row) != 1 || row[0] != " " {
		t.Errorf("GetRow() = %v, want [\" \"] for a zero UpdatedOn", row)
	}
}

func TestRepositoryGetRowAcceptsHyphenAndSpaceColumnSpellings(t *testing.T) {
	target := Repository{Name: "bitbucket-cli"}

	for _, spelling := range []string{"full name", "full-name", "full_name"} {
		row := target.GetRow([]string{spelling})
		if len(row) != 1 || row[0] != "" {
			t.Errorf("GetRow([%q]) = %v, want [\"\"] (FullName unset)", spelling, row)
		}
	}
}

func TestRepositoriesTableables(t *testing.T) {
	target := Repositories{
		{Slug: "bb"},
		{Slug: "widgets"},
	}

	if target.Size() != 2 {
		t.Errorf("Size() = %d, want 2", target.Size())
	}
	if row := target.GetRowAt(0, []string{"slug"}); len(row) != 1 || row[0] != "bb" {
		t.Errorf("GetRowAt(0, ...) = %v, want [\"bb\"]", row)
	}
	if row := target.GetRowAt(1, []string{"slug"}); len(row) != 1 || row[0] != "widgets" {
		t.Errorf("GetRowAt(1, ...) = %v, want [\"widgets\"]", row)
	}
	if row := target.GetRowAt(-1, []string{"slug"}); len(row) != 0 {
		t.Errorf("GetRowAt(-1, ...) = %v, want empty", row)
	}
	if row := target.GetRowAt(5, []string{"slug"}); len(row) != 0 {
		t.Errorf("GetRowAt(5, ...) = %v, want empty", row)
	}
}
