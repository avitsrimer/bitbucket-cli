package pipeline

// PipelineResult represents the result of a completed pipeline
type PipelineResult struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

// String returns the name of the result.
//
// implements fmt.Stringer
func (result PipelineResult) String() string {
	return result.Name
}
