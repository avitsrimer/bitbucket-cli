package comment

import (
	"net/http"
	"slices"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
	"github.com/spf13/cobra"
)

// TestValidArgsCompletesPullRequestIDsForArg0 proves every comment subcommand's
// ValidArgsFunction completes open pullrequest ids for the first (now positional) argument.
func TestValidArgsCompletesPullRequestIDsForArg0(t *testing.T) {
	tests := []struct {
		name        string
		validArgsFn func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective)
	}{
		{"create", createValidArgs},
		{"get", pullRequestAndCommentIDValidArgs},
		{"list", listValidArgs},
		{"update", pullRequestAndCommentIDValidArgs},
		{"delete", deleteValidArgs},
		{"reopen", pullRequestAndCommentIDValidArgs},
		{"resolve", pullRequestAndCommentIDValidArgs},
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

// TestValidArgsCompletesCommentIDsForArg1 proves every comment subcommand that takes a
// <comment-id> as its second positional completes it by reading the pullrequest-id from the
// already-typed args[0], not from a --pullrequest flag.
func TestValidArgsCompletesCommentIDsForArg1(t *testing.T) {
	tests := []struct {
		name        string
		validArgsFn func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective)
	}{
		{"get", pullRequestAndCommentIDValidArgs},
		{"update", pullRequestAndCommentIDValidArgs},
		{"delete", deleteValidArgs},
		{"reopen", pullRequestAndCommentIDValidArgs},
		{"resolve", pullRequestAndCommentIDValidArgs},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requests []*http.Request
			cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
				requests = append(requests, r)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"values":[{"id":452466,"content":{"raw":"first"}},{"id":452467,"content":{"raw":"second"}}]}`))
			}, false)

			ids, directive := tt.validArgsFn(cmd, []string{"42"}, "")
			if directive != cobra.ShellCompDirectiveNoFileComp {
				t.Errorf("%s: directive = %v, want ShellCompDirectiveNoFileComp", tt.name, directive)
			}
			want := []string{"452466", "452467"}
			if !slices.Equal(ids, want) {
				t.Errorf("%s: completed ids = %v, want %v", tt.name, ids, want)
			}
			if len(requests) != 1 {
				t.Fatalf("%s: expected exactly 1 request, got %d", tt.name, len(requests))
			}
			wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/pullrequests/42/comments"
			if requests[0].URL.Path != wantPath {
				t.Errorf("%s: request path = %s, want %s", tt.name, requests[0].URL.Path, wantPath)
			}
		})
	}
}
