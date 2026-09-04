package profile

import (
	"net/url"
	"reflect"
	"strings"
	"testing"
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
