package step

import (
	"testing"
	"time"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
)

func TestStepGetHeadersDefault(t *testing.T) {
	want := []string{"ID", "Name", "State", "Duration", "Image"}
	got := Step{}.GetHeaders(nil)
	if len(got) != len(want) {
		t.Fatalf("GetHeaders() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("GetHeaders()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestStepGetRowCoversEveryColumn iterates columns.Columns() (the single source of truth, not a
// hand-written header list) and requires every declared column to produce its real value instead
// of falling through to GetRow's default " " arm. This proves the three-way columns/GetRow/
// GetHeaders disagreement from upstream is fixed: "name" is now a real column, and "started_on"/
// "completed_on"/"run_number"/"max_time" (present in upstream's GetRow but absent from its columns
// table) are all declared here too. "logs-command" is gone entirely, along with the feature.
func TestStepGetRowCoversEveryColumn(t *testing.T) {
	id, err := common.ParseUUID("{cec5beef-dead-deed-bead-5ae1bedd9ada}")
	if err != nil {
		t.Fatalf("cannot parse fixture uuid: %v", err)
	}
	target := Step{
		ID:          id,
		Name:        "Test and Build",
		RunNumber:   1,
		State:       StepState{Name: "COMPLETED"},
		Image:       StepImage{Name: "golang:1.25"},
		Duration:    19 * time.Second,
		MaxTime:     120 * time.Second,
		StartedOn:   time.Date(2026, 1, 3, 7, 36, 40, 0, time.UTC),
		CompletedOn: time.Date(2026, 1, 3, 7, 36, 59, 0, time.UTC),
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

func TestStepGetRowUnknownColumnFillsPlaceholder(t *testing.T) {
	target := Step{Name: "build"}

	row := target.GetRow([]string{"name", "not-a-real-column"})
	if len(row) != 2 {
		t.Fatalf("GetRow() = %v, want exactly one cell per header", row)
	}
	if row[0] != "build" || row[1] != " " {
		t.Errorf("GetRow() = %v, want [\"build\", \" \"]", row)
	}
}

// TestStepGetRowZeroTimestampsFillPlaceholder proves a zero StartedOn/CompletedOn (a step that
// has not started, or is still running) renders " ", never the zero time.Time's
// "0001-01-01T00:00:00Z".
func TestStepGetRowZeroTimestampsFillPlaceholder(t *testing.T) {
	target := Step{Name: "build"}

	row := target.GetRow([]string{"started_on", "completed_on"})
	if len(row) != 2 || row[0] != " " || row[1] != " " {
		t.Errorf("GetRow() = %v, want [\" \", \" \"] for zero StartedOn/CompletedOn", row)
	}
}

func TestStepGetRowAcceptsHyphenAndSpaceColumnSpellings(t *testing.T) {
	target := Step{RunNumber: 7}

	for _, spelling := range []string{"run number", "run-number", "run_number"} {
		row := target.GetRow([]string{spelling})
		if len(row) != 1 || row[0] != "7" {
			t.Errorf("GetRow([%q]) = %v, want [\"7\"]", spelling, row)
		}
	}
}

func TestStepStringFallsBackToID(t *testing.T) {
	id, err := common.ParseUUID("{cec5beef-dead-deed-bead-5ae1bedd9ada}")
	if err != nil {
		t.Fatalf("cannot parse uuid: %v", err)
	}
	target := Step{ID: id}
	if got := target.String(); got != id.String() {
		t.Errorf("String() = %q, want %q", got, id.String())
	}
	target.Name = "Test and Build"
	if got := target.String(); got != "Test and Build" {
		t.Errorf("String() = %q, want %q", got, "Test and Build")
	}
}

func TestStepsTableables(t *testing.T) {
	target := Steps{
		{Name: "one"},
		{Name: "two"},
	}

	if target.Size() != 2 {
		t.Errorf("Size() = %d, want 2", target.Size())
	}
	if row := target.GetRowAt(0, []string{"name"}); len(row) != 1 || row[0] != "one" {
		t.Errorf("GetRowAt(0, ...) = %v, want [\"one\"]", row)
	}
	if row := target.GetRowAt(1, []string{"name"}); len(row) != 1 || row[0] != "two" {
		t.Errorf("GetRowAt(1, ...) = %v, want [\"two\"]", row)
	}
	if row := target.GetRowAt(-1, []string{"name"}); len(row) != 0 {
		t.Errorf("GetRowAt(-1, ...) = %v, want empty", row)
	}
	if row := target.GetRowAt(5, []string{"name"}); len(row) != 0 {
		t.Errorf("GetRowAt(5, ...) = %v, want empty", row)
	}
	if headers := target.GetHeaders(nil); len(headers) == 0 {
		t.Error("GetHeaders() = empty, want the default Step headers")
	}
}
