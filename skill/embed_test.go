package skill_test

import (
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/skill"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type frontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

func TestEmbeddedSkillMdExists(t *testing.T) {
	data, err := skill.Files.ReadFile("bitbucket-cli/SKILL.md")
	require.NoError(t, err)
	assert.NotEmpty(t, data)
}

func TestEmbeddedSkillMdFrontmatterParses(t *testing.T) {
	data, err := skill.Files.ReadFile("bitbucket-cli/SKILL.md")
	require.NoError(t, err)

	content := string(data)
	require.True(t, strings.HasPrefix(content, "---\n"), "SKILL.md must start with a YAML frontmatter block")

	rest := content[len("---\n"):]
	end := strings.Index(rest, "\n---")
	require.GreaterOrEqual(t, end, 0, "SKILL.md frontmatter must be closed with a second --- line")

	var fm frontmatter
	require.NoError(t, yaml.Unmarshal([]byte(rest[:end]), &fm))

	assert.Equal(t, "bitbucket-cli", fm.Name)
	assert.NotEmpty(t, fm.Description)

	for _, phrase := range []string{"pull request", "pipeline", "artifact"} {
		assert.Containsf(t, fm.Description, phrase, "description should mention the trigger phrase %q", phrase)
	}
}
