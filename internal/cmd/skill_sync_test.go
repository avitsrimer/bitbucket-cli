package cmd_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/cmd"
	"github.com/avitsrimer/bitbucket-cli/skill"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	fencedBlockRE = regexp.MustCompile("(?s)```(.*?)```")
	inlineSpanRE  = regexp.MustCompile("(?s)`([^`]*)`")
)

// extractBBCommandPaths scans SKILL.md's body for every `bb <words...>` occurrence -- inside a
// fenced code block (each line of the block scanned on its own) or an inline code span (which may
// wrap across several markdown source lines, joined into one before scanning) -- and returns the
// leading command words of each occurrence: every word after "bb" up to, but excluding, the first
// token that begins with "-", "<", or "[" (a flag, or a placeholder positional). An occurrence
// with zero such words (a bare mention of the "bb" binary name) is dropped.
func extractBBCommandPaths(body string) [][]string {
	var paths [][]string

	rest := body
	if locs := fencedBlockRE.FindAllStringIndex(body, -1); len(locs) > 0 {
		var sb strings.Builder
		last := 0
		for _, loc := range locs {
			sb.WriteString(body[last:loc[0]])
			last = loc[1]
			inner := body[loc[0]+3 : loc[1]-3]
			for line := range strings.SplitSeq(inner, "\n") {
				paths = append(paths, extractFromWords(strings.Fields(line))...)
			}
		}
		sb.WriteString(body[last:])
		rest = sb.String()
	}

	for _, m := range inlineSpanRE.FindAllStringSubmatch(rest, -1) {
		joined := strings.Join(strings.Fields(m[1]), " ")
		paths = append(paths, extractFromWords(strings.Fields(joined))...)
	}
	return paths
}

// extractFromWords finds every occurrence of the exact token "bb" in words and, for each, returns
// the run of words immediately following it up to the first token beginning with "-", "<", or
// "[". An occurrence directly followed by such a token, or by nothing, contributes no path.
func extractFromWords(words []string) [][]string {
	var paths [][]string
	for i, w := range words {
		if w != "bb" {
			continue
		}
		var cmdWords []string
	inner:
		for _, next := range words[i+1:] {
			if next == "" {
				break inner
			}
			switch next[0] {
			case '-', '<', '[':
				break inner
			}
			cmdWords = append(cmdWords, next)
		}
		if len(cmdWords) > 0 {
			paths = append(paths, cmdWords)
		}
	}
	return paths
}

// TestSkillDocCommandsMatchRealCommandTree asserts that every `bb <words...>` command path named
// in the embedded SKILL.md still exists in the real cobra command tree, at exactly the depth
// SKILL.md documents. cobra's Find does not error on an unknown LEAF verb -- it returns the
// deepest matched parent plus leftover args -- so a bare err == nil check would be vacuous;
// zero leftover args plus an exact CommandPath match is what actually catches a renamed,
// removed, or reflagged command that SKILL.md still describes in its old shape.
func TestSkillDocCommandsMatchRealCommandTree(t *testing.T) {
	data, err := skill.Files.ReadFile("bitbucket-cli/SKILL.md")
	require.NoError(t, err)

	paths := extractBBCommandPaths(string(data))
	require.NotEmpty(t, paths, "expected to find at least one `bb ...` command reference in SKILL.md")

	seen := map[string]bool{}
	for _, words := range paths {
		key := strings.Join(words, " ")
		if seen[key] {
			continue
		}
		seen[key] = true

		found, leftover, findErr := cmd.RootCmd.Find(words)
		require.NoErrorf(t, findErr, "bb %s: RootCmd.Find returned an error", key)
		assert.Emptyf(t, leftover, "bb %s: RootCmd.Find left over %v -- SKILL.md documents a command path that no longer exists", key, leftover)
		assert.Equalf(t, "bb "+key, found.CommandPath(), "bb %s: resolved to %q instead -- likely a rename, or SKILL.md using an alias instead of the canonical name", key, found.CommandPath())
	}
}
