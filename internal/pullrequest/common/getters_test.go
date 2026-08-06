package prcommon

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
	return testutil.SetupProfile(t, "prcommon-test", handler)
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
	wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/pullrequests"
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
