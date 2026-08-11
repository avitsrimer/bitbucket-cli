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

// TestRegisterCommentEditFlagsLineHelpTextDescribesSides proves --line and --from each describe
// their own file side: --line anchors to the new (head) version of the file, --from to the old
// (base) version, and neither copies the other's text.
func TestRegisterCommentEditFlagsLineHelpTextDescribesSides(t *testing.T) {
	cmd, _ := newIsolatedCommentEditCmd()

	line := cmd.Flags().Lookup("line")
	if line == nil {
		t.Fatal("--line flag not registered")
	}
	if !strings.Contains(line.Usage, "new") {
		t.Errorf("--line help text = %q, want it to describe the new (head) file side", line.Usage)
	}

	from := cmd.Flags().Lookup("from")
	if from == nil {
		t.Fatal("--from flag not registered")
	}
	if !strings.Contains(from.Usage, "old") {
		t.Errorf("--from help text = %q, want it to describe the old (base) file side", from.Usage)
	}

	if line.Usage == from.Usage {
		t.Errorf("--line and --from share identical help text %q, want each to describe its own side", line.Usage)
	}
}

// TestRegisterCommentEditFlagsRejectsUnknownToFlag verifies --to (removed: its documented "range
// end" semantics never matched the API) is rejected as an unknown flag rather than silently
// accepted as an alias of anything.
func TestRegisterCommentEditFlagsRejectsUnknownToFlag(t *testing.T) {
	cmd, _ := newIsolatedCommentEditCmd()
	cmd.SetArgs([]string{"42", "--comment", "hi", "--to", "5"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want an unknown flag error for --to")
	}
	if !strings.Contains(err.Error(), "unknown flag") {
		t.Errorf("error = %q, want an unknown flag error", err.Error())
	}
}

// TestRegisterCommentEditFlagsRejectsLineAndFromTogether verifies --line and --from remain
// mutually exclusive.
func TestRegisterCommentEditFlagsRejectsLineAndFromTogether(t *testing.T) {
	cmd, _ := newIsolatedCommentEditCmd()
	cmd.SetArgs([]string{"42", "--comment", "hi", "--file", "main.go", "--line", "10", "--from", "5"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want a mutual-exclusion error for --line/--from")
	}
	if !strings.Contains(err.Error(), "none of the others can be") {
		t.Errorf("error = %q, want a mutual-exclusion error", err.Error())
	}
}

// TestPayloadLineAnchorsNewSide proves the bug fix: --line now anchors to the new (head) file
// side (inline.to), not the old side (inline.from).
func TestPayloadLineAnchorsNewSide(t *testing.T) {
	cmd, options := newIsolatedCommentEditCmd()
	cmd.SetArgs([]string{"42", "--comment", "hi", "--file", "main.go", "--line", "1040"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	payload, err := options.payload(cmd)
	if err != nil {
		t.Fatalf("payload() error = %v", err)
	}
	if payload.Anchor == nil {
		t.Fatal("payload.Anchor = nil, want a file anchor")
	}
	if payload.Anchor.To != 1040 {
		t.Errorf("payload.Anchor.To = %d, want 1040", payload.Anchor.To)
	}
	if payload.Anchor.From != 0 {
		t.Errorf("payload.Anchor.From = %d, want 0 (unset)", payload.Anchor.From)
	}
}

// TestPayloadFromAnchorsOldSide proves --from still anchors to the old (base) file side
// (inline.from).
func TestPayloadFromAnchorsOldSide(t *testing.T) {
	cmd, options := newIsolatedCommentEditCmd()
	cmd.SetArgs([]string{"42", "--comment", "hi", "--file", "main.go", "--from", "990"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	payload, err := options.payload(cmd)
	if err != nil {
		t.Fatalf("payload() error = %v", err)
	}
	if payload.Anchor == nil {
		t.Fatal("payload.Anchor = nil, want a file anchor")
	}
	if payload.Anchor.From != 990 {
		t.Errorf("payload.Anchor.From = %d, want 990", payload.Anchor.From)
	}
	if payload.Anchor.To != 0 {
		t.Errorf("payload.Anchor.To = %d, want 0 (unset)", payload.Anchor.To)
	}
}

// TestPayloadLineZeroOrNegativeRejected verifies a zero, negative, or non-numeric --line/--from
// value is rejected rather than silently sent as (or coerced to) a valid line number.
func TestPayloadLineZeroOrNegativeRejected(t *testing.T) {
	tests := []struct {
		name  string
		flag  string
		value string
	}{
		{"line zero", "line", "0"},
		{"line negative", "line", "-5"},
		{"line non-numeric", "line", "abc"},
		{"from zero", "from", "0"},
		{"from negative", "from", "-5"},
		{"from non-numeric", "from", "abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, options := newIsolatedCommentEditCmd()
			cmd.SetArgs([]string{"42", "--comment", "hi", "--file", "main.go", "--" + tt.flag + "=" + tt.value})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}

			_, err := options.payload(cmd)
			if err == nil {
				t.Fatalf("payload() expected an error for --%s %q, got nil", tt.flag, tt.value)
			}
			if !strings.Contains(err.Error(), "--"+tt.flag) {
				t.Errorf("error = %q, want it to name --%s", err.Error(), tt.flag)
			}
		})
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
