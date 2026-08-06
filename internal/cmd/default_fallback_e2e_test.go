package cmd_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPullRequestListWorksWithOnlyProfileDefaultsSet reproduces review finding #1 (FR-12's
// --default-repository/--default-workspace fallback was dead in production): profile.Current is
// only ever populated by profile.GetProfileFromCommand/profile.getAll, and every repository-scoped
// RunE resolves the workspace/repository name (repository.GetRepositoryName /
// workspace.GetWorkspaceName) before either of those ever runs in a fresh process, so the
// profile-default rung saw profile.Current == nil and never fired -- "bb pullrequest list" failed
// with "argument repository is missing" even with both defaults set in the profile.
//
// This drives the REAL, compiled bb binary as its own child process, reading a real config file
// from disk via BB_CONFIG and talking to a real httptest server via BB_CONFIG's apiroot -- no test
// code in this file (or anywhere it can reach) ever assigns profile.Current directly, which is
// exactly the state no production invocation of "bb pullrequest list" could reach either.
func TestPullRequestListWorksWithOnlyProfileDefaultsSet(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a full binary; skipped in short mode")
	}

	const workspaceSlug = "fr12-e2e-ws"
	const repositorySlug = "fr12-e2e-repo"
	repositoryPath := "/2.0/repositories/" + workspaceSlug + "/" + repositorySlug
	pullrequestsPath := repositoryPath + "/pullrequests"

	var requestedPaths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPaths = append(requestedPaths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case repositoryPath:
			_, _ = w.Write([]byte(`{"type":"repository","uuid":"{66666666-6666-6666-6666-666666666666}","name":"FR12","full_name":"` + workspaceSlug + `/` + repositorySlug + `","slug":"` + repositorySlug + `"}`))
		case pullrequestsPath:
			_, _ = w.Write([]byte(`{"values":[{"type":"pullrequest","id":1,"title":"FR-12 end-to-end"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"type":"error","error":{"message":"unexpected request to ` + r.URL.Path + `"}}`))
		}
	}))
	defer server.Close()

	configPath := filepath.Join(t.TempDir(), "config-cli.yml")
	configContent := "" +
		"profiles:\n" +
		"  - name: fr12-e2e\n" +
		"    default: true\n" +
		"    apiroot: \"" + server.URL + "\"\n" +
		"    accesstoken: dummy-token\n" +
		"    outputformat: json\n" +
		"    defaultworkspace: " + workspaceSlug + "\n" +
		"    defaultrepository: " + repositorySlug + "\n"
	require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0o600))

	binary := filepath.Join(t.TempDir(), "bb-fr12-e2e")
	build := exec.Command("go", "build", "-o", binary, "github.com/avitsrimer/bitbucket-cli/cmd/bb")
	build.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	buildOut, err := build.CombinedOutput()
	require.NoError(t, err, "go build failed: %s", buildOut)

	// runDir carries no .git directory at all (a fresh t.TempDir), so remote.GetRemote finds no
	// git config to fall back to and the profile's own defaults are the only rung left standing.
	// HOME is likewise pointed at a fresh temp directory: RepositoryCache/WorkspaceCache persist to
	// disk under os.UserCacheDir(), which derives from $HOME, so a real developer's cache (or one
	// left behind by an earlier run of this very test) can never short-circuit the repository
	// lookup this test exists to observe.
	runDir := t.TempDir()

	var stdout, stderr bytes.Buffer
	run := exec.Command(binary, "pullrequest", "list")
	run.Dir = runDir
	run.Env = append(os.Environ(), "BB_CONFIG="+configPath, "HOME="+t.TempDir())
	run.Stdout = &stdout
	run.Stderr = &stderr
	err = run.Run()
	require.NoError(t, err, "bb pullrequest list with only profile defaults set must succeed, stderr: %s", stderr.String())
	require.NotContains(t, stderr.String(), "argument repository is missing")
	require.NotContains(t, stderr.String(), "argument workspace is missing")

	var pullrequests []struct {
		Title string `json:"title"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &pullrequests), "stdout = %q, want valid JSON", stdout.String())
	require.Len(t, pullrequests, 1)
	require.Equal(t, "FR-12 end-to-end", pullrequests[0].Title)

	require.Contains(t, requestedPaths, repositoryPath, "expected a repository lookup against the profile-default workspace/repository")
	require.Contains(t, requestedPaths, pullrequestsPath, "expected a pull request listing against the resolved repository")
}
