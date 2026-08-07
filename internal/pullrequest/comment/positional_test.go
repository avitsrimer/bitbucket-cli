package comment

import (
	"net/http"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
)

// failIfCalled returns a handler that fails the test if it is ever invoked, for asserting a
// rejected argument short-circuits before any HTTP request is issued.
func failIfCalled(t *testing.T) http.HandlerFunc {
	return testutil.FailIfCalled(t, "an invalid pullrequest-id")
}

func describeID(id string) string {
	return testutil.DescribeArg(id)
}

// TestSubcommandsRejectInvalidPullRequestID proves every comment subcommand validates its
// (now required, positional) pullrequest-id argument via common.ValidatePathIdentifier before
// it ever reaches a GetPath call, for each of the six values path.Join silently mishandles or
// that could otherwise splice extra path segments into the request.
func TestSubcommandsRejectInvalidPullRequestID(t *testing.T) {
	cases := []struct {
		name string
		run  func(t *testing.T, pullRequestID string) error
	}{
		{"create", func(t *testing.T, id string) error {
			withCommentEditOptions(t, &createOptions, func() { createOptions.Comment = "hi" })
			cmd := setupTest(t, failIfCalled(t), false)
			return createProcess(cmd, []string{id})
		}},
		{"get", func(t *testing.T, id string) error {
			cmd := setupTest(t, failIfCalled(t), false)
			return getProcess(cmd, []string{id, "7"})
		}},
		{"list", func(t *testing.T, id string) error {
			cmd := setupTest(t, failIfCalled(t), false)
			return listProcess(cmd, []string{id})
		}},
		{"update", func(t *testing.T, id string) error {
			withCommentEditOptions(t, &updateOptions, func() { updateOptions.Comment = "hi" })
			cmd := setupTest(t, failIfCalled(t), false)
			return updateProcess(cmd, []string{id, "7"})
		}},
		{"delete", func(t *testing.T, id string) error {
			cmd := setupTest(t, failIfCalled(t), false)
			return deleteProcess(cmd, []string{id, "7"})
		}},
		{"reopen", func(t *testing.T, id string) error {
			cmd := setupTest(t, failIfCalled(t), false)
			return reopenProcess(cmd, []string{id, "7"})
		}},
		{"resolve", func(t *testing.T, id string) error {
			cmd := setupTest(t, failIfCalled(t), false)
			return resolveProcess(cmd, []string{id, "7"})
		}},
	}

	for _, tc := range cases {
		for _, invalid := range []string{"", ".", "..", "../..", "../../..", "1/../../.."} {
			t.Run(tc.name+"/"+describeID(invalid), func(t *testing.T) {
				err := tc.run(t, invalid)
				if err == nil {
					t.Fatalf("%s(%q) expected an error, got nil", tc.name, invalid)
				}
				if !strings.Contains(err.Error(), "pullrequest-id") {
					t.Errorf("%s(%q) error = %q, want it to name pullrequest-id", tc.name, invalid, err.Error())
				}
			})
		}
	}
}

// TestSubcommandsRejectInvalidCommentID proves every comment subcommand that takes a
// <comment-id> second positional (get, update, reopen, resolve) validates it via
// common.ValidatePathIdentifier before it ever reaches a GetPath call, exactly like the
// pullrequest-id positional -- guarding against `bb pullrequest comment get 1 '../..'`
// collapsing repo.GetPath("pullrequests", "1", "comments", "../..") into a different resource.
func TestSubcommandsRejectInvalidCommentID(t *testing.T) {
	cases := []struct {
		name string
		run  func(t *testing.T, commentID string) error
	}{
		{"get", func(t *testing.T, id string) error {
			cmd := setupTest(t, failIfCalled(t), false)
			return getProcess(cmd, []string{"1", id})
		}},
		{"update", func(t *testing.T, id string) error {
			withCommentEditOptions(t, &updateOptions, func() { updateOptions.Comment = "hi" })
			cmd := setupTest(t, failIfCalled(t), false)
			return updateProcess(cmd, []string{"1", id})
		}},
		{"reopen", func(t *testing.T, id string) error {
			cmd := setupTest(t, failIfCalled(t), false)
			return reopenProcess(cmd, []string{"1", id})
		}},
		{"resolve", func(t *testing.T, id string) error {
			cmd := setupTest(t, failIfCalled(t), false)
			return resolveProcess(cmd, []string{"1", id})
		}},
	}

	for _, tc := range cases {
		for _, invalid := range []string{"", ".", "..", "../..", "../../.."} {
			t.Run(tc.name+"/"+describeID(invalid), func(t *testing.T) {
				err := tc.run(t, invalid)
				if err == nil {
					t.Fatalf("%s(%q) expected an error, got nil", tc.name, invalid)
				}
				if !strings.Contains(err.Error(), "comment-id") {
					t.Errorf("%s(%q) error = %q, want it to name comment-id", tc.name, invalid, err.Error())
				}
			})
		}
	}
}

// TestCommentDeleteRejectsInvalidCommentIDs proves "comment delete" validates every variadic
// comment-id positional (not just the pullrequest-id) via common.ValidatePathIdentifier before
// any request is sent -- the DeleteSubResources path FR-14's field report flagged as a path
// traversal: `bb pullrequest comment delete 1 ../../..` must never reach
// repo.GetPath("pullrequests", "1", "comments", "../../..").
func TestCommentDeleteRejectsInvalidCommentIDs(t *testing.T) {
	cmd := setupTest(t, failIfCalled(t), false)

	err := deleteProcess(cmd, []string{"1", "../../.."})
	if err == nil {
		t.Fatal("deleteProcess() expected an error for an invalid comment id, got nil")
	}
	if !strings.Contains(err.Error(), "comment-id") {
		t.Errorf("deleteProcess() error = %q, want it to name comment-id", err.Error())
	}
}

// TestCommentListRejectsDotDotSlashPullRequestID proves "comment list '../..'" -- a value the
// pre-fix ValidatePathIdentifier (which rejected only "", ".", "..") let through -- is rejected
// before any request is sent.
func TestCommentListRejectsDotDotSlashPullRequestID(t *testing.T) {
	cmd := setupTest(t, failIfCalled(t), false)

	err := listProcess(cmd, []string{"../.."})
	if err == nil {
		t.Fatal("listProcess() expected an error for '../..', got nil")
	}
	if !strings.Contains(err.Error(), "pullrequest-id") {
		t.Errorf("listProcess() error = %q, want it to name pullrequest-id", err.Error())
	}
}

// TestArgsValidation proves each comment subcommand's positional-arg count is enforced by cobra:
// too few args (missing-arg) errors for every subcommand, and too many args errors for every
// subcommand except delete (which takes a variadic list of comment ids).
func TestArgsValidation(t *testing.T) {
	type argsCase struct {
		name         string
		validate     func(args []string) error
		wantArgCount int
		variadic     bool
	}

	cases := []argsCase{
		{"create", func(args []string) error { return createCmd.Args(createCmd, args) }, 1, false},
		{"get", func(args []string) error { return getCmd.Args(getCmd, args) }, 2, false},
		{"list", func(args []string) error { return listCmd.Args(listCmd, args) }, 1, false},
		{"update", func(args []string) error { return updateCmd.Args(updateCmd, args) }, 2, false},
		{"delete", func(args []string) error { return deleteCmd.Args(deleteCmd, args) }, 2, true},
		{"reopen", func(args []string) error { return reopenCmd.Args(reopenCmd, args) }, 2, false},
		{"resolve", func(args []string) error { return resolveCmd.Args(resolveCmd, args) }, 2, false},
	}

	for _, tc := range cases {
		t.Run(tc.name+"/missing-arg", func(t *testing.T) {
			args := make([]string, tc.wantArgCount-1)
			for i := range args {
				args[i] = "1"
			}
			if err := tc.validate(args); err == nil {
				t.Errorf("%s: expected a missing-arg error with %d args, got nil", tc.name, len(args))
			}
		})
		if !tc.variadic {
			t.Run(tc.name+"/too-many-args", func(t *testing.T) {
				args := make([]string, tc.wantArgCount+1)
				for i := range args {
					args[i] = "1"
				}
				if err := tc.validate(args); err == nil {
					t.Errorf("%s: expected a too-many-args error with %d args, got nil", tc.name, len(args))
				}
			})
		}
	}
}
