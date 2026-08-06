package step

import (
	"context"
	"fmt"
	"strings"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	plcommon "github.com/avitsrimer/bitbucket-cli/internal/pipeline/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/gildas/go-core"
	"github.com/spf13/cobra"
)

// pipelineValidArgs backs shell completion of the <pipeline> positional shared by every step
// subcommand, via plcommon.GetPipelineIDs, which fetches every pipeline ID unbounded by --limit (a
// flag meant to cap a *listing*'s output must never truncate completion candidates).
func pipelineValidArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
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

// pipelineAndStepValidArgs is the ValidArgsFunction shared by get/logs/report/cases, whose two
// positionals are <pipeline> <pipeline-step-uuid-or-name>: arg 0 completes pipeline ids (like
// pipelineValidArgs), arg 1 completes the step names and UUIDs of the pipeline named in arg 0 via
// getStepNamesAndIDs, reading that pipeline from args[0] instead of a flag.
func pipelineAndStepValidArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	switch len(args) {
	case 0:
		return pipelineValidArgs(cmd, args, toComplete)
	case 1:
		names, err := getStepNamesAndIDs(cmd.Context(), cmd, args[0])
		if err != nil {
			cobra.CompErrorln(err.Error())
			return []string{}, cobra.ShellCompDirectiveError
		}
		return common.FilterValidArgs(names, args, toComplete), cobra.ShellCompDirectiveNoFileComp
	default:
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
}

// getSteps gets every step of the pipeline identified by pipelineID, for resolveStepID and shell
// completion. Uses profile.GetAllUnbounded, not profile.GetAll: cmd here is the calling command's
// own flags, which may carry an unrelated --limit meant to bound a different query, and both
// resolution and completion must enumerate every step regardless of it.
func getSteps(ctx context.Context, cmd *cobra.Command, pipelineID string) ([]Step, error) {
	repo, err := repository.GetRepository(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("cannot get repository: %w", err)
	}
	steps, err := profile.GetAllUnbounded[Step](ctx, cmd, repo.GetPath("pipelines", pipelineID, "steps"))
	if err != nil {
		return nil, fmt.Errorf("cannot get steps: %w", err)
	}
	return steps, nil
}

// getStepNamesAndIDs returns every step's name followed by every step's UUID, for shell
// completion of the <pipeline-step-uuid-or-name> positional: names are offered first since that
// is what a human reads in BitBucket's own UI, but a UUID always resolves too (per
// resolveStepID), so completion never suggests a value the command would reject.
func getStepNamesAndIDs(ctx context.Context, cmd *cobra.Command, pipelineID string) ([]string, error) {
	steps, err := getSteps(ctx, cmd, pipelineID)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(steps))
	for _, step := range steps {
		if step.Name != "" {
			names = append(names, step.Name)
		}
	}
	ids := core.Map(steps, func(step Step) string { return step.ID.String() })
	return append(names, ids...), nil
}

// resolveStepID resolves stepArg to a step UUID within the pipeline identified by pipelineID. A
// value that parses as common.ParseUUID passes through as-is, with NO list request issued.
// Otherwise the pipeline's steps are listed (getSteps) and matched on NAME, case-insensitively and
// trimmed: exactly one match resolves to its UUID; zero matches error, naming stepArg and listing
// the available step names; two or more matches (BitBucket allows duplicate step names within one
// pipeline) error listing the ambiguous candidates with their UUIDs and tell the caller to pass a
// UUID instead. Shared by get/logs/report/cases via rawStepOutput and getProcess.
func resolveStepID(ctx context.Context, cmd *cobra.Command, pipelineID, stepArg string) (string, error) {
	if parsed, err := common.ParseUUID(stepArg); err == nil {
		return parsed.String(), nil
	}

	steps, err := getSteps(ctx, cmd, pipelineID)
	if err != nil {
		return "", err
	}

	target := strings.ToLower(strings.TrimSpace(stepArg))
	var matches []Step
	for _, step := range steps {
		if strings.ToLower(strings.TrimSpace(step.Name)) == target {
			matches = append(matches, step)
		}
	}

	switch len(matches) {
	case 1:
		return matches[0].ID.String(), nil
	case 0:
		names := core.Map(steps, func(step Step) string { return step.Name })
		return "", fmt.Errorf("no step named %q found for pipeline %s; available step names: %s", stepArg, pipelineID, strings.Join(names, ", "))
	default:
		candidates := core.Map(matches, func(step Step) string { return fmt.Sprintf("%s (%s)", step.Name, step.ID.String()) })
		return "", fmt.Errorf("step name %q is ambiguous for pipeline %s (candidates: %s); pass a UUID instead", stepArg, pipelineID, strings.Join(candidates, ", "))
	}
}
