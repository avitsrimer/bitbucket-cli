package main

import (
	"context"
	"fmt"
	"os"

	"github.com/gildas/bitbucket-cli/cmd"
	"github.com/go-pkgz/lgr"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	lgr.Setup(lgr.Out(os.Stderr), lgr.Err(os.Stderr))
	cmd.RootCmd.Use = APP
	cmd.RootCmd.Version = Version()
	if err := cmd.Execute(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
