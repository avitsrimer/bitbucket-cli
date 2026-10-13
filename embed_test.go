package bitbucketcli_test

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	bitbucketcli "github.com/avitsrimer/bitbucket-cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// pluginDir is the plugin's directory relative to the module root; the marketplace entry's source
// must point at it, and the embed directive in embed.go reads its skills directory.
const pluginDir = "plugins/bitbucket-cli"

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

// readJSON decodes path into a generic value so key presence can be checked at any depth.
func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var v map[string]any
	require.NoError(t, json.Unmarshal(data, &v), "%s must be valid JSON", path)
	return v
}

// findVersionKeys returns the JSON path of every object key named "version" anywhere in v.
func findVersionKeys(v any, at string) []string {
	var found []string
	switch val := v.(type) {
	case map[string]any:
		for key, child := range val {
			if key == "version" {
				found = append(found, at+"."+key)
			}
			found = append(found, findVersionKeys(child, at+"."+key)...)
		}
	case []any:
		for i, child := range val {
			found = append(found, findVersionKeys(child, fmt.Sprintf("%s[%d]", at, i))...)
		}
	}
	return found
}

// TestPluginMarketplaceManifests pins the structural invariants of the Claude Code marketplace this
// repo publishes: one plugin at pluginDir, matching names, and no version key anywhere (the plugin
// tracks commits, so every merge to master is an update).
func TestPluginMarketplaceManifests(t *testing.T) {
	marketPath := filepath.Join(".claude-plugin", "marketplace.json")
	market := readJSON(t, marketPath)
	assert.Equal(t, "avitsrimer-bitbucket-cli", market["name"])
	owner, ok := market["owner"].(map[string]any)
	require.True(t, ok, "marketplace owner must be an object")
	assert.NotEmpty(t, owner["name"])
	assert.Empty(t, findVersionKeys(market, "$"), "%s must not pin a version", marketPath)

	plugins, ok := market["plugins"].([]any)
	require.True(t, ok, "marketplace plugins must be an array")
	require.Len(t, plugins, 1)
	entry, ok := plugins[0].(map[string]any)
	require.True(t, ok, "marketplace plugin entry must be an object")
	assert.Equal(t, "bitbucket-cli", entry["name"])
	assert.Equal(t, "./"+pluginDir, entry["source"])

	pluginPath := filepath.Join(filepath.FromSlash(pluginDir), ".claude-plugin", "plugin.json")
	plugin := readJSON(t, pluginPath)
	assert.Equal(t, entry["name"], plugin["name"], "plugin.json name must match the marketplace entry")
	assert.Empty(t, findVersionKeys(plugin, "$"), "%s must not pin a version", pluginPath)
}

// TestPluginSkillsAreInstallable checks every skill the plugin ships: its frontmatter name matches
// its directory, and it is the bitbucket-cli skill -- install-skill writes only that tree, so a
// second skill would ship through the marketplace but never through install-skill.
func TestPluginSkillsAreInstallable(t *testing.T) {
	skillsDir := filepath.Join(filepath.FromSlash(pluginDir), "skills")
	entries, err := os.ReadDir(skillsDir)
	require.NoError(t, err)

	var skills []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skills = append(skills, entry.Name())
		data, err := os.ReadFile(filepath.Join(skillsDir, entry.Name(), "SKILL.md"))
		require.NoError(t, err)
		assert.Equal(t, entry.Name(), readFrontmatter(t, data).Name, "skill name must match its directory")
	}
	assert.Equal(t, []string{"bitbucket-cli"}, skills,
		"install-skill writes only the bitbucket-cli skill; teach it about any other skill under %s", skillsDir)
}

// TestEmbeddedSkillMatchesPluginTree proves the binary embeds exactly the plugin's skills
// directory, file for file, so install-skill and the plugin route can never ship different content.
func TestEmbeddedSkillMatchesPluginTree(t *testing.T) {
	collect := func(fsys fs.FS, fromDisk bool) map[string]string {
		files := map[string]string{}
		require.NoError(t, fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			name := path.Base(p)
			if fromDisk && p != "." {
				if name == ".DS_Store" {
					return nil // gitignored finder metadata, never shipped
				}
				if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
					t.Errorf("%s is not embedded: go:embed skips names starting with \".\" or \"_\"; rename it", p)
					if d.IsDir() {
						return fs.SkipDir
					}
					return nil
				}
			}
			if d.IsDir() {
				return nil
			}
			data, err := fs.ReadFile(fsys, p)
			if err != nil {
				return fmt.Errorf("read %s: %w", p, err)
			}
			files[p] = string(data)
			return nil
		}))
		return files
	}

	want := collect(os.DirFS(filepath.Join(filepath.FromSlash(pluginDir), "skills")), true)
	require.NotEmpty(t, want)
	assert.Equal(t, want, collect(bitbucketcli.SkillFiles, false))
}
