package common

import (
	"fmt"

	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

// Verbose prints a message if the verbose flag is set
func Verbose(cmd *cobra.Command, format string, args ...any) {
	lgr.Printf("[DEBUG] "+format, args...)
	if cmd.Flag("verbose").Changed {
		fmt.Printf(format, args...)
		fmt.Println()
	}
}
