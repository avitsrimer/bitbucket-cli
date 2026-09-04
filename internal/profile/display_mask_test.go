package profile_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/zalando/go-keyring"
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
// reload from disk" step, so one call's in-memory state never leaks into the next. It also clears
// the --output flag cobra caches on the profile command singletons (see clearCachedFlags), so
// every call decides the flag's value and Changed bit from its own args alone, in whatever order
// the tests of this package happen to run.
func runProfileCommand(t *testing.T, configPath string, args ...string) string {
	t.Helper()
	clearCachedFlags(t, "output")
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
// --output table is passed explicitly rather than leaving the flag unset, because an unset
// --output lets BB_OUTPUT_FORMAT pick the format (see Profile.resolvedOutputFormat): a developer
// running the suite with that variable set in their own environment would otherwise get a
// different format here.
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

// vaultBackedProfileName/vaultBackedVaultKey name the fixture profile the tests below use to
// exercise the REALISTIC shape of a vault-backed secret: an access token created via
// `bb profile create --access-token` is stored in the OS vault, not the config file
// (see resolveVaultSecret) -- the persisted profile carries no accesstoken of its own at all, and
// LoadSecrets/loadAccessToken is what populates Profile.AccessToken from the vault at runtime, on
// demand. Both names are distinct from every other fixture in this package's tests, so
// loadAccessToken's real (non-test-isolated) os.UserCacheDir() file-cache lookup for this profile
// name is guaranteed to miss -- no test, in this run or any prior one, has ever written a token
// cache file under this name's hash -- and the mock vault seeded below is reached deterministically
// instead.
const vaultBackedProfileName = "fr7-vault-backed-profile"
const vaultBackedVaultKey = "fr7-vault-backed-vault-key"

// writeVaultBackedConfigWithOutputFormat writes a config carrying one profile with NO accesstoken
// of its own and outputformat set as given; see vaultBackedProfileName's doc comment for why this
// shape (rather than writeLiveTokenConfig's directly-embedded accesstoken) is the one that
// exercises LoadSecrets' vault fallback.
func writeVaultBackedConfigWithOutputFormat(t *testing.T, outputFormat string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config-cli.yml")
	content := "profiles:\n    - name: " + vaultBackedProfileName + "\n      default: true\n      outputformat: " + outputFormat + "\n      vaultkey: " + vaultBackedVaultKey + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("cannot write test config: %v", err)
	}
	return path
}

// TestProfileListBareCommandWithProfileOutputFormatJSONDoesNotLoadSecretsFromVault pins that a
// profile configured with `outputformat: json` must NOT make a bare `bb profile list` (no
// explicit -o/--output flag at all) fetch the real token from the vault and render it in
// cleartext, even though Profile.Print picks the profile's OWN OutputFormat ahead of -o with no
// flag and no signal to the caller: LoadSecrets runs ONLY when json/yaml output was explicitly
// requested via -o/--output on the command line.
func (suite *ProfileSuite) TestProfileListBareCommandWithProfileOutputFormatJSONDoesNotLoadSecretsFromVault() {
	defer resetProfilesState()()
	keyring.MockInit()
	suite.Require().NoError(keyring.Set(vaultBackedVaultKey, vaultBackedProfileName, liveLookingAccessToken))
	configPath := writeVaultBackedConfigWithOutputFormat(suite.T(), "json")

	output := runProfileCommand(suite.T(), configPath, "profile", "list")

	suite.NotContains(output, liveLookingAccessToken, "a bare `bb profile list` must not fetch or show the real vault-backed access token just because the profile's own outputFormat is json")
}

// TestProfileGetBareCommandWithProfileOutputFormatYAMLDoesNotLoadSecretsFromVault is
// TestProfileListBareCommandWithProfileOutputFormatJSONDoesNotLoadSecretsFromVault's
// `bb profile get` counterpart, using yaml instead of json.
func (suite *ProfileSuite) TestProfileGetBareCommandWithProfileOutputFormatYAMLDoesNotLoadSecretsFromVault() {
	defer resetProfilesState()()
	keyring.MockInit()
	suite.Require().NoError(keyring.Set(vaultBackedVaultKey, vaultBackedProfileName, liveLookingAccessToken))
	configPath := writeVaultBackedConfigWithOutputFormat(suite.T(), "yaml")

	output := runProfileCommand(suite.T(), configPath, "profile", "get", vaultBackedProfileName)

	suite.NotContains(output, liveLookingAccessToken, "a bare `bb profile get` must not fetch or show the real vault-backed access token just because the profile's own outputFormat is yaml")
}

// TestProfileListExplicitJSONFlagLoadsSecretFromVaultEvenWithProfileOutputFormatTable proves the
// converse: an EXPLICIT -o json on the command line still fetches and shows the real vault-backed
// secret, even when the profile's own outputFormat is left at its "table" default -- the documented
// scripting path (FR-7) must keep working.
func (suite *ProfileSuite) TestProfileListExplicitJSONFlagLoadsSecretFromVaultEvenWithProfileOutputFormatTable() {
	defer resetProfilesState()()
	keyring.MockInit()
	suite.Require().NoError(keyring.Set(vaultBackedVaultKey, vaultBackedProfileName, liveLookingAccessToken))
	configPath := writeVaultBackedConfigWithOutputFormat(suite.T(), "table")

	output := runProfileCommand(suite.T(), configPath, "profile", "list", "--output", "json")

	suite.Contains(output, liveLookingAccessToken, "an explicit -o json on the command line must still fetch and show the real vault-backed token")
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
