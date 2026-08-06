package comment

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
)

// TestListCmdRegistersLimitFlag proves --limit is registered on the real "pr comment list"
// command, not just plumbing exercised through a synthetic flag on a bare cobra.Command in an
// internal package test.
func TestListCmdRegistersLimitFlag(t *testing.T) {
	if listCmd.Flags().Lookup("limit") == nil {
		t.Fatal(`"pr comment list" has no --limit flag registered`)
	}
}

// TestListProcessRespectsLimitFlag is a regression test for --limit being wired onto a real
// command: it drives listProcess with a "limit" flag on its cmd, proving the value actually
// reaches GetAll and truncates the result instead of being permanently unreachable dead plumbing.
func TestListProcessRespectsLimitFlag(t *testing.T) {
	oldPullRequestIDValue := listOptions.PullRequestID.Value
	oldQuery := listOptions.Query
	t.Cleanup(func() {
		listOptions.PullRequestID.Value = oldPullRequestIDValue
		listOptions.Query = oldQuery
	})
	listOptions.PullRequestID.Value = "42"
	listOptions.Query = ""

	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[{"id":1,"content":{"raw":"first"}},{"id":2,"content":{"raw":"second"}}]}`))
	}, false)
	if err := cmd.Flags().Set("limit", "1"); err != nil {
		t.Fatalf("cannot set limit flag: %v", err)
	}

	stdout := testutil.CaptureStdout(t, func() {
		if err := listProcess(cmd, nil); err != nil {
			t.Fatalf("listProcess() error = %v", err)
		}
	})

	var comments []Comment
	if err := json.Unmarshal([]byte(stdout), &comments); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if len(comments) != 1 {
		t.Fatalf("expected exactly 1 comment with --limit 1, got %d", len(comments))
	}
}
