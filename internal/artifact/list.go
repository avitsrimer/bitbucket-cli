package artifact

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
	Short: "list the artifacts of the current repository",
	Args:  cobra.NoArgs,
	RunE:  listProcess,
}

func init() {
	Command.AddCommand(listCmd)

	common.RegisterListFlags(listCmd, columns, "artifacts")
	// --query has no package-level destination: listProcess reads it directly off cmd below, so a
	// bound variable here would only ever be write-only state.
	listCmd.Flags().String("query", "", "Query string to filter artifacts")
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
	if query := common.StringFlagValue(cmd, "query"); query != "" {
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
	if sortValue := common.SortFlagValue(cmd); sortValue != "" {
		common.Sort(artifacts, columns.SortBy(sortValue))
	}
	if err := profile.Current.Print(cmd.Context(), cmd, Artifacts(artifacts)); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}
