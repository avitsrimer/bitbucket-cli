package profile

import (
	"github.com/gildas/bitbucket-cli/cmd/common"
	"github.com/gildas/go-errors"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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
		return errors.ArgumentMissing.With("profile")
	}
	if _, err := GetProfileFromCommand(ctx, cmd); err != nil {
		return err
	}

	lgr.Printf("[DEBUG] using profile %s (valid names: %v)", args[0], Profiles.Names())
	profile, found := Profiles.Find(args[0])
	if !found {
		return errors.NotFound.With("profile", args[0])
	}
	if common.WhatIf(cmd, "Using profile %s as default", args[0]) {
		Profiles.SetCurrent(profile.Name)
		viper.Set("profiles", Profiles)
		return viper.WriteConfig()
	}
	return nil
}
