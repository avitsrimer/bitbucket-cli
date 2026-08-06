package profile_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
)

// liveLookingAccessToken is a realistic-shaped fake access token, distinct enough from any other
// fixture value in this package's tests that an accidental match elsewhere would be obvious.
const liveLookingAccessToken = "bb_live_9f8c7a6b5d4e3f2a1b0c"

// captureStdout redirects os.Stdout for the duration of fn and returns what was written. This
// external test package (profile_test) could import internal/testutil's equivalent without a
// cycle, but every stdout capture already in this package's own test files
// (update_internal_test.go, for the internal package) duplicates this same handful of lines
// rather than pull in the dependency, so this keeps the pattern consistent rather than mixing the
// two approaches.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("cannot create pipe: %v", err)
	}
	original := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = original }()

	captured := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(r)
		captured <- string(data)
	}()

	fn()

	_ = w.Close()
	return <-captured
}

// liveTokenProfileName is the profile name writeLiveTokenConfig writes into its fixture config,
// used as the "profile get <name>" positional argument by every test below.
const liveTokenProfileName = "livetoken"

// writeLiveTokenConfig writes a minimal plain-YAML config file (the on-disk shape Profiles.Load
// reads -- see testdata/config.yml) carrying one profile named liveTokenProfileName with
// AccessToken set to liveLookingAccessToken, returning the file's path.
func writeLiveTokenConfig(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config-cli.yml")
	content := "profiles:\n    - name: " + liveTokenProfileName + "\n      default: true\n      accesstoken: " + liveLookingAccessToken + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("cannot write test config: %v", err)
	}
	return path
}

// runProfileCommand executes profile.Command with args against configPath, returning captured
// stdout. It forces profile.Profiles/profile.Current back to nil and re-runs common.Initialize
// before every call, mirroring TestProfileCreateAndDeletePersistAcrossReloads' "force a fresh
// reload from disk" step, so one call's in-memory state never leaks into the next.
func runProfileCommand(t *testing.T, configPath string, args ...string) string {
	t.Helper()
	profile.Profiles = nil
	profile.Current = nil

	root := newTestRootCommand()
	root.AddCommand(profile.Command)
	if err := root.PersistentFlags().Set("config", configPath); err != nil {
		t.Fatalf("cannot set config flag: %v", err)
	}
	if err := common.Initialize(root); err != nil {
		t.Fatalf("cannot initialize config: %v", err)
	}
	root.SetArgs(args)

	return captureStdout(t, func() {
		if err := root.Execute(); err != nil {
			t.Fatalf("profile command %v failed: %v", args, err)
		}
	})
}

// TestProfileListTableMasksAccessTokenColumn proves `bb profile list --columns accesstoken`
// renders the masked (redactWithHash) form in table output, never the raw token -- the FR-7 fix.
//
// --output table is passed explicitly rather than relying on the empty-flag default: listCmd is
// a package-level singleton reused by every test in this file (and beyond) driven through its
// OWN, freshly built root command each time, but cobra's Command.parentsPflags is cached on the
// singleton itself the first time flag merging runs and is never invalidated for a later call's
// different root -- so an explicit --output on some other test using listCmd earlier in this
// (alphabetically ordered) suite run can otherwise leak into this one's unset default.
func (suite *ProfileSuite) TestProfileListTableMasksAccessTokenColumn() {
	defer resetProfilesState()()
	configPath := writeLiveTokenConfig(suite.T())

	output := runProfileCommand(suite.T(), configPath, "profile", "list", "--columns", "name,accesstoken", "--output", "table")

	suite.NotContains(output, liveLookingAccessToken, "table output must never render the raw access token")
	suite.Contains(output, "REDACTED-", "the accesstoken column must render through redactWithHash's masked form")
}

// TestProfileGetTableMasksAccessTokenColumn is TestProfileListTableMasksAccessTokenColumn's
// `bb profile get <name>` counterpart (see that test's comment for why --output table is
// explicit).
func (suite *ProfileSuite) TestProfileGetTableMasksAccessTokenColumn() {
	defer resetProfilesState()()
	configPath := writeLiveTokenConfig(suite.T())

	output := runProfileCommand(suite.T(), configPath, "profile", "get", liveTokenProfileName, "--columns", "name,accesstoken", "--output", "table")

	suite.NotContains(output, liveLookingAccessToken, "table output must never render the raw access token")
	suite.Contains(output, "REDACTED-", "the accesstoken column must render through redactWithHash's masked form")
}

// TestProfileListCSVMasksAccessTokenColumn proves the masking also covers csv/tsv output, which
// shares GetRow with the table renderer.
func (suite *ProfileSuite) TestProfileListCSVMasksAccessTokenColumn() {
	defer resetProfilesState()()
	configPath := writeLiveTokenConfig(suite.T())

	output := runProfileCommand(suite.T(), configPath, "profile", "list", "--columns", "name,accesstoken", "--output", "csv")

	suite.NotContains(output, liveLookingAccessToken, "csv output must never render the raw access token")
	suite.Contains(output, "REDACTED-", "the accesstoken column must render through redactWithHash's masked form")
}

// TestProfileListJSONShowsRealAccessToken proves the documented decision (MarshalJSON's doc
// comment) still holds: `-o json` is the scripting path and must show the real secret.
func (suite *ProfileSuite) TestProfileListJSONShowsRealAccessToken() {
	defer resetProfilesState()()
	configPath := writeLiveTokenConfig(suite.T())

	output := runProfileCommand(suite.T(), configPath, "profile", "list", "--output", "json")

	suite.Contains(output, liveLookingAccessToken, "json output is the documented scripting path and must show the real token")
}

// TestProfileGetYAMLShowsRealAccessToken is TestProfileListJSONShowsRealAccessToken's yaml/get
// counterpart.
func (suite *ProfileSuite) TestProfileGetYAMLShowsRealAccessToken() {
	defer resetProfilesState()()
	configPath := writeLiveTokenConfig(suite.T())

	output := runProfileCommand(suite.T(), configPath, "profile", "get", liveTokenProfileName, "--output", "yaml")

	suite.Contains(output, liveLookingAccessToken, "yaml output is the documented scripting path and must show the real token")
}

// TestProfileColumnsRejectsClientSecretAndPassword proves ClientSecret and Password have no
// --columns value at all (unlike AccessToken, which has one masked by redactWithHash): requesting
// either is rejected at cobra flag-parse time, before GetRow or any request runs, so there is no
// code path through which either secret could ever reach table/csv/tsv output.
func (suite *ProfileSuite) TestProfileColumnsRejectsClientSecretAndPassword() {
	defer resetProfilesState()()
	configPath := writeLiveTokenConfig(suite.T())

	for _, column := range []string{"clientsecret", "password"} {
		suite.Run(column, func() {
			profile.Profiles = nil
			profile.Current = nil

			root := newTestRootCommand()
			root.AddCommand(profile.Command)
			suite.Require().NoError(root.PersistentFlags().Set("config", configPath))
			suite.Require().NoError(common.Initialize(root))
			root.SetArgs([]string{"profile", "list", "--columns", column})

			err := root.Execute()
			suite.Error(err, "--columns %s must be rejected at flag-parse time", column)
		})
	}
}
