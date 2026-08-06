package commit

import (
	"net/http"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
)

func TestDiffProcessSingleCommit(t *testing.T) {
	const diffBody = "diff --git a/file.txt b/file.txt\n+added line\n"

	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(diffBody))
	}, false)

	stdout := testutil.CaptureStdout(t, func() {
		if err := diffProcess(cmd, []string{"aaaaaaa"}); err != nil {
			t.Fatalf("diffProcess() error = %v", err)
		}
	})

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/diff/aaaaaaa"
	if requests[0].URL.Path != wantPath {
		t.Errorf("path = %s, want %s", requests[0].URL.Path, wantPath)
	}
	if stdout != diffBody {
		t.Errorf("stdout = %q, want the raw diff body %q copied unchanged", stdout, diffBody)
	}
}

func TestDiffProcessTwoCommitsBuildsRangeSpec(t *testing.T) {
	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("diff"))
	}, false)

	testutil.CaptureStdout(t, func() {
		if err := diffProcess(cmd, []string{"aaaaaaa", "bbbbbbb"}); err != nil {
			t.Fatalf("diffProcess() error = %v", err)
		}
	})

	wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/diff/aaaaaaa..bbbbbbb"
	if requests[0].URL.Path != wantPath {
		t.Errorf("path = %s, want %s", requests[0].URL.Path, wantPath)
	}
}

func TestDiffProcessStatFlagUsesDiffstatPath(t *testing.T) {
	diffOptions.Stat = true
	t.Cleanup(func() { diffOptions.Stat = false })

	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("1 file changed"))
	}, false)

	testutil.CaptureStdout(t, func() {
		if err := diffProcess(cmd, []string{"aaaaaaa"}); err != nil {
			t.Fatalf("diffProcess() error = %v", err)
		}
	})

	wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/diffstat/aaaaaaa"
	if requests[0].URL.Path != wantPath {
		t.Errorf("path = %s, want %s", requests[0].URL.Path, wantPath)
	}
}

func TestDiffProcessAPIError(t *testing.T) {
	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	}, false)

	err := diffProcess(cmd, []string{"aaaaaaa"})
	if err == nil {
		t.Fatal("diffProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "cannot get diff") {
		t.Errorf("error = %q, want it to mention the diff failure", err.Error())
	}
}

func TestDiffProcessDryRun(t *testing.T) {
	var requestCount int
	cmd := setupTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, true)

	if err := diffProcess(cmd, []string{"aaaaaaa"}); err != nil {
		t.Fatalf("diffProcess() error = %v", err)
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request in dry-run mode, got %d", requestCount)
	}
}
