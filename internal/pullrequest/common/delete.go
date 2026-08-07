package prcommon

import (
	"fmt"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

// DeleteSubResources deletes each of ids, a sub-resource of the pullrequest identified by
// pullrequestID reached via repo.GetPath("pullrequests", pullrequestID, pathSegment, id). Each id
// is validated via common.ValidatePathIdentifier before it ever reaches GetPath (repo.GetPath is a
// bare path.Join with no escaping, so an unvalidated id could splice extra path segments into the
// request, up to and including a different resource entirely). A validated id is then GETed at
// that same path to confirm the sub-resource exists (this also validates the parent pullrequest's
// existence too), before the DELETE itself is gated on
// common.WhatIfPayload. Every per-item failure (invalid id, missing sub-resource, or a failed
// delete) is tolerated according to the profile's error tolerance (see common.TolerateErrors).
// singularNoun/pluralNoun describe the resource in lowercase (e.g. "comment"/"comments",
// "task"/"tasks") for the WhatIf prompt, the debug log, and the aggregate tolerance message.
func DeleteSubResources(cmd *cobra.Command, repo *repository.Repository, pullrequestID, pathSegment string, ids []string, singularNoun, pluralNoun string) error {
	currentProfile, err := profile.GetProfileFromCommand(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get profile: %w", err)
	}

	var errs []error
	// recordOrStop wraps err with singularNoun/id -- on its own (e.g. a bare "404 Not Found") err
	// names neither which id failed nor which kind of sub-resource it was, which is what lets
	// --warn-on-error's aggregate message (and a --stop-on-error abort) actually say which of
	// possibly several ids in this call failed. It returns a non-nil, stop-worthy error the
	// caller must return immediately under --stop-on-error, or nil after recording the wrapped
	// error and letting the loop continue to the next id.
	recordOrStop := func(id string, err error) error {
		wrapped := fmt.Errorf("%s %s: %w", singularNoun, id, err)
		if currentProfile.ShouldStopOnError(cmd) {
			return fmt.Errorf("failed to delete pullrequest %w", wrapped)
		}
		errs = append(errs, wrapped)
		return nil
	}

	for _, id := range ids {
		if err := common.ValidatePathIdentifier(singularNoun+"-id", id); err != nil {
			if stopErr := recordOrStop(id, err); stopErr != nil {
				return stopErr
			}
			continue
		}

		uripath := repo.GetPath("pullrequests", pullrequestID, pathSegment, id)
		if err := currentProfile.Get(cmd.Context(), uripath, nil); err != nil {
			if stopErr := recordOrStop(id, err); stopErr != nil {
				return stopErr
			}
			continue
		}
		if !common.WhatIfPayload(cmd, uripath, nil, "Deleting %s %s from pullrequest %s", singularNoun, id, pullrequestID) {
			continue
		}
		if err := currentProfile.Delete(cmd.Context(), uripath, nil); err != nil {
			if stopErr := recordOrStop(id, err); stopErr != nil {
				return stopErr
			}
			continue
		}
		lgr.Printf("[DEBUG] pullrequest %s %s deleted", singularNoun, id)
	}
	return common.TolerateErrors(cmd, currentProfile, errs, "delete these "+pluralNoun) //nolint:wrapcheck // TolerateErrors returns the same joined error verbatim (or nil); wrapping would prefix it with redundant noise
}
