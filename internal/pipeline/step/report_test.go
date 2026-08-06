package step

import (
	"net/http"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
)

func TestReportProcessSuccessUUID(t *testing.T) {
	const reportBody = `{"passed":10,"failed":0}`

	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(reportBody))
	}, false)

	stdout := testutil.CaptureStdout(t, func() {
		if err := reportProcess(cmd, []string{"42", "{cec5beef-dead-deed-bead-5ae1bedd9ada}"}); err != nil {
			t.Fatalf("reportProcess() error = %v", err)
		}
	})

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/pipelines/42/steps/{cec5beef-dead-deed-bead-5ae1bedd9ada}/test_reports"
	if requests[0].URL.Path != wantPath {
		t.Errorf("path = %s, want %s", requests[0].URL.Path, wantPath)
	}
	if stdout != reportBody {
		t.Errorf("stdout = %q, want the raw report body %q copied unchanged", stdout, reportBody)
	}
}

// TestReportProcessSuccessName proves a step name resolves to its UUID before the report request
// is issued.
func TestReportProcessSuccessName(t *testing.T) {
	const reportBody = `{"passed":10,"failed":0}`

	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		if strings.HasSuffix(r.URL.Path, "/steps") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"values":[{"type":"pipeline_step","uuid":"{cec5beef-dead-deed-bead-5ae1bedd9ada}","name":"Test"}]}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(reportBody))
	}, false)

	stdout := testutil.CaptureStdout(t, func() {
		if err := reportProcess(cmd, []string{"42", "Test"}); err != nil {
			t.Fatalf("reportProcess() error = %v", err)
		}
	})

	if len(requests) != 2 {
		t.Fatalf("expected exactly 2 requests (list then report), got %d", len(requests))
	}
	if stdout != reportBody {
		t.Errorf("stdout = %q, want the raw report body %q copied unchanged", stdout, reportBody)
	}
}

func TestReportProcessAPIError(t *testing.T) {
	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	}, false)

	err := reportProcess(cmd, []string{"42", "{cec5beef-dead-deed-bead-5ae1bedd9ada}"})
	if err == nil {
		t.Fatal("reportProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "cannot get test report for step {cec5beef-dead-deed-bead-5ae1bedd9ada}") {
		t.Errorf("error = %q, want it to mention the report failure", err.Error())
	}
}

func TestReportProcessDryRun(t *testing.T) {
	var requestCount int
	cmd := setupTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, true)

	if err := reportProcess(cmd, []string{"42", "{cec5beef-dead-deed-bead-5ae1bedd9ada}"}); err != nil {
		t.Fatalf("reportProcess() error = %v", err)
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request in dry-run mode, got %d", requestCount)
	}
}
