package commit

import (
	"net/http"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
)

func TestPatchProcessBuildsRangeSpec(t *testing.T) {
	const patchBody = "From aaaaaaa Mon Sep 17 00:00:00 2001\n"

	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(patchBody))
	}, false)

	stdout := testutil.CaptureStdout(t, func() {
		if err := patchProcess(cmd, []string{"aaaaaaa", "bbbbbbb"}); err != nil {
			t.Fatalf("patchProcess() error = %v", err)
		}
	})

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/patch/aaaaaaa..bbbbbbb"
	if requests[0].URL.Path != wantPath {
		t.Errorf("path = %s, want %s", requests[0].URL.Path, wantPath)
	}
	if stdout != patchBody {
		t.Errorf("stdout = %q, want the raw patch body %q copied unchanged", stdout, patchBody)
	}
}

func TestPatchProcessAPIError(t *testing.T) {
	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	}, false)

	err := patchProcess(cmd, []string{"aaaaaaa", "bbbbbbb"})
	if err == nil {
		t.Fatal("patchProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "cannot get patch") {
		t.Errorf("error = %q, want it to mention the patch failure", err.Error())
	}
}

func TestPatchProcessDryRun(t *testing.T) {
	var requestCount int
	cmd := setupTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, true)

	if err := patchProcess(cmd, []string{"aaaaaaa", "bbbbbbb"}); err != nil {
		t.Fatalf("patchProcess() error = %v", err)
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request in dry-run mode, got %d", requestCount)
	}
}
