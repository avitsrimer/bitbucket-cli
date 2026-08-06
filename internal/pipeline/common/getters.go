// Package plcommon holds getters shared between the pipeline command tree and its step
// subcommand package. pipeline imports step to register it as a subcommand, so step cannot import
// pipeline back without a cycle; a shared lookup lives here instead, mirroring
// internal/pullrequest/common.
package plcommon

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/gildas/go-core"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

// PipelineID is the minimal shape read from BitBucket's pipelines endpoint for completion: only
// the build number is needed to enumerate valid `pipeline get` arguments.
type PipelineID struct {
	ID uint64 `json:"build_number"`
}

// GetPipelineIDs gets the build numbers of the current repository's pipelines, for shell
// completion of a pipeline build number argument.
//
// This uses profile.GetAllUnbounded, not profile.GetAll: cmd here is the calling command's own
// flags, which may carry an unrelated --limit meant to bound a different query, and completion
// must still enumerate every pipeline regardless of it.
func GetPipelineIDs(ctx context.Context, cmd *cobra.Command, args []string, toComplete string) (ids []string, err error) {
	repo, err := repository.GetRepository(ctx, cmd)
	if err != nil {
		return []string{}, fmt.Errorf("cannot get repository: %w", err)
	}

	lgr.Printf("[DEBUG] getting pipelines for repository %s", repo)
	pipelines, err := profile.GetAllUnbounded[PipelineID](ctx, cmd, repo.GetPath("pipelines"))
	if err != nil {
		lgr.Printf("[ERROR] failed to get pipelines: %v", err)
		return []string{}, err
	}

	ids = core.Map(pipelines, func(pipeline PipelineID) string { return strconv.FormatUint(pipeline.ID, 10) })
	core.Sort(ids, func(a, b string) bool { return strings.ToLower(a) < strings.ToLower(b) })
	return ids, nil
}
