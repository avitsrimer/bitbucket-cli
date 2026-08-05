package profile

import (
	"strings"

	"github.com/gildas/bitbucket-cli/cmd/common"
	"github.com/gildas/go-errors"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var deleteCmd = &cobra.Command{
	Use:               "delete [flags] <profile-name>",
	Aliases:           []string{"remove", "rm"},
	Short:             "delete a profile by its <profile-name>.",
	Args:              cobra.MinimumNArgs(1),
	ValidArgsFunction: ValidProfileNames,
	PreRunE:           disableUnsupportedFlags,
	RunE:              deleteProcess,
}

var deleteOptions struct {
	All          bool
	StopOnError  bool
	WarnOnError  bool
	IgnoreErrors bool
}

func init() {
	Command.AddCommand(deleteCmd)

	deleteCmd.Flags().BoolVar(&deleteOptions.All, "all", false, "Delete all profiles")
	deleteCmd.Flags().BoolVar(&deleteOptions.StopOnError, "stop-on-error", false, "Stop on error")
	deleteCmd.Flags().BoolVar(&deleteOptions.WarnOnError, "warn-on-error", false, "Warn on error")
	deleteCmd.Flags().BoolVar(&deleteOptions.IgnoreErrors, "ignore-errors", false, "Ignore errors")
	deleteCmd.MarkFlagsMutuallyExclusive("stop-on-error", "warn-on-error", "ignore-errors")
	deleteCmd.SetHelpFunc(hideUnsupportedFlags)
}

func deleteProcess(cmd *cobra.Command, args []string) (err error) {
	ctx := cmd.Context()
	var deleted int

	_, err = GetProfileFromCommand(ctx, cmd)
	if errors.Is(err, errors.Empty) || len(Profiles) == 0 {
		if cmd.Flag("stop-on-error").Value.String() == "true" {
			return errors.Errorf("No profiles found")
		}
		common.Verbose(cmd, "Profiles list is empty, nothing to delete")
		return nil
	}
	if err != nil {
		return err
	}

	if deleteOptions.All {
		lgr.Printf("[DEBUG] deleting all profiles")
		if common.WhatIf(cmd, "Deleting all profiles") {
			deleteProfileCredentials(Profiles.Names())
			deleted = Profiles.Delete(Profiles.Names()...)
		}
	} else if common.WhatIf(cmd, "Deleting profiles %s", strings.Join(args, ", ")) {
		deleteProfileCredentials(args)
		deleted = Profiles.Delete(args...)
	}
	lgr.Printf("[DEBUG] deleted %d profiles", deleted)
	if deleted == 0 || cmd.Flag("dry-run").Changed {
		return nil
	}
	viper.Set("profiles", Profiles)
	return viper.WriteConfig()
}

// deleteProfileCredentials deletes the vault credential of each named profile, if any
func deleteProfileCredentials(names []string) {
	for _, profileName := range names {
		profile, found := Profiles.Find(profileName)
		if !found {
			continue
		}
		lgr.Printf("[DEBUG] deleting credential for profile %s", profile.Name)
		switch {
		case profile.ClientID != "":
			_ = profile.DeleteCredentialFromVault(profile.VaultKey, profile.ClientID)
			lgr.Printf("[DEBUG] deleted client secret for clientID %s from the %s vault", profile.ClientID, profile.VaultKey)
		case profile.User != "":
			_ = profile.DeleteCredentialFromVault(profile.VaultKey, profile.User)
			lgr.Printf("[DEBUG] deleted user password for user %s from the %s vault", profile.User, profile.VaultKey)
		case profile.Name != "":
			_ = profile.DeleteCredentialFromVault(profile.VaultKey, profile.Name)
			lgr.Printf("[DEBUG] deleted name secret for profile %s from the %s vault", profile.Name, profile.VaultKey)
		}
	}
}
