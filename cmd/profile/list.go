package profile

import (
	"errors"

	"github.com/gildas/bitbucket-cli/cmd/common"
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

var listOptions struct {
	Columns *common.EnumSliceFlag
	SortBy  *common.EnumFlag
}

func init() {
	Command.AddCommand(listCmd)

	listOptions.Columns = common.NewEnumSliceFlagWithAllAllowed(columns.Columns()...)
	listOptions.SortBy = common.NewEnumFlag(columns.Sorters()...)
	listCmd.Flags().Var(listOptions.Columns, "columns", "Comma-separated list of columns to display")
	listCmd.Flags().Var(listOptions.SortBy, "sort", "Column to sort by")
	_ = listCmd.RegisterFlagCompletionFunc(listOptions.Columns.CompletionFunc("columns"))
	_ = listCmd.RegisterFlagCompletionFunc(listOptions.SortBy.CompletionFunc("sort"))
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
	core.Sort(Profiles, columns.SortBy(listOptions.SortBy.Value))
	Profiles = core.Map(Profiles, func(profile *Profile) *Profile {
		_ = profile.Validate()
		_ = profile.LoadSecrets(ctx)
		return profile
	})
	if len(Profiles) == 1 {
		Profiles[0].Default = true
	}
	return profile.Print(ctx, cmd, Profiles)
}
