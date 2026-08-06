package common

import (
	"fmt"

	"github.com/spf13/cobra"
)

// SubcommandRequired returns a cobra.Command.Run function that prints "<noun> requires a
// subcommand:" followed by the names of cmd's direct subcommands. Use it for command groups that
// only host subcommands and have no behavior of their own when invoked bare.
func SubcommandRequired(noun string) func(cmd *cobra.Command, args []string) {
	return func(cmd *cobra.Command, args []string) {
		fmt.Printf("%s requires a subcommand:\n", noun)
		for _, command := range cmd.Commands() {
			fmt.Println(command.Name())
		}
	}
}
