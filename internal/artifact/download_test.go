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

// TestDownloadProcessNameWithSpecialCharacters proves the three cases that, before name was
// escaped before reaching GetPath, either silently sent the download request to the API root
// (a bare "%") or mistook part of the filename for a query string (a "?") -- verified empirically
// against Go's net/url: url.URL.JoinPath treats each path element as already percent-encoded, so
// an unescaped "%" that isn't a valid escape sequence makes its internal setPath fail; JoinPath
// swallows that error and returns the URL essentially unmodified (no path at all), and a
// downstream request would then hit the bare API root with an authenticated GET, downloading the
// API index document under the artifact's intended filename with no error.
func TestDownloadProcessNameWithSpecialCharacters(t *testing.T) {
	tests := []struct {
		name         string
		artifactName string
	}{
		{name: "bare percent sign", artifactName: "release (100%).zip"},
		{name: "question mark", artifactName: "a?b.zip"},
		{name: "literal percent-two-five", artifactName: "50%25.zip"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

			if err := downloadProcess(cmd, []string{tt.artifactName}); err != nil {
				t.Fatalf("downloadProcess() error = %v", err)
			}

			if len(requests) != 1 {
				t.Fatalf("expected exactly 1 request, got %d", len(requests))
			}
			wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/downloads/" + tt.artifactName
			if requests[0].URL.Path != wantPath {
				t.Errorf("request path = %q, want %q (the request must never silently target the API root)", requests[0].URL.Path, wantPath)
			}
			if requests[0].URL.Path == "" || requests[0].URL.Path == "/" {
				t.Fatalf("request path = %q, silently hit the API root instead of the downloads endpoint", requests[0].URL.Path)
			}

			data, err := os.ReadFile(filepath.Join(destDir, filepath.Base(tt.artifactName)))
			if err != nil {
				t.Fatalf("cannot read downloaded file: %v", err)
			}
			if string(data) != content {
				t.Errorf("downloaded content = %q, want %q", string(data), content)
			}
		})
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

// TestDownloadProcessPathTraversalNameRejected proves a "../"-laden artifact name is rejected by
// common.ValidatePathIdentifier before any request is sent or any local file written: it contains
// a path separator, which the destination path's own filepath.Base(name) sanitization would
// otherwise silently collapse to a different, unexpected local filename.
func TestDownloadProcessPathTraversalNameRejected(t *testing.T) {
	destDir := t.TempDir()

	var requestCount int
	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		requestCount++
		_, _ = w.Write([]byte("payload"))
	}, false)
	if err := cmd.Flags().Set("destination", destDir); err != nil {
		t.Fatalf("cannot set destination flag: %v", err)
	}

	err := downloadProcess(cmd, []string{"../../../etc/passwd"})
	if err == nil {
		t.Fatal("downloadProcess() expected an error for a name containing a path separator, got nil")
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request for an invalid name, got %d", requestCount)
	}

	entries, err := os.ReadDir(destDir)
	if err != nil {
		t.Fatalf("cannot read destination directory: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("destination directory entries = %v, want none", entries)
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

// TestDownloadProcessMissingDestinationErrors proves the documented behavior: "the destination
// directory itself is never created and must already exist". Passing a --destination that
// doesn't exist must surface an error (from the underlying os.CreateTemp), not silently succeed
// or panic.
func TestDownloadProcessMissingDestinationErrors(t *testing.T) {
	missingDir := filepath.Join(t.TempDir(), "does-not-exist")
	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("content"))
	}, false)
	if err := cmd.Flags().Set("destination", missingDir); err != nil {
		t.Fatalf("cannot set destination flag: %v", err)
	}

	err := downloadProcess(cmd, []string{"build.log"})
	if err == nil {
		t.Fatal("downloadProcess() expected an error for a missing destination directory, got nil")
	}
	if _, statErr := os.Stat(missingDir); statErr == nil {
		t.Error("downloadProcess() created the destination directory, want it to require the directory already exist")
	}
}

// TestDownloadProcessOverwritesExistingFile proves the documented overwrite behavior: a name that
// already exists at the destination is replaced with the newly downloaded content.
func TestDownloadProcessOverwritesExistingFile(t *testing.T) {
	destDir := t.TempDir()
	destPath := filepath.Join(destDir, "build.log")
	if err := os.WriteFile(destPath, []byte("stale content"), 0o644); err != nil {
		t.Fatalf("cannot seed existing destination file: %v", err)
	}

	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("fresh content"))
	}, false)
	if err := cmd.Flags().Set("destination", destDir); err != nil {
		t.Fatalf("cannot set destination flag: %v", err)
	}

	if err := downloadProcess(cmd, []string{"build.log"}); err != nil {
		t.Fatalf("downloadProcess() error = %v", err)
	}

	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("cannot read downloaded file: %v", err)
	}
	if string(data) != "fresh content" {
		t.Errorf("downloaded content = %q, want %q (overwritten)", string(data), "fresh content")
	}
}

// TestDownloadOneOverwritePreservesExistingMode proves downloaded artifacts no longer silently
// downgrade an existing destination file's mode to os.CreateTemp's owner-only 0600: overwriting a
// 0644 file must leave it 0644.
func TestDownloadOneOverwritePreservesExistingMode(t *testing.T) {
	destDir := t.TempDir()
	destPath := filepath.Join(destDir, "build.log")
	if err := os.WriteFile(destPath, []byte("stale"), 0o644); err != nil {
		t.Fatalf("cannot seed existing destination file: %v", err)
	}

	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("fresh"))
	}, false)
	if err := cmd.Flags().Set("destination", destDir); err != nil {
		t.Fatalf("cannot set destination flag: %v", err)
	}

	if err := downloadProcess(cmd, []string{"build.log"}); err != nil {
		t.Fatalf("downloadProcess() error = %v", err)
	}

	info, err := os.Stat(destPath)
	if err != nil {
		t.Fatalf("cannot stat downloaded file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Errorf("mode after overwrite = %v, want the pre-existing file's mode (0644) preserved, not os.CreateTemp's 0600", got)
	}
}

// TestDownloadOneNewFileIsNotOwnerOnlyMode proves a newly downloaded artifact (no pre-existing
// file at the destination) does not keep os.CreateTemp's owner-only 0600: it should land at
// defaultNewFileMode (0644) instead.
func TestDownloadOneNewFileIsNotOwnerOnlyMode(t *testing.T) {
	destDir := t.TempDir()
	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("content"))
	}, false)
	if err := cmd.Flags().Set("destination", destDir); err != nil {
		t.Fatalf("cannot set destination flag: %v", err)
	}

	if err := downloadProcess(cmd, []string{"build.log"}); err != nil {
		t.Fatalf("downloadProcess() error = %v", err)
	}

	info, err := os.Stat(filepath.Join(destDir, "build.log"))
	if err != nil {
		t.Fatalf("cannot stat downloaded file: %v", err)
	}
	if got := info.Mode().Perm(); got != defaultNewFileMode {
		t.Errorf("mode = %v, want defaultNewFileMode (%v), not os.CreateTemp's owner-only 0600", got, defaultNewFileMode)
	}
}

// TestDownloadProcessFailureNeverCorruptsExistingFile proves the whole point of the temp-file +
// rename design stated in download.go's own doc comment: a failing download must never corrupt a
// file already at that destination, since the destination path is only ever touched by the final
// rename, which never runs when the download itself failed.
func TestDownloadProcessFailureNeverCorruptsExistingFile(t *testing.T) {
	destDir := t.TempDir()
	destPath := filepath.Join(destDir, "build.log")
	const originalContent = "untouched original content"
	if err := os.WriteFile(destPath, []byte(originalContent), 0o644); err != nil {
		t.Fatalf("cannot seed existing destination file: %v", err)
	}

	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"server exploded"}}`))
	}, false)
	if err := cmd.Flags().Set("destination", destDir); err != nil {
		t.Fatalf("cannot set destination flag: %v", err)
	}

	if err := downloadProcess(cmd, []string{"build.log"}); err == nil {
		t.Fatal("downloadProcess() expected an error, got nil")
	}

	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("cannot read destination file after failed download: %v", err)
	}
	if string(data) != originalContent {
		t.Errorf("destination file content = %q after a failed download, want the original %q untouched", string(data), originalContent)
	}

	entries, err := os.ReadDir(destDir)
	if err != nil {
		t.Fatalf("cannot read destination directory: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("destination directory has %d entries after a failed download, want exactly 1 (no stray temp file)", len(entries))
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
// name is swallowed entirely (nil error returned, nothing on stderr), and processing continues to
// the remaining names rather than stopping at the first failure -- contrast the StopOnError test,
// which stops after exactly 1 request.
func TestDownloadProcessIgnoreErrorsSucceedsSilently(t *testing.T) {
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
	if err := cmd.Flags().Set("ignore-errors", "true"); err != nil {
		t.Fatalf("cannot set ignore-errors flag: %v", err)
	}

	stderr := testutil.CaptureStderr(t, func() {
		if err := downloadProcess(cmd, []string{"bad.log", "good.log"}); err != nil {
			t.Fatalf("downloadProcess() with --ignore-errors should not return an error, got %v", err)
		}
	})

	if len(requestPaths) != 2 {
		t.Fatalf("expected both names to be attempted despite the first failing, got %d requests: %v", len(requestPaths), requestPaths)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want no output under --ignore-errors", stderr)
	}
	if _, err := os.Stat(filepath.Join(destDir, "good.log")); err != nil {
		t.Errorf("expected good.log to have been downloaded despite bad.log failing: %v", err)
	}
}

// TestDownloadProcessIgnoreErrorsOnFullSuccessNeverLogsNilJoin is the regression test for the
// nil-join defect: with --ignore-errors set and every download succeeding, errs is empty and
// errors.Join(errs...) is nil. The buggy shape of this code logged the [WARN] "ignoring errors"
// line unconditionally once ShouldIgnoreErrors was true, formatting that nil error as the literal
// string "%!s(<nil>)" -- a warning about nothing, printed on every single successful
// --ignore-errors run. common.TolerateErrors (and, before that fix, an explicit `if joined ==
// nil` guard) must return before ever reaching that log line.
func TestDownloadProcessIgnoreErrorsOnFullSuccessNeverLogsNilJoin(t *testing.T) {
	destDir := t.TempDir()
	logBuf := testutil.CaptureLog(t)

	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}, false)
	if err := cmd.Flags().Set("destination", destDir); err != nil {
		t.Fatalf("cannot set destination flag: %v", err)
	}
	if err := cmd.Flags().Set("ignore-errors", "true"); err != nil {
		t.Fatalf("cannot set ignore-errors flag: %v", err)
	}

	if err := downloadProcess(cmd, []string{"good.log"}); err != nil {
		t.Fatalf("downloadProcess() error = %v", err)
	}

	logged := logBuf.String()
	if strings.Contains(logged, "<nil>") {
		t.Errorf("log output = %q, want no nil-formatted warning when every download succeeded", logged)
	}
	if strings.Contains(logged, "ignoring errors") {
		t.Errorf("log output = %q, want no \"ignoring errors\" warning when there was nothing to ignore", logged)
	}
}

// TestDownloadProcessRejectsEmptyDotAndDotDotName reproduces the FINAL CRITICAL GATE's priority-4
// finding: repo.GetPath("downloads", url.PathEscape(name)) runs its segments through path.Join,
// which collapses an empty, ".", or ".." name away instead of erroring -- "" or "." retargeted
// the request at the downloads *list* endpoint (an authenticated request whose JSON body would
// then be written to a temp file before the rename failed), and ".." removed "downloads"
// entirely, hitting the repository resource itself. downloadProcess must reject all three before
// ever sending a request.
func TestDownloadProcessRejectsEmptyDotAndDotDotName(t *testing.T) {
	for _, name := range []string{"", ".", ".."} {
		t.Run("name="+name, func(t *testing.T) {
			destDir := t.TempDir()
			var requestCount int
			cmd := setupTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, false)
			if err := cmd.Flags().Set("destination", destDir); err != nil {
				t.Fatalf("cannot set destination flag: %v", err)
			}

			err := downloadProcess(cmd, []string{name})
			if err == nil {
				t.Fatalf("downloadProcess(%q) error = nil, want an error", name)
			}
			if requestCount != 0 {
				t.Errorf("expected zero HTTP requests for name %q, got %d: it must be rejected before reaching the API", name, requestCount)
			}

			entries, err := os.ReadDir(destDir)
			if err != nil {
				t.Fatalf("cannot read destination directory: %v", err)
			}
			if len(entries) != 0 {
				t.Errorf("destination directory has %d entries, want zero: no temp/downloaded file for a rejected name", len(entries))
			}
		})
	}
}
