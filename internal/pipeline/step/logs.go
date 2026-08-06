package step

import (
	"github.com/spf13/cobra"
)

var logsCmd = &cobra.Command{
	Use:               "logs [flags] <pipeline> <pipeline-step-uuid-or-name>",
	Aliases:           []string{"log"},
	Short:             "display the logs of a pipeline step",
	Args:              cobra.ExactArgs(2),
	ValidArgsFunction: pipelineAndStepValidArgs,
	RunE:              logsProcess,
}

func init() {
	Command.AddCommand(logsCmd)
}

func logsProcess(cmd *cobra.Command, args []string) error {
	return rawStepOutput(cmd, args[0], args[1], "logs", "log")
}
