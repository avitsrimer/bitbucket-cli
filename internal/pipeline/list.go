package pipeline

import (
	"fmt"
	"net/url"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "list the pipelines of the current repository",
	Args:  cobra.NoArgs,
	RunE:  listProcess,
}

func init() {
	Command.AddCommand(listCmd)

	common.RegisterListFlags(listCmd, columns, "pipelines")
	// --query has no package-level destination: listProcess reads it directly off cmd below, so a
	// bound variable here would only ever be write-only state.
	listCmd.Flags().String("query", "", "Query string to filter pipelines")
}

func listProcess(cmd *cobra.Command, args []string) error {
	lgr.Printf("[DEBUG] listing pipelines")
	if !common.WhatIf(cmd, "Showing pipelines") {
		return nil
	}

	repo, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get repository: %w", err)
	}

	uriPath := repo.GetPath("pipelines") + "?sort=-created_on"
	if query := common.StringFlagValue(cmd, "query"); query != "" {
		uriPath += "&q=" + url.QueryEscape(query)
	}

	pipelines, err := profile.GetAll[Pipeline](cmd.Context(), cmd, uriPath)
	if err != nil {
		return fmt.Errorf("cannot get pipelines: %w", err)
	}
	if len(pipelines) == 0 {
		fmt.Println("No pipeline found")
		return nil
	}
	if sortValue := common.SortFlagValue(cmd); sortValue != "" {
		common.Sort(pipelines, columns.SortBy(sortValue))
	}
	if err := profile.Current.Print(cmd.Context(), cmd, Pipelines(pipelines)); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}
