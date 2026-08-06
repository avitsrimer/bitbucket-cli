package common

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func newCmdWithRepositoryFlag() *cobra.Command {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("repository", "", "")
	return cmd
}

func TestDisableUnsupportedFlags(t *testing.T) {
	t.Run("rejects a changed flag naming it and the command", func(t *testing.T) {
		cmd := newCmdWithRepositoryFlag()
		if err := cmd.Flags().Set("repository", "acme/widgets"); err != nil {
			t.Fatalf("cannot set repository flag: %v", err)
		}

		err := DisableUnsupportedFlags("repo get", "repository")(cmd, nil)

		if err == nil {
			t.Fatal("DisableUnsupportedFlags() expected an error for a changed flag, got nil")
		}
		if !strings.Contains(err.Error(), "--repository") {
			t.Errorf("error = %q, want it to name the flag", err.Error())
		}
		if !strings.Contains(err.Error(), "repo get") {
			t.Errorf("error = %q, want it to name the command", err.Error())
		}
	})

	t.Run("passes through when the flag was never set", func(t *testing.T) {
		cmd := newCmdWithRepositoryFlag()

		if err := DisableUnsupportedFlags("repo get", "repository")(cmd, nil); err != nil {
			t.Errorf("DisableUnsupportedFlags() unexpected error: %v", err)
		}
	})

	t.Run("only rejects the specific flag names given, not every flag on cmd", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		cmd.Flags().String("repository", "", "")
		cmd.Flags().String("workspace", "", "")
		if err := cmd.Flags().Set("workspace", "acme"); err != nil {
			t.Fatalf("cannot set workspace flag: %v", err)
		}

		if err := DisableUnsupportedFlags("repo get", "repository")(cmd, nil); err != nil {
			t.Errorf("DisableUnsupportedFlags() unexpected error for an unrelated changed flag: %v", err)
		}
	})
}

func TestHideUnsupportedFlags(t *testing.T) {
	t.Run("hides the named flags then delegates to the parent's help function", func(t *testing.T) {
		root := &cobra.Command{Use: "root"}
		var parentHelpCalledWith *cobra.Command
		root.SetHelpFunc(func(cmd *cobra.Command, _ []string) { parentHelpCalledWith = cmd })

		child := newCmdWithRepositoryFlag()
		root.AddCommand(child)

		HideUnsupportedFlags("repository")(child, []string{"arg"})

		if flag := child.Flags().Lookup("repository"); flag == nil || !flag.Hidden {
			t.Error("expected the repository flag to be marked hidden")
		}
		if parentHelpCalledWith != child {
			t.Errorf("expected the parent's help function to be called with child, got %v", parentHelpCalledWith)
		}
	})

	t.Run("panics on a parentless command", func(t *testing.T) {
		// cmd.Parent() is nil for a command that was never added to another via AddCommand;
		// HideUnsupportedFlags calls cmd.Parent().HelpFunc() unconditionally, so this is a real,
		// user-reachable panic risk for any command wired up without a parent (e.g. a bug in
		// init() that forgets Command.AddCommand before SetHelpFunc), not just a test artifact.
		orphan := newCmdWithRepositoryFlag()

		defer func() {
			if r := recover(); r == nil {
				t.Error("HideUnsupportedFlags() expected a panic on a parentless command, got none")
			}
		}()
		HideUnsupportedFlags("repository")(orphan, nil)
	})
}
