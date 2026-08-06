package step

import (
	"net/http"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
)

func TestLogsProcessSuccess(t *testing.T) {
	const logBody = "+ go build .\nok\n"

	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(logBody))
	}, false)
	if err := cmd.Flags().Set("pipeline", "42"); err != nil {
		t.Fatalf("cannot set pipeline flag: %v", err)
	}

	stdout := testutil.CaptureStdout(t, func() {
		if err := logsProcess(cmd, []string{"{cec5beef-dead-deed-bead-5ae1bedd9ada}"}); err != nil {
			t.Fatalf("logsProcess() error = %v", err)
		}
	})

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/pipelines/42/steps/{cec5beef-dead-deed-bead-5ae1bedd9ada}/log"
	if requests[0].URL.Path != wantPath {
		t.Errorf("path = %s, want %s", requests[0].URL.Path, wantPath)
	}
	if stdout != logBody {
		t.Errorf("stdout = %q, want the raw log body %q copied unchanged", stdout, logBody)
	}
}

func TestLogsProcessAPIError(t *testing.T) {
	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	}, false)
	if err := cmd.Flags().Set("pipeline", "42"); err != nil {
		t.Fatalf("cannot set pipeline flag: %v", err)
	}

	err := logsProcess(cmd, []string{"missing"})
	if err == nil {
		t.Fatal("logsProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "cannot get logs for step missing") {
		t.Errorf("error = %q, want it to mention the logs failure", err.Error())
	}
}

// TestLogsProcessDryRun proves the missing --dry-run check upstream skipped is now present.
func TestLogsProcessDryRun(t *testing.T) {
	var requestCount int
	cmd := setupTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, true)
	if err := cmd.Flags().Set("pipeline", "42"); err != nil {
		t.Fatalf("cannot set pipeline flag: %v", err)
	}

	if err := logsProcess(cmd, []string{"{cec5beef-dead-deed-bead-5ae1bedd9ada}"}); err != nil {
		t.Fatalf("logsProcess() error = %v", err)
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request in dry-run mode, got %d", requestCount)
	}
}
