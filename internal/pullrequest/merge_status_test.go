package pullrequest

import (
	"net/http"
	"strings"
	"testing"
)

// TestMergeStatusProcessRejectsInvalidTaskID proves --task-id is validated via
// common.ValidatePathIdentifier before any request is sent: `bb pullrequest merge-status 1
// --task-id ../..` must never reach
// repository.GetPath("pullrequests", "1", "merge", "task-status", "../..").
func TestMergeStatusProcessRejectsInvalidTaskID(t *testing.T) {
	old := mergeStatusOptions.TaskID
	t.Cleanup(func() { mergeStatusOptions.TaskID = old })
	mergeStatusOptions.TaskID = "../.."

	var requestCount int
	cmd := setupTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, false)

	err := mergeStatusProcess(cmd, []string{"1"})
	if err == nil {
		t.Fatal("mergeStatusProcess() expected an error for an invalid task id, got nil")
	}
	if !strings.Contains(err.Error(), "task-id") {
		t.Errorf("error = %q, want it to name task-id", err.Error())
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request for an invalid task id, got %d", requestCount)
	}
}
