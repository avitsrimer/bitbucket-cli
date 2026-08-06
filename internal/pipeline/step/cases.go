package step

import (
	"github.com/spf13/cobra"
)

var casesCmd = &cobra.Command{
	Use:               "cases [flags] <pipeline-step-uuid-or-name>",
	Short:             "list the test cases of a pipeline step",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: stepValidArgs,
	RunE:              casesProcess,
}

func init() {
	Command.AddCommand(casesCmd)

	registerPipelineFlag(casesCmd, "Pipeline the step belongs to")
}

func casesProcess(cmd *cobra.Command, args []string) error {
	return rawStepOutput(cmd, args[0], "test cases", "test_reports", "test_cases")
}
