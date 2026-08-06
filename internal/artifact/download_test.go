package artifact

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
)

func TestDownloadProcessWritesContentToDestination(t *testing.T) {
	const content = "artifact-bytes"
	destDir := t.TempDir()

	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		_, _ = w.Write([]byte(content))
	}, false)
	if err := cmd.Flags().Set("destination", destDir); err != nil {
		t.Fatalf("cannot set destination flag: %v", err)
	}

	if err := downloadProcess(cmd, []string{"build.log"}); err != nil {
		t.Fatalf("downloadProcess() error = %v", err)
	}

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/downloads/build.log"
	if requests[0].URL.Path != wantPath {
		t.Errorf("path = %s, want %s", requests[0].URL.Path, wantPath)
	}

	data, err := os.ReadFile(filepath.Join(destDir, "build.log"))
	if err != nil {
		t.Fatalf("cannot read downloaded file: %v", err)
	}
	if string(data) != content {
		t.Errorf("downloaded content = %q, want %q", string(data), content)
	}

	entries, err := os.ReadDir(destDir)
	if err != nil {
		t.Fatalf("cannot read destination directory: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("destination directory has %d entries, want exactly 1 (no stray temp file left behind)", len(entries))
	}
}

// TestDownloadProcessDefaultDestinationIsCurrentDirectory proves the documented default: an unset
// --destination writes into the current directory, not some other implicit location.
func TestDownloadProcessDefaultDestinationIsCurrentDirectory(t *testing.T) {
	t.Chdir(t.TempDir())

	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("content"))
	}, false)
	// destination flag is left unset ("")

	if err := downloadProcess(cmd, []string{"artifact.txt"}); err != nil {
		t.Fatalf("downloadProcess() error = %v", err)
	}

	data, err := os.ReadFile("artifact.txt")
	if err != nil {
		t.Fatalf("expected artifact.txt in the current directory: %v", err)
	}
	if string(data) != "content" {
		t.Errorf("downloaded content = %q, want %q", string(data), "content")
	}
}

// TestDownloadProcessPathTraversalNameSanitized proves the destination path is always
// filepath.Base(name) joined under --destination: a "../"-laden artifact name can never write
// outside the destination directory.
func TestDownloadProcessPathTraversalNameSanitized(t *testing.T) {
	destDir := t.TempDir()

	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("payload"))
	}, false)
	if err := cmd.Flags().Set("destination", destDir); err != nil {
		t.Fatalf("cannot set destination flag: %v", err)
	}

	if err := downloadProcess(cmd, []string{"../../../etc/passwd"}); err != nil {
		t.Fatalf("downloadProcess() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(destDir, "passwd"))
	if err != nil {
		t.Fatalf("expected the file at destDir/passwd (base name only), got: %v", err)
	}
	if string(data) != "payload" {
		t.Errorf("downloaded content = %q, want %q", string(data), "payload")
	}

	entries, err := os.ReadDir(destDir)
	if err != nil {
		t.Fatalf("cannot read destination directory: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "passwd" {
		t.Errorf("destination directory entries = %v, want exactly [passwd]", entries)
	}
}

// TestDownloadOneStripsAuthorizationOnCrossHostRedirect proves (rather than assumes) that
// BitBucket's /downloads/<name> 302 redirect to a different host (simulating bbuseruploads) never
// carries this client's Authorization header across: the redirect target here uses "localhost"
// while the initial request goes to "127.0.0.1" (httptest.NewServer's own host), which are
// different Hostname() strings even though both resolve to the loopback interface, so the standard
// library's cross-domain redirect header stripping applies exactly as it would against a real,
// distinct third-party host.
func TestDownloadOneStripsAuthorizationOnCrossHostRedirect(t *testing.T) {
	var uploadRequests int
	var uploadAuthorization string
	upload := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uploadRequests++
		uploadAuthorization = r.Header.Get("Authorization")
		_, _ = w.Write([]byte("artifact-bytes"))
	}))
	defer upload.Close()

	uploadURL, parseErr := url.Parse(upload.URL)
	if parseErr != nil {
		t.Fatalf("cannot parse upload server URL: %v", parseErr)
	}

	destDir := t.TempDir()
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		target := "http://localhost:" + uploadURL.Port() + "/artifact.zip"
		http.Redirect(w, r, target, http.StatusFound)
	}, false)
	if err := cmd.Flags().Set("destination", destDir); err != nil {
		t.Fatalf("cannot set destination flag: %v", err)
	}

	if err := downloadProcess(cmd, []string{"build.zip"}); err != nil {
		t.Fatalf("downloadProcess() error = %v", err)
	}

	if uploadRequests != 1 {
		t.Fatalf("expected the redirect target to receive exactly 1 request, got %d", uploadRequests)
	}
	if uploadAuthorization != "" {
		t.Errorf("Authorization header reached the redirect target as %q, want it stripped on a cross-host redirect", uploadAuthorization)
	}

	data, err := os.ReadFile(filepath.Join(destDir, "build.zip"))
	if err != nil {
		t.Fatalf("cannot read downloaded file: %v", err)
	}
	if string(data) != "artifact-bytes" {
		t.Errorf("downloaded content = %q, want %q", string(data), "artifact-bytes")
	}
}

// TestDownloadOneForwardsAuthorizationOnSameHostRedirect is the control for the test above: a
// redirect that stays on the same host must still carry Authorization, proving the stripping seen
// above is specifically host-based rather than every redirect losing the header (which would make
// the cross-host test above pass for the wrong reason).
func TestDownloadOneForwardsAuthorizationOnSameHostRedirect(t *testing.T) {
	var hopCount int
	var finalAuthorization string
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		hopCount++
		if hopCount == 1 {
			http.Redirect(w, r, r.URL.Path+"-final", http.StatusFound)
			return
		}
		finalAuthorization = r.Header.Get("Authorization")
		_, _ = w.Write([]byte("bytes"))
	}, false)
	destDir := t.TempDir()
	if err := cmd.Flags().Set("destination", destDir); err != nil {
		t.Fatalf("cannot set destination flag: %v", err)
	}

	if err := downloadProcess(cmd, []string{"build.zip"}); err != nil {
		t.Fatalf("downloadProcess() error = %v", err)
	}

	if hopCount != 2 {
		t.Fatalf("expected exactly 2 hops (initial + same-host redirect), got %d", hopCount)
	}
	if finalAuthorization == "" {
		t.Error("Authorization header missing on the final hop of a same-host redirect, want it forwarded")
	}
}

func TestDownloadProcessAPIErrorLeavesNoStrayFile(t *testing.T) {
	destDir := t.TempDir()
	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"artifact not found"}}`))
	}, false)
	if err := cmd.Flags().Set("destination", destDir); err != nil {
		t.Fatalf("cannot set destination flag: %v", err)
	}

	err := downloadProcess(cmd, []string{"missing.log"})
	if err == nil {
		t.Fatal("downloadProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "artifact not found") {
		t.Errorf("error = %q, want it to contain the BitBucket error message", err.Error())
	}

	entries, err := os.ReadDir(destDir)
	if err != nil {
		t.Fatalf("cannot read destination directory: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("destination directory has %d entries after a failed download, want 0 (no stray temp file)", len(entries))
	}
}

func TestDownloadProcessDryRun(t *testing.T) {
	destDir := t.TempDir()
	var requestCount int
	cmd := setupTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, true)
	if err := cmd.Flags().Set("destination", destDir); err != nil {
		t.Fatalf("cannot set destination flag: %v", err)
	}

	if err := downloadProcess(cmd, []string{"build.log"}); err != nil {
		t.Fatalf("downloadProcess() error = %v", err)
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request in dry-run mode, got %d", requestCount)
	}

	entries, err := os.ReadDir(destDir)
	if err != nil {
		t.Fatalf("cannot read destination directory: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("dry-run should not create any file, found %d entries", len(entries))
	}
}

// TestDownloadProcessStopOnErrorAbortsRemainingNames proves the default (StopOnError) branch of
// the error-tolerance matrix: the first failing name aborts the loop immediately, so a second name
// is never even attempted.
func TestDownloadProcessStopOnErrorAbortsRemainingNames(t *testing.T) {
	destDir := t.TempDir()
	var requestPaths []string
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requestPaths = append(requestPaths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"not found"}}`))
	}, false)
	if err := cmd.Flags().Set("destination", destDir); err != nil {
		t.Fatalf("cannot set destination flag: %v", err)
	}

	err := downloadProcess(cmd, []string{"first.log", "second.log"})
	if err == nil {
		t.Fatal("downloadProcess() expected an error, got nil")
	}
	if len(requestPaths) != 1 {
		t.Errorf("expected exactly 1 request (stop-on-error aborts before the second name), got %d: %v", len(requestPaths), requestPaths)
	}
}

// TestDownloadProcessWarnOnErrorProcessesAllNames proves the --warn-on-error branch: every name is
// still attempted, the failure is reported on stderr, and the command itself succeeds (nil error).
func TestDownloadProcessWarnOnErrorProcessesAllNames(t *testing.T) {
	destDir := t.TempDir()
	var requestPaths []string
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requestPaths = append(requestPaths, r.URL.Path)
		if strings.HasSuffix(r.URL.Path, "good.log") {
			_, _ = w.Write([]byte("ok"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"not found"}}`))
	}, false)
	if err := cmd.Flags().Set("destination", destDir); err != nil {
		t.Fatalf("cannot set destination flag: %v", err)
	}
	if err := cmd.Flags().Set("warn-on-error", "true"); err != nil {
		t.Fatalf("cannot set warn-on-error flag: %v", err)
	}

	stderr := testutil.CaptureStderr(t, func() {
		if err := downloadProcess(cmd, []string{"bad.log", "good.log"}); err != nil {
			t.Fatalf("downloadProcess() with --warn-on-error should not return an error, got %v", err)
		}
	})

	if len(requestPaths) != 2 {
		t.Fatalf("expected both names to be attempted, got %d requests: %v", len(requestPaths), requestPaths)
	}
	if !strings.Contains(stderr, "bad.log") {
		t.Errorf("stderr = %q, want it to mention the failed artifact", stderr)
	}
	if _, err := os.Stat(filepath.Join(destDir, "good.log")); err != nil {
		t.Errorf("expected good.log to have been downloaded despite bad.log failing: %v", err)
	}
}

// TestDownloadProcessIgnoreErrorsSucceedsSilently proves the --ignore-errors branch: a failing
// name is swallowed entirely (no stderr output required, nil error returned).
func TestDownloadProcessIgnoreErrorsSucceedsSilently(t *testing.T) {
	destDir := t.TempDir()
	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"not found"}}`))
	}, false)
	if err := cmd.Flags().Set("destination", destDir); err != nil {
		t.Fatalf("cannot set destination flag: %v", err)
	}
	if err := cmd.Flags().Set("ignore-errors", "true"); err != nil {
		t.Fatalf("cannot set ignore-errors flag: %v", err)
	}

	if err := downloadProcess(cmd, []string{"bad.log"}); err != nil {
		t.Fatalf("downloadProcess() with --ignore-errors should not return an error, got %v", err)
	}
}
