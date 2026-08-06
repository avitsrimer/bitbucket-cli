package pullrequest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// newIsolatedDescriptionFileCmd builds a throwaway *cobra.Command registering a plain
// --description string flag plus registerDescriptionFileFlag's real, production
// --description-file registration, so these tests exercise cobra's actual
// MarkFlagsMutuallyExclusive validation without touching the real singleton createCmd/updateCmd.
func newIsolatedDescriptionFileCmd() (cmd *cobra.Command, descriptionFile *string) {
	descriptionFile = new(string)
	cmd = &cobra.Command{
		Use:  "test",
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error { return nil },
	}
	cmd.Flags().String("description", "", "Description of the pullrequest")
	registerDescriptionFileFlag(cmd, descriptionFile)
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	return cmd, descriptionFile
}

// TestRegisterDescriptionFileFlagRejectsBothDescriptionAndDescriptionFile verifies --description
// and --description-file together are rejected.
func TestRegisterDescriptionFileFlagRejectsBothDescriptionAndDescriptionFile(t *testing.T) {
	cmd, _ := newIsolatedDescriptionFileCmd()
	cmd.SetArgs([]string{"--description", "hi", "--description-file", "body.md"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want a mutual-exclusion error")
	}
	if !strings.Contains(err.Error(), "none of the others can be") {
		t.Errorf("error = %q, want a mutual-exclusion error", err.Error())
	}
}

// TestRegisterDescriptionFileFlagAllowsNeitherFlag verifies that, unlike --comment/--comment-file,
// neither --description nor --description-file being given is NOT an error: description remains
// an optional field, matching create/update's existing required-ness.
func TestRegisterDescriptionFileFlagAllowsNeitherFlag(t *testing.T) {
	cmd, _ := newIsolatedDescriptionFileCmd()
	cmd.SetArgs(nil)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v, want no error when neither flag is given", err)
	}
}

// TestRegisterDescriptionFileFlagAcceptsDescriptionFileAlone verifies --description-file alone
// binds to descriptionFile and completes as a filename.
func TestRegisterDescriptionFileFlagAcceptsDescriptionFileAlone(t *testing.T) {
	cmd, descriptionFile := newIsolatedDescriptionFileCmd()
	cmd.SetArgs([]string{"--description-file", "body.md"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v, want no error for --description-file alone", err)
	}
	if *descriptionFile != "body.md" {
		t.Errorf("descriptionFile = %q, want %q", *descriptionFile, "body.md")
	}
}

func TestResolveDescriptionBody(t *testing.T) {
	tmp := t.TempDir()
	bodyPath := filepath.Join(tmp, "description.md")
	body := "Fixes the bug by running `make test` and checking $(git diff) first.\n"
	if err := os.WriteFile(bodyPath, []byte(body), 0o600); err != nil {
		t.Fatalf("cannot write fixture file: %v", err)
	}
	emptyPath := filepath.Join(tmp, "empty.md")
	if err := os.WriteFile(emptyPath, []byte("   \n"), 0o600); err != nil {
		t.Fatalf("cannot write fixture file: %v", err)
	}
	missingPath := filepath.Join(tmp, "does-not-exist.md")

	tests := []struct {
		name            string
		description     string
		descriptionFile string
		stdin           string
		want            string
		wantErrSubstr   string
	}{
		{name: "inline description passes through verbatim", description: "hello", want: "hello"},
		{name: "no descriptionFile, empty description is allowed (clears the field)", description: "", want: ""},
		{name: "descriptionFile reads the file verbatim, including backticks and $()", descriptionFile: bodyPath, want: body},
		{name: "descriptionFile - reads stdin verbatim", descriptionFile: "-", stdin: "from stdin `with backticks`\n", want: "from stdin `with backticks`\n"},
		{name: "empty descriptionFile content is rejected", descriptionFile: emptyPath, wantErrSubstr: "description body is empty"},
		{name: "nonexistent descriptionFile errors with the path", descriptionFile: missingPath, wantErrSubstr: missingPath},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}
			if tt.stdin != "" {
				cmd.SetIn(strings.NewReader(tt.stdin))
			}

			got, err := resolveDescriptionBody(cmd, tt.description, tt.descriptionFile)
			if tt.wantErrSubstr != "" {
				if err == nil {
					t.Fatalf("resolveDescriptionBody() error = nil, want it to contain %q", tt.wantErrSubstr)
				}
				if !strings.Contains(err.Error(), tt.wantErrSubstr) {
					t.Errorf("error = %q, want it to contain %q", err.Error(), tt.wantErrSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveDescriptionBody() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("resolveDescriptionBody() = %q, want %q", got, tt.want)
			}
		})
	}
}
