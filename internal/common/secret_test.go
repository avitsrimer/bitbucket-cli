package common_test

import (
	"os"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/spf13/cobra"
)

func TestReadSecretFromStdin(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "trims a trailing newline", input: "s3cr3t-token\n", want: "s3cr3t-token"},
		{name: "trims trailing CRLF", input: "s3cr3t-token\r\n", want: "s3cr3t-token"},
		{name: "trims trailing spaces and tabs", input: "s3cr3t-token \t\n", want: "s3cr3t-token"},
		{name: "reads the whole input, not just the first line", input: "line-one\nline-two\n", want: "line-one\nline-two"},
		{name: "no trailing newline still reads to EOF", input: "s3cr3t-token", want: "s3cr3t-token"},
		{name: "empty stdin errors", input: "", wantErr: true},
		{name: "whitespace-only stdin errors", input: "   \n\t\n", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}
			cmd.SetIn(strings.NewReader(tc.input))

			got, err := common.ReadSecretFromStdin(cmd)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ReadSecretFromStdin() error = nil, want an error for input %q", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("ReadSecretFromStdin() error = %v, want nil", err)
			}
			if got != tc.want {
				t.Errorf("ReadSecretFromStdin() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestStdinIsInteractive(t *testing.T) {
	t.Run("a replaced input stream is never interactive", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		cmd.SetIn(strings.NewReader("anything"))

		if common.StdinIsInteractive(cmd) {
			t.Error("StdinIsInteractive() = true, want false for a cmd.SetIn-replaced input stream")
		}
	})

	t.Run("the real os.Stdin redirected to a pipe is not interactive", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}

		r, w, err := os.Pipe()
		if err != nil {
			t.Fatalf("cannot create pipe: %v", err)
		}
		t.Cleanup(func() { _ = w.Close() })

		original := os.Stdin
		os.Stdin = r
		t.Cleanup(func() { os.Stdin = original })

		if common.StdinIsInteractive(cmd) {
			t.Error("StdinIsInteractive() = true, want false for a piped, non-character-device os.Stdin")
		}
	})
}
