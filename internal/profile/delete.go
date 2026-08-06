package profile

import (
	"errors"
	"strings"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
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
	if errors.Is(err, ErrNoProfiles) || len(Profiles) == 0 {
		if cmd.Flag("stop-on-error").Value.String() == "true" {
			return errors.New("no profiles found")
		}
		common.Verbose(cmd, "Profiles list is empty, nothing to delete")
		return nil
	}
	if err != nil {
		return err
	}

	// Snapshot which profiles' vault credentials will need purging before they are removed from
	// Profiles below (deleteProfileCredentials looks them up by name via Profiles.Find, which no
	// longer finds them once deleted); the vault purge itself only runs after the config has been
	// saved successfully, so a failed save leaves the profile -- and its credential -- intact
	// instead of destroying the credential for a profile that was never actually deleted.
	var names []string
	if deleteOptions.All {
		names = Profiles.Names()
	} else {
		names = args
	}
	toPurge := snapshotCredentialOwners(names)

	if deleteOptions.All {
		lgr.Printf("[DEBUG] deleting all profiles")
		if common.WhatIf(cmd, "Deleting all profiles") {
			deleted = Profiles.Delete(names...)
		}
	} else if common.WhatIf(cmd, "Deleting profiles %s", strings.Join(args, ", ")) {
		deleted = Profiles.Delete(args...)
	}
	lgr.Printf("[DEBUG] deleted %d profiles", deleted)
	if deleted == 0 || cmd.Flag("dry-run").Changed {
		return nil
	}
	if err := saveProfilesConfig(); err != nil {
		return err
	}
	purgeCredentials(toPurge)
	return nil
}

// credentialOwner names, for one profile about to be deleted, the vault (service, username) pair
// its credential -- if any -- is stored under.
type credentialOwner struct {
	vaultKey string
	username string
}

// snapshotCredentialOwners captures the vault (service, username) each named profile's credential
// is stored under, before the profile is removed from Profiles: Profiles.Find can no longer locate
// it afterward, so this must run first even though the vault entries themselves are only purged
// once the config save that actually removes the profiles has succeeded (see purgeCredentials).
func snapshotCredentialOwners(names []string) []credentialOwner {
	owners := make([]credentialOwner, 0, len(names))
	for _, profileName := range names {
		profile, found := Profiles.Find(profileName)
		if !found {
			continue
		}
		switch {
		case profile.ClientID != "":
			owners = append(owners, credentialOwner{vaultKey: profile.VaultKey, username: profile.ClientID})
		case profile.User != "":
			owners = append(owners, credentialOwner{vaultKey: profile.VaultKey, username: profile.User})
		case profile.Name != "":
			owners = append(owners, credentialOwner{vaultKey: profile.VaultKey, username: profile.Name})
		}
	}
	return owners
}

// purgeCredentials deletes each snapshotted credential from the vault. Called only after the
// profiles that owned them have actually been removed and that removal has been saved to disk, so
// a save failure never leaves a profile stripped of its credential while itself surviving.
func purgeCredentials(owners []credentialOwner) {
	for _, owner := range owners {
		lgr.Printf("[DEBUG] deleting credential for %s from the %s vault", owner.username, owner.vaultKey)
		_ = (Profile{}).DeleteCredentialFromVault(owner.vaultKey, owner.username)
	}
}
