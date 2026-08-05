package profile

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/gildas/bitbucket-cli/cmd/common"
	"github.com/gildas/go-errors"
	"github.com/gildas/go-logger"
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
	log := logger.Must(logger.FromContext(cmd.Context())).Child(cmd.Parent().Name(), "authorize")
	ctx := log.ToContext(cmd.Context())

	if len(args) == 0 {
		return errors.ArgumentMissing.With("profile")
	}

	_, err = GetProfileFromCommand(ctx, cmd)
	if errors.Is(err, errors.Empty) || len(Profiles) == 0 {
		return errors.Errorf("No profiles found")
	}
	if err != nil {
		return err
	}

	log.Infof("Authorizing profile %s (Valid names: %v)", args[0], Profiles.Names())
	profile, found := Profiles.Find(args[0])
	if !found {
		return errors.NotFound.With("profile", args[0])
	}
	if profile.CallbackPort == 0 {
		return errors.Join(errors.Errorf("Profile %s does not support Authorization Code Grant", profile.Name), errors.ArgumentInvalid.With("profile", profile.Name))
	}

	if !common.WhatIf(ctx, cmd, "Authorizing profile "+args[0]) {
		return nil
	}
	// Start a web server to listen for the Authorization Code Grant
	resultchan := make(chan error)
	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", profile.CallbackPort),
		Handler:           log.HttpHandler()(profile.CodeGrantCallback(resultchan)),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		if serveErr := server.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			log.Errorf("Failed to start server: %v", serveErr)
			resultchan <- serveErr
		}
	}()

	// Open the browser to the Authorization Code Grant URL
	common.Verbose(ctx, cmd, "Opening browser to authorize profile %s...", profile.Name)
	spinner := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	bitbucketAuthURL := url.URL{
		Scheme: "https",
		Host:   "bitbucket.org",
		Path:   "/site/oauth2/authorize",
		RawQuery: url.Values{
			"response_type": {"code"},
			"client_id":     {profile.ClientID},
		}.Encode(),
	}
	common.Verbose(ctx, cmd, "\nIf you are not redirected automatically, please open the following URL in your browser:\n%s\n", bitbucketAuthURL.String())

	if cmd.Flag("verbose").Changed {
		spinner.Reverse()
		_ = spinner.Color("blue", "bold")
		spinner.Start()
	}

	err = openBrowser(ctx, bitbucketAuthURL)
	if err != nil {
		log.Warnf("Failed to open browser: %s", err.Error())
		if cmd.Flag("stop-on-error").Value.String() == "true" {
			spinner.Stop()
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "\nPlease open the following URL in your browser:\n%s\n", bitbucketAuthURL.String())
	}

	// Wait until the user stops the server by pressing Ctrl+C
	results := <-resultchan

	spinner.Stop()
	log.Infof("Received results, shutting down server...")
	if err := server.Shutdown(ctx); err != nil {
		log.Errorf("Failed to shut down server: %v", err)
	}

	if results != nil {
		log.Errorf("Authorization process failed: %v", results)
		return results
	}
	common.Verbose(ctx, cmd, "Authorization process completed successfully")
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
			return errors.New("Cannot open browser in SSH session")
		}
		if common.IsWSL() {
			// If the flag interop=true is not set in /etc/wsl.conf, return an error
			if wslInteropDisabled() {
				return errors.New("Cannot open browser in WSL without interop enabled")
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
	return exec.CommandContext(ctx, cmd, args...).Start() //nolint:gosec // cmd is one of a fixed set of literals chosen from runtime.GOOS above, never external input
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
