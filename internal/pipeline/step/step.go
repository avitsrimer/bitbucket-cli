// Package step implements the "bb pipeline step" command tree: get, list, logs, report, and cases.
package step

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/gildas/go-core"
	"github.com/spf13/cobra"
)

// Step represents a single step of a pipeline run.
type Step struct {
	ID               common.UUID       `json:"uuid"`
	Name             string            `json:"name"`
	RunNumber        uint64            `json:"run_number"`
	Pipeline         PipelineReference `json:"pipeline"`
	State            StepState         `json:"state"`
	Image            StepImage         `json:"image"`
	SetupCommands    []StepCommand     `json:"setup_commands,omitempty"`
	ScriptCommands   []StepCommand     `json:"script_commands,omitempty"`
	TeardownCommands []StepCommand     `json:"teardown_commands,omitempty"`
	MaxTime          time.Duration     `json:"maxTime"`
	StartedOn        time.Time         `json:"started_on"`
	CompletedOn      time.Time         `json:"completed_on"`
	Duration         time.Duration     `json:"duration_in_seconds"`
}

// PipelineReference is the minimal shape of the "pipeline" object embedded in a Step's JSON
// payload: only the type and uuid BitBucket sends, enough to identify which pipeline a step run
// belongs to.
type PipelineReference struct {
	Type string      `json:"type"`
	ID   common.UUID `json:"uuid"`
}

// StepImage is the container image a step ran in.
type StepImage struct {
	Name string `json:"name"`
}

// StepCommand is a single setup/script/teardown command executed by a step.
type StepCommand struct {
	Type    string `json:"commandType"`
	Name    string `json:"name"`
	Command string `json:"command,omitempty"`
}

// StepResult is the outcome of a completed step's state (e.g. SUCCESSFUL, FAILED).
type StepResult struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

// String returns the name of the result.
//
// implements fmt.Stringer
func (result StepResult) String() string {
	return result.Name
}

// StepState represents the state of a pipeline step.
type StepState struct {
	Type   string      `json:"type"`
	Name   string      `json:"name"`
	Result *StepResult `json:"result,omitempty"`
}

// String returns a human-readable representation of the state, folding in the result when
// present.
//
// implements fmt.Stringer
func (state StepState) String() string {
	if state.Result != nil {
		return fmt.Sprintf("%s (%s)", state.Name, state.Result)
	}
	return state.Name
}

// Command represents this folder's command
var Command = &cobra.Command{
	Use:     "step",
	Aliases: []string{"steps"},
	Short:   "Manage pipeline steps",
	Run:     common.SubcommandRequired("Step"),
}

// columns is the single source of truth for the columns this command supports: GetHeaders'
// default subset, GetRow's switch, and the --columns/--sort completion all read from here.
var columns = common.Columns[Step]{
	{Name: "id", DefaultSorter: true, Compare: func(a, b Step) bool {
		return strings.ToLower(a.ID.String()) < strings.ToLower(b.ID.String())
	}},
	{Name: "name", DefaultSorter: false, Compare: func(a, b Step) bool {
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	}},
	{Name: "state", DefaultSorter: false, Compare: func(a, b Step) bool {
		return strings.ToLower(a.State.Name) < strings.ToLower(b.State.Name)
	}},
	{Name: "image", DefaultSorter: false, Compare: func(a, b Step) bool {
		return strings.ToLower(a.Image.Name) < strings.ToLower(b.Image.Name)
	}},
	{Name: "duration", DefaultSorter: false, Compare: func(a, b Step) bool {
		return a.Duration < b.Duration
	}},
	{Name: "started_on", DefaultSorter: false, Compare: func(a, b Step) bool {
		return a.StartedOn.Before(b.StartedOn)
	}},
	{Name: "completed_on", DefaultSorter: false, Compare: func(a, b Step) bool {
		if a.CompletedOn.IsZero() && b.CompletedOn.IsZero() {
			return false
		}
		if a.CompletedOn.IsZero() {
			return true
		}
		if b.CompletedOn.IsZero() {
			return false
		}
		return a.CompletedOn.Before(b.CompletedOn)
	}},
	{Name: "run_number", DefaultSorter: false, Compare: func(a, b Step) bool {
		return a.RunNumber < b.RunNumber
	}},
	{Name: "max_time", DefaultSorter: false, Compare: func(a, b Step) bool {
		return a.MaxTime < b.MaxTime
	}},
}

// GetType gets the type of the struct
//
// implements core.TypeCarrier
func (step Step) GetType() string {
	return "pipeline_step"
}

// GetHeaders gets the headers for a table
//
// implements common.Tableable
func (step Step) GetHeaders(cmd *cobra.Command) []string {
	if cmd != nil && cmd.Flag("columns") != nil && cmd.Flag("columns").Changed {
		if values, err := cmd.Flags().GetStringSlice("columns"); err == nil {
			return core.Map(values, func(column string) string { return strings.ReplaceAll(column, "_", " ") })
		}
	}
	return []string{"ID", "Name", "State", "Duration", "Image"}
}

// GetRow gets the row for a table
//
// implements common.Tableable
func (step Step) GetRow(headers []string) []string {
	row := make([]string, 0, len(headers))

	for _, header := range headers {
		switch common.NormalizeColumnKey(header) {
		case "id":
			row = append(row, step.ID.String())
		case "name":
			row = append(row, step.Name)
		case "state":
			row = append(row, step.State.String())
		case "image":
			row = append(row, step.Image.Name)
		case "duration":
			row = append(row, step.Duration.String())
		case "started_on":
			row = append(row, step.startedOnCell())
		case "completed_on":
			row = append(row, step.completedOnCell())
		case "run_number":
			row = append(row, strconv.FormatUint(step.RunNumber, 10))
		case "max_time":
			row = append(row, step.MaxTime.String())
		default:
			row = append(row, " ")
		}
	}
	return row
}

// startedOnCell returns StartedOn formatted with common.TableTimeFormat, or " " when it is zero
// (a step that has not started yet has no start time).
func (step Step) startedOnCell() string {
	if step.StartedOn.IsZero() {
		return " "
	}
	return step.StartedOn.Format(common.TableTimeFormat)
}

// completedOnCell returns CompletedOn formatted with common.TableTimeFormat, or " " when it is
// zero (a step still running has no completion time).
func (step Step) completedOnCell() string {
	if step.CompletedOn.IsZero() {
		return " "
	}
	return step.CompletedOn.Format(common.TableTimeFormat)
}

// String gets a string representation of this step
//
// implements fmt.Stringer
func (step Step) String() string {
	if step.Name != "" {
		return step.Name
	}
	return step.ID.String()
}

// MarshalJSON implements the json.Marshaler interface.
func (step Step) MarshalJSON() (data []byte, err error) {
	type surrogate Step

	var startedOn string
	if !step.StartedOn.IsZero() {
		startedOn = step.StartedOn.Format(common.JSONTimeFormat)
	}
	var completedOn string
	if !step.CompletedOn.IsZero() {
		completedOn = step.CompletedOn.Format(common.JSONTimeFormat)
	}

	data, err = json.Marshal(struct {
		Type string `json:"type"`
		surrogate
		StartedOn         string `json:"started_on,omitempty"`
		CompletedOn       string `json:"completed_on,omitempty"`
		MaxTime           uint64 `json:"maxTime"`
		DurationInSeconds uint64 `json:"duration_in_seconds"`
	}{
		Type:              step.GetType(),
		surrogate:         surrogate(step),
		StartedOn:         startedOn,
		CompletedOn:       completedOn,
		MaxTime:           uint64(step.MaxTime.Seconds()),
		DurationInSeconds: uint64(step.Duration.Seconds()),
	})
	if err != nil {
		return nil, fmt.Errorf("cannot marshal step to json: %w", err)
	}
	return data, nil
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (step *Step) UnmarshalJSON(data []byte) (err error) {
	type surrogate Step
	var inner struct {
		Type string `json:"type"`
		surrogate
		MaxTime           uint64 `json:"maxTime"`
		DurationInSeconds uint64 `json:"duration_in_seconds"`
	}
	if err = json.Unmarshal(data, &inner); err != nil {
		return fmt.Errorf("cannot unmarshal step: %w", err)
	}
	if inner.Type != "" && inner.Type != (Step{}).GetType() {
		return fmt.Errorf("cannot unmarshal step: invalid type %s, expected %s", inner.Type, (Step{}).GetType())
	}
	*step = Step(inner.surrogate)
	step.MaxTime = time.Duration(inner.MaxTime) * time.Second            //nolint:gosec // maxTime is a step time budget in seconds from BitBucket, far below the ~292 billion year range where this conversion could overflow
	step.Duration = time.Duration(inner.DurationInSeconds) * time.Second //nolint:gosec // duration_in_seconds is a step runtime in seconds from BitBucket, far below the ~292 billion year range where this conversion could overflow
	return nil
}
