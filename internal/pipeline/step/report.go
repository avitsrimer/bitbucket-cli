package step

import (
	"github.com/spf13/cobra"
)

var reportCmd = &cobra.Command{
	Use:               "report [flags] <pipeline> <pipeline-step-uuid-or-name>",
	Short:             "display the test report of a pipeline step",
	Args:              cobra.ExactArgs(2),
	ValidArgsFunction: pipelineAndStepValidArgs,
	RunE:              reportProcess,
}

func init() {
	Command.AddCommand(reportCmd)
}

func reportProcess(cmd *cobra.Command, args []string) error {
	return rawStepOutput(cmd, args[0], args[1], "test report", "test_reports")
}
