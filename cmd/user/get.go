package user

import (
	"fmt"

	"github.com/gildas/bitbucket-cli/cmd/common"
	"github.com/gildas/bitbucket-cli/cmd/profile"
	"github.com/gildas/go-flags"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var getCmd = &cobra.Command{
	Use:     "get",
	Aliases: []string{"show", "info", "display"},
	Short:   "get a user",
	Args:    cobra.ExactArgs(1),
	RunE:    getProcess,
}

var getOptions struct {
	Columns *flags.EnumSliceFlag
}

func init() {
	Command.AddCommand(getCmd)

	getOptions.Columns = flags.NewEnumSliceFlag(columns.Columns()...)
	getCmd.Flags().Var(getOptions.Columns, "columns", "Comma-separated list of columns to display")
	_ = getCmd.RegisterFlagCompletionFunc(getOptions.Columns.CompletionFunc("columns"))
}

func getProcess(cmd *cobra.Command, args []string) (err error) {
	profile, err := profile.GetProfileFromCommand(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get profile: %w", err)
	}

	lgr.Printf("[DEBUG] displaying user %s", args[0])
	if !common.WhatIf(cmd, "Showing user "+args[0]) {
		return nil
	}

	user, err := GetUser(cmd.Context(), cmd, args[0])
	if err != nil {
		return err
	}
	lgr.Printf("[DEBUG] user %s retrieved", args[0])
	if err := profile.Print(cmd.Context(), cmd, user); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}
