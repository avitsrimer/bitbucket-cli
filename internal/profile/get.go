package profile

import (
	"errors"
	"fmt"
	"os"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var getCmd = &cobra.Command{
	Use:               "get [flags] [<profile-name>]",
	Aliases:           []string{"show", "info", "display"},
	Short:             "get a profile by its <profile-name>, or the current profile with --current.",
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: ValidProfileNames,
	PreRunE:           disableUnsupportedFlags,
	RunE:              getProcess,
}

var getOptions struct {
	Current bool
}

func init() {
	Command.AddCommand(getCmd)

	getCmd.Flags().BoolVar(&getOptions.Current, "current", false, "Get the current profile")
	common.RegisterColumnsFlag(getCmd, columns)
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
		if !common.WhatIf(cmd, "Showing current profile") {
			return nil
		}
		return Current.Print(ctx, cmd, Current.displayPayload(cmd, Current))
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
	// LoadSecrets is only called when -o/--output json or yaml was given EXPLICITLY on the command
	// line (see explicitJSONOrYAMLOutput's doc comment on the list.go call site for why).
	if explicitJSONOrYAMLOutput(cmd) {
		_ = profile.LoadSecrets(ctx)
	}
	if len(Profiles) == 1 {
		profile.Default = true
	}

	return profile.Print(ctx, cmd, profile.displayPayload(cmd, profile))
}
