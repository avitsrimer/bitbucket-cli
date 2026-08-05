package profile

import (
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
	Use:               "authorize",
	Short:             "authorize an Authorization Code Grant profile",
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

	if len(args) == 0 {
		return errors.New("argument profile is missing")
	}

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
	resultchan := make(chan error)
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

// openBrowser opens the specified URL in the default web browser
func openBrowser(ctx context.Context, url url.URL) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "linux":
		cmd = "xdg-open"
		if _, exists := os.LookupEnv("SSH_CONNECTION"); exists {
			return errors.New("cannot open browser in SSH session")
		}
		if common.IsWSL() {
			// If the flag interop=true is not set in /etc/wsl.conf, return an error
			if wslInteropDisabled() {
				return errors.New("cannot open browser in WSL without interop enabled")
			}
			cmd = "cmd.exe"
			args = append(args, "/C", "start")
		}
	case "windows":
		cmd = "rundll32"
		args = append(args, "url.dll,FileProtocolHandler")
	case "darwin":
		cmd = "open"
	default:
		return errors.New("unsupported platform")
	}

	args = append(args, `"`+url.String()+`"`)
	if err := exec.CommandContext(ctx, cmd, args...).Start(); err != nil { //nolint:gosec // cmd is one of a fixed set of literals chosen from runtime.GOOS above, never external input
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
