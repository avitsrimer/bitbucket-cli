package step

import (
	"github.com/spf13/cobra"
)

var logsCmd = &cobra.Command{
	Use:               "logs [flags] <pipeline-step-uuid-or-name>",
	Aliases:           []string{"log"},
	Short:             "display the logs of a pipeline step",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: stepValidArgs,
	RunE:              logsProcess,
}

func init() {
	Command.AddCommand(logsCmd)

	registerPipelineFlag(logsCmd, "Pipeline the step belongs to")
}

func logsProcess(cmd *cobra.Command, args []string) error {
	return rawStepOutput(cmd, args[0], "logs", "log")
}
