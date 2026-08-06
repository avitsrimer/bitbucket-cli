package pipeline

import "fmt"

// PipelineState represents the state of a pipeline
type PipelineState struct {
	Type   string          `json:"type"`
	Name   string          `json:"name"`
	Stage  *PipelineStage  `json:"stage,omitempty"`
	Result *PipelineResult `json:"result,omitempty"`
}

// String returns a human-readable representation of the state, folding in the current stage
// and/or result when either is present.
//
// implements fmt.Stringer
func (state PipelineState) String() string {
	if state.Result != nil {
		if state.Stage != nil {
			return fmt.Sprintf("%s - %s (%s)", state.Name, state.Stage, state.Result)
		}
		return fmt.Sprintf("%s (%s)", state.Name, state.Result)
	}
	if state.Stage != nil {
		return fmt.Sprintf("%s - %s", state.Name, state.Stage)
	}
	return state.Name
}
