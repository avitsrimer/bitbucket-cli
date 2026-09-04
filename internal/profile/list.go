package profile

import (
	"errors"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
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
		common.Sort(Profiles, columns.SortBy(sortValue))
	}
	// an explicit -o/--output json or yaml on the command line (see explicitJSONOrYAMLOutput) is
	// the single opt-in for reading a stored secret back, and it gates both halves of that: the
	// vault fetch here, and the masking of the printed payload below (maskSecrets). A profile
	// merely CONFIGURED with outputFormat: json/yaml, or a BB_OUTPUT_FORMAT naming one, therefore
	// fetches nothing and prints secretMask for whatever secret is already in memory -- a bare
	// `bb profile list` renders no credential, whichever route picked its output format.
	loadSecrets := explicitJSONOrYAMLOutput(cmd)
	Profiles = common.Map(Profiles, func(profile *Profile) *Profile {
		_ = profile.Validate()
		if loadSecrets {
			_ = profile.LoadSecrets(ctx)
		}
		return profile
	})
	if len(Profiles) == 1 {
		Profiles[0].Default = true
	}
	// the masked copies go into a fresh slice of fresh pointers, built only after the Default
	// adjustment above: Profiles is the same package-level slice saveProfilesConfig marshals to
	// the config file, so a secretMask reachable from it would overwrite a real credential on the
	// next save.
	payload := any(Profiles)
	if profile.maskSecrets(cmd) {
		masked := make(profiles, len(Profiles))
		for i, listed := range Profiles {
			displayed := listed.forDisplay()
			masked[i] = &displayed
		}
		payload = masked
	}
	return profile.Print(ctx, cmd, payload)
}
