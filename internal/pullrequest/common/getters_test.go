package prcommon

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/avitsrimer/bitbucket-cli/internal/workspace"
	"github.com/spf13/cobra"
)

// fixtureWorkspaceSlug/fixtureRepositorySlug identify a repository primed once, directly into
// RepositoryCache/WorkspaceCache, so every test in this file resolves it without ever hitting the
// network for workspace/repository lookups; only the pullrequests call under test reaches the
// per-test httptest server.
const (
	fixtureWorkspaceSlug  = "acme"
	fixtureRepositorySlug = "widgets"
	fixtureRepositoryFlag = fixtureWorkspaceSlug + "/" + fixtureRepositorySlug
)

// setupTest primes the workspace/repository caches, points the profile client at a fresh
// httptest server, and returns a standalone command carrying the flags the getters read
// (profile, repository, output).
func setupTest(t *testing.T, handler http.HandlerFunc) *cobra.Command {
	t.Helper()

	ws := workspace.Workspace{Slug: fixtureWorkspaceSlug}
	repo := repository.Repository{Slug: fixtureRepositorySlug, Workspace: &ws}
	if err := workspace.WorkspaceCache.Set(ws, fixtureWorkspaceSlug); err != nil {
		t.Fatalf("cannot prime workspace cache: %v", err)
	}
	if err := repository.RepositoryCache.Set(repo, fixtureRepositoryFlag); err != nil {
		t.Fatalf("cannot prime repository cache: %v", err)
	}
	t.Cleanup(func() {
		removeCacheEntry(fixtureWorkspaceSlug)
		removeCacheEntry(fixtureRepositoryFlag)
	})

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	apiRoot, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("cannot parse test server URL: %v", err)
	}

	testProfile := &profile.Profile{Name: "prcommon-test", APIRoot: apiRoot, AccessToken: "dummy-token", OutputFormat: "json"}
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
	cmd.Flags().String("repository", fixtureRepositoryFlag, "")
	cmd.Flags().String("output", "", "")
	return cmd
}

// removeCacheEntry deletes the on-disk mirror of a primed cache entry so the test run does not
// leave residue behind in the real os.UserCacheDir().
func removeCacheEntry(key string) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return
	}
	sum := sha256.Sum256([]byte(key))
	_ = os.Remove(filepath.Join(dir, "bitbucket", hex.EncodeToString(sum[:])))
}

func TestGetPullRequestIDsWithStateSuccess(t *testing.T) {
	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[{"id":42},{"id":7}]}`))
	})

	ids, err := GetPullRequestIDsWithState(context.Background(), cmd, "OPEN")
	if err != nil {
		t.Fatalf("GetPullRequestIDsWithState() error = %v", err)
	}
	want := []string{"42", "7"}
	if !slices.Equal(ids, want) {
		t.Errorf("ids = %v, want %v", ids, want)
	}
	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	wantPath := "/2.0/repositories/" + fixtureRepositoryFlag + "/pullrequests"
	if requests[0].URL.Path != wantPath || requests[0].URL.Query().Get("state") != "OPEN" {
		t.Errorf("request = %s?%s, want path %s with state=OPEN", requests[0].URL.Path, requests[0].URL.RawQuery, wantPath)
	}
}

func TestGetPullRequestIDsWithStateAPIError(t *testing.T) {
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"boom"}}`))
	})

	_, err := GetPullRequestIDsWithState(context.Background(), cmd, "OPEN")
	if err == nil {
		t.Fatal("GetPullRequestIDsWithState() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error = %q, want it to contain the BitBucket error message", err.Error())
	}
}

func TestGetPullRequestIDsFallsBackToAllWhenNoneOpen(t *testing.T) {
	var openCalled, allCalled bool
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("state") {
		case "OPEN":
			openCalled = true
			_, _ = w.Write([]byte(`{"values":[]}`))
		case "ALL":
			allCalled = true
			_, _ = w.Write([]byte(`{"values":[{"id":5}]}`))
		default:
			t.Errorf("unexpected state query: %s", r.URL.Query().Get("state"))
		}
	})

	ids, err := GetPullRequestIDs(context.Background(), cmd, nil, "")
	if err != nil {
		t.Fatalf("GetPullRequestIDs() error = %v", err)
	}
	if !openCalled || !allCalled {
		t.Errorf("expected both OPEN and ALL to be queried, got openCalled=%v allCalled=%v", openCalled, allCalled)
	}
	want := []string{"5"}
	if !slices.Equal(ids, want) {
		t.Errorf("ids = %v, want %v", ids, want)
	}
}

func TestGetPullRequestIDsReturnsOpenWithoutFallback(t *testing.T) {
	var allCalled bool
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("state") {
		case "OPEN":
			_, _ = w.Write([]byte(`{"values":[{"id":1}]}`))
		case "ALL":
			allCalled = true
			_, _ = w.Write([]byte(`{"values":[{"id":1},{"id":2}]}`))
		}
	})

	ids, err := GetPullRequestIDs(context.Background(), cmd, nil, "")
	if err != nil {
		t.Fatalf("GetPullRequestIDs() error = %v", err)
	}
	if allCalled {
		t.Error("expected ALL not to be queried when OPEN already returned results")
	}
	want := []string{"1"}
	if !slices.Equal(ids, want) {
		t.Errorf("ids = %v, want %v", ids, want)
	}
}
