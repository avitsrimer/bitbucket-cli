package profile

import (
	"errors"
	"fmt"
	"os"

	"github.com/gildas/bitbucket-cli/cmd/common"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var getCmd = &cobra.Command{
	Use:               "get [flags] <profile-name>",
	Aliases:           []string{"show", "info", "display"},
	Short:             "get a profile by its <profile-name>.",
	ValidArgsFunction: ValidProfileNames,
	PreRunE:           disableUnsupportedFlags,
	RunE:              getProcess,
}

var getOptions struct {
	Current bool
	Columns *common.EnumSliceFlag
}

func init() {
	Command.AddCommand(getCmd)
	getOptions.Columns = common.NewEnumSliceFlag(columns.Columns()...)

	getCmd.Flags().BoolVar(&getOptions.Current, "current", false, "Get the current profile")
	getCmd.Flags().Var(getOptions.Columns, "columns", "Comma-separated list of columns to display")
	_ = getCmd.RegisterFlagCompletionFunc(getOptions.Columns.CompletionFunc("columns"))
	getCmd.SetHelpFunc(hideUnsupportedFlags)
}

func getProcess(cmd *cobra.Command, args []string) (err error) {
	ctx := cmd.Context()

	_, err = GetProfileFromCommand(ctx, cmd)
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

	if getOptions.Current {
		lgr.Printf("[DEBUG] displaying current profile")
		if Current == nil {
			if cmd.Flag("stop-on-error").Value.String() == "true" {
				return errors.New("there is no profile configured")
			}
			common.Verbose(cmd, "No profile is currently configured")
			return nil
		}
		return Current.Print(ctx, cmd, Current)
	}

	if len(args) == 0 {
		return errors.New("argument profile is missing")
	}

	lgr.Printf("[DEBUG] displaying profile %s (valid names: %v)", args[0], Profiles.Names())
	if !common.WhatIf(cmd, "Showing profile "+args[0]) {
		return nil
	}

	profile, found := Profiles.Find(args[0])
	if !found {
		return fmt.Errorf("profile %s not found", args[0])
	}
	if err := profile.Validate(); err != nil {
		if cmd.Flag("stop-on-error").Value.String() == "true" {
			return err
		}
		if cmd.Flag("warn-on-error").Value.String() == "true" {
			lgr.Printf("[WARN] profile %s is not valid: %v", profile.Name, err)
			fmt.Fprintln(os.Stderr, "Profile", profile.Name, "is not valid:", err)
		}
	}
	_ = profile.LoadSecrets(ctx)
	if len(Profiles) == 1 {
		profile.Default = true
	}

	return profile.Print(ctx, cmd, profile)
}
