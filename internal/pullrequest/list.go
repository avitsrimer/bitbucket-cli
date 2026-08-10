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
	// workspace nicknames, which this endpoint does not accept, so completing them would only
	// manufacture 404s.
	listCmd.Flags().String("author", "", "List pull requests authored by this user across every repository of the workspace (a UUID in braces or an Atlassian account ID)")
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

func listProcess(cmd *cobra.Command, args []string) (err error) {
	states := listStates(cmd)

	// author mode is resolved first and never touches repository resolution, so `--author`/`--mine`
	// work from any directory as long as the workspace resolves. Flag exclusivity is enforced at
	// parse time only, so a direct listProcess call with both --author and --commit set
	// deterministically takes this arm (and the --repository guard inside it still errors out).
	author, mine, authorMode := authorModeValue(cmd)

	var uripath, target string
	if authorMode {
		uripath, target, err = listAuthorRequest(cmd, states, author, mine)
	} else {
		uripath, target, err = listRepositoryRequest(cmd, states)
	}
	if err != nil {
		return err
	}

	lgr.Printf("[DEBUG] listing %s pull requests for %s", strings.Join(states, ","), target)
	if !common.WhatIf(cmd, fmt.Sprintf("Showing %s pull requests for %s", strings.Join(states, ","), target)) {
		return nil
	}

	pullrequests, err := profile.GetAll[PullRequest](cmd.Context(), cmd, uripath)
	if err != nil {
		// A 404 in author mode almost always means --author carried a value the endpoint does not
		// accept, so it is worth spelling out which forms it does. --mine resolves the identifier
		// itself, so a 404 there is about the workspace, not the author, and is left unwrapped.
		if author != "" && profile.IsNotFound(err) {
			return fmt.Errorf("cannot list pull requests for author %s: %w\n"+
				"--author accepts a UUID in braces (e.g. {01234567-89ab-cdef-0123-456789abcdef}) or an Atlassian account ID; "+
				"Bitbucket usernames are mostly defunct and the nicknames `bb workspace members` shows are not accepted -- "+
				"that command's ID column carries the UUID this flag wants. Use --mine for your own pull requests", author, err)
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
func authorModeValue(cmd *cobra.Command) (author string, mine, ok bool) {
	if cmd == nil {
		return "", false, false
	}
	author = common.StringFlagValue(cmd, "author")
	if cmd.Flags().Lookup("mine") != nil {
		mine, _ = cmd.Flags().GetBool("mine")
	}
	return author, mine, author != "" || mine
}

// listRepositoryRequest builds the repository-scoped request: the commit form when --commit is set,
// the plain pullrequests listing otherwise. target is the human-readable subject of the debug and
// dry-run lines.
func listRepositoryRequest(cmd *cobra.Command, states []string) (uripath, target string, err error) {
	repo, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return "", "", fmt.Errorf("cannot get repository: %w", err)
	}
	target = fmt.Sprintf("repository: %s", repo)

	if listOptions.Commit != "" {
		if err = common.ValidatePathIdentifier("commit", listOptions.Commit); err != nil {
			return "", "", fmt.Errorf("cannot list pull requests: %w", err)
		}
		return repo.GetPath("commit", listOptions.Commit, "pullrequests"), target, nil
	}
	return repo.GetPath("pullrequests") + "?" + listQuery(cmd, states).Encode(), target, nil
}

// listAuthorRequest builds the workspace-wide, author-scoped request. No repository is resolved
// here at all, so the command works from any directory whose workspace resolves.
func listAuthorRequest(cmd *cobra.Command, states []string, author string, mine bool) (uripath, target string, err error) {
	// --repository is a root persistent flag, so init()-time MarkFlagsMutuallyExclusive cannot pair
	// it with --author/--mine; the conflict is caught here instead. Author mode lists across the
	// whole workspace, so an explicitly-passed --repository would otherwise be silently ignored.
	if flag := cmd.Flags().Lookup("repository"); flag != nil && flag.Changed {
		return "", "", errors.New("--repository cannot be combined with --author or --mine: author mode lists pull requests across every repository of the workspace")
	}

	workspaceName, err := workspace.GetWorkspaceName(cmd.Context(), cmd)
	if err != nil {
		return "", "", fmt.Errorf("cannot get workspace: %w", err)
	}

	selected := author
	if mine {
		// --mine issues GET /user (cached in user.UserCache) before the WhatIf gate, exactly as the
		// repository-scoped branch resolves its repository first: the dry-run line names the author
		// it would query, and resolving it is itself a read.
		me, meErr := user.GetMe(cmd.Context(), cmd)
		if meErr != nil {
			return "", "", fmt.Errorf("cannot get current user: %w", meErr)
		}
		// common.UUID.String() already wraps the uuid in the braces this endpoint requires; adding
		// another pair would double them.
		selected = me.ID.String()
	} else if err = common.ValidatePathIdentifier("author", selected); err != nil {
		return "", "", fmt.Errorf("cannot list pull requests: %w", err)
	}

	// PathEscape on top of ValidatePathIdentifier: the latter rejects path separators and dot
	// segments but not "?" or "#", and resolveRequestURL splits the uripath on "?" to take
	// everything after it as the raw query -- so an unescaped "?" in the author value would replace
	// our own state/q parameters wholesale.
	uripath = "/workspaces/" + workspaceName + "/pullrequests/" + url.PathEscape(selected)
	return uripath + "?" + listQuery(cmd, states).Encode(), fmt.Sprintf("workspace: %s, author: %s", workspaceName, selected), nil
}

// listQuery builds the query parameters shared by the repository-scoped and author-scoped listings:
// one "state" value per resolved state, plus the "q" filter when --query/--source/--destination
// contributed one.
func listQuery(cmd *cobra.Command, states []string) url.Values {
	query := url.Values{}
	for _, state := range states {
		query.Add("state", strings.ToUpper(state))
	}
	if filter := listQueryFilter(cmd); filter != "" {
		query.Set("q", filter)
	}
	return query
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
