package step

import (
	"context"
	"fmt"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	plcommon "github.com/avitsrimer/bitbucket-cli/internal/pipeline/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/gildas/go-core"
	"github.com/spf13/cobra"
)

// registerPipelineFlag registers a required --pipeline flag on cmd, backed by shell completion
// over plcommon.GetPipelineIDs. --pipeline is a plain string flag rather than an EnumFlag
// validated at parse time: pipeline build numbers are an unbounded identifier space, not a small
// enumeration.
func registerPipelineFlag(cmd *cobra.Command, usage string) {
	cmd.Flags().String("pipeline", "", usage)
	_ = cmd.MarkFlagRequired("pipeline")
	_ = cmd.RegisterFlagCompletionFunc("pipeline", pipelineValidArgs)
}

// pipelineValidArgs backs shell completion of --pipeline via plcommon.GetPipelineIDs, which
// fetches every pipeline ID unbounded by --limit (completion candidates must never be truncated
// by a flag meant to cap a *listing*'s output).
func pipelineValidArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	ids, err := plcommon.GetPipelineIDs(cmd.Context(), cmd, args, toComplete)
	if err != nil {
		cobra.CompErrorln(err.Error())
		return []string{}, cobra.ShellCompDirectiveError
	}
	return common.FilterValidArgs(ids, args, toComplete), cobra.ShellCompDirectiveNoFileComp
}

// pipelineFlagValue reads cmd's own --pipeline value via common.StringFlagValue, per CLAUDE.md's
// flag-reading rule: every RunE function in this package reads its flags off the passed cmd, so
// behavior is identical whether cmd is the real command or a standalone test double carrying its
// own --pipeline flag.
func pipelineFlagValue(cmd *cobra.Command) string {
	return common.StringFlagValue(cmd, "pipeline")
}

// stepValidArgs backs shell completion of the <pipeline-step-uuid-or-name> positional argument
// shared by get/logs/report/cases, via getStepIDs. It resolves to no suggestions (rather than an
// error) when --pipeline has not been set yet, since the step list cannot be resolved without it.
func stepValidArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	pipelineID := pipelineFlagValue(cmd)
	if pipelineID == "" {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	ids, err := getStepIDs(cmd.Context(), cmd, pipelineID)
	if err != nil {
		cobra.CompErrorln(err.Error())
		return []string{}, cobra.ShellCompDirectiveError
	}
	return common.FilterValidArgs(ids, args, toComplete), cobra.ShellCompDirectiveNoFileComp
}

// getStepIDs gets the ids of a pipeline's steps, for shell completion of a step UUID argument.
// Uses profile.GetAllUnbounded, not profile.GetAll: cmd here is the calling command's own flags,
// which may carry an unrelated --limit meant to bound a different query, and completion must
// still enumerate every step regardless of it.
func getStepIDs(ctx context.Context, cmd *cobra.Command, pipelineID string) (ids []string, err error) {
	repo, err := repository.GetRepository(ctx, cmd)
	if err != nil {
		return []string{}, fmt.Errorf("cannot get repository: %w", err)
	}
	steps, err := profile.GetAllUnbounded[Step](ctx, cmd, repo.GetPath("pipelines", pipelineID, "steps"))
	if err != nil {
		return []string{}, fmt.Errorf("cannot get steps: %w", err)
	}
	return core.Map(steps, func(step Step) string { return step.ID.String() }), nil
}
