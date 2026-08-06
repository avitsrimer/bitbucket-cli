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
// difference between them is pathSuffix and the noun used in messages) and copies the response
// body straight to stdout. Shared by logs/report/cases, whose RunE bodies would otherwise be
// byte-for-byte identical apart from those two things (tripping dupl).
func rawStepOutput(cmd *cobra.Command, args, pathSuffix []string, noun string) error {
	pipelineID := pipelineFlagValue(cmd)

	repo, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get repository: %w", err)
	}

	lgr.Printf("[DEBUG] displaying %s for step %s of pipeline %s", noun, args[0], pipelineID)
	if !common.WhatIf(cmd, "Showing %s for step %s of pipeline %s", noun, args[0], pipelineID) {
		return nil
	}

	profileCurrent, err := profile.GetProfileFromCommand(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get profile: %w", err)
	}

	pathElements := append([]string{"pipelines", pipelineID, "steps", args[0]}, pathSuffix...)
	raw, err := profileCurrent.GetRaw(cmd.Context(), repo.GetPath(pathElements...))
	if err != nil {
		return fmt.Errorf("cannot get %s for step %s: %w", noun, args[0], err)
	}
	if _, err := io.Copy(os.Stdout, raw); err != nil {
		return fmt.Errorf("cannot write %s: %w", noun, err)
	}
	return nil
}
