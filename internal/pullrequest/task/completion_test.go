package task

import (
	"net/http"
	"slices"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
	"github.com/spf13/cobra"
)

// TestValidArgsCompletesPullRequestIDsForArg0 proves every task subcommand's ValidArgsFunction
// completes open pullrequest ids for the first (now positional) argument.
func TestValidArgsCompletesPullRequestIDsForArg0(t *testing.T) {
	tests := []struct {
		name        string
		validArgsFn func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective)
	}{
		{"create", createValidArgs},
		{"get", pullRequestAndTaskIDValidArgs},
		{"list", listValidArgs},
		{"update", pullRequestAndTaskIDValidArgs},
		{"delete", deleteValidArgs},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"values":[{"id":42},{"id":7}]}`))
			}, false)

			ids, directive := tt.validArgsFn(cmd, nil, "")
			if directive != cobra.ShellCompDirectiveNoFileComp {
				t.Errorf("%s: directive = %v, want ShellCompDirectiveNoFileComp", tt.name, directive)
			}
			want := []string{"42", "7"}
			if !slices.Equal(ids, want) {
				t.Errorf("%s: completed ids = %v, want %v", tt.name, ids, want)
			}
		})
	}
}

// TestValidArgsCompletesTaskIDsForArg1 proves every task subcommand that takes a <task-id> as
// its second positional completes it by reading the pullrequest-id from the already-typed
// args[0], not from a --pullrequest flag.
func TestValidArgsCompletesTaskIDsForArg1(t *testing.T) {
	tests := []struct {
		name        string
		validArgsFn func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective)
	}{
		{"get", pullRequestAndTaskIDValidArgs},
		{"update", pullRequestAndTaskIDValidArgs},
		{"delete", deleteValidArgs},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requests []*http.Request
			cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
				requests = append(requests, r)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"values":[{"id":1,"content":{"raw":"do X"}},{"id":2,"content":{"raw":"do Y"}}]}`))
			}, false)

			ids, directive := tt.validArgsFn(cmd, []string{"42"}, "")
			if directive != cobra.ShellCompDirectiveNoFileComp {
				t.Errorf("%s: directive = %v, want ShellCompDirectiveNoFileComp", tt.name, directive)
			}
			want := []string{"1", "2"}
			if !slices.Equal(ids, want) {
				t.Errorf("%s: completed ids = %v, want %v", tt.name, ids, want)
			}
			if len(requests) != 1 {
				t.Fatalf("%s: expected exactly 1 request, got %d", tt.name, len(requests))
			}
			wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/pullrequests/42/tasks"
			if requests[0].URL.Path != wantPath {
				t.Errorf("%s: request path = %s, want %s", tt.name, requests[0].URL.Path, wantPath)
			}
		})
	}
}

// TestCreateCommentIDCompletionReadsPullRequestIDFromArgs proves the --comment flag on
// "task create" (still a flag: it references an existing comment, not this command's own
// identity) completes candidates by reading the pullrequest-id positional cobra has already
// typed into args, rather than from a removed --pullrequest flag.
func TestCreateCommentIDCompletionReadsPullRequestIDFromArgs(t *testing.T) {
	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[{"id":452466,"content":{"raw":"first"}}]}`))
	}, false)

	ids, err := createOptions.CommentID.AllowedFunc(cmd.Context(), cmd, []string{"42"}, "")
	if err != nil {
		t.Fatalf("AllowedFunc() error = %v", err)
	}
	want := []string{"452466"}
	if !slices.Equal(ids, want) {
		t.Errorf("completed comment ids = %v, want %v", ids, want)
	}
	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/pullrequests/42/comments"
	if requests[0].URL.Path != wantPath {
		t.Errorf("request path = %s, want %s", requests[0].URL.Path, wantPath)
	}
}

// TestCreateCommentIDCompletionWithoutPullRequestID proves the --comment completion returns no
// candidates (and issues no request) when no pullrequest-id has been typed yet.
func TestCreateCommentIDCompletionWithoutPullRequestID(t *testing.T) {
	cmd := setupTest(t, func(http.ResponseWriter, *http.Request) {
		t.Error("unexpected HTTP request with no pullrequest-id typed yet")
	}, false)

	ids, err := createOptions.CommentID.AllowedFunc(cmd.Context(), cmd, nil, "")
	if err != nil {
		t.Fatalf("AllowedFunc() error = %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("completed comment ids = %v, want none", ids)
	}
}
