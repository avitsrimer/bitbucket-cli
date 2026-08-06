package plcommon

import (
	"context"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
	"github.com/spf13/cobra"
)

func TestMain(m *testing.M) {
	os.Exit(testutil.TempCaches(m))
}

// setupTest primes the fixture workspace/repository caches and points the profile client at a
// fresh httptest server, returning a standalone command carrying the flags the getters read
// (profile, repository, output).
func setupTest(t *testing.T, handler http.HandlerFunc) *cobra.Command {
	t.Helper()
	testutil.PrimeFixtureCaches(t)
	return testutil.SetupProfile(t, "plcommon-test", handler)
}

func TestGetPipelineIDsSuccess(t *testing.T) {
	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[{"build_number":42},{"build_number":7}]}`))
	})

	ids, err := GetPipelineIDs(context.Background(), cmd, nil, "")
	if err != nil {
		t.Fatalf("GetPipelineIDs() error = %v", err)
	}
	want := []string{"42", "7"}
	if !slices.Equal(ids, want) {
		t.Errorf("ids = %v, want %v", ids, want)
	}
	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/pipelines"
	if requests[0].URL.Path != wantPath {
		t.Errorf("path = %s, want %s", requests[0].URL.Path, wantPath)
	}
}

func TestGetPipelineIDsAPIError(t *testing.T) {
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"boom"}}`))
	})

	_, err := GetPipelineIDs(context.Background(), cmd, nil, "")
	if err == nil {
		t.Fatal("GetPipelineIDs() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error = %q, want it to contain the BitBucket error message", err.Error())
	}
}

func TestGetPipelineIDsEmpty(t *testing.T) {
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[]}`))
	})

	ids, err := GetPipelineIDs(context.Background(), cmd, nil, "")
	if err != nil {
		t.Fatalf("GetPipelineIDs() error = %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("ids = %v, want empty", ids)
	}
}

// TestGetPipelineIDsIgnoresLimitFlag proves a completion getter uses GetAllUnbounded, so a
// --limit flag registered on cmd (belonging to a different, unrelated output query) never
// truncates the enumeration.
func TestGetPipelineIDsIgnoresLimitFlag(t *testing.T) {
	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[{"build_number":42},{"build_number":7}]}`))
	})
	if err := cmd.Flags().Set("limit", "1"); err != nil {
		t.Fatalf("cannot set limit flag: %v", err)
	}

	ids, err := GetPipelineIDs(context.Background(), cmd, nil, "")
	if err != nil {
		t.Fatalf("GetPipelineIDs() error = %v", err)
	}
	if len(ids) != 2 {
		t.Errorf("ids = %v, want 2 ids despite --limit=1 on cmd", ids)
	}
}
