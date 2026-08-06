package artifact

import "github.com/spf13/cobra"

// Artifacts is a collection of Artifact, implementing common.Tableables for table/csv/tsv output.
type Artifacts []Artifact

// GetHeaders gets the header for a table
//
// implements common.Tableables
func (artifacts Artifacts) GetHeaders(cmd *cobra.Command) []string {
	return Artifact{}.GetHeaders(cmd)
}

// GetRowAt gets the row for a table
//
// implements common.Tableables
func (artifacts Artifacts) GetRowAt(index int, headers []string) []string {
	if index < 0 || index >= len(artifacts) {
		return []string{}
	}
	return artifacts[index].GetRow(headers)
}

// Size gets the number of elements
//
// implements common.Tableables
func (artifacts Artifacts) Size() int {
	return len(artifacts)
}
