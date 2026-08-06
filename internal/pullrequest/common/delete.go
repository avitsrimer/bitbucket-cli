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
// pullrequestID reached via repo.GetPath("pullrequests", pullrequestID, pathSegment, id),
// gating each deletion on common.WhatIf and tolerating per-item failures according to the
// profile's error tolerance (see common.TolerateErrors). singularNoun/pluralNoun describe the
// resource in lowercase (e.g. "comment"/"comments", "task"/"tasks") for the WhatIf prompt, the
// debug log, and the aggregate tolerance message.
func DeleteSubResources(cmd *cobra.Command, repo *repository.Repository, pullrequestID, pathSegment string, ids []string, singularNoun, pluralNoun string) error {
	currentProfile, err := profile.GetProfileFromCommand(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get profile: %w", err)
	}

	var errs []error
	for _, id := range ids {
		if !common.WhatIf(cmd, "Deleting %s %s from pullrequest %s", singularNoun, id, pullrequestID) {
			continue
		}
		if err := currentProfile.Delete(cmd.Context(), repo.GetPath("pullrequests", pullrequestID, pathSegment, id), nil); err != nil {
			// err on its own (e.g. a bare "404 Not Found") names neither which id failed nor
			// which kind of sub-resource it was; wrapping it with both here is what lets
			// --warn-on-error's aggregate message (and a --stop-on-error abort) actually say
			// which of possibly several ids in this call failed.
			wrapped := fmt.Errorf("%s %s: %w", singularNoun, id, err)
			if currentProfile.ShouldStopOnError(cmd) {
				return fmt.Errorf("failed to delete pullrequest %w", wrapped)
			}
			errs = append(errs, wrapped)
			continue
		}
		lgr.Printf("[DEBUG] pullrequest %s %s deleted", singularNoun, id)
	}
	return common.TolerateErrors(cmd, currentProfile, errs, "delete these "+pluralNoun) //nolint:wrapcheck // TolerateErrors returns the same joined error verbatim (or nil); wrapping would prefix it with redundant noise
}
