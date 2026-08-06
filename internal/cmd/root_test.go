package cmd_test

import (
	"context"
	"errors"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/cmd"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRootWorkspaceFlagAcceptsExplicitValueWithoutCallingAllowedFunc proves, against the real
// RootCmd wiring, that "--workspace <value>" parses without ever calling the flag's AllowedFunc
// (workspace.GetWorkspaceAllowedSlugs, which calls an endpoint needing read:workspace) during
// Set: a workspace-scoped token missing read:workspace must still be able to parse an explicit
// value for a command that might need no such scope at all. Parsing an explicit value must never
// depend on enumerating every candidate.
func TestRootWorkspaceFlagAcceptsExplicitValueWithoutCallingAllowedFunc(t *testing.T) {
	original := cmd.CmdOptions.Workspace.AllowedFunc
	originalValue := cmd.CmdOptions.Workspace.Value
	t.Cleanup(func() {
		cmd.CmdOptions.Workspace.AllowedFunc = original
		cmd.CmdOptions.Workspace.Value = originalValue
	})

	var calls int
	cmd.CmdOptions.Workspace.AllowedFunc = func(context.Context, *cobra.Command, []string, string) ([]string, error) {
		calls++
		return nil, errors.New("Your credentials lack one or more required privilege scopes. (required: read:workspace:bitbucket)")
	}

	err := cmd.RootCmd.PersistentFlags().Set("workspace", "sportpursuit")

	require.NoError(t, err, "parsing --workspace must succeed even when the workspace-listing endpoint is forbidden")
	assert.Equal(t, 0, calls, "the workspace-listing AllowedFunc must not be called while parsing an explicit --workspace value")
	assert.Equal(t, "sportpursuit", cmd.CmdOptions.Workspace.Value)
}
