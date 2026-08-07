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

// resetProfilesState saves the current package-level Profiles/Current globals and the
// process-global common.CurrentConfig(), returning a function that restores all three. Every
// test in this file calls common.Initialize, which overwrites common's config singleton, so
// without this a test that runs later (possibly in another package sharing the same test
// binary) could observe a config pointed at a deleted t.TempDir() from a prior test.
func resetProfilesState() func() {
	oldProfiles := profile.Profiles
	oldCurrent := profile.Current
	oldConfig := common.CurrentConfig()
	profile.Profiles = nil
	profile.Current = nil
	return func() {
		profile.Profiles = oldProfiles
		profile.Current = oldCurrent
		common.SetCurrentConfig(oldConfig)
	}
}

// TestLoadParsesTestdataConfigYAMLProfilesIdentically proves the plain-YAML loader decodes
// testdata/config.yml into the expected Profile values.
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

// TestGetProfileFromCommandHonorsBBProfileEnvironmentVariable reproduces the profile-selection half
// of major finding #7: the "--profile" flag's default value is populated from BB_PROFILE
// (common.GetEnvAsString("BB_PROFILE", "")) at the root command's package-init time, before main()
// has had a chance to load a .env file, so a BB_PROFILE set only via .env was invisible to that
// baked default -- and GetProfileFromCommand only ever consulted the flag's value when Changed was
// true, which a flag holding only its default value never is. GetProfileFromCommand must fall back
// to reading BB_PROFILE directly, selecting the named profile instead of silently falling through
// to the config's own default profile ("test", per testdata/config.yml).
func (suite *ProfileSuite) TestGetProfileFromCommandHonorsBBProfileEnvironmentVariable() {
	defer resetProfilesState()()
	suite.T().Setenv("BB_PROFILE", "simple")

	cmd := newTestRootCommand()
	suite.Require().NoError(cmd.PersistentFlags().Set("config", "../../testdata/config.yml"))
	suite.Require().NoError(common.Initialize(cmd))

	got, err := profile.GetProfileFromCommand(suite.Context, cmd)
	suite.Require().NoError(err)
	suite.Require().NotNil(got)
	suite.Equal("simple", got.Name, "BB_PROFILE must select \"simple\", not silently fall through to the config's default profile (\"test\")")
}

// TestLoadParsesCamelCaseConfigKeys proves camelCase config keys (the spelling documented by
// Profile's json tags) populate their fields instead of being silently dropped by yaml.v3's
// case-sensitive key matching.
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
	suite.Equal("myrepo", got.DefaultRepository)
	suite.Equal("abc123", got.ClientID)
	suite.Equal("s3cr3t", got.ClientSecret)
	suite.Equal("t0k3n", got.AccessToken)
	suite.Equal(uint16(8080), got.CallbackPort)
	suite.Equal("json", got.OutputFormat)
	suite.Equal(common.WarnOnError, got.ErrorProcessing)
	suite.Require().NotNil(got.APIRoot, "a string-form apiRoot must decode into a *url.URL")
	suite.Equal("https://api.bitbucket.org/camel", got.APIRoot.String())
}

// TestLoadParsesStringFormAPIRootAndErrorProcessing proves apiRoot written as a plain URL string
// (e.g. "apiRoot: https://api.bitbucket.org", the form a user would reasonably hand-write, and the
// spelling every other apiRoot-shaped value in this codebase accepts) decodes into url.URL, and
// errorProcessing written as its documented string name (e.g. "WarnOnError", the spelling
// MarshalJSON emits) decodes successfully too, alongside the lowercase key spelling GetSection's
// key-lowercasing produces.
func (suite *ProfileSuite) TestLoadParsesStringFormAPIRootAndErrorProcessing() {
	defer resetProfilesState()()

	cmd := newTestRootCommand()
	suite.Require().NoError(cmd.PersistentFlags().Set("config", "../../testdata/config-string-forms.yml"))
	suite.Require().NoError(common.Initialize(cmd))

	err := profile.Profiles.Load(suite.Context, cmd)
	suite.Require().NoError(err)
	suite.Require().Len(profile.Profiles, 1)

	got := profile.Profiles[0]
	suite.Equal("string-forms", got.Name)
	suite.Require().NotNil(got.APIRoot)
	suite.Equal("https://api.bitbucket.org/lower", got.APIRoot.String())
	suite.Equal(common.WarnOnError, got.ErrorProcessing)
}

// TestLoadParsesEmptyStringAPIRoot proves apiRoot written as an explicit empty string (apiroot set
// to a quoted empty value, something templating or a half-cleared config value can easily produce)
// decodes into a nil *url.URL instead of aborting Load with a yaml unmarshal error.
func (suite *ProfileSuite) TestLoadParsesEmptyStringAPIRoot() {
	defer resetProfilesState()()

	cmd := newTestRootCommand()
	suite.Require().NoError(cmd.PersistentFlags().Set("config", "../../testdata/config-empty-apiroot.yml"))
	suite.Require().NoError(common.Initialize(cmd))

	err := profile.Profiles.Load(suite.Context, cmd)
	suite.Require().NoError(err)
	suite.Require().Len(profile.Profiles, 1)

	got := profile.Profiles[0]
	suite.Equal("empty-apiroot", got.Name)
	suite.Nil(got.APIRoot, "an empty-string apiRoot must decode to a nil *url.URL, not error")
}

// TestLoadParsesStringFormAPIRootPreservesUserinfo proves a string-form apiRoot carrying userinfo
// (e.g. "https://user:pw@api.bitbucket.org") keeps its userinfo intact through decoding, rather
// than normalizing through url.URL's field-by-field mapping form, which marshals url.URL.User (a
// *url.Userinfo whose fields are all unexported) as an empty mapping and silently drops the
// credentials.
func (suite *ProfileSuite) TestLoadParsesStringFormAPIRootPreservesUserinfo() {
	defer resetProfilesState()()

	cmd := newTestRootCommand()
	suite.Require().NoError(cmd.PersistentFlags().Set("config", "../../testdata/config-userinfo-apiroot.yml"))
	suite.Require().NoError(common.Initialize(cmd))

	err := profile.Profiles.Load(suite.Context, cmd)
	suite.Require().NoError(err)
	suite.Require().Len(profile.Profiles, 1)

	got := profile.Profiles[0]
	suite.Equal("userinfo-apiroot", got.Name)
	suite.Require().NotNil(got.APIRoot)
	suite.Require().NotNil(got.APIRoot.User, "userinfo must survive decoding, not be dropped")
	suite.Equal("alice", got.APIRoot.User.Username())
	password, isSet := got.APIRoot.User.Password()
	suite.True(isSet, "password must survive decoding")
	suite.Equal("s3cr3t", password)
	suite.Equal("https://alice:s3cr3t@api.bitbucket.org/userinfo", got.APIRoot.String())
}

// TestLoadParsesNestedAPIRootMapping proves the nested field-by-field mapping form (what
// url.URL's own default yaml decoding already handled, and what this fix must not regress) still
// round-trips alongside the new string-form support.
func (suite *ProfileSuite) TestLoadParsesNestedAPIRootMapping() {
	defer resetProfilesState()()

	cmd := newTestRootCommand()
	suite.Require().NoError(cmd.PersistentFlags().Set("config", "../../testdata/config-nested-apiroot.yml"))
	suite.Require().NoError(common.Initialize(cmd))

	err := profile.Profiles.Load(suite.Context, cmd)
	suite.Require().NoError(err)
	suite.Require().Len(profile.Profiles, 1)

	got := profile.Profiles[0]
	suite.Require().NotNil(got.APIRoot)
	suite.Equal("https://api.bitbucket.org/nested-mapping", got.APIRoot.String())
}

// TestValidProfileNamesListsConfiguredProfiles covers the completion provider backing the root
// --profile flag.
func (suite *ProfileSuite) TestValidProfileNamesListsConfiguredProfiles() {
	defer resetProfilesState()()

	cmd := newTestRootCommand()
	suite.Require().NoError(cmd.PersistentFlags().Set("config", "../../testdata/config.yml"))
	suite.Require().NoError(common.Initialize(cmd))

	names, directive := profile.ValidProfileNames(cmd, nil, "")
	suite.Equal(cobra.ShellCompDirectiveNoFileComp, directive)
	suite.ElementsMatch([]string{"simple", "test"}, names)
}

// TestValidProfileNamesReturnsNoCompletionsWhenArgAlreadyProvided mirrors
// TestOpenPullRequestIDsCompletion's "argument already provided" case for this provider.
func (suite *ProfileSuite) TestValidProfileNamesReturnsNoCompletionsWhenArgAlreadyProvided() {
	defer resetProfilesState()()

	cmd := newTestRootCommand()
	names, directive := profile.ValidProfileNames(cmd, []string{"simple"}, "")
	suite.Equal(cobra.ShellCompDirectiveNoFileComp, directive)
	suite.Nil(names)
}

// TestValidProfileNamesReturnsEmptyWhenNoProfilesConfigured covers the "no profiles yet" path:
// GetProfileFromCommand fails with ErrNoProfiles and completion should degrade to an empty list
// rather than erroring out.
func (suite *ProfileSuite) TestValidProfileNamesReturnsEmptyWhenNoProfilesConfigured() {
	defer resetProfilesState()()

	cmd := newTestRootCommand()
	suite.Require().NoError(cmd.PersistentFlags().Set("config", filepath.Join(suite.T().TempDir(), "missing-config.yml")))
	suite.Require().NoError(common.Initialize(cmd))

	names, directive := profile.ValidProfileNames(cmd, nil, "")
	suite.Equal(cobra.ShellCompDirectiveNoFileComp, directive)
	suite.Empty(names)
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
