package pullrequest

import (
	"fmt"
	"net/http"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	prcommon "github.com/avitsrimer/bitbucket-cli/internal/pullrequest/common"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/avitsrimer/bitbucket-cli/internal/user"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

// actionSpec describes one of the simple pullrequest actions (approve, unapprove, decline,
// request-changes, remove-request-changes) that share the "resolve profile/repository/id ->
// WhatIf -> single API verb call -> print result" skeleton.
//
// merge keeps its own command definition (extra flags, async handling, Location-header
// parsing) but reuses openPullRequestIDsCompletion for its ValidArgsFunction.
type actionSpec struct {
	name     string   // command name, e.g. "approve"
	aliases  []string // command aliases, if any
	short    string   // short help text
	whatIf   string   // gerund phrase for the WhatIf/dry-run prompt, e.g. "Approving"
	errVerb  string   // infinitive phrase for error messages, e.g. "approve"
	endpoint string   // last path segment of the pullrequest resource, e.g. "approve"
	method   string   // http.MethodPost or http.MethodDelete
	logFetch bool     // whether ValidArgsFunction logs the fetched id count
}

// newActionCommand builds a cobra.Command for one of the simple pullrequest actions described by spec.
func newActionCommand(spec actionSpec) *cobra.Command {
	cmd := &cobra.Command{
		Use:     spec.name + " [flags] <pullrequest-id>",
		Aliases: spec.aliases,
		Short:   spec.short,
		Args:    cobra.MaximumNArgs(1),
	}
	cmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return openPullRequestIDsCompletion(cmd, args, toComplete, spec.logFetch)
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runAction(cmd, args, spec)
	}
	return cmd
}

// openPullRequestIDsCompletion completes <pullrequest-id> from the open pullrequests of the
// current repository; logFetch preserves each action's original debug-logging behavior.
func openPullRequestIDsCompletion(cmd *cobra.Command, args []string, toComplete string, logFetch bool) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	ids, err := prcommon.GetPullRequestIDsWithState(cmd.Context(), cmd, "OPEN")
	if err != nil {
		cobra.CompErrorln(err.Error())
		return []string{}, cobra.ShellCompDirectiveError
	}
	if logFetch {
		lgr.Printf("[DEBUG] fetched %d pullrequest ids", len(ids))
	}
	return common.FilterValidArgs(ids, args, toComplete), cobra.ShellCompDirectiveNoFileComp
}

// runAction resolves the profile, repository and pullrequest id, gates on WhatIf, then calls
// the single API verb described by spec and prints the result when there is one.
func runAction(cmd *cobra.Command, args []string, spec actionSpec) error {
	profile, err := profile.GetProfileFromCommand(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot %s pull request: %w", spec.errVerb, err)
	}

	repository, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot %s pull request: %w", spec.errVerb, err)
	}

	pullRequestID, err := GetPullRequestIDFromArgs(cmd.Context(), cmd, repository, args)
	if err != nil {
		return fmt.Errorf("cannot %s pull request: %w", spec.errVerb, err)
	}

	if !common.WhatIf(cmd, "%s pullrequest %s", spec.whatIf, pullRequestID) {
		return nil
	}

	uripath := repository.GetPath("pullrequests", pullRequestID, spec.endpoint)

	switch spec.method {
	case http.MethodPost:
		var participant user.Participant

		if err := profile.Post(cmd.Context(), cmd, uripath, nil, &participant); err != nil {
			return fmt.Errorf("failed to %s pull request %s: %w", spec.errVerb, pullRequestID, err)
		}
		if err := profile.Print(cmd.Context(), cmd, participant); err != nil {
			return fmt.Errorf("cannot print result: %w", err)
		}
		return nil
	case http.MethodDelete:
		if err := profile.Delete(cmd.Context(), cmd, uripath, nil); err != nil {
			return fmt.Errorf("failed to %s pull request %s: %w", spec.errVerb, pullRequestID, err)
		}
		return nil
	default:
		return fmt.Errorf("unsupported action method %s for pull request %s", spec.method, pullRequestID)
	}
}
