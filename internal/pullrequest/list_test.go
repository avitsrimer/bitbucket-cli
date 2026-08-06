package pullrequest

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
	"github.com/spf13/cobra"
)

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
				if len(pullrequests) != 2 {
					t.Fatalf("expected 2 pullrequests, got %d", len(pullrequests))
				}
				if pullrequests[0].ID != 1 || pullrequests[1].ID != 2 {
					t.Errorf("pullrequests = %+v, want sorted by id ascending (1, 2)", pullrequests)
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
				listOptions.Query = ""
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
// resolvePageLengthAndLimit), proving the value actually reaches GetAll and truncates the result.
func TestListProcessRespectsLimitFlag(t *testing.T) {
	withListOptions(t, func() {
		listOptions.Commit = ""
		listOptions.Query = ""
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
		listOptions.Query = ""
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

	// This is exactly what pflag does while parsing "--workspace sportpursuit" on the real
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
	if len(pullrequests) != 2 {
		t.Fatalf("expected 2 pullrequests, got %d", len(pullrequests))
	}
}
