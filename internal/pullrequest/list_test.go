package pullrequest

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
	"github.com/spf13/cobra"
)

// withStateFlag registers cmd's own "state" flag with the same repeatable EnumSliceFlag shape
// listCmd registers in init, so a test can call cmd.Flags().Set("state", ...) exactly like pflag
// would while parsing the real command line.
func withStateFlag(cmd *cobra.Command) {
	cmd.Flags().Var(common.NewEnumSliceFlagWithAllAllowed("declined", "merged", "open", "superseded"), "state", "")
}

func withListOptions(t *testing.T, mutate func()) {
	t.Helper()
	old := listOptions
	t.Cleanup(func() { listOptions = old })
	mutate()
}

// TestListProcess covers listProcess's success, empty-result, API-error, and dry-run paths.
func TestListProcess(t *testing.T) {
	fixture, err := os.ReadFile("../../testdata/pullrequests.json")
	if err != nil {
		t.Fatalf("cannot read testdata: %v", err)
	}

	tests := []struct {
		name          string
		handler       http.HandlerFunc
		dryRun        bool
		wantErrSubstr string
		validate      func(t *testing.T, requests []*http.Request, stdout string)
	}{
		{
			name: "success",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(fixture)
			},
			validate: func(t *testing.T, requests []*http.Request, stdout string) {
				t.Helper()
				if len(requests) != 1 {
					t.Fatalf("expected exactly 1 request, got %d", len(requests))
				}
				wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/pullrequests"
				if requests[0].URL.Path != wantPath {
					t.Errorf("path = %s, want %s", requests[0].URL.Path, wantPath)
				}
				if requests[0].URL.Query().Get("state") != "OPEN" {
					t.Errorf("state query = %s, want OPEN", requests[0].URL.Query().Get("state"))
				}
				var pullrequests []PullRequest
				if err := json.Unmarshal([]byte(stdout), &pullrequests); err != nil {
					t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
				}
				if len(pullrequests) != 3 {
					t.Fatalf("expected 3 pullrequests, got %d", len(pullrequests))
				}
				if pullrequests[0].ID != 1 || pullrequests[1].ID != 2 || pullrequests[2].ID != 3 {
					t.Errorf("pullrequests = %+v, want sorted by id ascending (1, 2, 3)", pullrequests)
				}
			},
		},
		{
			name: "no results",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"values":[]}`))
			},
			validate: func(t *testing.T, requests []*http.Request, stdout string) {
				t.Helper()
				if len(requests) != 1 {
					t.Fatalf("expected exactly 1 request, got %d", len(requests))
				}
				if strings.TrimSpace(stdout) != "" {
					t.Errorf("expected no output when there are no pullrequests, got %q", stdout)
				}
			},
		},
		{
			name: "api error",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"type":"error","error":{"message":"server exploded"}}`))
			},
			wantErrSubstr: "server exploded",
		},
		{
			name:    "dry run",
			handler: func(http.ResponseWriter, *http.Request) {},
			dryRun:  true,
			validate: func(t *testing.T, requests []*http.Request, _ string) {
				t.Helper()
				if len(requests) != 0 {
					t.Errorf("expected no HTTP request in dry-run mode, got %d", len(requests))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withListOptions(t, func() {
				listOptions.Commit = ""
			})

			var requests []*http.Request
			cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
				requests = append(requests, r)
				tt.handler(w, r)
			}, tt.dryRun)

			var err error
			stdout := testutil.CaptureStdout(t, func() {
				err = listProcess(cmd, nil)
			})

			if tt.wantErrSubstr != "" {
				if err == nil {
					t.Fatal("listProcess() expected an error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantErrSubstr) {
					t.Errorf("error = %q, want it to contain %q", err.Error(), tt.wantErrSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("listProcess() error = %v", err)
			}
			if tt.validate != nil {
				tt.validate(t, requests, stdout)
			}
		})
	}
}

// TestListCmdRegistersLimitFlag proves --limit is actually registered on the real "pullrequest
// list" command users invoke, not just plumbing exercised through a synthetic flag on a bare
// cobra.Command in an internal package test.
func TestListCmdRegistersLimitFlag(t *testing.T) {
	if listCmd.Flags().Lookup("limit") == nil {
		t.Fatal(`"pullrequest list" has no --limit flag registered`)
	}
}

// TestListProcessRespectsLimitFlag drives listProcess with a "limit" flag on its cmd (the same
// name and int type listCmd itself registers, read exactly the same way by profile.GetAll/
// profile.ResolvePageLengthAndLimit), proving the value actually reaches GetAll and truncates the
// result.
func TestListProcessRespectsLimitFlag(t *testing.T) {
	withListOptions(t, func() {
		listOptions.Commit = ""
	})

	fixture, err := os.ReadFile("../../testdata/pullrequests.json")
	if err != nil {
		t.Fatalf("cannot read testdata: %v", err)
	}

	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}, false)
	if err := cmd.Flags().Set("limit", "1"); err != nil {
		t.Fatalf("cannot set limit flag: %v", err)
	}

	stdout := testutil.CaptureStdout(t, func() {
		if err := listProcess(cmd, nil); err != nil {
			t.Fatalf("listProcess() error = %v", err)
		}
	})

	var pullrequests []PullRequest
	if err := json.Unmarshal([]byte(stdout), &pullrequests); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if len(pullrequests) != 1 {
		t.Fatalf("expected exactly 1 pullrequest with --limit 1, got %d", len(pullrequests))
	}
}

// TestListProcessSucceedsWithWorkspaceFlagWhenWorkspaceListingIsForbidden proves a token scoped
// only for read:repository+read:pullrequest can still run "pullrequest list --repository X
// --workspace Y": --workspace's root-level EnumFlag must not validate the value by enumerating
// every allowed workspace (an endpoint needing read:workspace) before the command itself --
// which never needs that scope, since the repository is already given explicitly -- ever runs.
// This drives the same shape of flag (a
// common.EnumFlag backed by an AllowedFunc that fails exactly the way an insufficient-scope 403
// would) through cmd.Flags().Set the way pflag itself calls it while parsing the command line, then
// runs listProcess to completion, proving the whole command succeeds despite the workspace-listing
// endpoint being unusable.
func TestListProcessSucceedsWithWorkspaceFlagWhenWorkspaceListingIsForbidden(t *testing.T) {
	withListOptions(t, func() {
		listOptions.Commit = ""
	})

	fixture, err := os.ReadFile("../../testdata/pullrequests.json")
	if err != nil {
		t.Fatalf("cannot read testdata: %v", err)
	}

	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}, false)

	var workspaceListingCalls int
	workspaceFlag := common.NewEnumFlagWithFunc("", func(context.Context, *cobra.Command, []string, string) ([]string, error) {
		workspaceListingCalls++
		return nil, errors.New("Your credentials lack one or more required privilege scopes. (required: read:workspace:bitbucket)")
	})
	cmd.Flags().Var(workspaceFlag, "workspace", "")

	// This is exactly what pflag does while parsing "--workspace acme" on the real
	// command line, before listProcess itself ever runs.
	if err := cmd.Flags().Set("workspace", testutil.FixtureWorkspaceSlug); err != nil {
		t.Fatalf("parsing --workspace failed even though the value was supplied explicitly: %v", err)
	}
	if workspaceListingCalls != 0 {
		t.Fatalf("parsing --workspace called the workspace-listing AllowedFunc %d times, want 0", workspaceListingCalls)
	}

	stdout := testutil.CaptureStdout(t, func() {
		if err := listProcess(cmd, nil); err != nil {
			t.Fatalf("listProcess() error = %v (workspace-listing endpoint being forbidden must not affect a command that never needed it)", err)
		}
	})

	var pullrequests []PullRequest
	if err := json.Unmarshal([]byte(stdout), &pullrequests); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if len(pullrequests) != 3 {
		t.Fatalf("expected 3 pullrequests, got %d", len(pullrequests))
	}
}

// TestListStatesDefaultsWhenFlagRegisteredButNotChanged proves listStates falls back to
// listDefaultState when cmd's --state flag is registered (so Lookup succeeds) but was never set
// on the command line, not just when the flag is entirely absent from cmd.
func TestListStatesDefaultsWhenFlagRegisteredButNotChanged(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	withStateFlag(cmd)

	got := listStates(cmd)
	want := []string{listDefaultState}
	if !slices.Equal(got, want) {
		t.Errorf("listStates() = %v, want %v", got, want)
	}
}

// TestListProcessResolvesStatesOnce proves one listProcess invocation resolves --state exactly once:
// the states naming the listing in the debug/dry-run line are the ones the request was built from,
// not a second read of the flag. A cmd whose "state" flag is registered with the wrong type makes
// that observable -- listStates warns about the mismatch, and a single command run must warn once.
func TestListProcessResolvesStatesOnce(t *testing.T) {
	withListOptions(t, func() {
		listOptions.Commit = ""
	})

	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[]}`))
	}, false)
	// deliberately the wrong type for the "state" flag listStates expects, and explicitly set so the
	// flag counts as Changed and the type assertion is actually reached.
	cmd.Flags().String("state", "", "")
	if err := cmd.Flags().Set("state", "open"); err != nil {
		t.Fatalf("cannot set state flag: %v", err)
	}

	logs := testutil.CaptureLog(t)
	if err := listProcess(cmd, nil); err != nil {
		t.Fatalf("listProcess() error = %v", err)
	}

	if got := strings.Count(logs.String(), "state flag is not an EnumSliceFlag"); got != 1 {
		t.Errorf("mis-registered state flag warned %d times, want exactly 1 per invocation (logs = %q)", got, logs.String())
	}
}

// TestListProcessRepeatableState proves --state can be passed more than once and emits one
// "state=" query parameter per value, in the order given, on top of the "open" default.
func TestListProcessRepeatableState(t *testing.T) {
	withListOptions(t, func() {
		listOptions.Commit = ""
	})

	fixture, err := os.ReadFile("../../testdata/pullrequests.json")
	if err != nil {
		t.Fatalf("cannot read testdata: %v", err)
	}

	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}, false)
	withStateFlag(cmd)
	if err := cmd.Flags().Set("state", "open"); err != nil {
		t.Fatalf("cannot set state flag: %v", err)
	}
	if err := cmd.Flags().Set("state", "merged"); err != nil {
		t.Fatalf("cannot set state flag: %v", err)
	}

	testutil.CaptureStdout(t, func() {
		if err := listProcess(cmd, nil); err != nil {
			t.Fatalf("listProcess() error = %v", err)
		}
	})

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	got := requests[0].URL.Query()["state"]
	want := []string{"OPEN", "MERGED"}
	if !slices.Equal(got, want) {
		t.Errorf("state query values = %v, want %v", got, want)
	}
}

// TestListProcessStateAllExpandsToEveryState proves the legacy "all" value is kept as sugar for
// every allowed state, rather than being dropped as a breaking change.
func TestListProcessStateAllExpandsToEveryState(t *testing.T) {
	withListOptions(t, func() {
		listOptions.Commit = ""
	})

	fixture, err := os.ReadFile("../../testdata/pullrequests.json")
	if err != nil {
		t.Fatalf("cannot read testdata: %v", err)
	}

	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}, false)
	withStateFlag(cmd)
	if err := cmd.Flags().Set("state", "all"); err != nil {
		t.Fatalf("cannot set state flag: %v", err)
	}

	testutil.CaptureStdout(t, func() {
		if err := listProcess(cmd, nil); err != nil {
			t.Fatalf("listProcess() error = %v", err)
		}
	})

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	got := requests[0].URL.Query()["state"]
	want := []string{"DECLINED", "MERGED", "OPEN", "SUPERSEDED"}
	if !slices.Equal(got, want) {
		t.Errorf("state query values = %v, want %v", got, want)
	}
}

// TestStateFlagRejectsInvalidValue proves an unrecognized --state value is rejected at parse
// time, on the real "pullrequest list" command's own --state flag.
func TestStateFlagRejectsInvalidValue(t *testing.T) {
	if err := listCmd.Flags().Set("state", "bogus"); err == nil {
		t.Fatal("expected an error setting an invalid --state value")
	}
}

// TestListProcessSourceDestinationFilters proves --source/--destination emit AND-joined
// "source.branch.name="/"destination.branch.name=" clauses in the "q=" query parameter.
func TestListProcessSourceDestinationFilters(t *testing.T) {
	withListOptions(t, func() {
		listOptions.Commit = ""
	})

	fixture, err := os.ReadFile("../../testdata/pullrequests.json")
	if err != nil {
		t.Fatalf("cannot read testdata: %v", err)
	}

	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}, false)
	cmd.Flags().String("source", "", "")
	cmd.Flags().String("destination", "", "")
	if err := cmd.Flags().Set("source", "feature/x"); err != nil {
		t.Fatalf("cannot set source flag: %v", err)
	}
	if err := cmd.Flags().Set("destination", "master"); err != nil {
		t.Fatalf("cannot set destination flag: %v", err)
	}

	testutil.CaptureStdout(t, func() {
		if err := listProcess(cmd, nil); err != nil {
			t.Fatalf("listProcess() error = %v", err)
		}
	})

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	want := `(source.branch.name="feature/x") AND (destination.branch.name="master")`
	if got := requests[0].URL.Query().Get("q"); got != want {
		t.Errorf("q query = %q, want %q", got, want)
	}
}

// TestListProcessComposesStateQueryAndBranchFilters proves --state, --query, --source, and
// --destination all compose into the same request together.
func TestListProcessComposesStateQueryAndBranchFilters(t *testing.T) {
	withListOptions(t, func() {
		listOptions.Commit = ""
	})

	fixture, err := os.ReadFile("../../testdata/pullrequests.json")
	if err != nil {
		t.Fatalf("cannot read testdata: %v", err)
	}

	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}, false)
	withStateFlag(cmd)
	cmd.Flags().String("query", "", "")
	cmd.Flags().String("source", "", "")
	cmd.Flags().String("destination", "", "")
	if err := cmd.Flags().Set("state", "merged"); err != nil {
		t.Fatalf("cannot set state flag: %v", err)
	}
	if err := cmd.Flags().Set("state", "declined"); err != nil {
		t.Fatalf("cannot set state flag: %v", err)
	}
	if err := cmd.Flags().Set("query", "updated_on > 2025-01-01"); err != nil {
		t.Fatalf("cannot set query flag: %v", err)
	}
	if err := cmd.Flags().Set("source", "feature/x"); err != nil {
		t.Fatalf("cannot set source flag: %v", err)
	}
	if err := cmd.Flags().Set("destination", "master"); err != nil {
		t.Fatalf("cannot set destination flag: %v", err)
	}

	testutil.CaptureStdout(t, func() {
		if err := listProcess(cmd, nil); err != nil {
			t.Fatalf("listProcess() error = %v", err)
		}
	})

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	gotStates := requests[0].URL.Query()["state"]
	wantStates := []string{"MERGED", "DECLINED"}
	if !slices.Equal(gotStates, wantStates) {
		t.Errorf("state query values = %v, want %v", gotStates, wantStates)
	}
	wantQ := `(updated_on > 2025-01-01) AND (source.branch.name="feature/x") AND (destination.branch.name="master")`
	if gotQ := requests[0].URL.Query().Get("q"); gotQ != wantQ {
		t.Errorf("q query = %q, want %q", gotQ, wantQ)
	}
}

// TestListProcessBranchFilterEscapesQuotes proves a branch name containing a double quote or
// backslash is escaped rather than breaking the emitted "q=" filter's quoting.
func TestListProcessBranchFilterEscapesQuotes(t *testing.T) {
	withListOptions(t, func() {
		listOptions.Commit = ""
	})

	fixture, err := os.ReadFile("../../testdata/pullrequests.json")
	if err != nil {
		t.Fatalf("cannot read testdata: %v", err)
	}

	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}, false)
	cmd.Flags().String("source", "", "")
	cmd.Flags().String("destination", "", "")
	if err := cmd.Flags().Set("source", `feature/"quoted"\branch`); err != nil {
		t.Fatalf("cannot set source flag: %v", err)
	}

	testutil.CaptureStdout(t, func() {
		if err := listProcess(cmd, nil); err != nil {
			t.Fatalf("listProcess() error = %v", err)
		}
	})

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	want := `source.branch.name="feature/\"quoted\"\\branch"`
	if got := requests[0].URL.Query().Get("q"); got != want {
		t.Errorf("q query = %q, want %q", got, want)
	}
}

// TestListProcessRejectsPathTraversalInCommit proves --commit is guarded by
// common.ValidatePathIdentifier before it ever reaches repository.GetPath: a value carrying a "/"
// must be rejected with zero HTTP requests issued, not silently spliced into the request path.
func TestListProcessRejectsPathTraversalInCommit(t *testing.T) {
	withListOptions(t, func() {
		listOptions.Commit = "../../../otherws/otherrepo/pullrequests"
	})

	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[]}`))
	}, false)

	err := listProcess(cmd, nil)
	if err == nil {
		t.Fatal("listProcess() expected an error for a path-traversal --commit value, got nil")
	}
	if !strings.Contains(err.Error(), "commit") {
		t.Errorf("error = %q, want it to name the commit argument", err.Error())
	}
	if len(requests) != 0 {
		t.Errorf("expected no HTTP request for an invalid --commit value, got %d", len(requests))
	}
}

// TestListProcessCommitScopedListing covers the --commit arm's success path: the repository-scoped
// by-commit endpoint, which takes no filter parameters of its own (--commit is mutually exclusive
// with --state/--query/--source/--destination), so the request carries no query string at all.
func TestListProcessCommitScopedListing(t *testing.T) {
	withListOptions(t, func() {
		listOptions.Commit = "ae86d5323477989fab3bf3879cd1234543565753"
	})

	fixture := loadPullRequestsFixture(t)

	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}, false)

	stdout := testutil.CaptureStdout(t, func() {
		if err := listProcess(cmd, nil); err != nil {
			t.Fatalf("listProcess() error = %v", err)
		}
	})

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/commit/ae86d5323477989fab3bf3879cd1234543565753/pullrequests"
	if requests[0].URL.Path != wantPath {
		t.Errorf("path = %s, want %s", requests[0].URL.Path, wantPath)
	}
	if got := requests[0].URL.RawQuery; got != "" {
		t.Errorf("query = %q, want none on the by-commit endpoint", got)
	}

	var pullrequests []PullRequest
	if err := json.Unmarshal([]byte(stdout), &pullrequests); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if len(pullrequests) != 3 {
		t.Fatalf("expected 3 pullrequests, got %d", len(pullrequests))
	}
}

// TestListProcessAuthorModeIgnoresCommit pins what a direct listProcess call with both --author and
// --commit set actually does. Cobra rejects the combination at parse time, so a real invocation never
// reaches this, but the dispatch has to be deterministic: author mode wins and the --commit value is
// dropped, rather than the commit path silently winning and the workspace-wide scope the user asked
// for being lost.
func TestListProcessAuthorModeIgnoresCommit(t *testing.T) {
	withListOptions(t, func() {
		listOptions.Commit = "ae86d5323477989fab3bf3879cd1234543565753"
	})

	fixture := loadPullRequestsFixture(t)

	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}, false)
	withAuthorModeFlags(cmd)
	setFixtureWorkspace(t, cmd)
	setAuthorFlag(t, cmd, "{11111111-1111-1111-1111-111111111111}")

	testutil.CaptureStdout(t, func() {
		if err := listProcess(cmd, nil); err != nil {
			t.Fatalf("listProcess() error = %v", err)
		}
	})

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	wantPath := "/2.0/workspaces/acme/pullrequests/%7B11111111-1111-1111-1111-111111111111%7D"
	if got := requests[0].URL.EscapedPath(); got != wantPath {
		t.Errorf("request path = %s, want the author-mode path %s", got, wantPath)
	}
}

// setRealListFlag sets name to value on the real listCmd singleton and registers a t.Cleanup that
// fully restores the flag's prior state -- both its Value (via DefValue) and its Changed bit --
// so a test driving the singleton directly (rather than a throwaway re-declaration) never leaks
// state into any test that runs afterward in the same binary.
func setRealListFlag(t *testing.T, name, value string) {
	t.Helper()
	flag := listCmd.Flags().Lookup(name)
	if flag == nil {
		t.Fatalf("listCmd has no --%s flag registered", name)
	}
	wasChanged := flag.Changed
	previous := flag.Value.String()
	if err := listCmd.Flags().Set(name, value); err != nil {
		t.Fatalf("cannot set --%s flag: %v", name, err)
	}
	t.Cleanup(func() {
		_ = flag.Value.Set(previous)
		flag.Changed = wasChanged
	})
}

// TestListCmdRepositoryColumnOnRepositoryScopedList proves the "repository" column is a first-class
// member of the shared column table rather than an author-mode-only addition: --columns repository
// and --sort repository are accepted and honored by the REAL listCmd on the repository-scoped
// listing too, where the column merely is not part of the defaults.
func TestListCmdRepositoryColumnOnRepositoryScopedList(t *testing.T) {
	if !slices.Contains(columns.Columns(), "repository") {
		t.Fatalf("columns table = %v, want it to declare a \"repository\" column", columns.Columns())
	}

	setRealColumnsFlag(t, "repository")
	setRealListFlag(t, "sort", "repository")

	pr := PullRequest{Destination: Endpoint{Repository: &repository.Repository{FullName: "acme/widgets"}}}
	headers := pr.GetHeaders(listCmd)
	if !slices.Equal(headers, []string{"repository"}) {
		t.Fatalf("GetHeaders() = %v, want [repository] from --columns repository", headers)
	}
	if row := pr.GetRow(headers); !slices.Equal(row, []string{"acme/widgets"}) {
		t.Errorf("GetRow() = %v, want [acme/widgets]", row)
	}

	if sortValue := common.SortFlagValue(listCmd); sortValue != "repository" {
		t.Fatalf("SortFlagValue() = %q, want repository", sortValue)
	}
	pullrequests := []PullRequest{
		{ID: 1, Destination: Endpoint{Repository: &repository.Repository{FullName: "acme/Zebra"}}},
		{ID: 2},
		{ID: 3, Destination: Endpoint{Repository: &repository.Repository{FullName: "acme/apples"}}},
	}
	common.Sort(pullrequests, columns.SortBy("repository"))
	// a pull request whose payload carried no destination repository sorts as an empty full name
	// (first) instead of panicking, then the rest sort case-insensitively
	ids := []uint64{pullrequests[0].ID, pullrequests[1].ID, pullrequests[2].ID}
	if !slices.Equal(ids, []uint64{2, 3, 1}) {
		t.Errorf("sorted ids = %v, want [2 3 1]", ids)
	}
}

// setRealColumnsFlag sets --columns to value on the real listCmd singleton, restoring both the
// EnumSliceFlag's accumulated Values and the flag's Changed bit afterwards. The generic
// setRealListFlag cannot be used here: EnumSliceFlag.Set appends rather than replaces, and its
// String() is a bracketed representation Set does not accept back.
func setRealColumnsFlag(t *testing.T, value string) {
	t.Helper()
	flag := listCmd.Flags().Lookup("columns")
	if flag == nil {
		t.Fatal("listCmd has no --columns flag registered")
	}
	enum, ok := flag.Value.(*common.EnumSliceFlag)
	if !ok {
		t.Fatalf("--columns flag value is %T, want *common.EnumSliceFlag", flag.Value)
	}
	previous, wasChanged := slices.Clone(enum.Values), flag.Changed
	t.Cleanup(func() {
		enum.Values = previous
		flag.Changed = wasChanged
	})
	if err := listCmd.Flags().Set("columns", value); err != nil {
		t.Fatalf("cannot set --columns flag: %v", err)
	}
}

// TestListCmdRealRegistration proves the REAL listCmd singleton (not a throwaway command
// re-declaring the same flags) actually registers every flag listProcess's tests drive through a
// synthetic command, and enforces every mutual-exclusivity pair among them -- a guard against the
// flag tests above passing even if listCmd's own init() registration were changed or dropped.
// Exclusivity cannot be exercised through listProcess: cobra validates flag groups in
// ValidateFlagGroups during Execute, never in RunE.
//
// Each subtest asserts the error NAMES both flags of the pair under test. ValidateFlagGroups
// validates every group registered on the shared singleton, so a bare "got an error" assertion would
// also be satisfied by an unrelated pair left conflicting by some other test -- proving nothing about
// the pair the subtest set.
func TestListCmdRealRegistration(t *testing.T) {
	for _, name := range []string{"state", "source", "destination", "commit", "query", "author", "mine"} {
		if listCmd.Flags().Lookup(name) == nil {
			t.Errorf("listCmd has no --%s flag registered", name)
		}
	}

	pairs := []struct{ first, second string }{
		{"commit", "state"},
		{"commit", "query"},
		{"commit", "source"},
		{"commit", "destination"},
		{"author", "mine"},
		{"commit", "author"},
		{"commit", "mine"},
	}
	for _, pair := range pairs {
		t.Run(pair.first+"-vs-"+pair.second, func(t *testing.T) {
			old := listOptions
			t.Cleanup(func() { listOptions = old })

			setRealListFlag(t, pair.first, exclusivityValueFor(pair.first))
			setRealListFlag(t, pair.second, exclusivityValueFor(pair.second))

			err := listCmd.ValidateFlagGroups()
			if err == nil {
				t.Fatalf("ValidateFlagGroups() = nil, want an error for --%s with --%s", pair.first, pair.second)
			}
			for _, name := range []string{pair.first, pair.second} {
				if !strings.Contains(err.Error(), name) {
					t.Errorf("ValidateFlagGroups() error = %q, want it to name --%s (the reported conflict may be another pair's)", err.Error(), name)
				}
			}
		})
	}
}

// exclusivityValueFor returns a valid value to set the named listCmd flag to when only "some value"
// is needed, as in the mutual-exclusivity pairs above.
func exclusivityValueFor(name string) string {
	switch name {
	case "mine":
		return "true"
	case "state":
		return "open"
	default:
		return "x"
	}
}
