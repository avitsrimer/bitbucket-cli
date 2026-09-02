package pullrequest

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// newIsolatedDraftStateCmd builds a throwaway *cobra.Command carrying registerDraftStateFlags'
// real, production --ready/--draft registration, so these tests exercise cobra's actual
// MarkFlagsMutuallyExclusive validation (which runs in the execute path, not on a direct RunE
// call) without touching the real singleton updateCmd.
func newIsolatedDraftStateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:  "test",
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error { return nil },
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
			cmd := newIsolatedDraftStateCmd()
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			if tt.wantErrSubstr != "" {
				if err == nil {
					t.Fatal("Execute() error = nil, want a mutual-exclusion error")
				}
				if !strings.Contains(err.Error(), tt.wantErrSubstr) {
					t.Errorf("error = %q, want it to contain %q", err.Error(), tt.wantErrSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Execute() error = %v, want no error", err)
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
