package commit

import (
	"net/http"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
)

func TestGetCommitsSuccess(t *testing.T) {
	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[{"type":"commit","hash":"aaaaaaa","date":"2026-01-01T00:00:00+00:00"}]}`))
	}, false)

	commits, err := GetCommits(t.Context(), cmd)
	if err != nil {
		t.Fatalf("GetCommits() error = %v", err)
	}
	if len(commits) != 1 || commits[0].Hash != "aaaaaaa" {
		t.Errorf("commits = %+v, want a single commit aaaaaaa", commits)
	}
	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
}

func TestGetCommitsAPIError(t *testing.T) {
	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"server exploded"}}`))
	}, false)

	_, err := GetCommits(t.Context(), cmd)
	if err == nil {
		t.Fatal("GetCommits() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "server exploded") {
		t.Errorf("error = %q, want it to contain the BitBucket error message", err.Error())
	}
}

func TestGetCommitHashesSuccessSortedCaseInsensitively(t *testing.T) {
	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[` +
			`{"type":"commit","hash":"Zeta","date":"2026-01-01T00:00:00+00:00"},` +
			`{"type":"commit","hash":"alpha","date":"2026-01-01T00:00:00+00:00"}` +
			`]}`))
	}, false)

	hashes, err := GetCommitHashes(t.Context(), cmd, nil, "")
	if err != nil {
		t.Fatalf("GetCommitHashes() error = %v", err)
	}
	want := []string{"alpha", "Zeta"}
	if len(hashes) != len(want) || hashes[0] != want[0] || hashes[1] != want[1] {
		t.Errorf("hashes = %v, want %v (sorted case-insensitively)", hashes, want)
	}
	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
}

func TestGetCommitHashesUsesToCompleteAsHashPrefixFilter(t *testing.T) {
	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[]}`))
	}, false)

	if _, err := GetCommitHashes(t.Context(), cmd, nil, "abc"); err != nil {
		t.Fatalf("GetCommitHashes() error = %v", err)
	}

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	if got := requests[0].URL.Query().Get("q"); got != `hash~"abc"` {
		t.Errorf("q query = %q, want %q", got, `hash~"abc"`)
	}
}

func TestGetCommitHashesAPIError(t *testing.T) {
	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"server exploded"}}`))
	}, false)

	_, err := GetCommitHashes(t.Context(), cmd, nil, "")
	if err == nil {
		t.Fatal("GetCommitHashes() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "server exploded") {
		t.Errorf("error = %q, want it to contain the BitBucket error message", err.Error())
	}
}

func TestGetLatestCommitSuccessRequestsSingleItemPage(t *testing.T) {
	const hash = "latesthash"

	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[{"type":"commit","hash":"` + hash + `","date":"2026-01-01T00:00:00+00:00"}]}`))
	}, false)

	target, err := GetLatestCommit(t.Context(), cmd)
	if err != nil {
		t.Fatalf("GetLatestCommit() error = %v", err)
	}
	if target.Hash != hash {
		t.Errorf("GetLatestCommit().Hash = %q, want %q", target.Hash, hash)
	}
	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	if got := requests[0].URL.Query().Get("pagelen"); got != "1" {
		t.Errorf("pagelen query = %q, want %q", got, "1")
	}
}

func TestGetLatestCommitNoCommitsReturnsError(t *testing.T) {
	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[]}`))
	}, false)

	if _, err := GetLatestCommit(t.Context(), cmd); err == nil {
		t.Fatal("GetLatestCommit() expected an error when the repository has no commits, got nil")
	}
}

func TestGetCommitByHashSuccess(t *testing.T) {
	const hash = "aaaaaaa"

	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"type":"commit","hash":"` + hash + `","date":"2026-01-01T00:00:00+00:00"}`))
	}, false)

	target, err := GetCommitByHash(t.Context(), cmd, hash)
	if err != nil {
		t.Fatalf("GetCommitByHash() error = %v", err)
	}
	if target.Hash != hash {
		t.Errorf("GetCommitByHash().Hash = %q, want %q", target.Hash, hash)
	}
	wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/commit/" + hash
	if len(requests) != 1 || requests[0].URL.Path != wantPath {
		t.Errorf("request path = %v, want exactly one request to %s", requests, wantPath)
	}
}

func TestGetCommitByHashAPIError(t *testing.T) {
	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"commit not found"}}`))
	}, false)

	_, err := GetCommitByHash(t.Context(), cmd, "deadbeef")
	if err == nil {
		t.Fatal("GetCommitByHash() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "commit not found") {
		t.Errorf("error = %q, want it to contain the BitBucket error message", err.Error())
	}
}
