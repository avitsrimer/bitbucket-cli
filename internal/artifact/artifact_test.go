package artifact

import (
	"net/http"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
)

func TestGetArtifactNamesSortsCaseInsensitively(t *testing.T) {
	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[{"name":"zeta.log"},{"name":"Alpha.log"}]}`))
	}, false)

	names, err := GetArtifactNames(cmd.Context(), cmd)
	if err != nil {
		t.Fatalf("GetArtifactNames() error = %v", err)
	}
	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/downloads"
	if requests[0].URL.Path != wantPath {
		t.Errorf("path = %s, want %s", requests[0].URL.Path, wantPath)
	}
	if len(names) != 2 || names[0] != "Alpha.log" || names[1] != "zeta.log" {
		t.Errorf("names = %v, want [Alpha.log zeta.log] sorted case-insensitively", names)
	}
}

// TestGetArtifactNamesIgnoresLimitFlag proves a completion getter uses GetAllUnbounded, so a
// --limit flag registered on cmd (belonging to a different, unrelated output query) never
// truncates the enumeration.
func TestGetArtifactNamesIgnoresLimitFlag(t *testing.T) {
	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[{"name":"one.log"},{"name":"two.log"}]}`))
	}, false)
	if err := cmd.Flags().Set("limit", "1"); err != nil {
		t.Fatalf("cannot set limit flag: %v", err)
	}

	names, err := GetArtifactNames(cmd.Context(), cmd)
	if err != nil {
		t.Fatalf("GetArtifactNames() error = %v", err)
	}
	if len(names) != 2 {
		t.Errorf("names = %v, want 2 names despite --limit=1 on cmd", names)
	}
}

func TestGetArtifactNamesAPIError(t *testing.T) {
	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"server exploded"}}`))
	}, false)

	_, err := GetArtifactNames(cmd.Context(), cmd)
	if err == nil {
		t.Fatal("GetArtifactNames() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "server exploded") {
		t.Errorf("error = %q, want it to contain the BitBucket error message", err.Error())
	}
}
