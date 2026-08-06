package step

import (
	"net/http"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
)

func TestCasesProcessSuccessUUID(t *testing.T) {
	const casesBody = `[{"name":"TestFoo","status":"PASSED"}]`

	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(casesBody))
	}, false)

	stdout := testutil.CaptureStdout(t, func() {
		if err := casesProcess(cmd, []string{"42", "{cec5beef-dead-deed-bead-5ae1bedd9ada}"}); err != nil {
			t.Fatalf("casesProcess() error = %v", err)
		}
	})

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/pipelines/42/steps/{cec5beef-dead-deed-bead-5ae1bedd9ada}/test_reports/test_cases"
	if requests[0].URL.Path != wantPath {
		t.Errorf("path = %s, want %s", requests[0].URL.Path, wantPath)
	}
	if stdout != casesBody {
		t.Errorf("stdout = %q, want the raw cases body %q copied unchanged", stdout, casesBody)
	}
}

// TestCasesProcessSuccessName proves a step name resolves to its UUID before the cases request is
// issued.
func TestCasesProcessSuccessName(t *testing.T) {
	const casesBody = `[{"name":"TestFoo","status":"PASSED"}]`

	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		if strings.HasSuffix(r.URL.Path, "/steps") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"values":[{"type":"pipeline_step","uuid":"{cec5beef-dead-deed-bead-5ae1bedd9ada}","name":"Test"}]}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(casesBody))
	}, false)

	stdout := testutil.CaptureStdout(t, func() {
		if err := casesProcess(cmd, []string{"42", "Test"}); err != nil {
			t.Fatalf("casesProcess() error = %v", err)
		}
	})

	if len(requests) != 2 {
		t.Fatalf("expected exactly 2 requests (list then cases), got %d", len(requests))
	}
	if stdout != casesBody {
		t.Errorf("stdout = %q, want the raw cases body %q copied unchanged", stdout, casesBody)
	}
}

func TestCasesProcessAPIError(t *testing.T) {
	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	}, false)

	err := casesProcess(cmd, []string{"42", "{cec5beef-dead-deed-bead-5ae1bedd9ada}"})
	if err == nil {
		t.Fatal("casesProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "cannot get test cases for step {cec5beef-dead-deed-bead-5ae1bedd9ada}") {
		t.Errorf("error = %q, want it to mention the cases failure", err.Error())
	}
}

func TestCasesProcessDryRun(t *testing.T) {
	var requestCount int
	cmd := setupTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, true)

	if err := casesProcess(cmd, []string{"42", "{cec5beef-dead-deed-bead-5ae1bedd9ada}"}); err != nil {
		t.Fatalf("casesProcess() error = %v", err)
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request in dry-run mode, got %d", requestCount)
	}
}
