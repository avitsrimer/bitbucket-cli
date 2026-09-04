package profile

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
	"gopkg.in/ini.v1"
)

var authorizeCmd = &cobra.Command{
	Use:               "authorize <profile-name>",
	Short:             "authorize an Authorization Code Grant profile",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: ValidProfileNames,
	PreRunE:           disableUnsupportedFlags,
	RunE:              authorizeProcess,
}

func init() {
	Command.AddCommand(authorizeCmd)
	authorizeCmd.SetHelpFunc(hideUnsupportedFlags)
}

func authorizeProcess(cmd *cobra.Command, args []string) (err error) {
	ctx := cmd.Context()

	_, err = GetProfileFromCommand(ctx, cmd)
	if errors.Is(err, ErrNoProfiles) || len(Profiles) == 0 {
		return errors.New("no profiles found")
	}
	if err != nil {
		return err
	}

	lgr.Printf("[DEBUG] authorizing profile %s (valid names: %v)", args[0], Profiles.Names())
	profile, found := Profiles.Find(args[0])
	if !found {
		return fmt.Errorf("profile %s not found", args[0])
	}
	if profile.CallbackPort == 0 {
		return fmt.Errorf("profile %s does not support Authorization Code Grant", profile.Name)
	}

	if !common.WhatIf(cmd, "Authorizing profile "+args[0]) {
		return nil
	}
	// Start a web server to listen for the Authorization Code Grant
	// Buffered so a second callback request (browser reload/prefetch) arriving after the first
	// result was already delivered can send without blocking: CodeGrantCallback's handler uses a
	// non-blocking send, but a receiver-less unbuffered channel would still make that send race
	// with this goroutine moving on to server.Shutdown, which itself waits on that same
	// in-flight handler, hanging the CLI forever.
	resultchan := make(chan error, 1)
	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", profile.CallbackPort),
		Handler:           profile.CodeGrantCallback(resultchan),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		if serveErr := server.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			lgr.Printf("[ERROR] failed to start server: %v", serveErr)
			resultchan <- serveErr
		}
	}()

	// Open the browser to the Authorization Code Grant URL
	common.Verbose(cmd, "Opening browser to authorize profile %s...", profile.Name)
	bitbucketAuthURL := url.URL{
		Scheme: "https",
		Host:   "bitbucket.org",
		Path:   "/site/oauth2/authorize",
		RawQuery: url.Values{
			"response_type": {"code"},
			"client_id":     {profile.ClientID},
		}.Encode(),
	}
	common.Verbose(cmd, "\nIf you are not redirected automatically, please open the following URL in your browser:\n%s\n", bitbucketAuthURL.String())

	err = openBrowser(ctx, bitbucketAuthURL)
	if err != nil {
		lgr.Printf("[WARN] failed to open browser: %s", err.Error())
		if cmd.Flag("stop-on-error").Value.String() == "true" {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "\nPlease open the following URL in your browser:\n%s\n", bitbucketAuthURL.String())
	}

	fmt.Fprintln(cmd.OutOrStdout(), "waiting for browser authorization...")

	// Wait until the user stops the server by pressing Ctrl+C
	results := <-resultchan

	lgr.Printf("[DEBUG] received results, shutting down server...")
	if err := server.Shutdown(ctx); err != nil {
		lgr.Printf("[ERROR] failed to shut down server: %v", err)
	}

	if results != nil {
		lgr.Printf("[ERROR] authorization process failed: %v", results)
		return results
	}
	common.Verbose(cmd, "Authorization process completed successfully")
	return nil
}

// browserEnv describes the host properties that decide which launcher opens a URL. Passing it
// explicitly is what makes browserCommand pure and its argv assertable on any host OS.
type browserEnv struct {
	goos            string
	wsl             bool
	interopDisabled bool
	sshSession      bool
}

// browserCommand reports the launcher name and argv that open u on the host described by env.
//
// Only the WSL branch wraps the URL in double quotes: cmd.exe re-parses its command line, so the
// quotes are consumed by that parsing. Every other branch is handed straight to the target process
// as argv with no shell in between, where quotes would be literal characters in the argument.
func browserCommand(env browserEnv, u url.URL) (name string, args []string, err error) {
	switch env.goos {
	case "linux":
		name = "xdg-open"
		if env.sshSession {
			return "", nil, errors.New("cannot open browser in SSH session")
		}
		if env.wsl {
			if env.interopDisabled {
				return "", nil, errors.New("cannot open browser in WSL without interop enabled")
			}
			return "cmd.exe", []string{"/C", "start", `"` + u.String() + `"`}, nil
		}
	case "windows":
		name = "rundll32"
		args = append(args, "url.dll,FileProtocolHandler")
	case "darwin":
		name = "open"
	default:
		return "", nil, errors.New("unsupported platform")
	}

	return name, append(args, u.String()), nil
}

// openBrowser opens the specified URL in the default web browser
func openBrowser(ctx context.Context, url url.URL) error {
	env := browserEnv{goos: runtime.GOOS, wsl: common.IsWSL()}
	_, env.sshSession = os.LookupEnv("SSH_CONNECTION")
	if env.wsl {
		// reading /etc/wsl.conf only matters under WSL, so it stays behind that check
		env.interopDisabled = wslInteropDisabled()
	}

	name, args, err := browserCommand(env, url)
	if err != nil {
		return err
	}

	var stderr bytes.Buffer
	launch := exec.CommandContext(ctx, name, args...) //nolint:gosec // name is one of a fixed set of literals chosen from runtime.GOOS in browserCommand, never external input
	launch.Stderr = &stderr
	if err := launch.Run(); err != nil {
		if message := strings.TrimSpace(stderr.String()); message != "" {
			return fmt.Errorf("cannot open browser: %s", message)
		}
		return fmt.Errorf("cannot open browser: %w", err)
	}
	return nil
}

// wslInteropDisabled reports whether /etc/wsl.conf explicitly disables WSL interop.
//
// Any failure reading or parsing the file is treated as interop not being disabled.
func wslInteropDisabled() bool {
	content, err := os.ReadFile("/etc/wsl.conf")
	if err != nil {
		return false
	}
	data, err := ini.Load(content)
	if err != nil {
		return false
	}
	section, err := data.GetSection("interop")
	if err != nil {
		return false
	}
	key, err := section.GetKey("enabled")
	if err != nil {
		return false
	}
	return strings.ToLower(key.String()) != "true"
}
