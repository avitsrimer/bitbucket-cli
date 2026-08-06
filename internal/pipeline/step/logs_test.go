package step

import (
	"net/http"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
)

func TestLogsProcessSuccessUUID(t *testing.T) {
	const logBody = "+ go build .\nok\n"

	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(logBody))
	}, false)

	stdout := testutil.CaptureStdout(t, func() {
		if err := logsProcess(cmd, []string{"42", "{cec5beef-dead-deed-bead-5ae1bedd9ada}"}); err != nil {
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

// TestLogsProcessSuccessName proves a step name resolves to its UUID (via a list request) before
// the logs request is issued against the resolved UUID's path.
func TestLogsProcessSuccessName(t *testing.T) {
	const logBody = "+ go build .\nok\n"

	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		if strings.HasSuffix(r.URL.Path, "/steps") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"values":[{"type":"pipeline_step","uuid":"{cec5beef-dead-deed-bead-5ae1bedd9ada}","name":"Build"}]}`))
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(logBody))
	}, false)

	stdout := testutil.CaptureStdout(t, func() {
		if err := logsProcess(cmd, []string{"42", "Build"}); err != nil {
			t.Fatalf("logsProcess() error = %v", err)
		}
	})

	if len(requests) != 2 {
		t.Fatalf("expected exactly 2 requests (list then logs), got %d", len(requests))
	}
	wantLogsPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/pipelines/42/steps/{cec5beef-dead-deed-bead-5ae1bedd9ada}/log"
	if requests[1].URL.Path != wantLogsPath {
		t.Errorf("second request path = %s, want %s", requests[1].URL.Path, wantLogsPath)
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

	err := logsProcess(cmd, []string{"42", "{cec5beef-dead-deed-bead-5ae1bedd9ada}"})
	if err == nil {
		t.Fatal("logsProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "cannot get logs for step {cec5beef-dead-deed-bead-5ae1bedd9ada}") {
		t.Errorf("error = %q, want it to mention the logs failure", err.Error())
	}
}

// TestLogsProcessDryRun proves logsProcess checks --dry-run before issuing any request.
func TestLogsProcessDryRun(t *testing.T) {
	var requestCount int
	cmd := setupTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, true)

	if err := logsProcess(cmd, []string{"42", "{cec5beef-dead-deed-bead-5ae1bedd9ada}"}); err != nil {
		t.Fatalf("logsProcess() error = %v", err)
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request in dry-run mode, got %d", requestCount)
	}
}
