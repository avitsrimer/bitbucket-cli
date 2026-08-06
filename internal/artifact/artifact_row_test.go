package artifact

import (
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/user"
	"github.com/spf13/cobra"
)

func TestArtifactGetHeadersDefault(t *testing.T) {
	want := []string{"Name", "Size", "Downloads", "Owner"}
	got := Artifact{}.GetHeaders(nil)
	if len(got) != len(want) {
		t.Fatalf("GetHeaders() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("GetHeaders()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestArtifactGetHeadersColumnsFlagOverride(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().StringSlice("columns", nil, "")
	if err := cmd.Flags().Set("columns", "name,size"); err != nil {
		t.Fatalf("cannot set columns flag: %v", err)
	}

	got := Artifact{}.GetHeaders(cmd)
	want := []string{"name", "size"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("GetHeaders() = %v, want %v", got, want)
	}
}

// TestArtifactGetRowCoversEveryColumn iterates columns.Columns() (the single source of truth, not
// a hand-written header list) and requires every declared column to produce its real value instead
// of falling through to GetRow's default " " arm.
func TestArtifactGetRowCoversEveryColumn(t *testing.T) {
	target := Artifact{
		Name:      "build.log",
		Size:      1024,
		Downloads: 3,
		User:      user.User{Name: "Jane Doe"},
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

func TestArtifactGetRowUnknownColumnFillsPlaceholder(t *testing.T) {
	target := Artifact{Name: "build.log"}

	row := target.GetRow([]string{"name", "not-a-real-column"})
	if len(row) != 2 {
		t.Fatalf("GetRow() = %v, want exactly one cell per header", row)
	}
	if row[0] != "build.log" || row[1] != " " {
		t.Errorf("GetRow() = %v, want [\"build.log\", \" \"]", row)
	}
}

func TestArtifactGetRowAcceptsHyphenAndSpaceColumnSpellings(t *testing.T) {
	target := Artifact{Downloads: 7}

	for _, spelling := range []string{"downloads", "Downloads"} {
		row := target.GetRow([]string{spelling})
		if len(row) != 1 || row[0] != "7" {
			t.Errorf("GetRow([%q]) = %v, want [\"7\"]", spelling, row)
		}
	}
}

func TestArtifactsTableables(t *testing.T) {
	target := Artifacts{
		{Name: "build.log"},
		{Name: "coverage.xml"},
	}

	if target.Size() != 2 {
		t.Errorf("Size() = %d, want 2", target.Size())
	}
	if row := target.GetRowAt(0, []string{"name"}); len(row) != 1 || row[0] != "build.log" {
		t.Errorf("GetRowAt(0, ...) = %v, want [\"build.log\"]", row)
	}
	if row := target.GetRowAt(1, []string{"name"}); len(row) != 1 || row[0] != "coverage.xml" {
		t.Errorf("GetRowAt(1, ...) = %v, want [\"coverage.xml\"]", row)
	}
	if row := target.GetRowAt(-1, []string{"name"}); len(row) != 0 {
		t.Errorf("GetRowAt(-1, ...) = %v, want empty", row)
	}
	if row := target.GetRowAt(5, []string{"name"}); len(row) != 0 {
		t.Errorf("GetRowAt(5, ...) = %v, want empty", row)
	}
}
