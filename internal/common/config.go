package common

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// Config represents the plain YAML configuration file on disk.
//
// Data holds the raw top-level content (e.g. the "profiles" key); it is empty, not nil, when the
// config file does not exist yet, so a first save can populate it.
type Config struct {
	Path string
	Data map[string]any
}

// currentConfig is the configuration loaded by the most recent call to Initialize
var currentConfig *Config

// Initialize configures the logger and loads the configuration file
func Initialize(cmd *cobra.Command) (err error) {
	initializeLogger(cmd)
	currentConfig, err = LoadConfig(cmd)
	return err
}

// CurrentConfig returns the configuration loaded by the most recent call to Initialize, or nil if
// Initialize has not run yet
func CurrentConfig() *Config {
	return currentConfig
}

// initializeLogger configures the logger based on the command line flags and environment variables
func initializeLogger(cmd *cobra.Command) {
	options := []lgr.Option{lgr.Out(os.Stderr), lgr.Err(os.Stderr)}
	if cmd.Root().PersistentFlags().Changed("debug") && cmd.Root().PersistentFlags().Lookup("debug").Value.String() == "true" {
		options = append(options, lgr.Debug, lgr.CallerFile, lgr.CallerFunc, lgr.Msec, lgr.LevelBraces)
	}
	lgr.Setup(options...)
}

// ConfigPath resolves the configuration file path from, in order: the --config flag (which also
// carries the BB_CONFIG environment variable as its default value, so it wins whether it came
// from the flag or the environment), os.UserConfigDir()/bitbucket/config-cli.yml, or
// ~/.bitbucket-cli
func ConfigPath(cmd *cobra.Command) (string, error) {
	if flag := cmd.Root().PersistentFlags().Lookup("config"); flag != nil && flag.Value.String() != "" {
		return flag.Value.String(), nil
	}
	if configDir, _ := os.UserConfigDir(); configDir != "" {
		return filepath.Join(configDir, "bitbucket", "config-cli.yml"), nil
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(homeDir, ".bitbucket-cli"), nil
}

// LoadConfig resolves the configuration file path and loads its content.
//
// A missing file is not an error: it logs a warning and returns an empty Config for the resolved
// path, so callers can create the file on first save. A malformed file is a clear error.
func LoadConfig(cmd *cobra.Command) (*Config, error) {
	path, err := ConfigPath(cmd)
	if err != nil {
		return nil, err
	}
	config := &Config{Path: path, Data: map[string]any{}}

	content, err := os.ReadFile(path) //nolint:gosec // path is resolved from --config/UserConfigDir/HomeDir, never external input
	if errors.Is(err, os.ErrNotExist) {
		lgr.Printf("[WARN] config file not found: %s", path)
		return config, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read config file %s: %w", path, err)
	}
	if err := yaml.Unmarshal(content, &config.Data); err != nil {
		return nil, fmt.Errorf("cannot parse config file %s: %w", path, err)
	}
	lgr.Printf("[DEBUG] config file: %s", path)
	return config, nil
}

// GetSection decodes the config's top-level key into target, leaving target untouched when the
// key is absent
//
// Mapping keys are lowercased before decoding (mirroring viper's insensitivise behavior, which
// wrote this file historically): yaml.v3 matches keys case-sensitively against the lower-cased
// struct field name it defaults to, so a camelCase key like defaultWorkspace would otherwise be
// silently ignored instead of populating the DefaultWorkspace field.
func (config *Config) GetSection(key string, target any) error {
	value, found := config.Data[key]
	if !found {
		return nil
	}
	data, err := yaml.Marshal(lowercaseKeys(value))
	if err != nil {
		return fmt.Errorf("cannot re-marshal config section %s: %w", key, err)
	}
	if err := yaml.Unmarshal(data, target); err != nil {
		return fmt.Errorf("cannot decode config section %s: %w", key, err)
	}
	return nil
}

// lowercaseKeys recursively lowercases the keys of any map[string]any found within value,
// descending into slices, so mapping keys match the case-sensitive, lower-cased default that
// yaml.v3 expects for untagged struct fields.
func lowercaseKeys(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for k, v := range typed {
			result[strings.ToLower(k)] = lowercaseKeys(v)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for i, v := range typed {
			result[i] = lowercaseKeys(v)
		}
		return result
	default:
		return value
	}
}

// SetSection sets a top-level key in the config and saves the file
func (config *Config) SetSection(key string, value any) error {
	if config.Data == nil {
		config.Data = map[string]any{}
	}
	config.Data[key] = value
	return config.Save()
}

// Save atomically writes the configuration back to its Path with 0600 permissions
func (config *Config) Save() error {
	data, err := yaml.Marshal(config.Data)
	if err != nil {
		return fmt.Errorf("cannot marshal config: %w", err)
	}

	dir := filepath.Dir(config.Path)
	if mkdirErr := os.MkdirAll(dir, 0o750); mkdirErr != nil {
		return fmt.Errorf("cannot create config directory %s: %w", dir, mkdirErr)
	}

	tempFile, err := os.CreateTemp(dir, ".config-*.yml.tmp")
	if err != nil {
		return fmt.Errorf("cannot create temp config file: %w", err)
	}
	tempPath := tempFile.Name()
	defer func() { _ = os.Remove(tempPath) }() // no-op once the rename below succeeds

	if _, err := tempFile.Write(data); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("cannot write temp config file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("cannot close temp config file: %w", err)
	}
	if err := os.Chmod(tempPath, 0o600); err != nil {
		return fmt.Errorf("cannot set config file permissions: %w", err)
	}
	if err := os.Rename(tempPath, config.Path); err != nil {
		return fmt.Errorf("cannot rename temp config file: %w", err)
	}
	return nil
}
