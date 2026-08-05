package profile

import (
	"errors"

	"github.com/gildas/bitbucket-cli/cmd/common"
	"github.com/spf13/cobra"
)

var whichCmd = &cobra.Command{
	Use:     "which",
	Short:   "display the current profile name",
	Args:    cobra.NoArgs,
	PreRunE: disableUnsupportedFlags,
	RunE:    whichProcess,
}

func init() {
	Command.AddCommand(whichCmd)

	whichCmd.SetHelpFunc(hideUnsupportedFlags)
}

func whichProcess(cmd *cobra.Command, args []string) (err error) {
	ctx := cmd.Context()

	profile, err := GetProfileFromCommand(ctx, cmd)
	if errors.Is(err, ErrNoProfiles) || len(Profiles) == 0 {
		if cmd.Flag("stop-on-error").Value.String() == "true" {
			return errors.New("no profiles found")
		}
		common.Verbose(cmd, "No profile is currently configured")
		return nil
	}
	if err != nil {
		return err
	}
	if Current == nil {
		if cmd.Flag("stop-on-error").Value.String() == "true" {
			return errors.New("there is no profile configured")
		}
		common.Verbose(cmd, "No profile is currently configured")
		return nil
	}

	return profile.Print(ctx, cmd, Current)
}
