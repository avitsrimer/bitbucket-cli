package profile

import (
	"bytes"
	"context"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestBrowserCommand(t *testing.T) {
	target := url.URL{Scheme: "https", Host: "bitbucket.org", Path: "/site/oauth2/authorize", RawQuery: "client_id=abc&response_type=code"}
	bare := target.String()

	tests := []struct {
		name     string
		env      browserEnv
		wantName string
		wantArgs []string
		wantErr  string
	}{
		{
			name:     "darwin",
			env:      browserEnv{goos: "darwin"},
			wantName: "open",
			wantArgs: []string{bare},
		},
		{
			name:     "windows",
			env:      browserEnv{goos: "windows"},
			wantName: "rundll32",
			wantArgs: []string{"url.dll,FileProtocolHandler", bare},
		},
		{
			name:     "linux plain",
			env:      browserEnv{goos: "linux"},
			wantName: "xdg-open",
			wantArgs: []string{bare},
		},
		{
			// the only row whose argv keeps the double quotes: cmd.exe re-parses its command
			// line, so the quotes are argv bytes the shell consumes rather than literal
			// characters handed to the target. this argv is preserved byte-identically and is
			// unverified — nobody on this project can test WSL, and `cmd.exe /C start "<url>"`
			// plausibly makes start read the quoted argument as a window title. it is kept as-is
			// deliberately rather than changed blind.
			name:     "linux wsl with interop enabled",
			env:      browserEnv{goos: "linux", wsl: true},
			wantName: "cmd.exe",
			wantArgs: []string{"/C", "start", `"` + bare + `"`},
		},
		{
			name:    "linux in ssh session",
			env:     browserEnv{goos: "linux", sshSession: true},
			wantErr: "cannot open browser in SSH session",
		},
		{
			name:    "linux wsl with interop disabled",
			env:     browserEnv{goos: "linux", wsl: true, interopDisabled: true},
			wantErr: "cannot open browser in WSL without interop enabled",
		},
		{
			name:    "unsupported platform",
			env:     browserEnv{goos: "plan9"},
			wantErr: "unsupported platform",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, args, err := browserCommand(tt.env, target)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error %q, got name %q args %q", tt.wantErr, name, args)
				}
				if err.Error() != tt.wantErr {
					t.Errorf("error mismatch: want %q, got %q", tt.wantErr, err.Error())
				}
				if name != "" || args != nil {
					t.Errorf("expected no command on error, got name %q args %q", name, args)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if name != tt.wantName {
				t.Errorf("name mismatch: want %q, got %q", tt.wantName, name)
			}
			if !reflect.DeepEqual(args, tt.wantArgs) {
				t.Errorf("args mismatch:\nwant %#v\ngot  %#v", tt.wantArgs, args)
			}
		})
	}
}

// TestBrowserCommandQuotingIsWSLOnly pins the asymmetry the argv table exists for: the WSL branch
// passes a quote-wrapped URL because cmd.exe re-parses it, every other branch passes the bare URL
// because exec hands argv straight to the target with no shell in between.
func TestBrowserCommandQuotingIsWSLOnly(t *testing.T) {
	target := url.URL{Scheme: "https", Host: "example.com", Path: "/"}
	bare := target.String()
	quoted := `"` + bare + `"`

	tests := []struct {
		name       string
		env        browserEnv
		wantQuoted bool
	}{
		{name: "darwin passes the bare url", env: browserEnv{goos: "darwin"}},
		{name: "windows passes the bare url", env: browserEnv{goos: "windows"}},
		{name: "linux plain passes the bare url", env: browserEnv{goos: "linux"}},
		{name: "linux wsl passes the quoted url", env: browserEnv{goos: "linux", wsl: true}, wantQuoted: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, args, err := browserCommand(tt.env, target)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var found string
			for _, arg := range args {
				if strings.Contains(arg, bare) {
					found = arg
				}
			}
			if found == "" {
				t.Fatalf("no argument carries the url: %#v", args)
			}

			want := bare
			if tt.wantQuoted {
				want = quoted
			}
			if found != want {
				t.Errorf("url argument mismatch: want %q, got %q", want, found)
			}
			if strings.Contains(found, `"`) != tt.wantQuoted {
				t.Errorf("quoting mismatch: wantQuoted=%v, got %q", tt.wantQuoted, found)
			}
		})
	}
}

// newIsolatedAuthorizeCmd builds a throwaway *cobra.Command carrying exactly the flags
// authorizeProcess reads off the cmd it is handed, so a test drives the real RunE the same way a
// fully wired invocation does. --verbose stays registered but unset, which is what makes the URL
// writes' unconditional nature assertable.
func newIsolatedAuthorizeCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "authorize", Args: cobra.ExactArgs(1), RunE: authorizeProcess, SilenceUsage: true, SilenceErrors: true}
	cmd.Flags().String("profile", "", "")
	cmd.Flags().String("config", "", "")
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().Bool("verbose", false, "")
	cmd.Flags().Bool("stop-on-error", false, "")
	return cmd
}

// occupiedCallbackPort returns a port an open listener holds for the rest of the test.
// authorizeProcess's own ListenAndServe then fails to bind immediately and delivers that failure
// to its buffered result channel, so the final receive in authorizeProcess always has a value
// waiting instead of blocking on an OAuth callback that never arrives.
func occupiedCallbackPort(t *testing.T) uint16 {
	t.Helper()

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("cannot open a listener: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("unexpected listener address type %T", listener.Addr())
	}
	// a kernel-assigned ephemeral port always fits uint16, which is what Profile.CallbackPort is
	return uint16(address.Port)
}

// unlaunchableBrowser empties PATH so whichever launcher browserCommand picks for the host cannot
// be resolved, making openBrowser fail deterministically on every platform.
func unlaunchableBrowser(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
}

// TestAuthorizeProcessAlwaysPrintsTheAuthorizationURL pins that the grant URL reaches the user
// with --verbose unset, on stderr, and that a failed browser launch still leaves a manual URL
// behind when --stop-on-error is off.
//
// Both cases must reach a terminating return: with --stop-on-error=true authorizeProcess returns
// the launch error before the final receive, and with it off the receive is satisfied by the bind
// failure occupiedCallbackPort forces. Each case is additionally run under a timeout so a future
// change that reintroduces an unbounded wait fails the test instead of hanging the package.
func TestAuthorizeProcessAlwaysPrintsTheAuthorizationURL(t *testing.T) {
	const (
		profileName    = "authorize-url-test"
		clientID       = "authorize-url-client-id"
		fallbackNotice = "\nPlease open the following URL in your browser:"
	)

	tests := []struct {
		name         string
		stopOnError  bool
		wantFallback bool
	}{
		{name: "stop on error returns before the fallback", stopOnError: true},
		{name: "without stop on error the fallback still prints", wantFallback: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withIsolatedProfilesConfig(t)
			unlaunchableBrowser(t)

			target := &Profile{Name: profileName, ClientID: clientID, CallbackPort: occupiedCallbackPort(t)}
			Profiles = append(Profiles, target)
			Current = target

			cmd := newIsolatedAuthorizeCmd()
			var stdout, stderr bytes.Buffer
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)
			cmd.SetContext(context.Background())
			args := []string{profileName}
			if tt.stopOnError {
				args = append(args, "--stop-on-error")
			}
			cmd.SetArgs(args)

			done := make(chan error, 1)
			go func() { done <- cmd.Execute() }()
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("expected an error from the failed browser launch, got nil")
				}
			case <-time.After(30 * time.Second):
				t.Fatal("authorizeProcess did not return: it is blocking on the authorization callback")
			}

			if !strings.Contains(stderr.String(), "Opening browser to authorize profile "+profileName) {
				t.Errorf("stderr does not announce the browser launch:\n%s", stderr.String())
			}
			if !strings.Contains(stderr.String(), "https://bitbucket.org/site/oauth2/authorize?client_id="+clientID+"&response_type=code") {
				t.Errorf("stderr does not carry the authorization url:\n%s", stderr.String())
			}
			if strings.Contains(stdout.String(), "bitbucket.org/site/oauth2/authorize") {
				t.Errorf("the authorization url must not reach stdout:\n%s", stdout.String())
			}

			if got := strings.Contains(stderr.String(), fallbackNotice); got != tt.wantFallback {
				t.Errorf("manual url fallback present = %v, want %v:\n%s", got, tt.wantFallback, stderr.String())
			}
			if tt.wantFallback && !strings.Contains(stdout.String(), "waiting for browser authorization...") {
				t.Errorf("stdout does not carry the wait notice:\n%s", stdout.String())
			}
		})
	}
}

// setupOpenShim replaces the "open" binary resolved via PATH with a fake script that records its
// exact argv (one argument per line) to the returned file, writes $OPEN_SHIM_STDERR to stderr when
// set, and exits with $OPEN_SHIM_EXIT_CODE (0 if unset). Modeled on setupGitShim in
// internal/repository/clone_test.go: the real launcher is never invoked.
func setupOpenShim(t *testing.T) (argvFile string) {
	t.Helper()

	binDir := t.TempDir()
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$@\" > \"$OPEN_SHIM_ARGV_FILE\"\n" +
		"if [ -n \"$OPEN_SHIM_STDERR\" ]; then printf '%s\\n' \"$OPEN_SHIM_STDERR\" >&2; fi\n" +
		"exit \"${OPEN_SHIM_EXIT_CODE:-0}\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "open"), []byte(script), 0o755); err != nil {
		t.Fatalf("cannot write open shim: %v", err)
	}

	argvFile = filepath.Join(t.TempDir(), "argv")
	t.Setenv("OPEN_SHIM_ARGV_FILE", argvFile)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return argvFile
}

// TestOpenBrowserExecArgv runs openBrowser's real os/exec path against the shim, pinning the argv
// the launcher actually receives and how the child's exit status surfaces.
//
// browserCommand resolves the launcher from runtime.GOOS, so an "open" shim only stands in for the
// real one on darwin; the skip is for local linux runs, since the ubuntu CI job only builds and
// vets and never executes tests.
func TestOpenBrowserExecArgv(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip(`openBrowser resolves "open" only on darwin`)
	}

	target := url.URL{Scheme: "https", Host: "bitbucket.org", Path: "/site/oauth2/authorize", RawQuery: "client_id=abc&response_type=code"}

	tests := []struct {
		name         string
		exitCode     string
		shimStderr   string
		wantErr      bool
		wantErrParts []string
	}{
		{
			name:     "zero exit yields no error",
			exitCode: "0",
		},
		{
			name:         "non zero exit surfaces the child stderr",
			exitCode:     "3",
			shimStderr:   "shim refused to open the url",
			wantErr:      true,
			wantErrParts: []string{"cannot open browser", "shim refused to open the url"},
		},
		{
			name:         "non zero exit with a silent child still errors",
			exitCode:     "4",
			wantErr:      true,
			wantErrParts: []string{"cannot open browser", "exit status 4"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			argvFile := setupOpenShim(t)
			t.Setenv("OPEN_SHIM_EXIT_CODE", tt.exitCode)
			t.Setenv("OPEN_SHIM_STDERR", tt.shimStderr)

			err := openBrowser(context.Background(), target)
			switch {
			case tt.wantErr && err == nil:
				t.Fatal("expected an error from the non-zero shim exit, got nil")
			case !tt.wantErr && err != nil:
				t.Fatalf("unexpected error: %v", err)
			}
			for _, part := range tt.wantErrParts {
				if !strings.Contains(err.Error(), part) {
					t.Errorf("error %q does not contain %q", err.Error(), part)
				}
			}

			recorded, readErr := os.ReadFile(argvFile)
			if readErr != nil {
				t.Fatalf("cannot read the recorded argv: %v", readErr)
			}
			argv := strings.Split(strings.TrimSuffix(string(recorded), "\n"), "\n")
			if want := []string{target.String()}; !reflect.DeepEqual(argv, want) {
				t.Errorf("recorded argv mismatch:\nwant %#v\ngot  %#v", want, argv)
			}
			// a literal double quote in the recorded argv means the URL was quoted for a
			// launcher that never re-parses its command line
			if strings.Contains(string(recorded), `"`) {
				t.Errorf("recorded argv carries a literal double quote: %q", string(recorded))
			}
		})
	}
}
