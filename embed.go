// Package bitbucketcli embeds the Claude Code plugin's skills directory so the bb binary can write
// the bitbucket-cli skill with install-skill from any directory, independent of the source
// checkout. The plugin directory (plugins/bitbucket-cli) is the single source of truth for both
// install routes: the Claude Code plugin marketplace serves it directly, and install-skill writes
// this embedded copy of it.
package bitbucketcli

import (
	"embed"
	"fmt"
	"io/fs"
)

// The embed directive below silently skips any file or directory under the skills directory whose
// name starts with "." or "_" (Go's documented default matching behavior) -- a future helper file
// named that way (e.g. a ".helpers.md" or "_scripts" directory) would be dropped from the binary
// with no build error or warning. Name any file meant to ship with a skill without a leading dot
// or underscore, or add an explicit second embed pattern for it.
//
//go:embed plugins/bitbucket-cli/skills
var embedded embed.FS

// skillsDir is the embed directive's path, which fs.Sub roots SkillFiles at.
const skillsDir = "plugins/bitbucket-cli/skills"

// SkillFiles is the embedded plugin skills directory, rooted so each skill tree lives under its
// own name (e.g. "bitbucket-cli/SKILL.md"). install-skill writes the bitbucket-cli tree to
// <to>/skills/.
var SkillFiles = func() fs.FS {
	sub, err := fs.Sub(embedded, skillsDir)
	if err != nil {
		panic(fmt.Sprintf("embedded skills tree: %v", err))
	}
	return sub
}()
