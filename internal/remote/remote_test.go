package remote_test

import (
	"context"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/remote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanGetRepositoryNameWithGitAt(t *testing.T) {
	payload := `
[core]
	repositoryformatversion = 0
	filemode = true
	bare = false
	logallrefupdates = true
[remote "origin"]
	url = git@bitbucket.org:myworkspace/bitbucket-cli.git
	fetch = +refs/heads/*:refs/remotes/origin/*
[remote "alternate"]
	url = git@bitbucket.org:myworkspace/bitbucket-cli
	fetch = +refs/heads/*:refs/remotes/origin/*
[branch "master"]
	remote = origin
	merge = refs/heads/master
[branch "dev"]
	remote = origin
	merge = refs/heads/dev
	`
	r, err := remote.GetRemoteFromReader(context.Background(), strings.NewReader(payload), "origin")
	require.NoError(t, err)
	assert.NotNil(t, r)
	assert.Equal(t, "myworkspace/bitbucket-cli", r.RepositoryName())

	r, err = remote.GetRemoteFromReader(context.Background(), strings.NewReader(payload), "alternate")
	require.NoError(t, err)
	assert.NotNil(t, r)
	assert.Equal(t, "myworkspace/bitbucket-cli", r.RepositoryName())
}

func TestCanGetRepositoryNameWithHTTPS(t *testing.T) {
	payload := `
[core]
	repositoryformatversion = 0
	filemode = true
	bare = false
	logallrefupdates = true
[remote "origin"]
	url = https://bitbucket.org/myworkspace/bitbucket-cli.git
	fetch = +refs/heads/*:refs/remotes/origin/*
[remote "alternate"]
	url = https://bitbucket.org/myworkspace/bitbucket-cli
	fetch = +refs/heads/*:refs/remotes/origin/*
[branch "master"]
	remote = origin
	merge = refs/heads/master
[branch "dev"]
	remote = origin
	merge = refs/heads/dev
	`
	r, err := remote.GetRemoteFromReader(context.Background(), strings.NewReader(payload), "origin")
	require.NoError(t, err)
	assert.NotNil(t, r)
	assert.Equal(t, "myworkspace/bitbucket-cli", r.RepositoryName())

	r, err = remote.GetRemoteFromReader(context.Background(), strings.NewReader(payload), "alternate")
	require.NoError(t, err)
	assert.NotNil(t, r)
	assert.Equal(t, "myworkspace/bitbucket-cli", r.RepositoryName())
}

func TestCanGetWorkspaceNameWithGitAt(t *testing.T) {
	payload := `
[core]
	repositoryformatversion = 0
	filemode = true
	bare = false
	logallrefupdates = true
[remote "origin"]
	url = git@bitbucket.org:myworkspace/bitbucket-cli.git
	fetch = +refs/heads/*:refs/remotes/origin/*
[remote "alternate"]
	url = git@bitbucket.org:myworkspace/bitbucket-cli
	fetch = +refs/heads/*:refs/remotes/origin/*
[branch "master"]
	remote = origin
	merge = refs/heads/master
[branch "dev"]
	remote = origin
	merge = refs/heads/dev
	`
	r, err := remote.GetRemoteFromReader(context.Background(), strings.NewReader(payload), "origin")
	require.NoError(t, err)
	assert.NotNil(t, r)
	assert.Equal(t, "myworkspace", r.WorkspaceName())

	r, err = remote.GetRemoteFromReader(context.Background(), strings.NewReader(payload), "alternate")
	require.NoError(t, err)
	assert.NotNil(t, r)
	assert.Equal(t, "myworkspace", r.WorkspaceName())
}

func TestCanGetWorkspaceNameWithHTTPS(t *testing.T) {
	payload := `
[core]
	repositoryformatversion = 0
	filemode = true
	bare = false
	logallrefupdates = true
[remote "origin"]
	url = https://bitbucket.org/myworkspace/bitbucket-cli.git
	fetch = +refs/heads/*:refs/remotes/origin/*
[remote "alternate"]
	url = https://bitbucket.org/myworkspace/bitbucket-cli
	fetch = +refs/heads/*:refs/remotes/origin/*
[branch "master"]
	remote = origin
	merge = refs/heads/master
[branch "dev"]
	remote = origin
	merge = refs/heads/dev
	`
	r, err := remote.GetRemoteFromReader(context.Background(), strings.NewReader(payload), "origin")
	require.NoError(t, err)
	assert.NotNil(t, r)
	assert.Equal(t, "myworkspace", r.WorkspaceName())

	r, err = remote.GetRemoteFromReader(context.Background(), strings.NewReader(payload), "alternate")
	require.NoError(t, err)
	assert.NotNil(t, r)
	assert.Equal(t, "myworkspace", r.WorkspaceName())
}

func TestCanGetWorkspaceNameWithoutSlash(t *testing.T) {
	r := remote.Remote{URL: "https://bitbucket.org"}
	assert.Empty(t, r.WorkspaceName())
}

// TestGetRemoteFromReaderReportsMissingRemoteAsNotFound reproduces the regression where
// GetGitSection used ini.File.Section (which fabricates and returns an empty section instead of
// reporting one is missing) instead of GetSection: a genuinely absent --git-remote name produced
// an empty, url-less section that read as "not a bitbucket remote" rather than "does not exist".
func TestGetRemoteFromReaderReportsMissingRemoteAsNotFound(t *testing.T) {
	payload := `
[remote "origin"]
	url = git@bitbucket.org:myworkspace/bitbucket-cli.git
	fetch = +refs/heads/*:refs/remotes/origin/*
	`
	_, err := remote.GetRemoteFromReader(context.Background(), strings.NewReader(payload), "does-not-exist")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}
