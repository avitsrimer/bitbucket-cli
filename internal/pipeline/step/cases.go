package step

import (
	"github.com/spf13/cobra"
)

var casesCmd = &cobra.Command{
	Use:               "cases [flags] <pipeline> <pipeline-step-uuid-or-name>",
	Short:             "list the test cases of a pipeline step",
	Args:              cobra.ExactArgs(2),
	ValidArgsFunction: pipelineAndStepValidArgs,
	RunE:              casesProcess,
}

func init() {
	Command.AddCommand(casesCmd)
}

func casesProcess(cmd *cobra.Command, args []string) error {
	return rawStepOutput(cmd, args[0], args[1], "test cases", "test_reports", "test_cases")
}
