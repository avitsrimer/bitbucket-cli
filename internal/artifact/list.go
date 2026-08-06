package artifact

import (
	"fmt"
	"net/url"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/gildas/go-core"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "list the artifacts of the current repository",
	Args:  cobra.NoArgs,
	RunE:  listProcess,
}

var listOptions struct {
	Query   string
	Columns *common.EnumSliceFlag
	SortBy  *common.EnumFlag
}

func init() {
	Command.AddCommand(listCmd)

	listOptions.Columns = common.NewEnumSliceFlagWithAllAllowed(columns.Columns()...)
	listOptions.SortBy = common.NewEnumFlag(columns.Sorters()...)
	listCmd.Flags().StringVar(&listOptions.Query, "query", "", "Query string to filter artifacts")
	listCmd.Flags().Var(listOptions.Columns, "columns", "Comma-separated list of columns to display")
	listCmd.Flags().Var(listOptions.SortBy, "sort", "Column to sort by")
	listCmd.Flags().Int("page-length", 0, "Number of items per page to retrieve from Bitbucket. Default is the profile's default page length")
	listCmd.Flags().Int("limit", 0, "Maximum total number of artifacts to retrieve. Default is to retrieve all of them")
	_ = listCmd.RegisterFlagCompletionFunc(listOptions.Columns.CompletionFunc("columns"))
	_ = listCmd.RegisterFlagCompletionFunc(listOptions.SortBy.CompletionFunc("sort"))
}

func listProcess(cmd *cobra.Command, args []string) error {
	lgr.Printf("[DEBUG] listing artifacts")
	if !common.WhatIf(cmd, "Showing artifacts") {
		return nil
	}

	repo, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get repository: %w", err)
	}

	uriPath := repo.GetPath("downloads")
	if query := queryFlagValue(cmd); query != "" {
		uriPath += "?q=" + url.QueryEscape(query)
	}

	artifacts, err := profile.GetAll[Artifact](cmd.Context(), cmd, uriPath)
	if err != nil {
		return fmt.Errorf("cannot get artifacts: %w", err)
	}
	if len(artifacts) == 0 {
		fmt.Println("No artifact found")
		return nil
	}
	if sortFlag := cmd.Flag("sort"); sortFlag != nil && sortFlag.Changed {
		core.Sort(artifacts, columns.SortBy(listOptions.SortBy.Value))
	}
	if err := profile.Current.Print(cmd.Context(), cmd, Artifacts(artifacts)); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}

// queryFlagValue reads cmd's own --query flag directly (rather than the package-level
// listOptions.Query, which is only ever populated on the real listCmd instance), so listProcess
// behaves the same whether cmd is listCmd itself or a standalone test command carrying its own
// --query flag.
func queryFlagValue(cmd *cobra.Command) string {
	flag := cmd.Flag("query")
	if flag == nil || !flag.Changed {
		return ""
	}
	value, err := cmd.Flags().GetString("query")
	if err != nil {
		return ""
	}
	return value
}
