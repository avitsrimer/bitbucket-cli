package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/pullrequest"
	"github.com/avitsrimer/bitbucket-cli/internal/user"
	"github.com/avitsrimer/bitbucket-cli/internal/workspace"
	"github.com/gildas/go-core"
	"github.com/spf13/cobra"
)

// RootOptions describes the global flags whose value is read back through the EnumFlag
// type itself (dynamic allowed-value resolution, shell completion) rather than through
// cmd.Flag(name); every other persistent flag is read via cmd.Flag(name) at its point of
// use instead of being bound to a struct field here.
type RootOptions struct {
	Workspace    *common.EnumFlag
	OutputFormat common.EnumFlag
}

// CmdOptions contains the options for the application
var CmdOptions RootOptions

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:     "bb",
	Short:   "BitBucket Command Line Interface",
	Version: Version(),
	Long: `BitBucket Command Line Interface is a tool to manage your BitBucket.
You can manage your pull requests, issues, profiles, etc.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("bb requires a command:")
		for _, command := range cmd.Commands() {
			fmt.Println(command.Name())
		}
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute(context context.Context) error {
	return RootCmd.ExecuteContext(context) //nolint:wrapcheck // main.go prints this error verbatim; wrapping would prefix every command failure with redundant noise
}

func init() {
	// configDir only feeds the --config flag's help text below (the default path shown to the
	// user); an error here (e.g. no $HOME in a container/CI) must not abort every invocation,
	// including bb --version or bb --help, which don't otherwise need it.
	configDir, _ := os.UserConfigDir()

	// Global flags
	CmdOptions.Workspace = common.NewEnumFlagWithFunc(RootCmd, "", workspace.GetWorkspaceAllowedSlugs)
	CmdOptions.OutputFormat = common.EnumFlag{Allowed: []string{"csv", "json", "yaml", "table", "tsv"}, Value: core.GetEnvAsString("BB_OUTPUT_FORMAT", "")}
	RootCmd.PersistentFlags().String("config", core.GetEnvAsString("BB_CONFIG", ""), "config file, also read from BB_CONFIG (default is "+filepath.Join(configDir, "bitbucket", "config-cli.yml")+")")
	RootCmd.PersistentFlags().StringP("profile", "p", core.GetEnvAsString("BB_PROFILE", ""), "Profile to use. Overrides the default profile")
	RootCmd.PersistentFlags().Var(CmdOptions.Workspace, "workspace", "Workspace to use. Overrides the default workspace of the profile. \nBy default, the workspace is determined from the git or profile configuration")
	RootCmd.PersistentFlags().String("repository", "", "Repository to use. Overrides the default repository of the profile. \nBy default, the repository is determined from the git configuration")
	RootCmd.PersistentFlags().Bool("dry-run", false, "Dry run, the command will not modify anything but tell what it would do. \nAlso known as --noop or --whatif")
	RootCmd.PersistentFlags().Bool("noop", false, "Dry run, the command will not modify anything but tell what it would do. \nAlso known as --dry-run or --whatif")
	RootCmd.PersistentFlags().Bool("whatif", false, "Dry run, the command will not modify anything but tell what it would do. \nAlso known as --dry-run or --noop")
	RootCmd.PersistentFlags().Bool("debug", false, "logs are written at DEBUG level")
	RootCmd.PersistentFlags().BoolP("verbose", "v", false, "Verbose mode")
	RootCmd.PersistentFlags().VarP(&CmdOptions.OutputFormat, "output", "o", "Output format (csv, json, yaml, table, tsv), also read from BB_OUTPUT_FORMAT. Overrides the default output format of the profile")
	RootCmd.PersistentFlags().Bool("stop-on-error", false, "Stop on error")
	RootCmd.PersistentFlags().Bool("warn-on-error", false, "Warn on error")
	RootCmd.PersistentFlags().Bool("ignore-errors", false, "Ignore errors")
	RootCmd.MarkFlagsMutuallyExclusive("stop-on-error", "warn-on-error", "ignore-errors")
	_ = RootCmd.MarkFlagFilename("config")
	_ = RootCmd.RegisterFlagCompletionFunc("profile", profile.ValidProfileNames)
	_ = RootCmd.RegisterFlagCompletionFunc(CmdOptions.OutputFormat.CompletionFunc("output"))
	_ = RootCmd.RegisterFlagCompletionFunc(CmdOptions.Workspace.CompletionFunc("workspace"))

	RootCmd.AddCommand(profile.Command)
	RootCmd.AddCommand(pullrequest.Command)
	RootCmd.AddCommand(user.Command)

	RootCmd.SilenceUsage = true // Do not show usage when an error occurs
	cobra.OnInitialize(func() {
		if err := common.Initialize(RootCmd); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to initialize: %s\n", err)
			os.Exit(1)
		}
	})
}
