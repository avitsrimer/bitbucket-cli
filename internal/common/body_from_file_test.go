package common_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/spf13/cobra"
)

func TestReadBodyFromFileOrStdin(t *testing.T) {
	t.Run("reads a file verbatim, including backticks and $() shell hazards", func(t *testing.T) {
		body := "Looks good, but run `go test ./...` and check $(pwd) first.\n\ntrailing space kept  \n"
		path := filepath.Join(t.TempDir(), "body.md")
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("cannot write fixture file: %v", err)
		}

		cmd := &cobra.Command{Use: "test"}
		got, err := common.ReadBodyFromFileOrStdin(cmd, path)
		if err != nil {
			t.Fatalf("ReadBodyFromFileOrStdin() error = %v", err)
		}
		if got != body {
			t.Errorf("ReadBodyFromFileOrStdin() = %q, want %q (verbatim)", got, body)
		}
	})

	t.Run("reads stdin verbatim when path is -", func(t *testing.T) {
		body := "stdin body with `backticks` and $(command) untouched\n"
		cmd := &cobra.Command{Use: "test"}
		cmd.SetIn(strings.NewReader(body))

		got, err := common.ReadBodyFromFileOrStdin(cmd, "-")
		if err != nil {
			t.Fatalf("ReadBodyFromFileOrStdin() error = %v", err)
		}
		if got != body {
			t.Errorf("ReadBodyFromFileOrStdin() = %q, want %q (verbatim)", got, body)
		}
	})

	t.Run("empty stdin returns empty string, not an error", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		cmd.SetIn(strings.NewReader(""))

		got, err := common.ReadBodyFromFileOrStdin(cmd, "-")
		if err != nil {
			t.Fatalf("ReadBodyFromFileOrStdin() error = %v", err)
		}
		if got != "" {
			t.Errorf("ReadBodyFromFileOrStdin() = %q, want empty string", got)
		}
	})

	t.Run("nonexistent file errors with the path named", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		path := filepath.Join(t.TempDir(), "does-not-exist.md")

		_, err := common.ReadBodyFromFileOrStdin(cmd, path)
		if err == nil {
			t.Fatal("ReadBodyFromFileOrStdin() error = nil, want an error for a nonexistent file")
		}
		if !strings.Contains(err.Error(), path) {
			t.Errorf("error = %q, want it to name the path %q", err.Error(), path)
		}
	})
}
