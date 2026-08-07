package profile

import (
	"errors"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/gildas/go-core"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:     "list",
	Short:   "list all profiles",
	Args:    cobra.NoArgs,
	PreRunE: disableUnsupportedFlags,
	RunE:    listProcess,
}

func init() {
	Command.AddCommand(listCmd)

	// profiles are read from the local config file, not paged from an API, so this registers
	// only --columns/--sort via the two RegisterListFlags building blocks and skips
	// common.RegisterListFlags itself, which also adds --page-length/--limit -- neither flag
	// would do anything here.
	common.RegisterColumnsFlag(listCmd, columns)
	common.RegisterSortFlag(listCmd, columns)
	listCmd.SetHelpFunc(hideUnsupportedFlags)
}

func listProcess(cmd *cobra.Command, args []string) (err error) {
	ctx := cmd.Context()

	lgr.Printf("[DEBUG] listing all profiles")
	if !common.WhatIf(cmd, "Showing profiles") {
		return nil
	}

	profile, err := GetProfileFromCommand(ctx, cmd)
	if errors.Is(err, ErrNoProfiles) || len(Profiles) == 0 {
		if cmd.Flag("stop-on-error").Value.String() == "true" {
			return errors.New("no profiles found")
		}
		common.Verbose(cmd, "No profiles found")
		return nil
	}
	if err != nil {
		return err
	}
	if sortValue := common.SortFlagValue(cmd); sortValue != "" {
		core.Sort(Profiles, columns.SortBy(sortValue))
	}
	// LoadSecrets is only called when -o/--output json or yaml was given EXPLICITLY on the command
	// line (see explicitJSONOrYAMLOutput): a profile merely CONFIGURED with outputFormat: json/yaml
	// must not, on its own, make a bare `bb profile list` load every profile's secret from the
	// vault and then render it in cleartext (Print picks the profile's own OutputFormat ahead of
	// -o, with no flag and no signal that a secret is about to be shown).
	loadSecrets := explicitJSONOrYAMLOutput(cmd)
	Profiles = core.Map(Profiles, func(profile *Profile) *Profile {
		_ = profile.Validate()
		if loadSecrets {
			_ = profile.LoadSecrets(ctx)
		}
		return profile
	})
	if len(Profiles) == 1 {
		Profiles[0].Default = true
	}
	return profile.Print(ctx, cmd, Profiles)
}
