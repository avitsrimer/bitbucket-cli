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

// forbidWorkspaceGet 403s any request whose path contains "/workspaces/" with the exact scope
// error a token lacking read:workspace gets back, while recording every request so a test can
// assert none of them ever landed here. Duplicated from testutil.WorkspaceScopeDeniedHandler:
// this package cannot import internal/testutil (see setupNoCacheTest's comment for the cycle).
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

// TestGetRepositoryBySlugOrIDExplicitPrefixNeverFetchesWorkspace proves "bb repo get
// <workspace>/<repository>": the explicit "workspace/repo" form
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

// TestGetRepositoryBySlugOrIDRejectsEmptyComponents proves a "workspace/repository" argument with
// an empty component (e.g. "/foo" or "foo/") is rejected outright instead of silently retargeting
// the request: path.Join/JoinPath collapse the resulting double slash ("/repositories//foo"),
// landing on the *list repositories in workspace "foo"* endpoint instead of erroring, which
// previously surfaced as a confusing "cannot unmarshal repository" error instead of a clean
// "argument ... is missing".
func TestGetRepositoryBySlugOrIDRejectsEmptyComponents(t *testing.T) {
	tests := []struct {
		name     string
		slugOrID string
		wantErr  string
	}{
		{name: "empty workspace component", slugOrID: "/foo", wantErr: "workspace"},
		{name: "empty repository component", slugOrID: "foo/", wantErr: "repository"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var requests []*http.Request
			cmd := setupNoCacheTest(t, "fr11-empty-component-"+test.name, func(w http.ResponseWriter, r *http.Request) {
				requests = append(requests, r)
				t.Errorf("unexpected request to %s: an empty component must be rejected before any request is sent", r.URL.Path)
			})

			_, err := GetRepositoryBySlugOrID(t.Context(), cmd, test.slugOrID)
			if err == nil {
				t.Fatal("GetRepositoryBySlugOrID() expected an error for an empty component, got nil")
			}
			if !strings.Contains(err.Error(), test.wantErr) {
				t.Errorf("GetRepositoryBySlugOrID() error = %q, want it to mention %q", err.Error(), test.wantErr)
			}
			if len(requests) != 0 {
				t.Errorf("expected zero requests, got %d", len(requests))
			}
		})
	}
}

// TestGetRepositoryBySlugOrIDNormalizesWorkspaceUUID proves a bare (unbraced) UUID given as the
// workspace segment of a "workspace/repository" argument is normalized to BitBucket's canonical
// braced form before it is sent, the same normalization the repository segment already received:
// without it, "--workspace <bare-uuid>" 404s while the braced form works, an asymmetry with no
// reason to exist.
func TestGetRepositoryBySlugOrIDNormalizesWorkspaceUUID(t *testing.T) {
	const bareWorkspaceUUID = "33333333-3333-3333-3333-333333333333"
	const canonicalWorkspaceUUID = "{33333333-3333-3333-3333-333333333333}"
	const repositorySlug = "fr11-uuid-repo"
	repositoryPath := "/2.0/repositories/" + canonicalWorkspaceUUID + "/" + repositorySlug

	var requests []*http.Request
	cmd := setupNoCacheTest(t, "fr11-workspace-uuid", forbidWorkspaceGet(t, &requests, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case repositoryPath:
			_, _ = w.Write([]byte(`{"type":"repository","uuid":"{55555555-5555-5555-5555-555555555555}","name":"FR11","full_name":"acme/` + repositorySlug + `","slug":"` + repositorySlug + `"}`))
		default:
			t.Errorf("unexpected request to %s", r.URL.Path)
		}
	}))

	repo, err := GetRepositoryBySlugOrID(t.Context(), cmd, bareWorkspaceUUID+"/"+repositorySlug)
	if err != nil {
		t.Fatalf("GetRepositoryBySlugOrID() error = %v, want the bare workspace UUID normalized to its canonical braced form", err)
	}
	if repo.Slug != repositorySlug {
		t.Errorf("repo.Slug = %q, want %q", repo.Slug, repositorySlug)
	}
}
