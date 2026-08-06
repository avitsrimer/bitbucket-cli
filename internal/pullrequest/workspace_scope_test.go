package pullrequest

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
	"github.com/spf13/cobra"
)

// setupWorkspaceScopeTest returns a standalone command for --repository/--workspace resolution
// tests that must never touch /workspaces/{slug}: unlike setupTest/setupTestNamed, it does not
// call testutil.PrimeFixtureCaches, so a regression that fell back to fetching a Workspace object
// cannot silently pass by resolving from RepositoryCache/WorkspaceCache instead of the network.
// profileName and the workspace/repository slugs used by callers must be unique per test since
// RepositoryCache/WorkspaceCache are package-level globals shared across the whole test binary.
func setupWorkspaceScopeTest(t *testing.T, profileName, repositorySlug string, handler http.HandlerFunc) *cobra.Command {
	t.Helper()

	cmd := testutil.SetupProfile(t, profileName, handler)
	cmd.Flags().String("workspace", "", "")
	if err := cmd.Flags().Set("repository", repositorySlug); err != nil {
		t.Fatalf("cannot set repository flag: %v", err)
	}
	return cmd
}

// forbidWorkspaceGet is an http.HandlerFunc wrapper that fails any request whose path contains
// "/workspaces/" with a 403 shaped like BitBucket's actual scope error, while recording every
// request so a test can assert none of them ever landed here. Requests to /user/workspaces (the
// different, root---workspace-flag-validation endpoint) are unaffected since that path does not
// contain "/workspaces/" as a segment of a slug lookup here -- this helper only guards the
// workspace *object* endpoints resolving --repository/--workspace must never fetch.
func forbidWorkspaceGet(t *testing.T, requests *[]*http.Request, onOtherRequest http.HandlerFunc) http.HandlerFunc {
	t.Helper()
	deniedHandler := testutil.WorkspaceScopeDeniedHandler(t, onOtherRequest)
	return func(w http.ResponseWriter, r *http.Request) {
		*requests = append(*requests, r)
		deniedHandler(w, r)
	}
}

// assertNoWorkspaceGetRequests fails the test if any recorded request's path contains
// "/workspaces/", proving structurally -- not merely via the absence of a scope error -- that a
// resolution path never fetched a Workspace object.
func assertNoWorkspaceGetRequests(t *testing.T, requests []*http.Request) {
	t.Helper()
	for _, req := range requests {
		if strings.Contains(req.URL.Path, "/workspaces/") {
			t.Errorf("unexpected request to %s: resolving --workspace/--repository must never fetch a Workspace object", req.URL.Path)
		}
	}
}

// TestListProcessResolvesRepositoryWithoutWorkspaceRequest proves "bb pullrequest list
// --repository <slug> --workspace <slug>" against a token that can read the repository and its
// pullrequests but lacks read:workspace succeeds, because resolving the repository must never
// fetch a Workspace object purely to read back the slug the --workspace flag already supplied
// verbatim.
func TestListProcessResolvesRepositoryWithoutWorkspaceRequest(t *testing.T) {
	withListOptions(t, func() {
		listOptions.Commit = ""
		listOptions.Query = ""
	})

	const workspaceSlug = "fr3-list-ws"
	const repositorySlug = "fr3-list-repo"

	repositoryPath := "/2.0/repositories/" + workspaceSlug + "/" + repositorySlug

	var requests []*http.Request
	cmd := setupWorkspaceScopeTest(t, "fr3-pullrequest-list", repositorySlug, forbidWorkspaceGet(t, &requests, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == repositoryPath:
			_, _ = w.Write([]byte(`{"type":"repository","uuid":"{11111111-1111-1111-1111-111111111111}","name":"FR3 Repo","full_name":"` + workspaceSlug + `/` + repositorySlug + `","slug":"` + repositorySlug + `","workspace":{"type":"workspace","uuid":"{33333333-3333-3333-3333-333333333333}","slug":"` + workspaceSlug + `","name":"FR3 Workspace"}}`))
		case strings.HasSuffix(r.URL.Path, "/pullrequests"):
			_, _ = w.Write([]byte(`{"values":[{"id":42,"title":"Add feature"}]}`))
		default:
			t.Errorf("unexpected request to %s", r.URL.Path)
		}
	}))
	if err := cmd.Flags().Set("workspace", workspaceSlug); err != nil {
		t.Fatalf("cannot set workspace flag: %v", err)
	}

	stdout := testutil.CaptureStdout(t, func() {
		if err := listProcess(cmd, nil); err != nil {
			t.Fatalf("listProcess() error = %v (must succeed on a token that lacks read:workspace)", err)
		}
	})

	assertNoWorkspaceGetRequests(t, requests)

	var pullrequests []PullRequest
	if err := json.Unmarshal([]byte(stdout), &pullrequests); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if len(pullrequests) != 1 || pullrequests[0].Title != "Add feature" {
		t.Errorf("pullrequests = %+v, want one pullrequest titled %q", pullrequests, "Add feature")
	}

	var pullrequestsRequests int
	for _, req := range requests {
		if req.URL.Path == "/2.0/repositories/"+workspaceSlug+"/"+repositorySlug+"/pullrequests" {
			pullrequestsRequests++
		}
	}
	if pullrequestsRequests != 1 {
		t.Errorf("expected exactly 1 request to the pullrequests endpoint, got %d (requests: %v)", pullrequestsRequests, requests)
	}
}

// TestGetProcessResolvesRepositoryWithoutWorkspaceRequest is TestListProcessResolvesRepository...
// for "bb pullrequest get <id> --repository <slug> --workspace <slug>".
func TestGetProcessResolvesRepositoryWithoutWorkspaceRequest(t *testing.T) {
	const workspaceSlug = "fr3-get-ws"
	const repositorySlug = "fr3-get-repo"

	repositoryPath := "/2.0/repositories/" + workspaceSlug + "/" + repositorySlug

	var requests []*http.Request
	cmd := setupWorkspaceScopeTest(t, "fr3-pullrequest-get", repositorySlug, forbidWorkspaceGet(t, &requests, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == repositoryPath:
			_, _ = w.Write([]byte(`{"type":"repository","uuid":"{11111111-1111-1111-1111-111111111111}","name":"FR3 Repo","full_name":"` + workspaceSlug + `/` + repositorySlug + `","slug":"` + repositorySlug + `","workspace":{"type":"workspace","uuid":"{33333333-3333-3333-3333-333333333333}","slug":"` + workspaceSlug + `","name":"FR3 Workspace"}}`))
		case strings.HasSuffix(r.URL.Path, "/pullrequests/42"):
			_, _ = w.Write([]byte(`{"id":42,"title":"Add feature","state":"OPEN"}`))
		default:
			t.Errorf("unexpected request to %s", r.URL.Path)
		}
	}))
	if err := cmd.Flags().Set("workspace", workspaceSlug); err != nil {
		t.Fatalf("cannot set workspace flag: %v", err)
	}

	stdout := testutil.CaptureStdout(t, func() {
		if err := getProcess(cmd, []string{"42"}); err != nil {
			t.Fatalf("getProcess() error = %v (must succeed on a token that lacks read:workspace)", err)
		}
	})

	assertNoWorkspaceGetRequests(t, requests)

	var pr PullRequest
	if err := json.Unmarshal([]byte(stdout), &pr); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if pr.Title != "Add feature" {
		t.Errorf("printed pullrequest title = %q, want %q", pr.Title, "Add feature")
	}

	var pullrequestRequests int
	for _, req := range requests {
		if req.URL.Path == "/2.0/repositories/"+workspaceSlug+"/"+repositorySlug+"/pullrequests/42" {
			pullrequestRequests++
		}
	}
	if pullrequestRequests != 1 {
		t.Errorf("expected exactly 1 request to the pullrequest endpoint, got %d (requests: %v)", pullrequestRequests, requests)
	}
}
