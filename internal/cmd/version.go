package cmd

// version is the build revision, set at link time via
// -X github.com/avitsrimer/bitbucket-cli/internal/cmd.version=<rev>; "dev" for un-stamped builds.
var version = "dev"

// Version returns the current version of the application.
func Version() string {
	return version
}
