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
