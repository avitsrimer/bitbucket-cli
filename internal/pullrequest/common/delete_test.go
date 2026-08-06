package prcommon

import (
	"net/http"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
)

// TestDeleteSubResourcesWarnOnErrorNamesTheFailingIDAndSkipsItsSuccessLog reproduces the FINAL
// CRITICAL GATE's priority-5 finding: a failed deletion's error (a bare "404 Not Found") named
// neither which id failed nor which kind of sub-resource it was, and -- because there was no
// `continue` after recording it -- still logged the item as deleted. `bb pr comment delete 1 2
// --warn-on-error` with id 2 failing must report an error naming "comment 2" (not just the raw
// HTTP status), and must not claim id 2 was deleted.
func TestDeleteSubResourcesWarnOnErrorNamesTheFailingIDAndSkipsItsSuccessLog(t *testing.T) {
	var deletedPaths []string
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/2") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"type":"error","error":{"message":"Not Found"}}`))
			return
		}
		deletedPaths = append(deletedPaths, r.URL.Path)
	})
	if err := cmd.Flags().Set("warn-on-error", "true"); err != nil {
		t.Fatalf("cannot set warn-on-error flag: %v", err)
	}

	repo, err := repository.GetRepository(t.Context(), cmd)
	if err != nil {
		t.Fatalf("cannot get fixture repository: %v", err)
	}

	// DeleteSubResources itself calls profile.GetProfileFromCommand -> Profiles.Load, which
	// reinitializes the global lgr logger (resetting it to os.Stderr, undoing CaptureLog below)
	// whenever common.CurrentConfig() is nil -- give it a config so that reinitialization is
	// skipped and the capture below actually captures.
	oldConfig := common.CurrentConfig()
	t.Cleanup(func() { common.SetCurrentConfig(oldConfig) })
	common.SetCurrentConfig(&common.Config{Path: "irrelevant-for-this-test", Data: map[string]any{}})

	logBuf := testutil.CaptureLog(t)
	stderr := testutil.CaptureStderr(t, func() {
		if err := DeleteSubResources(cmd, repo, "42", "comments", []string{"1", "2"}, "comment", "comments"); err != nil {
			t.Fatalf("DeleteSubResources() with --warn-on-error should not return an error, got %v", err)
		}
	})

	if len(deletedPaths) != 1 {
		t.Fatalf("expected exactly 1 successful deletion (id 1), got %d: %v", len(deletedPaths), deletedPaths)
	}

	if !strings.Contains(stderr, "comment 2") {
		t.Errorf("stderr = %q, want it to name the failing id (\"comment 2\"), not just the raw HTTP status", stderr)
	}

	if strings.Contains(logBuf.String(), "comment 2 deleted") {
		t.Errorf("debug log = %q, must NOT claim comment 2 was deleted -- it failed", logBuf.String())
	}
	if !strings.Contains(logBuf.String(), "comment 1 deleted") {
		t.Errorf("debug log = %q, want it to confirm comment 1 (the one that actually succeeded) was deleted", logBuf.String())
	}
}
