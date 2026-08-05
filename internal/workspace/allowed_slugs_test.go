package workspace_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/workspace"
	"github.com/spf13/cobra"
)

// setupAllowedSlugsTest points the profile client at a fresh httptest server and returns a
// standalone command carrying the flags GetAll reads (profile, page-length, limit); it backs
// the root --workspace flag's dynamic allowed-value resolution and shell completion.
func setupAllowedSlugsTest(t *testing.T, handler http.HandlerFunc) *cobra.Command {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	apiRoot, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("cannot parse test server URL: %v", err)
	}

	testProfile := &profile.Profile{Name: "workspace-allowed-slugs-test", APIRoot: apiRoot, AccessToken: "dummy-token"}
	oldProfiles, oldCurrent := profile.Profiles, profile.Current
	profile.Profiles = append(profile.Profiles, testProfile)
	profile.Current = testProfile
	t.Cleanup(func() {
		profile.Profiles = oldProfiles
		profile.Current = oldCurrent
	})

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("profile", "", "")
	cmd.Flags().Int("page-length", 0, "")
	cmd.Flags().Int("limit", 0, "")
	return cmd
}

func TestGetWorkspaceAllowedSlugsSuccess(t *testing.T) {
	var requests []*http.Request
	cmd := setupAllowedSlugsTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[{"type":"workspace","slug":"zeta"},{"type":"workspace","slug":"acme"}]}`))
	})

	slugs, err := workspace.GetWorkspaceAllowedSlugs(t.Context(), cmd, nil, "")
	if err != nil {
		t.Fatalf("GetWorkspaceAllowedSlugs() error = %v", err)
	}
	want := []string{"acme", "zeta"}
	if len(slugs) != len(want) || slugs[0] != want[0] || slugs[1] != want[1] {
		t.Errorf("slugs = %v, want %v (sorted case-insensitively)", slugs, want)
	}
	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	if requests[0].URL.Path != "/2.0/user/workspaces" {
		t.Errorf("path = %s, want /2.0/user/workspaces", requests[0].URL.Path)
	}
}

func TestGetWorkspaceAllowedSlugsAPIError(t *testing.T) {
	cmd := setupAllowedSlugsTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"server exploded"}}`))
	})

	_, err := workspace.GetWorkspaceAllowedSlugs(t.Context(), cmd, nil, "")
	if err == nil {
		t.Fatal("GetWorkspaceAllowedSlugs() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "server exploded") {
		t.Errorf("error = %q, want it to contain the BitBucket error message", err.Error())
	}
}
