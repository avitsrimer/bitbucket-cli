package pipeline

import (
	"testing"
	"time"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/user"
)

func TestPipelineGetHeadersDefault(t *testing.T) {
	want := []string{"Build Number", "State", "Branch", "Creator", "Duration", "Created On"}
	got := Pipeline{}.GetHeaders(nil)
	if len(got) != len(want) {
		t.Fatalf("GetHeaders() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("GetHeaders()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestPipelineGetRowCoversEveryColumn iterates columns.Columns() (the single source of truth, not
// a hand-written header list) and requires every declared column to produce its real value instead
// of falling through to GetRow's default " " arm. Each column name in the table is unique: a
// duplicate entry would not change this test's outcome, but would fail `go vet`/tests elsewhere if
// columns.Columns() ever grew a repeat.
func TestPipelineGetRowCoversEveryColumn(t *testing.T) {
	id, err := common.ParseUUID("{a1b2c3d4-e5f6-7890-abcd-ef1234567890}")
	if err != nil {
		t.Fatalf("cannot parse fixture uuid: %v", err)
	}
	target := Pipeline{
		ID:          id,
		BuildNumber: 42,
		State:       PipelineState{Name: "COMPLETED"},
		Creator:     user.User{Name: "Jane Doe"},
		Target:      Target{RefName: "main"},
		Duration:    330 * time.Second,
		CreatedOn:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		CompletedOn: time.Date(2026, 1, 1, 0, 5, 0, 0, time.UTC),
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

func TestPipelineGetRowUnknownColumnFillsPlaceholder(t *testing.T) {
	target := Pipeline{BuildNumber: 42}

	row := target.GetRow([]string{"build_number", "not-a-real-column"})
	if len(row) != 2 {
		t.Fatalf("GetRow() = %v, want exactly one cell per header", row)
	}
	if row[0] != "42" || row[1] != " " {
		t.Errorf("GetRow() = %v, want [\"42\", \" \"]", row)
	}
}

func TestPipelineGetRowZeroCompletedOnFillsPlaceholder(t *testing.T) {
	target := Pipeline{BuildNumber: 42}

	row := target.GetRow([]string{"completed_on"})
	if len(row) != 1 || row[0] != " " {
		t.Errorf("GetRow() = %v, want [\" \"] for a zero CompletedOn", row)
	}
}

func TestPipelineGetRowAcceptsHyphenAndSpaceColumnSpellings(t *testing.T) {
	target := Pipeline{BuildNumber: 7}

	for _, spelling := range []string{"build number", "build-number", "build_number"} {
		row := target.GetRow([]string{spelling})
		if len(row) != 1 || row[0] != "7" {
			t.Errorf("GetRow([%q]) = %v, want [\"7\"]", spelling, row)
		}
	}
}

func TestPipelineString(t *testing.T) {
	p := Pipeline{BuildNumber: 123}
	if got := p.String(); got != "#123" {
		t.Errorf("String() = %q, want %q", got, "#123")
	}
}

func TestPipelinesTableables(t *testing.T) {
	target := Pipelines{
		{BuildNumber: 1},
		{BuildNumber: 2},
	}

	if target.Size() != 2 {
		t.Errorf("Size() = %d, want 2", target.Size())
	}
	if row := target.GetRowAt(0, []string{"build_number"}); len(row) != 1 || row[0] != "1" {
		t.Errorf("GetRowAt(0, ...) = %v, want [\"1\"]", row)
	}
	if row := target.GetRowAt(1, []string{"build_number"}); len(row) != 1 || row[0] != "2" {
		t.Errorf("GetRowAt(1, ...) = %v, want [\"2\"]", row)
	}
	if row := target.GetRowAt(-1, []string{"build_number"}); len(row) != 0 {
		t.Errorf("GetRowAt(-1, ...) = %v, want empty", row)
	}
	if row := target.GetRowAt(5, []string{"build_number"}); len(row) != 0 {
		t.Errorf("GetRowAt(5, ...) = %v, want empty", row)
	}
}

func TestPipelineStateStringNameOnly(t *testing.T) {
	state := PipelineState{Name: "IN_PROGRESS"}
	if got := state.String(); got != "IN_PROGRESS" {
		t.Errorf("String() = %q, want %q", got, "IN_PROGRESS")
	}
}

func TestPipelineStateStringWithStage(t *testing.T) {
	state := PipelineState{Name: "IN_PROGRESS", Stage: &PipelineStage{Name: "building"}}
	if got := state.String(); got != "IN_PROGRESS - building" {
		t.Errorf("String() = %q, want %q", got, "IN_PROGRESS - building")
	}
}

func TestPipelineStateStringWithResult(t *testing.T) {
	state := PipelineState{Name: "COMPLETED", Result: &PipelineResult{Name: "SUCCESSFUL"}}
	if got := state.String(); got != "COMPLETED (SUCCESSFUL)" {
		t.Errorf("String() = %q, want %q", got, "COMPLETED (SUCCESSFUL)")
	}
}

func TestPipelineStateStringWithStageAndResult(t *testing.T) {
	state := PipelineState{Name: "COMPLETED", Stage: &PipelineStage{Name: "building"}, Result: &PipelineResult{Name: "SUCCESSFUL"}}
	if got := state.String(); got != "COMPLETED - building (SUCCESSFUL)" {
		t.Errorf("String() = %q, want %q", got, "COMPLETED - building (SUCCESSFUL)")
	}
}
