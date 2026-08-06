package pipeline

import (
	"fmt"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	plcommon "github.com/avitsrimer/bitbucket-cli/internal/pipeline/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var getCmd = &cobra.Command{
	Use:               "get [flags] <pipeline-uuid-or-build-number>",
	Aliases:           []string{"show", "info", "display"},
	Short:             "get a pipeline by its UUID or build number",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: getValidArgs,
	RunE:              getProcess,
}

func init() {
	Command.AddCommand(getCmd)

	common.RegisterColumnsFlag(getCmd, columns)
}

func getValidArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	ids, err := plcommon.GetPipelineIDs(cmd.Context(), cmd, args, toComplete)
	if err != nil {
		cobra.CompErrorln(err.Error())
		return []string{}, cobra.ShellCompDirectiveError
	}
	return common.FilterValidArgs(ids, args, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func getProcess(cmd *cobra.Command, args []string) error {
	if err := common.ValidatePathIdentifier("pipeline", args[0]); err != nil {
		return fmt.Errorf("cannot get pipeline: %w", err)
	}

	repo, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get repository: %w", err)
	}

	lgr.Printf("[DEBUG] displaying pipeline %s", args[0])
	if !common.WhatIf(cmd, "Showing pipeline "+args[0]) {
		return nil
	}

	profileCurrent, err := profile.GetProfileFromCommand(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get profile: %w", err)
	}

	var target Pipeline
	if err := profileCurrent.Get(cmd.Context(), repo.GetPath("pipelines", args[0]), &target); err != nil {
		return fmt.Errorf("cannot get pipeline %s: %w", args[0], err)
	}
	if err := profileCurrent.Print(cmd.Context(), cmd, target); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}
