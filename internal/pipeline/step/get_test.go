package step

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
)

func TestGetProcessSuccessUUID(t *testing.T) {
	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"type":"pipeline_step","uuid":"{cec5beef-dead-deed-bead-5ae1bedd9ada}","name":"Test and Build","run_number":1,"pipeline":{"type":"pipeline","uuid":"{3edaa916-baad-beef-dead-28846deafec1}"},"state":{"type":"pipeline_step_state_completed","name":"COMPLETED"},"image":{"name":"golang:1.25"},"maxTime":120,"duration_in_seconds":19,"started_on":"2026-01-03T07:36:40.109572944Z","completed_on":"2026-01-03T07:36:59.423783406Z"}`))
	}, false)

	stdout := testutil.CaptureStdout(t, func() {
		if err := getProcess(cmd, []string{"42", "{cec5beef-dead-deed-bead-5ae1bedd9ada}"}); err != nil {
			t.Fatalf("getProcess() error = %v", err)
		}
	})

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/pipelines/42/steps/{cec5beef-dead-deed-bead-5ae1bedd9ada}"
	if requests[0].URL.Path != wantPath {
		t.Errorf("path = %s, want %s", requests[0].URL.Path, wantPath)
	}

	var got Step
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if got.Name != "Test and Build" {
		t.Errorf("printed step name = %q, want %q", got.Name, "Test and Build")
	}
}

// TestGetProcessSuccessName proves a step name resolves to its UUID before the step is fetched:
// the resolution request lists steps, and the step-fetch request lands on the resolved UUID's
// path, not the literal name.
func TestGetProcessSuccessName(t *testing.T) {
	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/steps") {
			_, _ = w.Write([]byte(`{"values":[{"type":"pipeline_step","uuid":"{cec5beef-dead-deed-bead-5ae1bedd9ada}","name":"Test and Build"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"type":"pipeline_step","uuid":"{cec5beef-dead-deed-bead-5ae1bedd9ada}","name":"Test and Build","run_number":1,"state":{"type":"pipeline_step_state_completed","name":"COMPLETED"},"image":{"name":"golang:1.25"}}`))
	}, false)

	if err := getProcess(cmd, []string{"42", "Test and Build"}); err != nil {
		t.Fatalf("getProcess() error = %v", err)
	}

	if len(requests) != 2 {
		t.Fatalf("expected exactly 2 requests (list then get), got %d", len(requests))
	}
	wantListPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/pipelines/42/steps"
	if requests[0].URL.Path != wantListPath {
		t.Errorf("first request path = %s, want %s", requests[0].URL.Path, wantListPath)
	}
	wantGetPath := wantListPath + "/{cec5beef-dead-deed-bead-5ae1bedd9ada}"
	if requests[1].URL.Path != wantGetPath {
		t.Errorf("second request path = %s, want %s", requests[1].URL.Path, wantGetPath)
	}
}

// TestGetProcessSuccessNameContainingSlash proves a step whose name legitimately contains a "/"
// (a real bitbucket-pipelines.yml step name, e.g. "build/test") resolves successfully:
// ValidatePathIdentifier guards the resolved UUID that actually reaches GetPath, not the
// user-typed name, which is free to contain a "/".
func TestGetProcessSuccessNameContainingSlash(t *testing.T) {
	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/steps") {
			_, _ = w.Write([]byte(`{"values":[{"type":"pipeline_step","uuid":"{cec5beef-dead-deed-bead-5ae1bedd9ada}","name":"build/test"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"type":"pipeline_step","uuid":"{cec5beef-dead-deed-bead-5ae1bedd9ada}","name":"build/test"}`))
	}, false)

	if err := getProcess(cmd, []string{"42", "build/test"}); err != nil {
		t.Fatalf("getProcess() error = %v, want a step name containing '/' to resolve", err)
	}

	if len(requests) != 2 {
		t.Fatalf("expected exactly 2 requests (list then get), got %d", len(requests))
	}
	wantGetPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/pipelines/42/steps/{cec5beef-dead-deed-bead-5ae1bedd9ada}"
	if requests[1].URL.Path != wantGetPath {
		t.Errorf("second request path = %s, want %s", requests[1].URL.Path, wantGetPath)
	}
}

func TestGetProcessUnknownName(t *testing.T) {
	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[{"type":"pipeline_step","uuid":"{cec5beef-dead-deed-bead-5ae1bedd9ada}","name":"Build"}]}`))
	}, false)

	err := getProcess(cmd, []string{"42", "Deploy"})
	if err == nil {
		t.Fatal("getProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), `"Deploy"`) || !strings.Contains(err.Error(), "Build") {
		t.Errorf("error = %q, want it to name the unresolved value and list available step names", err.Error())
	}
}

func TestGetProcessAmbiguousName(t *testing.T) {
	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[` +
			`{"type":"pipeline_step","uuid":"{11111111-1111-1111-1111-111111111111}","name":"Build"},` +
			`{"type":"pipeline_step","uuid":"{22222222-2222-2222-2222-222222222222}","name":"Build"}` +
			`]}`))
	}, false)

	err := getProcess(cmd, []string{"42", "Build"})
	if err == nil {
		t.Fatal("getProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "{11111111-1111-1111-1111-111111111111}") ||
		!strings.Contains(err.Error(), "{22222222-2222-2222-2222-222222222222}") {
		t.Errorf("error = %q, want it to list both ambiguous uuids", err.Error())
	}
}

func TestGetProcessAPIError(t *testing.T) {
	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"step not found"}}`))
	}, false)

	err := getProcess(cmd, []string{"42", "{cec5beef-dead-deed-bead-5ae1bedd9ada}"})
	if err == nil {
		t.Fatal("getProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "cannot get step {cec5beef-dead-deed-bead-5ae1bedd9ada}") {
		t.Errorf("error = %q, want it to mention the step argument", err.Error())
	}
	if !strings.Contains(err.Error(), "step not found") {
		t.Errorf("error = %q, want it to contain the BitBucket error message", err.Error())
	}
}

func TestGetProcessDryRunSkipsFetchAndPrinting(t *testing.T) {
	var requestCount int
	cmd := setupTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, true)

	stdout := testutil.CaptureStdout(t, func() {
		if err := getProcess(cmd, []string{"42", "{cec5beef-dead-deed-bead-5ae1bedd9ada}"}); err != nil {
			t.Fatalf("getProcess() error = %v", err)
		}
	})
	if requestCount != 0 {
		t.Errorf("expected no HTTP request in dry-run mode, got %d", requestCount)
	}
	if stdout != "" {
		t.Errorf("expected no printed output in dry-run mode, got %q", stdout)
	}
}
