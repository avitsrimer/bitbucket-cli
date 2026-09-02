package pullrequest

import (
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// mirrors cobra's unexported mutually-exclusive flag annotation key (flag_groups.go)
const cobraMutuallyExclusiveAnnotation = "cobra_annotation_mutually_exclusive"

// newIsolatedDraftStateCmd builds a throwaway *cobra.Command carrying registerDraftStateFlags'
// real, production --ready/--draft registration, so these tests exercise cobra's actual
// MarkFlagsMutuallyExclusive validation (which runs in the execute path, not on a direct RunE
// call) without touching the real singleton updateCmd.
func newIsolatedDraftStateCmd(ran *bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:  "test",
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			*ran = true
			return nil
		},
	}
	registerDraftStateFlags(cmd)
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	return cmd
}

func TestRegisterDraftStateFlags(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		wantErrSubstr string
		wantReady     bool
		wantDraft     bool
	}{
		{name: "--ready and --draft together are rejected", args: []string{"--ready", "--draft"}, wantErrSubstr: "none of the others can be"},
		{name: "--ready alone executes cleanly", args: []string{"--ready"}, wantReady: true},
		{name: "--draft alone executes cleanly", args: []string{"--draft"}, wantDraft: true},
		{name: "neither flag given is not an error", args: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ran bool
			cmd := newIsolatedDraftStateCmd(&ran)
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			if tt.wantErrSubstr != "" {
				if err == nil {
					t.Fatal("Execute() error = nil, want a mutual-exclusion error")
				}
				if !strings.Contains(err.Error(), tt.wantErrSubstr) {
					t.Errorf("error = %q, want it to contain %q", err.Error(), tt.wantErrSubstr)
				}
				// the rejection has to land before RunE, not inside it: a real invocation must
				// fail without ever resolving a profile or sending a request
				if ran {
					t.Error("RunE ran despite the mutual-exclusion error, want it skipped entirely")
				}
				return
			}
			if err != nil {
				t.Fatalf("Execute() error = %v, want no error", err)
			}
			if !ran {
				t.Error("RunE did not run, want it to for an accepted flag combination")
			}
			if got := cmd.Flag("ready").Changed; got != tt.wantReady {
				t.Errorf("ready.Changed = %v, want %v", got, tt.wantReady)
			}
			if got := cmd.Flag("draft").Changed; got != tt.wantDraft {
				t.Errorf("draft.Changed = %v, want %v", got, tt.wantDraft)
			}
		})
	}
}

// TestUpdateCmdRegistersDraftStateFlags asserts the production updateCmd itself carries the
// --ready/--draft pair and cobra's mutual-exclusion annotation for it. Every other test in this
// package registers the flags on a command of its own, so without this a deleted
// registerDraftStateFlags(updateCmd) call in init() would leave the whole suite green while
// applySimpleFieldUpdates' unguarded cmd.Flag("ready").Changed nil-dereferenced on every single
// `bb pullrequest update` invocation, --ready or not.
func TestUpdateCmdRegistersDraftStateFlags(t *testing.T) {
	for _, name := range []string{"ready", "draft"} {
		t.Run(name, func(t *testing.T) {
			flag := updateCmd.Flags().Lookup(name)
			if flag == nil {
				t.Fatalf("updateCmd has no --%s flag", name)
			}
			if flag.Value.Type() != "bool" {
				t.Errorf("--%s type = %q, want bool", name, flag.Value.Type())
			}
			groups := flag.Annotations[cobraMutuallyExclusiveAnnotation]
			if !slices.Contains(groups, "ready draft") {
				t.Errorf("--%s mutual-exclusion groups = %v, want one of them to be \"ready draft\"", name, groups)
			}
		})
	}
}
