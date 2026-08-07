package step

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	plcommon "github.com/avitsrimer/bitbucket-cli/internal/pipeline/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
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
		return common.FilterValidArgs(names, args[1:], toComplete), cobra.ShellCompDirectiveNoFileComp
	default:
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
}

// getSteps gets every step of the pipeline identified by pipelineID within repo, for
// resolveStepID and shell completion. Uses profile.GetAllUnbounded, not profile.GetAll: cmd here
// is the calling command's own flags, which may carry an unrelated --limit meant to bound a
// different query, and both resolution and completion must enumerate every step regardless of it.
// Callers that already hold repo (get.go, raw.go) pass it straight through instead of this
// re-resolving it via a second repository.GetRepository call.
func getSteps(ctx context.Context, cmd *cobra.Command, repo *repository.Repository, pipelineID string) ([]Step, error) {
	steps, err := profile.GetAllUnbounded[Step](ctx, cmd, repo.GetPath("pipelines", pipelineID, "steps"))
	if err != nil {
		return nil, fmt.Errorf("cannot get steps: %w", err)
	}
	return steps, nil
}

// getStepNamesAndIDs returns every step's name followed by every step's UUID, for shell
// completion of the <pipeline-step-uuid-or-name> positional: names are offered first since that
// is what a human reads in BitBucket's own UI, but a UUID always resolves too (per
// resolveStepID), so completion never suggests a value the command would reject. Names are
// trimmed and empty-after-trim ones excluded, matching resolveStepID's own
// strings.TrimSpace(step.Name) match: a whitespace-only step name offered untrimmed here would
// otherwise complete to a value resolveStepID's own blank-target guard rejects. Unlike
// resolveStepID's callers, a completion function has no repo already resolved, so this resolves
// its own.
func getStepNamesAndIDs(ctx context.Context, cmd *cobra.Command, pipelineID string) ([]string, error) {
	repo, err := repository.GetRepository(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("cannot get repository: %w", err)
	}
	steps, err := getSteps(ctx, cmd, repo, pipelineID)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(steps))
	for _, step := range steps {
		if name := strings.TrimSpace(step.Name); name != "" {
			names = append(names, name)
		}
	}
	ids := common.Map(steps, func(step Step) string { return step.ID.String() })
	return append(names, ids...), nil
}

// resolveStepID resolves stepArg to a step UUID within the pipeline identified by pipelineID of
// repo. A value that parses as common.ParseUUID passes through as-is, with NO list request
// issued. An empty or all-whitespace stepArg is rejected up front (before any list request):
// BitBucket allows steps declared without a name, so matching a blank target against
// strings.TrimSpace(step.Name) would otherwise silently resolve to one of those unnamed steps
// instead of erroring on missing input. This guard lives here rather than in
// common.ValidatePathIdentifier so it does not reintroduce the '/'-in-step-name rejection that
// guard used to cause upstream of this function. Otherwise the pipeline's steps are listed
// (getSteps) and matched on NAME, case-insensitively and trimmed, skipping any step with no name
// at all: exactly one match resolves to its UUID; zero matches error, naming stepArg and listing
// the available step names (or saying none of the pipeline's steps have a name at all, when every
// step.Name is empty); two or more matches (BitBucket allows duplicate step names within one
// pipeline) error listing the ambiguous candidates with their UUIDs and tell the caller to pass a
// UUID instead. Both success paths return through guardResolvedStepID, so the resolved UUID is
// validated once here rather than re-guarded by every call site. Shared by get/logs/report/cases
// via rawStepOutput and getProcess, both of which already hold repo and pass it straight through.
func resolveStepID(ctx context.Context, cmd *cobra.Command, repo *repository.Repository, pipelineID, stepArg string) (string, error) {
	if parsed, err := common.ParseUUID(stepArg); err == nil {
		return guardResolvedStepID(parsed.String())
	}

	target := strings.TrimSpace(stepArg)
	if target == "" {
		return "", errors.New("argument pipeline-step-uuid-or-name is missing")
	}

	steps, err := getSteps(ctx, cmd, repo, pipelineID)
	if err != nil {
		return "", err
	}

	var matches []Step
	for _, step := range steps {
		if name := strings.TrimSpace(step.Name); name != "" && strings.EqualFold(name, target) {
			matches = append(matches, step)
		}
	}

	switch len(matches) {
	case 1:
		return guardResolvedStepID(matches[0].ID.String())
	case 0:
		var names []string
		for _, step := range steps {
			if name := strings.TrimSpace(step.Name); name != "" {
				names = append(names, name)
			}
		}
		if len(names) == 0 {
			return "", fmt.Errorf("no step named %q found for pipeline %s; none of its steps have a name, pass a UUID instead", stepArg, pipelineID)
		}
		return "", fmt.Errorf("no step named %q found for pipeline %s; available step names: %s", stepArg, pipelineID, strings.Join(names, ", "))
	default:
		candidates := common.Map(matches, func(step Step) string { return fmt.Sprintf("%s (%s)", step.Name, step.ID.String()) })
		return "", fmt.Errorf("step name %q is ambiguous for pipeline %s (candidates: %s); pass a UUID instead", stepArg, pipelineID, strings.Join(candidates, ", "))
	}
}

// guardResolvedStepID applies common.ValidatePathIdentifier to stepID -- a name or a UUID the user
// typed is deliberately not guarded by ValidatePathIdentifier upstream of resolveStepID, since a
// legitimate step name may contain "/" (e.g. a bitbucket-pipelines.yml step named "build/test"),
// and stepArg never reaches GetPath directly, only the resolved stepID does -- before either of
// resolveStepID's success paths returns it. Both paths (common.ParseUUID passthrough and a
// resolved step's own .ID) always produce a canonical UUID string this guard can never actually
// reject, but it still runs here, once, rather than being re-applied to the same already-canonical
// value again at every call site.
func guardResolvedStepID(stepID string) (string, error) {
	if err := common.ValidatePathIdentifier("pipeline-step-uuid-or-name", stepID); err != nil {
		return "", fmt.Errorf("cannot resolve step: %w", err)
	}
	return stepID, nil
}
