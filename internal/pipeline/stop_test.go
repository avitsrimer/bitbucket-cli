package pipeline

import (
	"net/http"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
	"github.com/spf13/cobra"
)

// setupStopTest extends setupTest with the --force flag stopProcess reads directly off cmd.
func setupStopTest(t *testing.T, handler http.HandlerFunc, dryRun bool) *cobra.Command {
	t.Helper()
	cmd := setupTest(t, handler, dryRun)
	cmd.Flags().Bool("force", false, "")
	return cmd
}

// stopPreflightHandler responds to the preflight existence GET with a minimal pipeline body,
// dispatching any other method to handleWrite -- the shape every stopProcess test needs since the
// preflight GET now always runs before the confirmation prompt and the stop write itself.
func stopPreflightHandler(requests *[]*http.Request, handleWrite http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		*requests = append(*requests, r)
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"uuid":"{11111111-1111-1111-1111-111111111111}"}`))
			return
		}
		handleWrite(w, r)
	}
}

func TestStopProcessConfirmYesProceeds(t *testing.T) {
	var requests []*http.Request
	cmd := setupStopTest(t, stopPreflightHandler(&requests, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), false)
	cmd.SetIn(strings.NewReader("y\n"))

	stdout := testutil.CaptureStdout(t, func() {
		if err := stopProcess(cmd, []string{"42"}); err != nil {
			t.Fatalf("stopProcess() error = %v", err)
		}
	})

	if len(requests) != 2 {
		t.Fatalf("expected exactly 2 requests (preflight GET, stop POST), got %d", len(requests))
	}
	if requests[0].Method != http.MethodGet {
		t.Errorf("first request method = %s, want GET (preflight existence check)", requests[0].Method)
	}
	wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/pipelines/42/stopPipeline"
	if requests[1].URL.Path != wantPath {
		t.Errorf("path = %s, want %s", requests[1].URL.Path, wantPath)
	}
	if !strings.Contains(stdout, "204 No Content") {
		t.Errorf("stdout = %q, want it to report the actual API status", stdout)
	}
}

func TestStopProcessConfirmNoZeroHandlerHits(t *testing.T) {
	var requests []*http.Request
	cmd := setupStopTest(t, stopPreflightHandler(&requests, func(http.ResponseWriter, *http.Request) {
		t.Error("no stop request expected")
	}), false)
	cmd.SetIn(strings.NewReader("n\n"))

	stdout := testutil.CaptureStdout(t, func() {
		if err := stopProcess(cmd, []string{"42"}); err != nil {
			t.Fatalf("stopProcess() error = %v", err)
		}
	})

	if len(requests) != 1 || requests[0].Method != http.MethodGet {
		t.Errorf("requests = %v, want exactly one preflight GET and no stop request when confirmation is declined", requests)
	}
	if !strings.Contains(stdout, "canceled") {
		t.Errorf("stdout = %q, want it to mention the stop was canceled", stdout)
	}
}

func TestStopProcessForceSkipsPrompt(t *testing.T) {
	var requests []*http.Request
	cmd := setupStopTest(t, stopPreflightHandler(&requests, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), false)
	_ = cmd.Flags().Set("force", "true")
	cmd.SetIn(poisonStdin{t})

	testutil.CaptureStdout(t, func() {
		if err := stopProcess(cmd, []string{"42"}); err != nil {
			t.Fatalf("stopProcess() error = %v", err)
		}
	})

	if len(requests) != 2 {
		t.Errorf("expected exactly 2 HTTP requests (preflight GET, stop POST) with --force, got %d", len(requests))
	}
}

func TestStopProcessDryRunWithoutForceOrTTYDoesNotError(t *testing.T) {
	var requests []*http.Request
	cmd := setupStopTest(t, stopPreflightHandler(&requests, func(http.ResponseWriter, *http.Request) {
		t.Error("no stop request expected in dry-run mode")
	}), true)
	swapStdinToNonInteractivePipe(t)

	if err := stopProcess(cmd, []string{"42"}); err != nil {
		t.Fatalf("stopProcess() error = %v, want nil in dry-run mode", err)
	}
	if len(requests) != 1 || requests[0].Method != http.MethodGet {
		t.Errorf("requests = %v, want exactly one preflight GET and no stop request in dry-run mode", requests)
	}
}

func TestStopProcessNonInteractiveWithoutForceErrors(t *testing.T) {
	var requests []*http.Request
	cmd := setupStopTest(t, stopPreflightHandler(&requests, func(http.ResponseWriter, *http.Request) {
		t.Error("no stop request expected")
	}), false)
	swapStdinToNonInteractivePipe(t)

	err := stopProcess(cmd, []string{"42"})
	if err == nil {
		t.Fatal("stopProcess() expected an error for non-interactive stdin without --force, got nil")
	}
	if len(requests) != 1 || requests[0].Method != http.MethodGet {
		t.Errorf("requests = %v, want exactly one preflight GET and no stop request", requests)
	}
}

// TestStopProcessPreflightAPIError verifies that a nonexistent pipeline (a failure of the
// preflight existence GET) fails stopProcess before the confirmation prompt or any stop request,
// with the same error a real stop against that id would eventually produce.
func TestStopProcessPreflightAPIError(t *testing.T) {
	var requests []*http.Request
	cmd := setupStopTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"pipeline not found"}}`))
	}, false)
	_ = cmd.Flags().Set("force", "true")

	err := stopProcess(cmd, []string{"99"})
	if err == nil {
		t.Fatal("stopProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "pipeline not found") {
		t.Errorf("error = %q, want it to contain the BitBucket error message", err.Error())
	}
	if len(requests) != 1 || requests[0].Method != http.MethodGet {
		t.Errorf("requests = %v, want exactly one preflight GET and no stop request", requests)
	}
}

// TestStopProcessWriteAPIError verifies that a failure of the stop request itself (as opposed to
// the preflight existence GET) still surfaces the BitBucket error, after the preflight GET
// succeeded.
func TestStopProcessWriteAPIError(t *testing.T) {
	var requests []*http.Request
	cmd := setupStopTest(t, stopPreflightHandler(&requests, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"pipeline already stopped"}}`))
	}), false)
	_ = cmd.Flags().Set("force", "true")

	err := stopProcess(cmd, []string{"42"})
	if err == nil {
		t.Fatal("stopProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "pipeline already stopped") {
		t.Errorf("error = %q, want it to contain the BitBucket error message", err.Error())
	}
	if len(requests) != 2 {
		t.Errorf("expected exactly 2 requests (preflight GET, stop POST), got %d", len(requests))
	}
}

// TestStopProcessRejectsInvalidPipelineID proves the <pipeline-uuid-or-build-number> positional
// is validated via common.ValidatePathIdentifier before any request is sent -- this is a
// mutating command, so an unvalidated "../.." could otherwise reach the preflight GET and, worse,
// the stop POST against a different, unintended resource.
func TestStopProcessRejectsInvalidPipelineID(t *testing.T) {
	var requestCount int
	cmd := setupStopTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, false)
	_ = cmd.Flags().Set("force", "true")

	err := stopProcess(cmd, []string{"../.."})
	if err == nil {
		t.Fatal("stopProcess() expected an error for '../..', got nil")
	}
	if !strings.Contains(err.Error(), "pipeline") {
		t.Errorf("error = %q, want it to name the pipeline argument", err.Error())
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request for an invalid pipeline id, got %d", requestCount)
	}
}
