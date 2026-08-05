package profile

import (
	"errors"
	"fmt"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var useCmd = &cobra.Command{
	Use:               "use [flags] <profile-name>",
	Aliases:           []string{"default"},
	Short:             "set the default profile by its <profile-name>.",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: ValidProfileNames,
	PreRunE:           disableUnsupportedFlags,
	RunE:              useProcess,
}

func init() {
	Command.AddCommand(useCmd)

	useCmd.SetHelpFunc(hideUnsupportedFlags)
}

func useProcess(cmd *cobra.Command, args []string) (err error) {
	ctx := cmd.Context()

	if len(args) == 0 {
		return errors.New("argument profile is missing")
	}
	if _, err := GetProfileFromCommand(ctx, cmd); err != nil {
		return err
	}

	lgr.Printf("[DEBUG] using profile %s (valid names: %v)", args[0], Profiles.Names())
	profile, found := Profiles.Find(args[0])
	if !found {
		return fmt.Errorf("profile %s not found", args[0])
	}
	if common.WhatIf(cmd, "Using profile %s as default", args[0]) {
		Profiles.SetCurrent(profile.Name)
		if err := saveProfilesConfig(); err != nil {
			return err
		}
	}
	return nil
}
