package step

import (
	"github.com/spf13/cobra"
)

var reportCmd = &cobra.Command{
	Use:               "report [flags] <pipeline-step-uuid-or-name>",
	Short:             "display the test report of a pipeline step",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: stepValidArgs,
	RunE:              reportProcess,
}

func init() {
	Command.AddCommand(reportCmd)

	registerPipelineFlag(reportCmd, "Pipeline the step belongs to")
}

func reportProcess(cmd *cobra.Command, args []string) error {
	return rawStepOutput(cmd, args[0], "test report", "test_reports")
}
