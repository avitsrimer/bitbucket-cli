// Package bitbucketcli embeds the Claude Code plugin's bitbucket-cli skill tree so the bb binary
// can write it with install-skill from any directory, independent of the source checkout. The
// plugin directory (plugins/bitbucket-cli) is the single source of truth for both install routes:
// the Claude Code plugin marketplace serves it directly, and install-skill writes this embedded
// copy of it.
package bitbucketcli

import (
	"embed"
	"fmt"
	"io/fs"
)

// skillsRoot is the plugin's skills directory, relative to the module root.
const skillsRoot = "plugins/bitbucket-cli/skills"

// The embed directive below silently skips any file or directory under the skill whose name starts
// with "." or "_" (Go's documented default matching behavior) -- a future helper file named that
// way (e.g. a ".helpers.md" or "_scripts" directory) would be dropped from the binary with no build
// error or warning. Name any file meant to ship with the skill without a leading dot or underscore,
// or add an explicit second embed pattern for it.
//
//go:embed plugins/bitbucket-cli/skills/bitbucket-cli
var embedded embed.FS

// SkillFiles is the embedded plugin skills directory, rooted so the skill tree lives under
// "bitbucket-cli/" (e.g. "bitbucket-cli/SKILL.md"). install-skill writes it to <to>/skills/.
var SkillFiles = mustSub(embedded, skillsRoot)

// mustSub roots fsys at dir. fs.Sub fails only on a syntactically invalid dir, and dir is a
// constant, so the panic is a programming error surfaced at package init. A dir that is valid but
// does not match the embed pattern yields an empty tree instead, which
// TestEmbeddedSkillMatchesPluginTree catches.
func mustSub(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(fmt.Sprintf("embedded skill tree %q: %v", dir, err))
	}
	return sub
}
