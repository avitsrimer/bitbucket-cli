package common

import (
	"errors"
	"io"
	"strings"
	"testing"
)

// withFakeControllingTTY substitutes openControllingTTY with a stub returning tty/err instead of
// opening a real /dev/tty, restoring the original on cleanup. This is what makes ReadSecret's
// read/trim/error-handling logic testable at all: there is no controlling terminal to open in CI,
// and a fake substituted here is never a *os.File, so ReadSecret's stty toggle is skipped
// entirely -- only the read/trim/error paths are exercised, matching this package's own doc
// comment on openControllingTTY.
func withFakeControllingTTY(t *testing.T, tty io.ReadCloser, err error) {
	t.Helper()
	original := openControllingTTY
	openControllingTTY = func() (io.ReadCloser, error) { return tty, err }
	t.Cleanup(func() { openControllingTTY = original })
}

// fakeTTY adapts a strings.Reader into the io.ReadCloser openControllingTTY returns, deliberately
// not a *os.File so ReadSecret never attempts to toggle echo against it via stty.
type fakeTTY struct {
	io.Reader
}

func (fakeTTY) Close() error { return nil }

func TestReadSecretReadsAndTrims(t *testing.T) {
	tests := []struct {
		name       string
		ttyContent string
		wantSecret string
	}{
		{name: "trims a trailing newline", ttyContent: "s3cr3t\n", wantSecret: "s3cr3t"},
		{name: "trims a trailing CRLF", ttyContent: "s3cr3t\r\n", wantSecret: "s3cr3t"},
		{name: "keeps reading to EOF when there is no trailing newline", ttyContent: "s3cr3t", wantSecret: "s3cr3t"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			withFakeControllingTTY(t, fakeTTY{strings.NewReader(tc.ttyContent)}, nil)

			secret, err := ReadSecret("Password:")
			if err != nil {
				t.Fatalf("ReadSecret() error = %v, want nil", err)
			}
			if secret != tc.wantSecret {
				t.Errorf("ReadSecret() = %q, want %q", secret, tc.wantSecret)
			}
		})
	}
}

func TestReadSecretRejectsAnEmptyLine(t *testing.T) {
	withFakeControllingTTY(t, fakeTTY{strings.NewReader("\n")}, nil)

	_, err := ReadSecret("Password:")
	if err == nil {
		t.Fatal("ReadSecret() error = nil, want an error for an empty line")
	}
	if !strings.Contains(err.Error(), "no secret entered") {
		t.Errorf("ReadSecret() error = %q, want it to mention that no secret was entered", err)
	}
}

func TestReadSecretErrorsWhenNoControllingTTYIsAvailable(t *testing.T) {
	withFakeControllingTTY(t, nil, errors.New("no such device or address"))

	_, err := ReadSecret("Password:")
	if err == nil {
		t.Fatal("ReadSecret() error = nil, want an error when no controlling terminal is available")
	}
	if !strings.Contains(err.Error(), "--password-stdin") || !strings.Contains(err.Error(), "--access-token-stdin") {
		t.Errorf("ReadSecret() error = %q, want it to name --password-stdin and --access-token-stdin as the non-interactive alternative", err)
	}
}
