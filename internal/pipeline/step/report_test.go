package step

import (
	"net/http"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
)

func TestReportProcessSuccess(t *testing.T) {
	const reportBody = `{"passed":10,"failed":0}`

	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(reportBody))
	}, false)
	if err := cmd.Flags().Set("pipeline", "42"); err != nil {
		t.Fatalf("cannot set pipeline flag: %v", err)
	}

	stdout := testutil.CaptureStdout(t, func() {
		if err := reportProcess(cmd, []string{"{cec5beef-dead-deed-bead-5ae1bedd9ada}"}); err != nil {
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

func TestReportProcessAPIError(t *testing.T) {
	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	}, false)
	if err := cmd.Flags().Set("pipeline", "42"); err != nil {
		t.Fatalf("cannot set pipeline flag: %v", err)
	}

	err := reportProcess(cmd, []string{"missing"})
	if err == nil {
		t.Fatal("reportProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "cannot get test report for step missing") {
		t.Errorf("error = %q, want it to mention the report failure", err.Error())
	}
}

func TestReportProcessDryRun(t *testing.T) {
	var requestCount int
	cmd := setupTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, true)
	if err := cmd.Flags().Set("pipeline", "42"); err != nil {
		t.Fatalf("cannot set pipeline flag: %v", err)
	}

	if err := reportProcess(cmd, []string{"{cec5beef-dead-deed-bead-5ae1bedd9ada}"}); err != nil {
		t.Fatalf("reportProcess() error = %v", err)
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request in dry-run mode, got %d", requestCount)
	}
}
