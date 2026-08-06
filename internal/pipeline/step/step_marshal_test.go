package step

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

// loadTestData reads a fixture from the repo-root testdata directory (three levels up from
// internal/pipeline/step).
func loadTestData(t *testing.T, filename string) []byte {
	t.Helper()
	data, err := os.ReadFile("../../../testdata/" + filename)
	if err != nil {
		t.Fatalf("cannot read testdata/%s: %v", filename, err)
	}
	return data
}

// TestStepUnmarshalFixture proves the fixture unmarshals into every field Step models.
func TestStepUnmarshalFixture(t *testing.T) {
	var target Step
	if err := json.Unmarshal(loadTestData(t, "pipeline-step.json"), &target); err != nil {
		t.Fatalf("cannot unmarshal pipeline-step.json: %v", err)
	}

	if target.ID.String() != "{cec5beef-dead-deed-bead-5ae1bedd9ada}" {
		t.Errorf("ID = %s, want {cec5beef-dead-deed-bead-5ae1bedd9ada}", target.ID.String())
	}
	if target.Name != "Test and Build" {
		t.Errorf("Name = %q, want %q", target.Name, "Test and Build")
	}
	if target.RunNumber != 1 {
		t.Errorf("RunNumber = %d, want 1", target.RunNumber)
	}
	if target.Pipeline.ID.String() != "{3edaa916-baad-beef-dead-28846deafec1}" {
		t.Errorf("Pipeline.ID = %s, want {3edaa916-baad-beef-dead-28846deafec1}", target.Pipeline.ID.String())
	}
	if target.Image.Name != "golang:1.25" {
		t.Errorf("Image.Name = %q, want %q", target.Image.Name, "golang:1.25")
	}
	if target.State.Name != "COMPLETED" || target.State.Result == nil || target.State.Result.Name != "FAILED" {
		t.Errorf("State = %+v, want COMPLETED/FAILED", target.State)
	}
	if target.Duration != 19*time.Second {
		t.Errorf("Duration = %s, want 19s", target.Duration)
	}
	if target.MaxTime != 120*time.Second {
		t.Errorf("MaxTime = %s, want 120s", target.MaxTime)
	}
	if len(target.SetupCommands) != 12 {
		t.Errorf("len(SetupCommands) = %d, want 12", len(target.SetupCommands))
	}
	if len(target.ScriptCommands) != 4 {
		t.Errorf("len(ScriptCommands) = %d, want 4", len(target.ScriptCommands))
	}
	if len(target.TeardownCommands) != 2 {
		t.Errorf("len(TeardownCommands) = %d, want 2", len(target.TeardownCommands))
	}

	wantStarted := time.Date(2026, 1, 3, 7, 36, 40, 109572944, time.UTC)
	wantCompleted := time.Date(2026, 1, 3, 7, 36, 59, 423783406, time.UTC)
	if !target.StartedOn.Equal(wantStarted) {
		t.Errorf("StartedOn = %s, want %s", target.StartedOn, wantStarted)
	}
	if !target.CompletedOn.Equal(wantCompleted) {
		t.Errorf("CompletedOn = %s, want %s", target.CompletedOn, wantCompleted)
	}
}

// TestStepMarshalUnmarshalRoundTrip proves MarshalJSON writes "started_on" (the same field
// UnmarshalJSON reads), not "created_on" (a mismatch that would silently drop StartedOn on a
// second unmarshal, since nothing populates a "created_on" key on the way back out). Marshaling
// the fixture-derived Step and unmarshaling the result again must reproduce the exact same
// StartedOn/CompletedOn/Duration/MaxTime, proving the value survives a full round trip rather than
// being renamed into a field nothing reads back.
func TestStepMarshalUnmarshalRoundTrip(t *testing.T) {
	var original Step
	if err := json.Unmarshal(loadTestData(t, "pipeline-step.json"), &original); err != nil {
		t.Fatalf("cannot unmarshal pipeline-step.json: %v", err)
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("cannot marshal step: %v", err)
	}

	// the marshaled JSON must carry "started_on", never "created_on", since nothing in Step ever
	// reads a "created_on" key back on unmarshal.
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("cannot unmarshal marshaled step into a map: %v", err)
	}
	if _, present := raw["created_on"]; present {
		t.Errorf("marshaled step carries a \"created_on\" key %v, want it absent", raw["created_on"])
	}
	if _, present := raw["started_on"]; !present {
		t.Error("marshaled step is missing \"started_on\"")
	}

	var roundTripped Step
	if err := json.Unmarshal(data, &roundTripped); err != nil {
		t.Fatalf("cannot unmarshal marshaled step: %v", err)
	}

	if !roundTripped.StartedOn.Equal(original.StartedOn) {
		t.Errorf("round-tripped StartedOn = %s, want %s", roundTripped.StartedOn, original.StartedOn)
	}
	if !roundTripped.CompletedOn.Equal(original.CompletedOn) {
		t.Errorf("round-tripped CompletedOn = %s, want %s", roundTripped.CompletedOn, original.CompletedOn)
	}
	if roundTripped.Duration != original.Duration {
		t.Errorf("round-tripped Duration = %s, want %s", roundTripped.Duration, original.Duration)
	}
	if roundTripped.MaxTime != original.MaxTime {
		t.Errorf("round-tripped MaxTime = %s, want %s", roundTripped.MaxTime, original.MaxTime)
	}
	if roundTripped.ID != original.ID || roundTripped.Name != original.Name {
		t.Errorf("round-tripped step = %+v, want ID/Name to match original %+v", roundTripped, original)
	}
}

// TestStepMarshalOmitsCompletedOnWhenZero proves a still-running step (zero CompletedOn) omits the
// key entirely on marshal, rather than emitting the zero time.Time's "0001-01-01T00:00:00Z".
func TestStepMarshalOmitsCompletedOnWhenZero(t *testing.T) {
	target := Step{Name: "building", StartedOn: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}

	data, err := json.Marshal(target)
	if err != nil {
		t.Fatalf("cannot marshal step: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("cannot unmarshal marshaled step: %v", err)
	}
	if _, present := raw["completed_on"]; present {
		t.Errorf("marshaled step carries a \"completed_on\" key %v for a zero CompletedOn, want it absent", raw["completed_on"])
	}
}

// TestStepMarshalOmitsStartedOnWhenZero proves a not-yet-started step (zero StartedOn, e.g. a
// PENDING step) omits the key entirely on marshal too, rather than emitting the zero time.Time's
// "0001-01-01T00:00:00Z" -- the same defect CompletedOn was already guarded against, but StartedOn
// was not.
func TestStepMarshalOmitsStartedOnWhenZero(t *testing.T) {
	target := Step{Name: "pending"}

	data, err := json.Marshal(target)
	if err != nil {
		t.Fatalf("cannot marshal step: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("cannot unmarshal marshaled step: %v", err)
	}
	if _, present := raw["started_on"]; present {
		t.Errorf("marshaled step carries a \"started_on\" key %v for a zero StartedOn, want it absent", raw["started_on"])
	}
}

// TestStepUnmarshalRejectsWrongType proves UnmarshalJSON validates the "type" discriminator.
func TestStepUnmarshalRejectsWrongType(t *testing.T) {
	var target Step
	err := json.Unmarshal([]byte(`{"type":"not_a_step","uuid":"{00000000-0000-0000-0000-000000000000}"}`), &target)
	if err == nil {
		t.Fatal("Unmarshal() expected an error for a mismatched type, got nil")
	}
}

func TestStepResultAndStateString(t *testing.T) {
	if got := (StepResult{Name: "FAILED"}).String(); got != "FAILED" {
		t.Errorf("StepResult.String() = %q, want %q", got, "FAILED")
	}
	if got := (StepState{Name: "IN_PROGRESS"}).String(); got != "IN_PROGRESS" {
		t.Errorf("StepState.String() = %q, want %q", got, "IN_PROGRESS")
	}
	if got := (StepState{Name: "COMPLETED", Result: &StepResult{Name: "FAILED"}}).String(); got != "COMPLETED (FAILED)" {
		t.Errorf("StepState.String() = %q, want %q", got, "COMPLETED (FAILED)")
	}
}
