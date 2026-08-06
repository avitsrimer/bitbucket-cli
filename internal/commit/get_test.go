package commit

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
)

func TestGetProcessSuccessWithHash(t *testing.T) {
	const hash = "0265607aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"type":"commit","hash":"` + hash + `","message":"Add feature","date":"2026-01-01T00:00:00+00:00"}`))
	}, false)

	stdout := testutil.CaptureStdout(t, func() {
		if err := getProcess(cmd, []string{hash}); err != nil {
			t.Fatalf("getProcess() error = %v", err)
		}
	})

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/commit/" + hash
	if requests[0].URL.Path != wantPath {
		t.Errorf("path = %s, want %s", requests[0].URL.Path, wantPath)
	}

	// Unmarshal into a minimal local struct rather than commit.Commit: the fixture response
	// carries no "repository" object, so the printed output's embedded repository is empty, and
	// Repository.UnmarshalJSON's Validate call would reject round-tripping that back through the
	// full Commit type.
	var got struct {
		Hash string `json:"hash"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if got.Hash != hash {
		t.Errorf("printed commit hash = %q, want %q", got.Hash, hash)
	}
}

func TestGetProcessSuccessDefaultsToLatestCommit(t *testing.T) {
	const hash = "aaaaaaabbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[{"type":"commit","hash":"` + hash + `","message":"Latest","date":"2026-01-02T00:00:00+00:00"}]}`))
	}, false)

	stdout := testutil.CaptureStdout(t, func() {
		if err := getProcess(cmd, nil); err != nil {
			t.Fatalf("getProcess() error = %v", err)
		}
	})

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/commits"
	if requests[0].URL.Path != wantPath {
		t.Errorf("path = %s, want %s", requests[0].URL.Path, wantPath)
	}
	if got := requests[0].URL.Query().Get("pagelen"); got != "1" {
		t.Errorf("pagelen query = %q, want %q", got, "1")
	}

	var got struct {
		Hash string `json:"hash"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if got.Hash != hash {
		t.Errorf("printed commit hash = %q, want %q", got.Hash, hash)
	}
}

func TestGetProcessAPIError(t *testing.T) {
	const hash = "deadbeef"

	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"commit not found"}}`))
	}, false)

	err := getProcess(cmd, []string{hash})
	if err == nil {
		t.Fatal("getProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "cannot get commit "+hash) {
		t.Errorf("error = %q, want it to mention the commit hash", err.Error())
	}
	if !strings.Contains(err.Error(), "commit not found") {
		t.Errorf("error = %q, want it to contain the BitBucket error message", err.Error())
	}
}

// TestGetProcessDryRunSkipsPrinting proves --dry-run gates the Print call: getProcess still needs
// to fetch the target commit to describe it in the dry-run message (there is no commit cache,
// unlike repository/workspace get's cached resolution), but nothing is printed to stdout.
func TestGetProcessDryRunSkipsPrinting(t *testing.T) {
	const hash = "deadbeef"

	var requestCount int
	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"type":"commit","hash":"` + hash + `","date":"2026-01-01T00:00:00+00:00"}`))
	}, true)

	stdout := testutil.CaptureStdout(t, func() {
		if err := getProcess(cmd, []string{hash}); err != nil {
			t.Fatalf("getProcess() error = %v", err)
		}
	})
	if requestCount != 1 {
		t.Errorf("expected exactly 1 HTTP request to resolve the commit to describe, got %d", requestCount)
	}
	if stdout != "" {
		t.Errorf("expected no printed output in dry-run mode, got %q", stdout)
	}
}
