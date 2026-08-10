package pullrequest

import (
	"encoding/json"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
	"github.com/spf13/cobra"
)

// withAuthorFlags registers cmd's own --author/--mine flags with the same names and types listCmd
// registers in init, plus the --workspace flag author mode resolves the workspace from (root
// registers it on the real command, and testutil.SetupProfile deliberately does not).
func withAuthorFlags(cmd *cobra.Command) {
	cmd.Flags().String("author", "", "")
	cmd.Flags().Bool("mine", false, "")
	cmd.Flags().String("workspace", "", "")
}

// setAuthorFlag sets --author on cmd, failing the test if pflag rejects the value the way it would
// on the real command line.
func setAuthorFlag(t *testing.T, cmd *cobra.Command, value string) {
	t.Helper()
	if err := cmd.Flags().Set("author", value); err != nil {
		t.Fatalf("cannot set author flag: %v", err)
	}
}

// setFixtureWorkspace sets --workspace to the shared fixture workspace slug on cmd.
func setFixtureWorkspace(t *testing.T, cmd *cobra.Command) {
	t.Helper()
	if err := cmd.Flags().Set("workspace", testutil.FixtureWorkspaceSlug); err != nil {
		t.Fatalf("cannot set workspace flag: %v", err)
	}
}

// loadPullRequestsFixture reads the shared pull request list payload every author-mode test serves.
func loadPullRequestsFixture(t *testing.T) []byte {
	t.Helper()
	fixture, err := os.ReadFile("../../testdata/pullrequests.json")
	if err != nil {
		t.Fatalf("cannot read testdata: %v", err)
	}
	return fixture
}

// TestListProcessAuthorMode proves --author switches listProcess to the workspace-wide
// GET /workspaces/{workspace}/pullrequests/{selected_user} endpoint, with the author value placed
// in the path in its percent-encoded form and the existing --state/--query/--source filters
// composing exactly as they do on the repository-scoped listing.
func TestListProcessAuthorMode(t *testing.T) {
	fixture := loadPullRequestsFixture(t)

	tests := []struct {
		name         string
		author       string
		states       []string
		query        string
		source       string
		wantPath     string
		wantStates   []string
		wantQ        string
		wantPRsCount int
	}{
		{
			name:         "atlassian account id",
			author:       "557058:11111111-2222-3333-4444-555555555555",
			wantPath:     "/2.0/workspaces/acme/pullrequests/557058:11111111-2222-3333-4444-555555555555",
			wantStates:   []string{"OPEN"},
			wantPRsCount: 2,
		},
		{
			name:         "braced uuid",
			author:       "{11111111-1111-1111-1111-111111111111}",
			wantPath:     "/2.0/workspaces/acme/pullrequests/%7B11111111-1111-1111-1111-111111111111%7D",
			wantStates:   []string{"OPEN"},
			wantPRsCount: 2,
		},
		{
			name:         "repeated state",
			author:       "{11111111-1111-1111-1111-111111111111}",
			states:       []string{"merged", "declined"},
			wantPath:     "/2.0/workspaces/acme/pullrequests/%7B11111111-1111-1111-1111-111111111111%7D",
			wantStates:   []string{"MERGED", "DECLINED"},
			wantPRsCount: 2,
		},
		{
			name:         "query and source compose",
			author:       "{11111111-1111-1111-1111-111111111111}",
			query:        "updated_on > 2025-01-01",
			source:       "feature/x",
			wantPath:     "/2.0/workspaces/acme/pullrequests/%7B11111111-1111-1111-1111-111111111111%7D",
			wantStates:   []string{"OPEN"},
			wantQ:        `(updated_on > 2025-01-01) AND (source.branch.name="feature/x")`,
			wantPRsCount: 2,
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
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(fixture)
			}, false)
			withAuthorFlags(cmd)
			withStateFlag(cmd)
			cmd.Flags().String("query", "", "")
			cmd.Flags().String("source", "", "")
			cmd.Flags().String("destination", "", "")
			setFixtureWorkspace(t, cmd)
			setAuthorFlag(t, cmd, tt.author)
			for _, state := range tt.states {
				if err := cmd.Flags().Set("state", state); err != nil {
					t.Fatalf("cannot set state flag: %v", err)
				}
			}
			if tt.query != "" {
				if err := cmd.Flags().Set("query", tt.query); err != nil {
					t.Fatalf("cannot set query flag: %v", err)
				}
			}
			if tt.source != "" {
				if err := cmd.Flags().Set("source", tt.source); err != nil {
					t.Fatalf("cannot set source flag: %v", err)
				}
			}

			stdout := testutil.CaptureStdout(t, func() {
				if err := listProcess(cmd, nil); err != nil {
					t.Fatalf("listProcess() error = %v", err)
				}
			})

			if len(requests) != 1 {
				t.Fatalf("expected exactly 1 request, got %d", len(requests))
			}
			// EscapedPath, not Path: an httptest handler's r.URL.Path is already decoded, so it
			// cannot tell an escaped author segment from a raw one.
			if got := requests[0].URL.EscapedPath(); got != tt.wantPath {
				t.Errorf("request path = %s, want %s", got, tt.wantPath)
			}
			if got := requests[0].URL.Query()["state"]; !slices.Equal(got, tt.wantStates) {
				t.Errorf("state query values = %v, want %v", got, tt.wantStates)
			}
			if got := requests[0].URL.Query().Get("q"); got != tt.wantQ {
				t.Errorf("q query = %q, want %q", got, tt.wantQ)
			}

			var pullrequests []PullRequest
			if err := json.Unmarshal([]byte(stdout), &pullrequests); err != nil {
				t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
			}
			if len(pullrequests) != tt.wantPRsCount {
				t.Fatalf("expected %d pullrequests, got %d", tt.wantPRsCount, len(pullrequests))
			}
		})
	}
}

// TestListProcessMineResolvesCurrentUser proves --mine resolves the author through GET /user and
// uses the returned UUID verbatim as the path segment -- common.UUID.String() already carries the
// braces the endpoint requires, so the escaped segment must show exactly one pair of them.
func TestListProcessMineResolvesCurrentUser(t *testing.T) {
	withListOptions(t, func() {
		listOptions.Commit = ""
	})

	fixture := loadPullRequestsFixture(t)

	var requests []*http.Request
	cmd := setupTestNamed(t, "test-mine", func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/2.0/user" {
			_, _ = w.Write([]byte(`{"type":"user","uuid":"{33333333-3333-3333-3333-333333333333}","display_name":"Me"}`))
			return
		}
		_, _ = w.Write(fixture)
	}, false)
	withAuthorFlags(cmd)
	setFixtureWorkspace(t, cmd)
	if err := cmd.Flags().Set("mine", "true"); err != nil {
		t.Fatalf("cannot set mine flag: %v", err)
	}

	stdout := testutil.CaptureStdout(t, func() {
		if err := listProcess(cmd, nil); err != nil {
			t.Fatalf("listProcess() error = %v", err)
		}
	})

	if len(requests) != 2 {
		t.Fatalf("expected 2 requests (GET /user then the listing), got %d", len(requests))
	}
	if requests[0].URL.Path != "/2.0/user" {
		t.Fatalf("first request path = %s, want /2.0/user", requests[0].URL.Path)
	}
	wantPath := "/2.0/workspaces/acme/pullrequests/%7B33333333-3333-3333-3333-333333333333%7D"
	if got := requests[1].URL.EscapedPath(); got != wantPath {
		t.Errorf("listing path = %s, want %s (braces must not be doubled)", got, wantPath)
	}
	if got := requests[1].URL.Query()["state"]; !slices.Equal(got, []string{"OPEN"}) {
		t.Errorf("state query values = %v, want [OPEN]", got)
	}

	var pullrequests []PullRequest
	if err := json.Unmarshal([]byte(stdout), &pullrequests); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if len(pullrequests) != 2 {
		t.Fatalf("expected 2 pullrequests, got %d", len(pullrequests))
	}
}

// TestListProcessAuthorEscapesQueryInjection proves an --author value carrying a "?" cannot take
// over the request's query string. ValidatePathIdentifier alone would let it through (it rejects
// path separators and dot segments, not "?"), and resolveRequestURL splits the uripath on the first
// "?" to build RawQuery -- so without url.PathEscape the attacker's "state=MERGED" would replace
// our own state/q parameters wholesale.
func TestListProcessAuthorEscapesQueryInjection(t *testing.T) {
	withListOptions(t, func() {
		listOptions.Commit = ""
	})

	fixture := loadPullRequestsFixture(t)

	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}, false)
	withAuthorFlags(cmd)
	setFixtureWorkspace(t, cmd)
	setAuthorFlag(t, cmd, "victim?state=MERGED")

	testutil.CaptureStdout(t, func() {
		if err := listProcess(cmd, nil); err != nil {
			t.Fatalf("listProcess() error = %v", err)
		}
	})

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	if got := requests[0].URL.EscapedPath(); got != "/2.0/workspaces/acme/pullrequests/victim%3Fstate=MERGED" {
		t.Errorf("request path = %s, want the author's %q escaped into the path segment", got, "?")
	}
	if got := requests[0].URL.Query()["state"]; !slices.Equal(got, []string{"OPEN"}) {
		t.Errorf("state query values = %v, want [OPEN] (our own state parameter must survive)", got)
	}
	if strings.Contains(requests[0].RequestURI, "?state=MERGED") {
		t.Errorf("RequestURI = %s, want the injected query not to appear as a real query parameter", requests[0].RequestURI)
	}
}

// TestListProcessAuthorModeNeedsNoRepository proves author mode never resolves a repository: the
// --repository flag is emptied (without being marked as explicitly set, which would trip the guard
// instead), so repository.GetRepository could not possibly succeed, yet the command still runs.
func TestListProcessAuthorModeNeedsNoRepository(t *testing.T) {
	withListOptions(t, func() {
		listOptions.Commit = ""
	})

	fixture := loadPullRequestsFixture(t)

	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}, false)
	withAuthorFlags(cmd)
	setFixtureWorkspace(t, cmd)
	setAuthorFlag(t, cmd, "{11111111-1111-1111-1111-111111111111}")
	// Value.Set rather than Flags().Set: the latter would mark the flag Changed, which is exactly
	// the conflict the runtime guard rejects.
	if err := cmd.Flags().Lookup("repository").Value.Set(""); err != nil {
		t.Fatalf("cannot clear repository flag value: %v", err)
	}

	testutil.CaptureStdout(t, func() {
		if err := listProcess(cmd, nil); err != nil {
			t.Fatalf("listProcess() error = %v (author mode must not resolve a repository)", err)
		}
	})

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
}

// TestListProcessAuthorModeDryRun proves the plain WhatIf gate still short-circuits author mode
// before the listing request, and that the dry-run line names the workspace and author instead of a
// repository.
func TestListProcessAuthorModeDryRun(t *testing.T) {
	withListOptions(t, func() {
		listOptions.Commit = ""
	})

	var requests []*http.Request
	cmd := setupTest(t, func(_ http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
	}, true)
	withAuthorFlags(cmd)
	setFixtureWorkspace(t, cmd)
	setAuthorFlag(t, cmd, "{11111111-1111-1111-1111-111111111111}")

	stderr := testutil.CaptureStderr(t, func() {
		if err := listProcess(cmd, nil); err != nil {
			t.Fatalf("listProcess() error = %v", err)
		}
	})

	if len(requests) != 0 {
		t.Errorf("expected no HTTP request in dry-run mode, got %d", len(requests))
	}
	want := "Showing open pull requests for workspace: acme, author: {11111111-1111-1111-1111-111111111111}"
	if !strings.Contains(stderr, want) {
		t.Errorf("dry-run output = %q, want it to contain %q", stderr, want)
	}
}

// TestListProcessAuthorModeErrors covers author mode's four failure paths, each of which must abort
// before any listing request is issued.
func TestListProcessAuthorModeErrors(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(t *testing.T, cmd *cobra.Command)
		wantErrSubstr string
	}{
		{
			name: "author with path separator",
			setup: func(t *testing.T, cmd *cobra.Command) {
				t.Helper()
				setFixtureWorkspace(t, cmd)
				setAuthorFlag(t, cmd, "../../otherws/otherrepo/pullrequests")
			},
			wantErrSubstr: "argument author is invalid",
		},
		{
			name: "unresolvable workspace",
			setup: func(t *testing.T, cmd *cobra.Command) {
				t.Helper()
				setAuthorFlag(t, cmd, "{11111111-1111-1111-1111-111111111111}")
			},
			wantErrSubstr: "cannot get workspace",
		},
		{
			name: "explicit repository with mine",
			setup: func(t *testing.T, cmd *cobra.Command) {
				t.Helper()
				setFixtureWorkspace(t, cmd)
				if err := cmd.Flags().Set("mine", "true"); err != nil {
					t.Fatalf("cannot set mine flag: %v", err)
				}
				if err := cmd.Flags().Set("repository", testutil.FixtureRepositoryFlag); err != nil {
					t.Fatalf("cannot set repository flag: %v", err)
				}
			},
			wantErrSubstr: "--repository cannot be combined with --author or --mine",
		},
		{
			name: "explicit repository with author",
			setup: func(t *testing.T, cmd *cobra.Command) {
				t.Helper()
				setFixtureWorkspace(t, cmd)
				setAuthorFlag(t, cmd, "{11111111-1111-1111-1111-111111111111}")
				if err := cmd.Flags().Set("repository", testutil.FixtureRepositoryFlag); err != nil {
					t.Fatalf("cannot set repository flag: %v", err)
				}
			},
			wantErrSubstr: "--repository cannot be combined with --author or --mine",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withListOptions(t, func() {
				listOptions.Commit = ""
			})

			cmd := setupTest(t, testutil.FailIfCalled(t, "an invalid author-mode invocation"), false)
			withAuthorFlags(cmd)
			tt.setup(t, cmd)

			err := listProcess(cmd, nil)
			if err == nil {
				t.Fatal("listProcess() expected an error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErrSubstr) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tt.wantErrSubstr)
			}
		})
	}
}

// TestListProcessAuthorNotFoundExplainsAcceptedForms proves a 404 from the author endpoint is
// wrapped with guidance about which author forms the endpoint accepts -- the common case is an
// --author value copied from `bb workspace members`' nickname column, which it does not.
func TestListProcessAuthorNotFoundExplainsAcceptedForms(t *testing.T) {
	withListOptions(t, func() {
		listOptions.Commit = ""
	})

	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"No such user"}}`))
	}, false)
	withAuthorFlags(cmd)
	setFixtureWorkspace(t, cmd)
	setAuthorFlag(t, cmd, "jsmith")

	err := listProcess(cmd, nil)
	if err == nil {
		t.Fatal("listProcess() expected an error for a 404 author, got nil")
	}
	for _, want := range []string{"jsmith", "No such user", "UUID in braces", "Atlassian account ID", "bb workspace members", "--mine"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to mention %q", err.Error(), want)
		}
	}
}

// TestListProcessAuthorNotFoundLeavesOtherErrorsAlone proves the accepted-forms guidance is only
// attached to a 404: a 403 (the shape an insufficiently-scoped token produces on this
// workspace-level endpoint) surfaces unchanged.
func TestListProcessAuthorNotFoundLeavesOtherErrorsAlone(t *testing.T) {
	withListOptions(t, func() {
		listOptions.Commit = ""
	})

	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"Your credentials lack required scopes"}}`))
	}, false)
	withAuthorFlags(cmd)
	setFixtureWorkspace(t, cmd)
	setAuthorFlag(t, cmd, "jsmith")

	err := listProcess(cmd, nil)
	if err == nil {
		t.Fatal("listProcess() expected an error for a 403, got nil")
	}
	if !strings.Contains(err.Error(), "Your credentials lack required scopes") {
		t.Errorf("error = %q, want the API message", err.Error())
	}
	if strings.Contains(err.Error(), "UUID in braces") {
		t.Errorf("error = %q, want no accepted-author-forms guidance on a non-404", err.Error())
	}
}

// TestAuthorModeValue pins the shared author-mode predicate both listProcess and
// PullRequest.GetHeaders read. The "flags registered but unset" case is the important one: reading
// --mine through flag.Value.String() instead of GetBool would see "false" -- non-empty -- and flip
// every plain `pullrequest list`/`get` into author mode.
func TestAuthorModeValue(t *testing.T) {
	tests := []struct {
		name       string
		cmd        func() *cobra.Command
		wantAuthor string
		wantMine   bool
		wantOK     bool
	}{
		{
			name: "nil cmd",
			cmd:  func() *cobra.Command { return nil },
		},
		{
			name: "flags not registered",
			cmd:  func() *cobra.Command { return &cobra.Command{Use: "get"} },
		},
		{
			name: "flags registered but unset",
			cmd: func() *cobra.Command {
				cmd := &cobra.Command{Use: "list"}
				withAuthorFlags(cmd)
				return cmd
			},
		},
		{
			name: "author set",
			cmd: func() *cobra.Command {
				cmd := &cobra.Command{Use: "list"}
				withAuthorFlags(cmd)
				_ = cmd.Flags().Set("author", "jsmith")
				return cmd
			},
			wantAuthor: "jsmith",
			wantOK:     true,
		},
		{
			name: "mine set",
			cmd: func() *cobra.Command {
				cmd := &cobra.Command{Use: "list"}
				withAuthorFlags(cmd)
				_ = cmd.Flags().Set("mine", "true")
				return cmd
			},
			wantMine: true,
			wantOK:   true,
		},
		{
			name: "mine explicitly false",
			cmd: func() *cobra.Command {
				cmd := &cobra.Command{Use: "list"}
				withAuthorFlags(cmd)
				_ = cmd.Flags().Set("mine", "false")
				return cmd
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			author, mine, ok := authorModeValue(tt.cmd())
			if author != tt.wantAuthor || mine != tt.wantMine || ok != tt.wantOK {
				t.Errorf("authorModeValue() = (%q, %t, %t), want (%q, %t, %t)", author, mine, ok, tt.wantAuthor, tt.wantMine, tt.wantOK)
			}
		})
	}
}

// TestListCmdAuthorModeRegistration proves the REAL listCmd singleton registers --author/--mine and
// enforces every mutual-exclusivity pair they take part in. Exclusivity cannot be exercised through
// listProcess: cobra validates flag groups in ValidateFlagGroups during Execute, never in RunE.
func TestListCmdAuthorModeRegistration(t *testing.T) {
	for _, name := range []string{"author", "mine"} {
		if listCmd.Flags().Lookup(name) == nil {
			t.Errorf("listCmd has no --%s flag registered", name)
		}
	}

	pairs := []struct{ first, second string }{
		{"author", "mine"},
		{"commit", "author"},
		{"commit", "mine"},
	}
	for _, pair := range pairs {
		t.Run(pair.first+"-vs-"+pair.second, func(t *testing.T) {
			old := listOptions
			t.Cleanup(func() { listOptions = old })

			setRealListFlag(t, pair.first, flagValueFor(pair.first))
			setRealListFlag(t, pair.second, flagValueFor(pair.second))

			if err := listCmd.ValidateFlagGroups(); err == nil {
				t.Errorf("ValidateFlagGroups() = nil, want an error for --%s with --%s", pair.first, pair.second)
			}
		})
	}
}

// flagValueFor returns a valid value to set the named listCmd flag to when only "some value" is
// needed, as in the mutual-exclusivity pairs above.
func flagValueFor(name string) string {
	if name == "mine" {
		return "true"
	}
	return "x"
}
