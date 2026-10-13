package bitbucketcli_test

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	bitbucketcli "github.com/avitsrimer/bitbucket-cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type frontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// readFrontmatter parses the YAML frontmatter block at the top of a SKILL.md.
func readFrontmatter(t *testing.T, data []byte) frontmatter {
	t.Helper()
	content := string(data)
	require.True(t, strings.HasPrefix(content, "---\n"), "SKILL.md must start with a YAML frontmatter block")

	rest := content[len("---\n"):]
	end := strings.Index(rest, "\n---")
	require.GreaterOrEqual(t, end, 0, "SKILL.md frontmatter must be closed with a second --- line")

	var fm frontmatter
	require.NoError(t, yaml.Unmarshal([]byte(rest[:end]), &fm))
	return fm
}

func TestEmbeddedSkillMdExists(t *testing.T) {
	data, err := fs.ReadFile(bitbucketcli.SkillFiles, "bitbucket-cli/SKILL.md")
	require.NoError(t, err)
	assert.NotEmpty(t, data)
}

func TestEmbeddedSkillMdFrontmatterParses(t *testing.T) {
	data, err := fs.ReadFile(bitbucketcli.SkillFiles, "bitbucket-cli/SKILL.md")
	require.NoError(t, err)

	fm := readFrontmatter(t, data)
	assert.Equal(t, "bitbucket-cli", fm.Name)
	assert.NotEmpty(t, fm.Description)
	for _, phrase := range []string{"pull request", "pipeline", "artifact"} {
		assert.Containsf(t, fm.Description, phrase, "description should mention the trigger phrase %q", phrase)
	}
}

type marketplaceManifest struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Owner   struct {
		Name string `json:"name"`
	} `json:"owner"`
	Metadata struct {
		Version string `json:"version"`
	} `json:"metadata"`
	Plugins []struct {
		Name    string `json:"name"`
		Source  string `json:"source"`
		Version string `json:"version"`
	} `json:"plugins"`
}

type pluginManifest struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, v), "%s must be valid JSON", path)
}

// TestPluginMarketplaceManifests pins the structural invariants of the Claude Code marketplace this
// repo publishes: one plugin, a relative source inside the repo, matching names, and no version
// field anywhere (the plugin tracks commits, so every merge to master is an update).
func TestPluginMarketplaceManifests(t *testing.T) {
	var market marketplaceManifest
	readJSON(t, filepath.Join(".claude-plugin", "marketplace.json"), &market)

	assert.Equal(t, "avitsrimer-bitbucket-cli", market.Name)
	assert.NotEmpty(t, market.Owner.Name)
	assert.Empty(t, market.Version, "marketplace must not pin a version")
	assert.Empty(t, market.Metadata.Version, "marketplace must not pin a version")
	require.Len(t, market.Plugins, 1)

	entry := market.Plugins[0]
	assert.Equal(t, "bitbucket-cli", entry.Name)
	assert.Empty(t, entry.Version, "plugin entry must not pin a version")
	require.True(t, strings.HasPrefix(entry.Source, "./"), "plugin source must be a ./ relative path, got %q", entry.Source)
	require.NotContains(t, entry.Source, "..", "plugin source must stay inside the repo")

	pluginDir := filepath.FromSlash(strings.TrimPrefix(entry.Source, "./"))
	var plugin pluginManifest
	readJSON(t, filepath.Join(pluginDir, ".claude-plugin", "plugin.json"), &plugin)
	assert.Equal(t, entry.Name, plugin.Name, "plugin.json name must match the marketplace entry")
	assert.Empty(t, plugin.Version, "plugin.json must not pin a version")

	skillDir := filepath.Join(pluginDir, "skills", "bitbucket-cli")
	data, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Base(skillDir), readFrontmatter(t, data).Name, "skill name must match its directory")
}

// TestEmbeddedSkillMatchesPluginTree proves the binary embeds exactly the plugin's skill tree, file
// for file, so install-skill and the plugin route can never ship different content.
func TestEmbeddedSkillMatchesPluginTree(t *testing.T) {
	onDisk := os.DirFS(filepath.Join("plugins", "bitbucket-cli", "skills"))

	collect := func(fsys fs.FS) map[string]string {
		files := map[string]string{}
		require.NoError(t, fs.WalkDir(fsys, "bitbucket-cli", func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil || d.IsDir() {
				return walkErr
			}
			data, err := fs.ReadFile(fsys, path)
			if err != nil {
				return fmt.Errorf("read %s: %w", path, err)
			}
			files[path] = string(data)
			return nil
		}))
		return files
	}

	want := collect(onDisk)
	require.NotEmpty(t, want)
	assert.Equal(t, want, collect(bitbucketcli.SkillFiles))
}
