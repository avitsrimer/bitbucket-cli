package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/avitsrimer/bitbucket-cli/internal/cmd"
	"github.com/go-pkgz/lgr"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	lgr.Setup(lgr.Out(os.Stderr), lgr.Err(os.Stderr))

	// signal.NotifyContext cancels ctx on SIGINT/SIGTERM instead of leaving the default action
	// (immediate process termination) in place: every outgoing HTTP request goes through
	// net/http with this same context, so a Ctrl-C during a long-running list/download now lets
	// the in-flight request abort and its own error handling/cleanup (temp file removal on a
	// download, TolerateErrors' partial-failure reporting) run normally, instead of the process
	// simply vanishing mid-request. This is a separate concern from internal/common.ReadSecret's
	// own SIGINT/SIGTERM guard around its no-echo prompt window: that code shells out to `stty`
	// and blocks on a raw terminal read outside of any context, so nothing here would reach it in
	// time regardless -- ReadSecret needs (and has) its own direct signal handling.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := cmd.Execute(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
