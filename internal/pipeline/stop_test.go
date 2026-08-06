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

func TestStopProcessConfirmYesProceeds(t *testing.T) {
	var requests []*http.Request
	cmd := setupStopTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.WriteHeader(http.StatusNoContent)
	}, false)
	cmd.SetIn(strings.NewReader("y\n"))

	stdout := testutil.CaptureStdout(t, func() {
		if err := stopProcess(cmd, []string{"42"}); err != nil {
			t.Fatalf("stopProcess() error = %v", err)
		}
	})

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/pipelines/42/stopPipeline"
	if requests[0].URL.Path != wantPath {
		t.Errorf("path = %s, want %s", requests[0].URL.Path, wantPath)
	}
	if !strings.Contains(stdout, "204 No Content") {
		t.Errorf("stdout = %q, want it to report the actual API status", stdout)
	}
}

func TestStopProcessConfirmNoZeroHandlerHits(t *testing.T) {
	var requestCount int
	cmd := setupStopTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, false)
	cmd.SetIn(strings.NewReader("n\n"))

	stdout := testutil.CaptureStdout(t, func() {
		if err := stopProcess(cmd, []string{"42"}); err != nil {
			t.Fatalf("stopProcess() error = %v", err)
		}
	})

	if requestCount != 0 {
		t.Errorf("expected no HTTP request when confirmation is declined, got %d", requestCount)
	}
	if !strings.Contains(stdout, "canceled") {
		t.Errorf("stdout = %q, want it to mention the stop was canceled", stdout)
	}
}

func TestStopProcessForceSkipsPrompt(t *testing.T) {
	var requestCount int
	cmd := setupStopTest(t, func(w http.ResponseWriter, _ *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusNoContent)
	}, false)
	_ = cmd.Flags().Set("force", "true")
	cmd.SetIn(poisonStdin{t})

	testutil.CaptureStdout(t, func() {
		if err := stopProcess(cmd, []string{"42"}); err != nil {
			t.Fatalf("stopProcess() error = %v", err)
		}
	})

	if requestCount != 1 {
		t.Errorf("expected exactly 1 HTTP request with --force, got %d", requestCount)
	}
}

func TestStopProcessDryRunWithoutForceOrTTYDoesNotError(t *testing.T) {
	var requestCount int
	cmd := setupStopTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, true)
	swapStdinToNonInteractivePipe(t)

	if err := stopProcess(cmd, []string{"42"}); err != nil {
		t.Fatalf("stopProcess() error = %v, want nil in dry-run mode", err)
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request in dry-run mode, got %d", requestCount)
	}
}

func TestStopProcessNonInteractiveWithoutForceErrors(t *testing.T) {
	var requestCount int
	cmd := setupStopTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, false)
	swapStdinToNonInteractivePipe(t)

	err := stopProcess(cmd, []string{"42"})
	if err == nil {
		t.Fatal("stopProcess() expected an error for non-interactive stdin without --force, got nil")
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request, got %d", requestCount)
	}
}

func TestStopProcessAPIError(t *testing.T) {
	cmd := setupStopTest(t, func(w http.ResponseWriter, _ *http.Request) {
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
}
