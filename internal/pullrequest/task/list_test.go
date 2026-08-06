package task

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
)

func withListOptions(t *testing.T, mutate func()) {
	t.Helper()
	oldQuery := listOptions.Query
	t.Cleanup(func() {
		listOptions.Query = oldQuery
	})
	mutate()
}

// TestListProcess covers listProcess's success, empty-result, API-error, and dry-run paths.
func TestListProcess(t *testing.T) {
	tests := []struct {
		name          string
		handler       http.HandlerFunc
		dryRun        bool
		wantErrSubstr string
		validate      func(t *testing.T, requests []*http.Request, stdout string)
	}{
		{
			name: "success",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"values":[{"id":1,"content":{"raw":"do X"}},{"id":2,"content":{"raw":"do Y"}}]}`))
			},
			validate: func(t *testing.T, requests []*http.Request, stdout string) {
				t.Helper()
				if len(requests) != 1 {
					t.Fatalf("expected exactly 1 request, got %d", len(requests))
				}
				wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/pullrequests/42/tasks"
				if requests[0].URL.Path != wantPath {
					t.Errorf("path = %s, want %s", requests[0].URL.Path, wantPath)
				}
				var tasks []Task
				if err := json.Unmarshal([]byte(stdout), &tasks); err != nil {
					t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
				}
				if len(tasks) != 2 {
					t.Fatalf("expected 2 tasks, got %d", len(tasks))
				}
			},
		},
		{
			name: "no results",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"values":[]}`))
			},
			validate: func(t *testing.T, requests []*http.Request, stdout string) {
				t.Helper()
				if len(requests) != 1 {
					t.Fatalf("expected exactly 1 request, got %d", len(requests))
				}
				if strings.TrimSpace(stdout) != "" {
					t.Errorf("expected no output when there are no tasks, got %q", stdout)
				}
			},
		},
		{
			name: "api error",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"type":"error","error":{"message":"server exploded"}}`))
			},
			wantErrSubstr: "server exploded",
		},
		{
			name:    "dry run",
			handler: func(http.ResponseWriter, *http.Request) {},
			dryRun:  true,
			validate: func(t *testing.T, requests []*http.Request, _ string) {
				t.Helper()
				if len(requests) != 0 {
					t.Errorf("expected no HTTP request in dry-run mode, got %d", len(requests))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withListOptions(t, func() {
				listOptions.Query = ""
			})

			var requests []*http.Request
			cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
				requests = append(requests, r)
				tt.handler(w, r)
			}, tt.dryRun)

			var err error
			stdout := testutil.CaptureStdout(t, func() {
				err = listProcess(cmd, []string{"42"})
			})

			if tt.wantErrSubstr != "" {
				if err == nil {
					t.Fatal("listProcess() expected an error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantErrSubstr) {
					t.Errorf("error = %q, want it to contain %q", err.Error(), tt.wantErrSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("listProcess() error = %v", err)
			}
			if tt.validate != nil {
				tt.validate(t, requests, stdout)
			}
		})
	}
}

func TestListProcessWithQuery(t *testing.T) {
	withListOptions(t, func() {
		listOptions.Query = `content.raw~"fix"`
	})

	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[]}`))
	}, false)

	if err := listProcess(cmd, []string{"42"}); err != nil {
		t.Fatalf("listProcess() error = %v", err)
	}
	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	want := `content.raw~"fix"`
	if got := requests[0].URL.Query().Get("q"); got != want {
		t.Errorf("q query parameter = %q, want %q", got, want)
	}
}

// TestListCmdRegistersLimitFlag proves --limit is registered on the real "pr task list" command.
func TestListCmdRegistersLimitFlag(t *testing.T) {
	if listCmd.Flags().Lookup("limit") == nil {
		t.Fatal(`"pr task list" has no --limit flag registered`)
	}
}

// TestListProcessRespectsLimitFlag is a regression test for --limit being wired onto a real
// command: it drives listProcess with a "limit" flag on its cmd, proving the value actually
// reaches GetAll and truncates the result instead of being permanently unreachable dead plumbing.
func TestListProcessRespectsLimitFlag(t *testing.T) {
	withListOptions(t, func() {
		listOptions.Query = ""
	})

	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[{"id":1,"content":{"raw":"do X"}},{"id":2,"content":{"raw":"do Y"}}]}`))
	}, false)
	if err := cmd.Flags().Set("limit", "1"); err != nil {
		t.Fatalf("cannot set limit flag: %v", err)
	}

	stdout := testutil.CaptureStdout(t, func() {
		if err := listProcess(cmd, []string{"42"}); err != nil {
			t.Fatalf("listProcess() error = %v", err)
		}
	})

	var tasks []Task
	if err := json.Unmarshal([]byte(stdout), &tasks); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected exactly 1 task with --limit 1, got %d", len(tasks))
	}
}
