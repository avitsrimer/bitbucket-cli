package profile_test

import (
	"os"
	"path/filepath"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/spf13/cobra"
)

// newTestRootCommand builds a bare root command carrying the same persistent flags the real
// root command registers, so profile subcommands can be exercised without importing cmd (which
// would create an import cycle with this package's own external test package).
func newTestRootCommand() *cobra.Command {
	root := &cobra.Command{Use: "bb"}
	root.PersistentFlags().String("config", "", "config file")
	root.PersistentFlags().StringP("profile", "p", "", "profile to use")
	root.PersistentFlags().String("repository", "", "repository to use")
	root.PersistentFlags().String("workspace", "", "workspace to use")
	root.PersistentFlags().StringP("output", "o", "", "output format")
	root.PersistentFlags().Bool("debug", false, "debug")
	root.PersistentFlags().BoolP("verbose", "v", false, "verbose")
	root.PersistentFlags().Bool("dry-run", false, "dry run")
	root.PersistentFlags().Bool("noop", false, "dry run")
	root.PersistentFlags().Bool("whatif", false, "dry run")
	root.PersistentFlags().Bool("stop-on-error", false, "stop on error")
	root.PersistentFlags().Bool("warn-on-error", false, "warn on error")
	root.PersistentFlags().Bool("ignore-errors", false, "ignore errors")
	return root
}

// resetProfilesState saves the current package-level Profiles/Current globals and returns a
// function that restores them, so tests exercising the real commands don't leak state
func resetProfilesState() func() {
	oldProfiles := profile.Profiles
	oldCurrent := profile.Current
	profile.Profiles = nil
	profile.Current = nil
	return func() {
		profile.Profiles = oldProfiles
		profile.Current = oldCurrent
	}
}

// TestLoadParsesTestdataConfigYAMLProfilesIdentically proves the plain-YAML loader decodes
// testdata/config.yml (the shape viper used to write) into the same Profile values as before.
func (suite *ProfileSuite) TestLoadParsesTestdataConfigYAMLProfilesIdentically() {
	defer resetProfilesState()()

	cmd := newTestRootCommand()
	suite.Require().NoError(cmd.PersistentFlags().Set("config", "../../testdata/config.yml"))
	// force a fresh singleton load bound to this test's --config value: common's currentConfig
	// is process-global and may already be populated by an earlier test in this package
	suite.Require().NoError(common.Initialize(cmd))

	err := profile.Profiles.Load(suite.Context, cmd)
	suite.Require().NoError(err)
	suite.Require().Len(profile.Profiles, 2)

	suite.Equal("simple", profile.Profiles[0].Name)
	suite.Equal("user1", profile.Profiles[0].User)
	suite.Equal("s3cr3tP@ssw0rd", profile.Profiles[0].Password)
	suite.False(profile.Profiles[0].Default)
	suite.Empty(profile.Profiles[0].AccessToken)

	suite.Equal("test", profile.Profiles[1].Name)
	suite.True(profile.Profiles[1].Default)
	suite.Equal("s3cr3tT0k3n", profile.Profiles[1].AccessToken)
	suite.Empty(profile.Profiles[1].User)
}

// TestLoadParsesCamelCaseConfigKeys is a regression test proving camelCase config keys (the
// spelling documented by Profile's json tags, and the shape viper used to accept before the
// plain-YAML loader replaced it) still populate their fields instead of being silently dropped
// by yaml.v3's case-sensitive key matching.
func (suite *ProfileSuite) TestLoadParsesCamelCaseConfigKeys() {
	defer resetProfilesState()()

	cmd := newTestRootCommand()
	suite.Require().NoError(cmd.PersistentFlags().Set("config", "../../testdata/config-camelcase.yml"))
	suite.Require().NoError(common.Initialize(cmd))

	err := profile.Profiles.Load(suite.Context, cmd)
	suite.Require().NoError(err)
	suite.Require().Len(profile.Profiles, 1)

	got := profile.Profiles[0]
	suite.Equal("camel", got.Name)
	suite.Equal("user1", got.User)
	suite.Equal("myworkspace", got.DefaultWorkspace)
	suite.Equal("abc123", got.ClientID)
	suite.Equal("s3cr3t", got.ClientSecret)
	suite.Equal("t0k3n", got.AccessToken)
	suite.Equal(uint16(8080), got.CallbackPort)
	suite.Equal("json", got.OutputFormat)
	suite.Equal(common.WarnOnError, got.ErrorProcessing)
}

// TestProfileCreateAndDeletePersistAcrossReloads drives the real create/delete commands against
// a temp config file, reloading Profiles from disk between commands, to prove CRUD persistence
// round-trips through the plain-YAML config the same way the old viper-backed one did.
func (suite *ProfileSuite) TestProfileCreateAndDeletePersistAcrossReloads() {
	defer resetProfilesState()()

	configPath := filepath.Join(suite.T().TempDir(), "config-cli.yml")

	createRoot := newTestRootCommand()
	createRoot.AddCommand(profile.Command)
	suite.Require().NoError(createRoot.PersistentFlags().Set("config", configPath))
	suite.Require().NoError(common.Initialize(createRoot))
	createRoot.SetArgs([]string{
		"profile", "create",
		"--name", "temptest",
		"--access-token", "abc123",
		"--no-vault",
	})
	suite.Require().NoError(createRoot.Execute())
	suite.Require().Len(profile.Profiles, 1)

	raw, err := os.ReadFile(configPath)
	suite.Require().NoError(err)
	suite.Contains(string(raw), "profiles:")
	suite.Contains(string(raw), "name: temptest")
	suite.Contains(string(raw), "accesstoken: abc123")

	// force a fresh reload from disk, as a separate command invocation would
	profile.Profiles = nil
	profile.Current = nil

	deleteRoot := newTestRootCommand()
	deleteRoot.AddCommand(profile.Command)
	suite.Require().NoError(deleteRoot.PersistentFlags().Set("config", configPath))
	suite.Require().NoError(common.Initialize(deleteRoot))
	deleteRoot.SetArgs([]string{"profile", "delete", "temptest"})
	suite.Require().NoError(deleteRoot.Execute())
	suite.Require().Empty(profile.Profiles)

	raw, err = os.ReadFile(configPath)
	suite.Require().NoError(err)
	suite.NotContains(string(raw), "temptest")

	info, err := os.Stat(configPath)
	suite.Require().NoError(err)
	suite.Equal(os.FileMode(0o600), info.Mode().Perm())
}
