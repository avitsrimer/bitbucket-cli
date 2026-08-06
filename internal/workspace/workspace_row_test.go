package workspace

import (
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/user"
)

func TestWorkspaceGetHeadersDefault(t *testing.T) {
	want := []string{"ID", "Name", "Slug"}
	got := Workspace{}.GetHeaders(nil)
	if len(got) != len(want) {
		t.Fatalf("GetHeaders() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("GetHeaders()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestWorkspaceGetRowCoversEveryColumn iterates columns.Columns() (the single source of truth,
// not a hand-written header list) and requires every declared column to produce its real value
// instead of falling through to GetRow's default " " arm.
func TestWorkspaceGetRowCoversEveryColumn(t *testing.T) {
	target := Workspace{Name: "Acme Corp", Slug: "acme"}

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

func TestWorkspaceGetRowUnknownColumnFillsPlaceholder(t *testing.T) {
	target := Workspace{Name: "Acme Corp", Slug: "acme"}

	row := target.GetRow([]string{"slug", "not-a-real-column"})
	if len(row) != 2 {
		t.Fatalf("GetRow() = %v, want exactly one cell per header", row)
	}
	if row[0] != "acme" || row[1] != " " {
		t.Errorf("GetRow() = %v, want [\"acme\", \" \"]", row)
	}
}

func TestWorkspaceGetRowAcceptsHyphenAndSpaceColumnSpellings(t *testing.T) {
	target := Workspace{Name: "Acme Corp"}

	for _, spelling := range []string{"name", "Name"} {
		row := target.GetRow([]string{spelling})
		if len(row) != 1 || row[0] != "Acme Corp" {
			t.Errorf("GetRow([%q]) = %v, want [\"Acme Corp\"]", spelling, row)
		}
	}
}

func TestWorkspacesTableables(t *testing.T) {
	target := Workspaces{
		{Name: "Acme Corp", Slug: "acme"},
		{Name: "Beta Inc", Slug: "beta"},
	}

	if target.Size() != 2 {
		t.Errorf("Size() = %d, want 2", target.Size())
	}
	if row := target.GetRowAt(0, []string{"slug"}); len(row) != 1 || row[0] != "acme" {
		t.Errorf("GetRowAt(0, ...) = %v, want [\"acme\"]", row)
	}
	if row := target.GetRowAt(1, []string{"slug"}); len(row) != 1 || row[0] != "beta" {
		t.Errorf("GetRowAt(1, ...) = %v, want [\"beta\"]", row)
	}
	if row := target.GetRowAt(-1, []string{"slug"}); len(row) != 0 {
		t.Errorf("GetRowAt(-1, ...) = %v, want empty", row)
	}
	if row := target.GetRowAt(5, []string{"slug"}); len(row) != 0 {
		t.Errorf("GetRowAt(5, ...) = %v, want empty", row)
	}
}

func TestMemberGetHeadersDefault(t *testing.T) {
	want := []string{"ID", "Name", "Workspace"}
	got := Member{}.GetHeaders(nil)
	if len(got) != len(want) {
		t.Fatalf("GetHeaders() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("GetHeaders()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestMemberGetRowCoversEveryColumn iterates memberColumns.Columns() and requires every declared
// column to produce its real value instead of falling through to GetRow's default " " arm.
func TestMemberGetRowCoversEveryColumn(t *testing.T) {
	target := Member{
		User:      user.User{Name: "Jane Doe", Username: "jdoe"},
		Workspace: Workspace{Slug: "acme"},
	}

	for _, name := range memberColumns.Columns() {
		row := target.GetRow([]string{name})
		if len(row) != 1 {
			t.Fatalf("GetRow([%q]) = %v, want exactly one cell", name, row)
		}
		if row[0] == " " {
			t.Errorf("column %q hit GetRow's default arm instead of a real case", name)
		}
	}
}

func TestMemberGetRowUnknownColumnFillsPlaceholder(t *testing.T) {
	target := Member{User: user.User{Name: "Jane Doe"}}

	row := target.GetRow([]string{"name", "not-a-real-column"})
	if len(row) != 2 {
		t.Fatalf("GetRow() = %v, want exactly one cell per header", row)
	}
	if row[0] != "Jane Doe" || row[1] != " " {
		t.Errorf("GetRow() = %v, want [\"Jane Doe\", \" \"]", row)
	}
}

func TestMembersTableables(t *testing.T) {
	target := Members{
		{User: user.User{Name: "Jane Doe"}},
		{User: user.User{Name: "John Doe"}},
	}

	if target.Size() != 2 {
		t.Errorf("Size() = %d, want 2", target.Size())
	}
	if row := target.GetRowAt(0, []string{"name"}); len(row) != 1 || row[0] != "Jane Doe" {
		t.Errorf("GetRowAt(0, ...) = %v, want [\"Jane Doe\"]", row)
	}
	if row := target.GetRowAt(1, []string{"name"}); len(row) != 1 || row[0] != "John Doe" {
		t.Errorf("GetRowAt(1, ...) = %v, want [\"John Doe\"]", row)
	}
	if row := target.GetRowAt(-1, []string{"name"}); len(row) != 0 {
		t.Errorf("GetRowAt(-1, ...) = %v, want empty", row)
	}
	if row := target.GetRowAt(5, []string{"name"}); len(row) != 0 {
		t.Errorf("GetRowAt(5, ...) = %v, want empty", row)
	}
}
