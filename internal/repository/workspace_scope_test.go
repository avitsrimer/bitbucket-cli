package repository

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/spf13/cobra"
)

// setupNoCacheTest is setupTest without priming WorkspaceCache: it exists so a regression that
// reintroduced a live Workspace fetch in GetRepositoryBySlugOrID could not silently pass by
// resolving from the cache instead of the network. profileName must be unique across the test
// binary for the same reason as setupTest's.
func setupNoCacheTest(t *testing.T, profileName string, handler http.HandlerFunc) *cobra.Command {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	apiRoot, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("cannot parse test server URL: %v", err)
	}

	testProfile := &profile.Profile{Name: profileName, APIRoot: apiRoot, AccessToken: "dummy-token", OutputFormat: "json"}
	oldProfiles, oldCurrent := profile.Profiles, profile.Current
	profile.Profiles = append(profile.Profiles, testProfile)
	profile.Current = testProfile
	t.Cleanup(func() {
		profile.Profiles = oldProfiles
		profile.Current = oldCurrent
	})

	cmd := &cobra.Command{Use: "test"}
	cmd.SetContext(context.Background())
	cmd.Flags().String("profile", "", "")
	cmd.Flags().String("output", "", "")
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().String("repository", "", "")
	cmd.Flags().String("workspace", "", "")
	cmd.Flags().Int("page-length", 0, "")
	cmd.Flags().Int("limit", 0, "")
	return cmd
}

// forbidWorkspaceGet403s any request whose path contains "/workspaces/" with the exact scope
// error field report FR-3 reproduced ("required: read:workspace"), while recording every request
// so a test can assert none of them ever landed here.
func forbidWorkspaceGet(t *testing.T, requests *[]*http.Request, onOtherRequest http.HandlerFunc) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		*requests = append(*requests, r)
		if strings.Contains(r.URL.Path, "/workspaces/") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"type":"error","error":{"message":"Your credentials lack one or more required privilege scopes. (required: read:workspace:bitbucket)"}}`))
			return
		}
		onOtherRequest(w, r)
	}
}

func assertNoWorkspaceGetRequests(t *testing.T, requests []*http.Request) {
	t.Helper()
	for _, req := range requests {
		if strings.Contains(req.URL.Path, "/workspaces/") {
			t.Errorf("unexpected request to %s: resolving a repository must never fetch a Workspace object", req.URL.Path)
		}
	}
}

// TestGetRepositoryBySlugOrIDExplicitPrefixNeverFetchesWorkspace is field report FR-3's
// regression test for "bb repo get <workspace>/<repository>": the explicit "workspace/repo" form
// already carries the workspace slug the caller typed, so resolving the repository must never
// fetch a Workspace object for it, even when WorkspaceCache holds nothing and /workspaces/{slug}
// answers 403.
func TestGetRepositoryBySlugOrIDExplicitPrefixNeverFetchesWorkspace(t *testing.T) {
	const workspaceSlug = "fr3-repo-prefix-ws"
	const repositorySlug = "fr3-repo-prefix-repo"
	repositoryPath := "/2.0/repositories/" + workspaceSlug + "/" + repositorySlug

	var requests []*http.Request
	cmd := setupNoCacheTest(t, "fr3-repository-prefix", forbidWorkspaceGet(t, &requests, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case repositoryPath:
			_, _ = w.Write([]byte(`{"type":"repository","uuid":"{11111111-1111-1111-1111-111111111111}","name":"FR3","full_name":"` + workspaceSlug + `/` + repositorySlug + `","slug":"` + repositorySlug + `"}`))
		default:
			t.Errorf("unexpected request to %s", r.URL.Path)
		}
	}))

	repo, err := GetRepositoryBySlugOrID(t.Context(), cmd, workspaceSlug+"/"+repositorySlug)
	if err != nil {
		t.Fatalf("GetRepositoryBySlugOrID() error = %v (must succeed on a token that lacks read:workspace)", err)
	}
	if repo.Slug != repositorySlug {
		t.Errorf("repo.Slug = %q, want %q", repo.Slug, repositorySlug)
	}
	assertNoWorkspaceGetRequests(t, requests)

	var repositoryRequests int
	for _, req := range requests {
		if req.URL.Path == repositoryPath {
			repositoryRequests++
		}
	}
	if repositoryRequests != 1 {
		t.Errorf("expected exactly 1 request to %s, got %d", repositoryPath, repositoryRequests)
	}
}

// TestGetProcessExplicitPrefixNeverFetchesWorkspace drives "bb repo get <workspace>/<repository>"
// through getProcess itself, not just the underlying GetRepositoryBySlugOrID, proving the same
// guarantee holds for the real CLI command's RunE.
func TestGetProcessExplicitPrefixNeverFetchesWorkspace(t *testing.T) {
	const workspaceSlug = "fr3-repo-getprocess-ws"
	const repositorySlug = "fr3-repo-getprocess-repo"
	repositoryPath := "/2.0/repositories/" + workspaceSlug + "/" + repositorySlug

	var requests []*http.Request
	cmd := setupNoCacheTest(t, "fr3-repository-getprocess", forbidWorkspaceGet(t, &requests, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case repositoryPath:
			_, _ = w.Write([]byte(`{"type":"repository","uuid":"{44444444-4444-4444-4444-444444444444}","name":"FR3","full_name":"` + workspaceSlug + `/` + repositorySlug + `","slug":"` + repositorySlug + `"}`))
		default:
			t.Errorf("unexpected request to %s", r.URL.Path)
		}
	}))
	cmd.Flags().StringSlice("columns", []string{}, "")

	stdout := captureStdout(t, func() {
		if err := getProcess(cmd, []string{workspaceSlug + "/" + repositorySlug}); err != nil {
			t.Fatalf("getProcess() error = %v (must succeed on a token that lacks read:workspace)", err)
		}
	})
	if !strings.Contains(stdout, repositorySlug) {
		t.Errorf("stdout = %q, want it to mention the repository slug %q", stdout, repositorySlug)
	}
	assertNoWorkspaceGetRequests(t, requests)
}

// TestGetRepositoryBySlugOrIDWorkspaceFlagFallbackNeverFetchesWorkspace covers the bare-slug
// form, where the workspace comes from the --workspace flag (workspace.GetWorkspaceName): the
// same guarantee as the explicit-prefix test above, for "bb pullrequest list --repository
// <repo> --workspace <ws>" and its peers.
func TestGetRepositoryBySlugOrIDWorkspaceFlagFallbackNeverFetchesWorkspace(t *testing.T) {
	const workspaceSlug = "fr3-repo-flag-ws"
	const repositorySlug = "fr3-repo-flag-repo"
	repositoryPath := "/2.0/repositories/" + workspaceSlug + "/" + repositorySlug

	var requests []*http.Request
	cmd := setupNoCacheTest(t, "fr3-repository-flag", forbidWorkspaceGet(t, &requests, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case repositoryPath:
			_, _ = w.Write([]byte(`{"type":"repository","uuid":"{22222222-2222-2222-2222-222222222222}","name":"FR3","full_name":"` + workspaceSlug + `/` + repositorySlug + `","slug":"` + repositorySlug + `"}`))
		default:
			t.Errorf("unexpected request to %s", r.URL.Path)
		}
	}))
	if err := cmd.Flags().Set("workspace", workspaceSlug); err != nil {
		t.Fatalf("cannot set workspace flag: %v", err)
	}

	repo, err := GetRepositoryBySlugOrID(t.Context(), cmd, repositorySlug)
	if err != nil {
		t.Fatalf("GetRepositoryBySlugOrID() error = %v (must succeed on a token that lacks read:workspace)", err)
	}
	if repo.Slug != repositorySlug {
		t.Errorf("repo.Slug = %q, want %q", repo.Slug, repositorySlug)
	}
	assertNoWorkspaceGetRequests(t, requests)

	var repositoryRequests int
	for _, req := range requests {
		if req.URL.Path == repositoryPath {
			repositoryRequests++
		}
	}
	if repositoryRequests != 1 {
		t.Errorf("expected exactly 1 request to %s, got %d", repositoryPath, repositoryRequests)
	}
}
