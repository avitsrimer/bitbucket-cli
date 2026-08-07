package common

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newConfigTestCommand builds a bare command with a "config" persistent flag, mirroring the one
// registered on the real root command, optionally marked as explicitly set on the command line
func newConfigTestCommand(configFlagValue string, changed bool) *cobra.Command {
	cmd := &cobra.Command{Use: "test"}
	cmd.PersistentFlags().String("config", "", "config file")
	if changed {
		_ = cmd.PersistentFlags().Set("config", configFlagValue)
	}
	return cmd
}

func TestConfigPath(t *testing.T) {
	t.Run("returns the --config flag value when set", func(t *testing.T) {
		cmd := newConfigTestCommand("/tmp/my-config.yml", true)

		path, err := ConfigPath(cmd)

		require.NoError(t, err)
		assert.Equal(t, "/tmp/my-config.yml", path)
	})

	t.Run("falls back to UserConfigDir/bitbucket/config-cli.yml when the flag is not set", func(t *testing.T) {
		cmd := newConfigTestCommand("", false)

		path, err := ConfigPath(cmd)

		require.NoError(t, err)
		configDir, dirErr := os.UserConfigDir()
		require.NoError(t, dirErr)
		assert.Equal(t, filepath.Join(configDir, "bitbucket", "config-cli.yml"), path)
	})

	// This proves BB_CONFIG is honored even when supplied only as the flag's default value: on
	// the real root command the flag's *default* value is populated from BB_CONFIG
	// (common.GetEnvAsString("BB_CONFIG", "")), which never marks the flag as Changed, so ConfigPath
	// must read the flag's current value directly rather than checking Changed("config").
	t.Run("honors BB_CONFIG supplied only as the flag's default value", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		cmd.PersistentFlags().String("config", "/from/env/config.yml", "config file")

		path, err := ConfigPath(cmd)

		require.NoError(t, err)
		assert.Equal(t, "/from/env/config.yml", path)
		assert.False(t, cmd.PersistentFlags().Changed("config"), "the flag must not be Changed for this regression to be meaningful")
	})

	// This reproduces the .env-loading-order regression: the real root command's "config" flag
	// default is baked from BB_CONFIG at package-init time (cmd/root.go's init()), which runs
	// before main() has a chance to call godotenv.Load(), so a BB_CONFIG set only via a .env file
	// is invisible to that baked default. ConfigPath must fall back to reading the environment
	// variable directly -- which, by the time any command actually runs, reflects the .env file --
	// rather than trusting only the (possibly stale) flag default.
	t.Run("honors BB_CONFIG set in the environment after the flag was registered", func(t *testing.T) {
		cmd := newConfigTestCommand("", false)
		t.Setenv("BB_CONFIG", "/from/dotenv/config.yml")

		path, err := ConfigPath(cmd)

		require.NoError(t, err)
		assert.Equal(t, "/from/dotenv/config.yml", path)
	})
}

func TestLoadConfig(t *testing.T) {
	t.Run("loads an existing config file", func(t *testing.T) {
		cmd := newConfigTestCommand("../../testdata/config.yml", true)

		config, err := LoadConfig(cmd)

		require.NoError(t, err)
		require.NotNil(t, config)
		assert.Equal(t, "../../testdata/config.yml", config.Path)
		assert.Contains(t, config.Data, "profiles")
	})

	t.Run("is non-fatal when the file does not exist", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "does-not-exist.yml")
		cmd := newConfigTestCommand(missing, true)

		config, err := LoadConfig(cmd)

		require.NoError(t, err)
		require.NotNil(t, config)
		assert.Equal(t, missing, config.Path)
		assert.Empty(t, config.Data)
	})

	t.Run("returns a clear error for a malformed file", func(t *testing.T) {
		malformed := filepath.Join(t.TempDir(), "malformed.yml")
		require.NoError(t, os.WriteFile(malformed, []byte("profiles: [this is: not, valid: yaml"), 0o600))
		cmd := newConfigTestCommand(malformed, true)

		config, err := LoadConfig(cmd)

		require.Error(t, err)
		assert.Nil(t, config)
	})
}

func TestConfigSaveRoundTrip(t *testing.T) {
	type profileFixture struct {
		Name        string `yaml:"name"`
		User        string `yaml:"user,omitempty"`
		AccessToken string `yaml:"accesstoken,omitempty"`
		Default     bool   `yaml:"default,omitempty"`
	}

	t.Run("SetSection writes a section that GetSection reads back identically", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "bitbucket", "config-cli.yml")
		config := &Config{Path: path, Data: map[string]any{}}
		want := []profileFixture{
			{Name: "simple", User: "user1"},
			{Name: "test", Default: true, AccessToken: "s3cr3tT0k3n"},
		}

		require.NoError(t, config.SetSection("profiles", want))

		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

		reloaded, err := LoadConfig(newConfigTestCommand(path, true))
		require.NoError(t, err)

		var got []profileFixture
		require.NoError(t, reloaded.GetSection("profiles", &got))
		assert.Equal(t, want, got)
	})

	t.Run("GetSection is a no-op when the key is absent", func(t *testing.T) {
		config := &Config{Path: filepath.Join(t.TempDir(), "config.yml"), Data: map[string]any{}}

		var got []profileFixture
		require.NoError(t, config.GetSection("profiles", &got))
		assert.Empty(t, got)
	})
}

// TestGetSectionLowercasesCamelCaseKeys proves a camelCase key such as accessToken populates
// AccessToken: yaml.v3 matches mapping keys case-sensitively against the lower-cased field name it
// defaults to for untagged struct fields, so GetSection must match keys case-insensitively itself.
func TestGetSectionLowercasesCamelCaseKeys(t *testing.T) {
	type fixture struct {
		Name        string `yaml:",omitempty"`
		AccessToken string `yaml:",omitempty"`
		ClientID    string `yaml:",omitempty"`
	}

	config := &Config{
		Path: filepath.Join(t.TempDir(), "config.yml"),
		Data: map[string]any{
			"profiles": []any{
				map[string]any{
					"name":        "camel",
					"accessToken": "t0k3n",
					"clientID":    "abc123",
				},
			},
		},
	}

	var got []fixture
	require.NoError(t, config.GetSection("profiles", &got))
	require.Len(t, got, 1)
	assert.Equal(t, "camel", got[0].Name)
	assert.Equal(t, "t0k3n", got[0].AccessToken)
	assert.Equal(t, "abc123", got[0].ClientID)
}
