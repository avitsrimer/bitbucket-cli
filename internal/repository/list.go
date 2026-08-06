package repository

import (
	"fmt"
	"net/url"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/gildas/go-core"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:     "list",
	Short:   "list the repositories of the current workspace",
	Args:    cobra.NoArgs,
	PreRunE: common.DisableUnsupportedFlags("repository", "repository"),
	RunE:    listProcess,
}

var listOptions struct {
	Role    *common.EnumFlag
	Columns *common.EnumSliceFlag
	SortBy  *common.EnumFlag
}

func init() {
	Command.AddCommand(listCmd)

	listOptions.Role = common.NewEnumFlag("all", "+owner", "admin", "contributor", "member")
	listOptions.Columns, listOptions.SortBy = registerListFlags(listCmd, columns, "repositories")
	listCmd.Flags().Var(listOptions.Role, "role", "Role of the user in the repository (all, owner, admin, contributor, member). Default: owner")
	_ = listCmd.RegisterFlagCompletionFunc(listOptions.Role.CompletionFunc("role"))
	listCmd.SetHelpFunc(common.HideUnsupportedFlags("repository"))
}

func listProcess(cmd *cobra.Command, args []string) error {
	lgr.Printf("[DEBUG] listing repositories with role %s", listOptions.Role.Value)
	if !common.WhatIf(cmd, "Showing repositories with role %s", listOptions.Role.Value) {
		return nil
	}

	query := url.Values{}
	if listOptions.Role.Value != "all" {
		query.Add("role", listOptions.Role.Value)
	}

	repositories, err := GetRepositoriesWithQuery(cmd.Context(), cmd, query)
	if err != nil {
		return fmt.Errorf("failed to retrieve repositories: %w", err)
	}
	if len(repositories) == 0 {
		fmt.Println("No repository found")
		return nil
	}
	if sortFlag := cmd.Flag("sort"); sortFlag != nil && sortFlag.Changed {
		core.Sort(repositories, columns.SortBy(listOptions.SortBy.Value))
	}
	if err := profile.Current.Print(cmd.Context(), cmd, Repositories(repositories)); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}
