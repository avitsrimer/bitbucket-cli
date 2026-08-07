// Package skill embeds the bitbucket-cli Claude skill tree so the installed binary can
// write it from any directory, independent of the source checkout.
package skill

import "embed"

// Files is the embedded bitbucket-cli Claude skill tree, installed by the install-skill command.
//
// The embed directive below silently skips any file or directory under bitbucket-cli whose name
// starts with "." or "_" (Go's documented default matching behavior) -- a future helper file
// named that way (e.g. a ".helpers.md" or "_scripts" directory) would be dropped from the binary
// with no build error or warning. Name any file meant to ship with the skill without a leading
// dot or underscore, or add an explicit second embed pattern for it.
//
//go:embed bitbucket-cli
var Files embed.FS
