package step

import "github.com/spf13/cobra"

// Steps is a collection of Step
type Steps []Step

// GetHeaders gets the header for a table
//
// implements common.Tableables
func (steps Steps) GetHeaders(cmd *cobra.Command) []string {
	return Step{}.GetHeaders(cmd)
}

// GetRowAt gets the row for a table
//
// implements common.Tableables
func (steps Steps) GetRowAt(index int, headers []string) []string {
	if index < 0 || index >= len(steps) {
		return []string{}
	}
	return steps[index].GetRow(headers)
}

// Size gets the number of elements
//
// implements common.Tableables
func (steps Steps) Size() int {
	return len(steps)
}
