package step

import (
	"fmt"
	"io"
	"os"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

// rawStepOutput fetches a step's raw sub-resource (logs, test report, or test cases -- the only
// difference between them is pathSuffix and noun) and copies the response body straight to
// stdout. Shared by logs/report/cases, whose RunE bodies would otherwise be byte-for-byte
// identical apart from those two things (tripping dupl). pathSuffix is always 1-2 literal path
// elements at every call site (e.g. "log", or "test_reports", "test_cases"), so it is variadic
// rather than a []string the caller has to build.
func rawStepOutput(cmd *cobra.Command, pipelineID, stepArg, noun string, pathSuffix ...string) error {
	if err := common.ValidatePathIdentifier("pipeline", pipelineID); err != nil {
		return fmt.Errorf("cannot get %s: %w", noun, err)
	}
	if err := common.ValidatePathIdentifier("pipeline-step-uuid-or-name", stepArg); err != nil {
		return fmt.Errorf("cannot get %s: %w", noun, err)
	}

	repo, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get repository: %w", err)
	}

	lgr.Printf("[DEBUG] displaying %s for step %s of pipeline %s", noun, stepArg, pipelineID)
	if !common.WhatIf(cmd, "Showing %s for step %s of pipeline %s", noun, stepArg, pipelineID) {
		return nil
	}

	stepID, err := resolveStepID(cmd.Context(), cmd, repo, pipelineID, stepArg)
	if err != nil {
		return fmt.Errorf("cannot resolve step %s: %w", stepArg, err)
	}

	profileCurrent, err := profile.GetProfileFromCommand(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get profile: %w", err)
	}

	pathElements := append([]string{"pipelines", pipelineID, "steps", stepID}, pathSuffix...)
	raw, err := profileCurrent.GetRaw(cmd.Context(), repo.GetPath(pathElements...))
	if err != nil {
		return fmt.Errorf("cannot get %s for step %s: %w", noun, stepArg, err)
	}
	if _, err := io.Copy(os.Stdout, raw); err != nil {
		return fmt.Errorf("cannot write %s: %w", noun, err)
	}
	return nil
}
