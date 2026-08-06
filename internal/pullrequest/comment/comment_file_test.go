package comment

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// newIsolatedCommentEditCmd builds a throwaway *cobra.Command carrying exactly the flag
// registration registerCommentEditFlags performs (the real, production registration function),
// so these tests exercise cobra's actual MarkFlagsOneRequired/MarkFlagsMutuallyExclusive
// validation without touching the real singleton createCmd/updateCmd and their package-level
// Changed state.
func newIsolatedCommentEditCmd() (*cobra.Command, *commentEditOptions) {
	options := &commentEditOptions{}
	cmd := &cobra.Command{
		Use:  "create",
		Args: cobra.ExactArgs(1),
		RunE: func(*cobra.Command, []string) error { return nil },
	}
	registerCommentEditFlags(cmd, options, "Comment of the pullrequest")
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	return cmd, options
}

// TestRegisterCommentEditFlagsRequiresOneOfCommentOrCommentFile verifies that neither --comment
// nor --comment-file given is rejected at cobra's own flag-validation stage, replacing the old
// plain MarkFlagRequired("comment").
func TestRegisterCommentEditFlagsRequiresOneOfCommentOrCommentFile(t *testing.T) {
	cmd, _ := newIsolatedCommentEditCmd()
	cmd.SetArgs([]string{"42"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want an error when neither --comment nor --comment-file is given")
	}
	if !strings.Contains(err.Error(), "comment") || !strings.Contains(err.Error(), "comment-file") {
		t.Errorf("error = %q, want it to name both comment and comment-file", err.Error())
	}
}

// TestRegisterCommentEditFlagsRejectsBothCommentAndCommentFile verifies --comment and
// --comment-file together are rejected.
func TestRegisterCommentEditFlagsRejectsBothCommentAndCommentFile(t *testing.T) {
	cmd, _ := newIsolatedCommentEditCmd()
	cmd.SetArgs([]string{"42", "--comment", "hi", "--comment-file", "body.md"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want a mutual-exclusion error")
	}
	if !strings.Contains(err.Error(), "none of the others can be") {
		t.Errorf("error = %q, want a mutual-exclusion error", err.Error())
	}
}

// TestRegisterCommentEditFlagsAcceptsCommentFileAlone verifies --comment-file alone (no
// --comment) passes cobra's flag validation and binds to CommentFile.
func TestRegisterCommentEditFlagsAcceptsCommentFileAlone(t *testing.T) {
	cmd, options := newIsolatedCommentEditCmd()
	cmd.SetArgs([]string{"42", "--comment-file", "body.md"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v, want no error for --comment-file alone", err)
	}
	if options.CommentFile != "body.md" {
		t.Errorf("CommentFile = %q, want %q", options.CommentFile, "body.md")
	}
}

// TestRegisterCommentEditFlagsAcceptsCommentAlone verifies --comment alone (no --comment-file)
// still passes cobra's flag validation, matching the pre-existing behavior.
func TestRegisterCommentEditFlagsAcceptsCommentAlone(t *testing.T) {
	cmd, options := newIsolatedCommentEditCmd()
	cmd.SetArgs([]string{"42", "--comment", "hi"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v, want no error for --comment alone", err)
	}
	if options.Comment != "hi" {
		t.Errorf("Comment = %q, want %q", options.Comment, "hi")
	}
}

// TestRegisterCommentEditFlagsLineHelpTextDescribesLine proves the FR-11 fix: --line's help
// string describes --line, not a copy-pasted description of --from.
func TestRegisterCommentEditFlagsLineHelpTextDescribesLine(t *testing.T) {
	cmd, _ := newIsolatedCommentEditCmd()

	flag := cmd.Flags().Lookup("line")
	if flag == nil {
		t.Fatal("--line flag not registered")
	}
	if !strings.Contains(flag.Usage, "--line") && !strings.HasPrefix(flag.Usage, "Line to comment on") {
		t.Errorf("--line help text = %q, want it to describe --line rather than --from", flag.Usage)
	}
	if strings.Contains(flag.Usage, "From line to comment on") {
		t.Errorf("--line help text = %q, still carries --from's copy-pasted description", flag.Usage)
	}
}

// TestPayloadPendingFlagSetsPending proves the --pending flag actually reaches CommentPayload.Pending
// when explicitly set -- the branch in commentEditOptions.payload is otherwise never exercised by
// any existing test.
func TestPayloadPendingFlagSetsPending(t *testing.T) {
	cmd, options := newIsolatedCommentEditCmd()
	cmd.SetArgs([]string{"42", "--comment", "hi", "--pending"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	payload, err := options.payload(cmd)
	if err != nil {
		t.Fatalf("payload() error = %v", err)
	}
	if payload.Pending == nil || !*payload.Pending {
		t.Errorf("payload.Pending = %v, want a pointer to true", payload.Pending)
	}
}

// TestPayloadWithoutPendingFlagOmitsPending proves that when --pending is never set, the payload
// carries no Pending value at all (nil, not false) so it is omitted from the request body via
// omitempty.
func TestPayloadWithoutPendingFlagOmitsPending(t *testing.T) {
	cmd, options := newIsolatedCommentEditCmd()
	cmd.SetArgs([]string{"42", "--comment", "hi"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	payload, err := options.payload(cmd)
	if err != nil {
		t.Fatalf("payload() error = %v", err)
	}
	if payload.Pending != nil {
		t.Errorf("payload.Pending = %v, want nil when --pending was never set", *payload.Pending)
	}
}
