// Package skill embeds the bitbucket-cli Claude skill tree so the installed binary can
// write it from any directory, independent of the source checkout.
package skill

import "embed"

// Files is the embedded bitbucket-cli Claude skill tree, installed by the install-skill command.
//
//go:embed bitbucket-cli
var Files embed.FS
