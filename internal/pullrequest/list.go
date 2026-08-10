package pullrequest

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/avitsrimer/bitbucket-cli/internal/user"
	"github.com/avitsrimer/bitbucket-cli/internal/workspace"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "list all pullrequests",
	Args:  cobra.NoArgs,
	RunE:  listProcess,
}

var listOptions struct {
	Commit string
}

// listDefaultState is the state --state fetches when the flag is never explicitly set on the
// command line.
const listDefaultState = "open"

func init() {
	Command.AddCommand(listCmd)

	listCmd.Flags().StringVar(&listOptions.Commit, "commit", "", "List pull requests by commit hash")
	stateFlag := common.NewEnumSliceFlagWithAllAllowed("declined", "merged", "open", "superseded")
	listCmd.Flags().Var(stateFlag, "state", "Pull request state(s) to fetch (repeatable, or \"all\"). Defaults to \""+listDefaultState+"\"")
	listCmd.Flags().String("query", "", "Query string to filter pull requests")
	listCmd.Flags().String("source", "", "Filter by source branch name")
	listCmd.Flags().String("destination", "", "Filter by destination branch name")
	// No shell completion for --author: the only cheap value source (GetReviewerNicknames) returns
	// workspace nicknames, which are not the identifiers this endpoint resolves, so completing them
	// would offer values that list nothing.
	listCmd.Flags().String("author", "", "List pull requests authored by this user across every repository of the workspace (a UUID in braces, an Atlassian account ID, or a username)")
	listCmd.Flags().Bool("mine", false, "List your own pull requests across every repository of the workspace")
	common.RegisterListFlags(listCmd, columns, "pull requests")
	listCmd.MarkFlagsMutuallyExclusive("commit", "state")
	listCmd.MarkFlagsMutuallyExclusive("commit", "query")
	listCmd.MarkFlagsMutuallyExclusive("commit", "source")
	listCmd.MarkFlagsMutuallyExclusive("commit", "destination")
	listCmd.MarkFlagsMutuallyExclusive("author", "mine")
	listCmd.MarkFlagsMutuallyExclusive("commit", "author")
	listCmd.MarkFlagsMutuallyExclusive("commit", "mine")
	_ = listCmd.RegisterFlagCompletionFunc(stateFlag.CompletionFunc("state"))
}

// listRequest is one resolved listing: the uripath to GET, the human-readable target it addresses
// (the subject of the debug and dry-run lines, and of author mode's 404 message), and the pull
// request states it asks for. The builders resolve the states themselves and hand them back rather
// than listProcess reading the --state flag a second time: listStates warns when cmd registered
// "state" with the wrong type, and one invocation must warn once.
type listRequest struct {
	uripath string
	target  string
	states  []string
}

func listProcess(cmd *cobra.Command, args []string) (err error) {
	// author mode is resolved first and never touches repository resolution, so `--author`/`--mine`
	// work from any directory as long as the workspace resolves. Flag exclusivity is enforced at
	// parse time only, so a direct listProcess call with both --author and --commit set
	// deterministically takes this arm and the --commit value is ignored (an explicitly-passed
	// --repository is still rejected by the guard inside it).
	_, mine, authorMode := authorModeValue(cmd)

	var request listRequest
	if authorMode {
		request, err = listAuthorRequest(cmd)
	} else {
		request, err = listRepositoryRequest(cmd)
	}
	if err != nil {
		return err
	}
	states := strings.Join(request.states, ",")

	lgr.Printf("[DEBUG] listing %s pull requests for %s", states, request.target)
	if !common.WhatIf(cmd, fmt.Sprintf("Showing %s pull requests for %s", states, request.target)) {
		return nil
	}

	pullrequests, err := profile.GetAll[PullRequest](cmd.Context(), cmd, request.uripath)
	if err != nil {
		// A 404 in author mode means either the workspace or the author was not found, so the
		// message names both resolved values (target carries them) rather than blaming the flag the
		// user happened to type. Only --author's own value can be in a form the endpoint rejects
		// outright, so the accepted-forms guidance is attached to that flag alone: --mine resolves
		// the identifier itself, and telling its user to look up a UUID they never typed would send
		// them after the wrong problem.
		if authorMode && profile.IsNotFound(err) {
			guidance := "the workspace must exist and be visible to your token"
			if !mine {
				guidance = "--author takes a UUID in braces (e.g. {01234567-89ab-cdef-0123-456789abcdef}), an Atlassian account ID, or a username; " +
					"the nicknames `bb workspace members` shows in its Name column are not usernames, so they usually resolve to nothing " +
					"(that command's ID column carries the UUID this flag wants). Use --mine for your own pull requests"
			}
			return fmt.Errorf("cannot list pull requests for %s: %w; %s", request.target, err, guidance)
		}
		return err
	}
	if len(pullrequests) == 0 {
		lgr.Printf("[DEBUG] no pullrequest found")
		return err
	}
	if sortValue := common.SortFlagValue(cmd); sortValue != "" {
		common.Sort(pullrequests, columns.SortBy(sortValue))
	}
	if err := profile.Current.Print(cmd.Context(), cmd, PullRequests(pullrequests)); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}

// authorModeValue reports whether cmd asks for author mode -- the workspace-wide
// GET /workspaces/{workspace}/pullrequests/{selected_user} listing -- and with which author. It is
// the single source of truth both listProcess and PullRequest.GetHeaders consult, so the request
// path and the default column set can never disagree about which mode is active.
//
// Both flags are read as their declared types, nil-safely: a cmd that never registered them (every
// other command reaching GetHeaders, `pullrequest get` in particular) is not author mode. Reading
// --mine through flag.Value.String() instead would be a silent disaster -- an unset bool flag
// stringifies as "false", which is non-empty, flipping every plain list/get into author mode.
//
// Author mode keys off --author having been SET, not off it carrying a non-empty value: an
// explicitly empty `--author ""` (a shell variable that did not expand, say) must fail with
// ValidatePathIdentifier's "argument author is missing" rather than silently falling back to a
// repository-scoped listing of everybody's pull requests -- which would also slip past the
// --repository guard.
func authorModeValue(cmd *cobra.Command) (author string, mine, ok bool) {
	if cmd == nil {
		return "", false, false
	}
	author = common.StringFlagValue(cmd, "author")
	mine, _ = cmd.Flags().GetBool("mine")
	return author, mine, cmd.Flags().Changed("author") || mine
}

// listRepositoryRequest builds the repository-scoped request: the commit form when --commit is set,
// the plain pullrequests listing otherwise.
func listRepositoryRequest(cmd *cobra.Command) (listRequest, error) {
	repo, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return listRequest{}, fmt.Errorf("cannot get repository: %w", err)
	}
	target := fmt.Sprintf("repository: %s", repo)

	if listOptions.Commit != "" {
		if err = common.ValidatePathIdentifier("commit", listOptions.Commit); err != nil {
			return listRequest{}, fmt.Errorf("cannot list pull requests: %w", err)
		}
		// the by-commit endpoint takes no filter parameters of its own, so no query is built here --
		// the states are still resolved for the message naming what is being listed.
		return listRequest{
			uripath: repo.GetPath("commit", listOptions.Commit, "pullrequests"),
			target:  target,
			states:  listStates(cmd),
		}, nil
	}
	query, states := listQuery(cmd)
	return listRequest{
		uripath: repo.GetPath("pullrequests") + "?" + query.Encode(),
		target:  target,
		states:  states,
	}, nil
}

// listAuthorRequest builds the workspace-wide, author-scoped request. No repository is resolved
// here at all, so the command works from any directory whose workspace resolves.
func listAuthorRequest(cmd *cobra.Command) (listRequest, error) {
	author, mine, _ := authorModeValue(cmd)

	// --repository is a root persistent flag, so init()-time MarkFlagsMutuallyExclusive cannot pair
	// it with --author/--mine; the conflict is caught here instead. Author mode lists across the
	// whole workspace, so an explicitly-passed --repository would otherwise be silently ignored.
	if flag := cmd.Flags().Lookup("repository"); flag != nil && flag.Changed {
		return listRequest{}, errors.New("--repository cannot be combined with --author or --mine: author mode lists pull requests across every repository of the workspace")
	}

	workspaceName, err := workspace.GetWorkspaceName(cmd.Context(), cmd)
	if err != nil {
		return listRequest{}, fmt.Errorf("cannot get workspace: %w", err)
	}
	// The workspace is user-supplied too (--workspace, a git remote's workspace segment, or the
	// profile default) and lands in the same hand-built path, so it gets the same treatment as the
	// author segment below: validated as a single path identifier -- a slug never legitimately spans
	// two segments here, unlike --repository's "workspace/repository" form -- and escaped.
	if err = common.ValidatePathIdentifier("workspace", workspaceName); err != nil {
		return listRequest{}, fmt.Errorf("cannot list pull requests: %w", err)
	}

	selected := author
	if mine {
		// --mine issues GET /user (cached in user.UserCache) before the WhatIf gate, exactly as the
		// repository-scoped branch resolves its repository first: the dry-run line names the author
		// it would query, and resolving it is itself a read.
		me, meErr := user.GetMe(cmd.Context(), cmd)
		if meErr != nil {
			return listRequest{}, fmt.Errorf("cannot get current user: %w", meErr)
		}
		// A payload with a null or missing "uuid" unmarshals to the zero UUID, whose String() is a
		// perfectly well-formed {00000000-...} -- a request for an account that cannot exist. Fail
		// on the response that is actually wrong instead of sending it and reporting a 404.
		if me.ID.IsNil() {
			return listRequest{}, errors.New("cannot list your own pull requests: the current user carries no uuid (check `bb user me` and that the token has the read:user scope)")
		}
		// common.UUID.String() already wraps the uuid in the braces this endpoint requires; adding
		// another pair would double them.
		selected = me.ID.String()
	} else if err = common.ValidatePathIdentifier("author", selected); err != nil {
		return listRequest{}, fmt.Errorf("cannot list pull requests: %w", err)
	}

	// PathEscape on top of ValidatePathIdentifier: the latter rejects path separators and dot
	// segments but not "?" or "#", and resolveRequestURL splits the uripath on "?" to take
	// everything after it as the raw query -- so an unescaped "?" in the author value would replace
	// our own state/q parameters wholesale.
	uripath := "/workspaces/" + url.PathEscape(workspaceName) + "/pullrequests/" + url.PathEscape(selected)
	query, states := listQuery(cmd)
	return listRequest{
		uripath: uripath + "?" + query.Encode(),
		target:  fmt.Sprintf("workspace: %s, author: %s", workspaceName, selected),
		states:  states,
	}, nil
}

// listQuery builds the query parameters shared by the repository-scoped and author-scoped listings:
// one "state" value per resolved state, plus the "q" filter when --query/--source/--destination
// contributed one. It also returns the resolved states, so a caller naming them in a message does
// not have to resolve them a second time.
func listQuery(cmd *cobra.Command) (url.Values, []string) {
	query := url.Values{}
	states := listStates(cmd)
	for _, state := range states {
		query.Add("state", strings.ToUpper(state))
	}
	if filter := listQueryFilter(cmd); filter != "" {
		query.Set("q", filter)
	}
	return query, states
}

// listStates resolves cmd's own --state flag, read directly off cmd rather than a package-level
// binding (the flag's repeatable EnumSliceFlag value does not round-trip through
// cmd.Flags().GetStringSlice, whose CSV-based parsing does not understand EnumSliceFlag.String's
// bracketed representation). Defaults to listDefaultState when the flag was never registered on
// cmd or never set explicitly, "all" resolves to every allowed state via
// common.NewEnumSliceFlagWithAllAllowed.
func listStates(cmd *cobra.Command) []string {
	flag := cmd.Flags().Lookup("state")
	if flag == nil || !flag.Changed {
		return []string{listDefaultState}
	}
	states, ok := flag.Value.(*common.EnumSliceFlag)
	if !ok {
		// cmd's own "state" flag was registered as something other than *common.EnumSliceFlag --
		// a programming error, not a user input problem. Falling back to the default state keeps
		// the command from crashing, but this is loud rather than a silent no-op so the mismatch
		// is never mistaken for "the user asked for the default state".
		lgr.Printf("[WARN] state flag is not an EnumSliceFlag (got %T), falling back to %q", flag.Value, listDefaultState)
		return []string{listDefaultState}
	}
	return states.GetSlice()
}

// listQueryFilter builds the "q=" filter from cmd's own --query, --source, and --destination
// flags (all read direct-off-cmd via common.StringFlagValue, not a package-level options
// binding), ANDing every non-empty piece together so all three compose. Branch names are
// double-quoted with embedded double quotes/backslashes escaped for Bitbucket's query syntax;
// --query is passed through verbatim since it is already a raw Bitbucket query expression --
// when it is combined with a --source/--destination clause, it is parenthesized first so a
// disjunction inside it (e.g. `state="OPEN" OR state="MERGED"`) cannot have AND bind tighter than
// the caller intended and silently apply the branch filter to only one disjunct. --query alone is
// returned unparenthesized, matching its pre-existing output exactly.
func listQueryFilter(cmd *cobra.Command) string {
	var clauses []string
	if query := common.StringFlagValue(cmd, "query"); query != "" {
		clauses = append(clauses, query)
	}
	if source := common.StringFlagValue(cmd, "source"); source != "" {
		clauses = append(clauses, "source.branch.name="+quoteBranchFilter(source))
	}
	if destination := common.StringFlagValue(cmd, "destination"); destination != "" {
		clauses = append(clauses, "destination.branch.name="+quoteBranchFilter(destination))
	}
	if len(clauses) < 2 {
		return strings.Join(clauses, " AND ")
	}
	for i, clause := range clauses {
		clauses[i] = "(" + clause + ")"
	}
	return strings.Join(clauses, " AND ")
}

// quoteBranchFilter double-quotes value for a Bitbucket q= filter clause, backslash-escaping any
// embedded backslash or double quote.
func quoteBranchFilter(value string) string {
	escaped := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value)
	return `"` + escaped + `"`
}
