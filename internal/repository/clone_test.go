package repository

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/workspace"
)

// setCloneProtocolForTest overrides the package-level cloneOptions.Protocol.Value (the real
// --protocol flag is bound to it via cobra) and restores the original value once the test ends.
func setCloneProtocolForTest(t *testing.T, value string) {
	t.Helper()
	original := cloneOptions.Protocol.Value
	cloneOptions.Protocol.Value = value
	t.Cleanup(func() { cloneOptions.Protocol.Value = original })
}

// setCloneSSHKeyFileForTest overrides the package-level cloneOptions.SSHKeyFilename and restores
// the original value once the test ends.
func setCloneSSHKeyFileForTest(t *testing.T, value string) {
	t.Helper()
	original := cloneOptions.SSHKeyFilename
	cloneOptions.SSHKeyFilename = value
	t.Cleanup(func() { cloneOptions.SSHKeyFilename = original })
}

// setupGitShim replaces the "git" binary resolved via PATH with a fake script that records its
// exact argv (one argument per line) to argvFile, and, if GIT_SSH_COMMAND is set, mirrors it to
// sshFile; it exits with $GIT_SHIM_EXIT_CODE (0 if unset). The real git binary already on PATH is
// never invoked by tests using this helper.
func setupGitShim(t *testing.T) (argvFile, sshFile string) {
	t.Helper()

	binDir := t.TempDir()
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$@\" > \"$GIT_SHIM_ARGV_FILE\"\n" +
		"if [ -n \"$GIT_SSH_COMMAND\" ]; then printf '%s' \"$GIT_SSH_COMMAND\" > \"$GIT_SHIM_SSH_FILE\"; fi\n" +
		"exit \"${GIT_SHIM_EXIT_CODE:-0}\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "git"), []byte(script), 0o755); err != nil {
		t.Fatalf("cannot write git shim: %v", err)
	}

	argvFile = filepath.Join(t.TempDir(), "argv")
	sshFile = filepath.Join(t.TempDir(), "ssh")
	t.Setenv("GIT_SHIM_ARGV_FILE", argvFile)
	t.Setenv("GIT_SHIM_SSH_FILE", sshFile)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return argvFile, sshFile
}

// failIfCalled is an http.HandlerFunc that fails the test if the HTTP server is ever hit; clone
// tests prime RepositoryCache directly so GetRepositoryBySlugOrID never needs the network.
func failIfCalled(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(http.ResponseWriter, *http.Request) {
		t.Error("unexpected HTTP request: repository resolution should have hit the cache")
	}
}

// primeRepositoryForClone puts a Repository directly into RepositoryCache so tests never need a
// real HTTP round trip to resolve the clone target. The cache round-trips through JSON (see
// Cache.Set/Get), and Repository.UnmarshalJSON's Validate call requires ID/Name/FullName, so the
// fixture needs all three to actually survive being read back.
func primeRepositoryForClone(t *testing.T, slug string) {
	t.Helper()
	id, err := common.ParseUUID("{aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa}")
	if err != nil {
		t.Fatalf("cannot parse fixture uuid: %v", err)
	}
	if err := RepositoryCache.Set(testWorkspaceSlug+"/"+slug, Repository{ID: id, Slug: slug, Name: slug, FullName: testWorkspaceSlug + "/" + slug}); err != nil {
		t.Fatalf("cannot prime repository cache: %v", err)
	}
}

func TestCloneProcessArgvAndProtocolPrecedence(t *testing.T) {
	tests := []struct {
		name            string
		flagProtocol    string
		profileProtocol string
		wantURL         string
	}{
		{
			name:            "flag beats profile and default",
			flagProtocol:    "https",
			profileProtocol: "ssh",
			wantURL:         "https://bitbucket.org/" + testWorkspaceSlug + "/clone-precedence-flag.git",
		},
		{
			name:            "profile beats default when flag unset",
			flagProtocol:    "",
			profileProtocol: "ssh",
			wantURL:         "ssh://git@bitbucket.org/" + testWorkspaceSlug + "/clone-precedence-profile.git",
		},
		{
			name:            "default git form when neither flag nor profile set",
			flagProtocol:    "",
			profileProtocol: "",
			wantURL:         "git@bitbucket.org:" + testWorkspaceSlug + "/clone-precedence-default.git",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// derive a stable, unique slug per case from the expected URL's repo component, so
			// each subtest primes a distinct cache entry.
			slug := tt.wantURL[strings.LastIndex(tt.wantURL, "/")+1:]
			slug = strings.TrimSuffix(slug, ".git")

			argvFile, _ := setupGitShim(t)
			cmd := setupTest(t, "repository-clone-precedence-"+slug, failIfCalled(t), false)
			primeRepositoryForClone(t, slug)

			setCloneProtocolForTest(t, tt.flagProtocol)
			profile.Current.CloneProtocol = tt.profileProtocol
			t.Cleanup(func() { profile.Current.CloneProtocol = "" })

			if err := cloneProcess(cmd, []string{slug}); err != nil {
				t.Fatalf("cloneProcess() error = %v", err)
			}

			gotArgv := readLines(t, argvFile)
			wantArgv := []string{"clone", tt.wantURL, slug}
			if !slicesEqual(gotArgv, wantArgv) {
				t.Errorf("git argv = %q, want %q", gotArgv, wantArgv)
			}
		})
	}
}

func TestCloneProcessDestinationDefaultsToSlug(t *testing.T) {
	const slug = "clone-destination-default"

	argvFile, _ := setupGitShim(t)
	cmd := setupTest(t, "repository-clone-destination-default", failIfCalled(t), false)
	primeRepositoryForClone(t, slug)
	setCloneProtocolForTest(t, "")

	if err := cloneProcess(cmd, []string{slug}); err != nil {
		t.Fatalf("cloneProcess() error = %v", err)
	}

	gotArgv := readLines(t, argvFile)
	wantArgv := []string{"clone", "git@bitbucket.org:" + testWorkspaceSlug + "/" + slug + ".git", slug}
	if !slicesEqual(gotArgv, wantArgv) {
		t.Errorf("git argv = %q, want %q", gotArgv, wantArgv)
	}
}

func TestCloneProcessExplicitDestination(t *testing.T) {
	const slug = "clone-explicit-destination"
	const destination = "some/other/path"

	argvFile, _ := setupGitShim(t)
	cmd := setupTest(t, "repository-clone-explicit-destination", failIfCalled(t), false)
	primeRepositoryForClone(t, slug)
	setCloneProtocolForTest(t, "")

	if err := cloneProcess(cmd, []string{slug, destination}); err != nil {
		t.Fatalf("cloneProcess() error = %v", err)
	}

	gotArgv := readLines(t, argvFile)
	wantArgv := []string{"clone", "git@bitbucket.org:" + testWorkspaceSlug + "/" + slug + ".git", destination}
	if !slicesEqual(gotArgv, wantArgv) {
		t.Errorf("git argv = %q, want %q", gotArgv, wantArgv)
	}
}

func TestCloneProcessSetsGitSSHCommandWhenKeyFileConfigured(t *testing.T) {
	const slug = "clone-ssh-key"
	const keyFile = "/home/tester/.ssh/id_ed25519"

	_, sshFile := setupGitShim(t)
	cmd := setupTest(t, "repository-clone-ssh-key", failIfCalled(t), false)
	primeRepositoryForClone(t, slug)
	setCloneProtocolForTest(t, "ssh")
	setCloneSSHKeyFileForTest(t, keyFile)

	if err := cloneProcess(cmd, []string{slug}); err != nil {
		t.Fatalf("cloneProcess() error = %v", err)
	}

	got, err := os.ReadFile(sshFile)
	if err != nil {
		t.Fatalf("cannot read GIT_SSH_COMMAND capture file: %v", err)
	}
	want := "ssh -i " + shellQuoteSingle(keyFile)
	if string(got) != want {
		t.Errorf("GIT_SSH_COMMAND = %q, want %q", string(got), want)
	}
}

// verifyGitSSHCommandSurvivesShell takes the GIT_SSH_COMMAND value cloneProcess constructed
// (captured via setupGitShim's ssh file) and feeds it through an actual /bin/sh, the same way
// git itself interprets GIT_SSH_COMMAND (as a shell command line, not an argv vector, per
// git-config(1)). It fails the test unless the shell parses it into exactly one "ssh", one "-i",
// and one path argument equal to keyFile — proving the value survived shell re-parsing intact.
// This is what actually distinguishes the quoted fix from the unquoted regression: capturing the
// raw string (as the other clone tests do) cannot, since env vars are never shell-parsed when set.
func verifyGitSSHCommandSurvivesShell(t *testing.T, sshFile, keyFile string) {
	t.Helper()

	captured, err := os.ReadFile(sshFile)
	if err != nil {
		t.Fatalf("cannot read GIT_SSH_COMMAND capture file: %v", err)
	}

	sshShimDir := t.TempDir()
	sshArgvFile := filepath.Join(t.TempDir(), "ssh-argv")
	sshMarkerFile := filepath.Join(t.TempDir(), "injection-marker")
	sshShimScript := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + shellQuoteSingle(sshArgvFile) + "\n"
	if writeErr := os.WriteFile(filepath.Join(sshShimDir, "ssh"), []byte(sshShimScript), 0o755); writeErr != nil {
		t.Fatalf("cannot write ssh shim: %v", writeErr)
	}
	if writeErr := os.WriteFile(filepath.Join(sshShimDir, "touch"), []byte("#!/bin/sh\n: > "+shellQuoteSingle(sshMarkerFile)+"\n"), 0o755); writeErr != nil {
		t.Fatalf("cannot write touch shim: %v", writeErr)
	}

	// git invokes GIT_SSH_COMMAND as a shell command line, appending the ssh host/remote-command
	// arguments; reproduce that here without going through git itself.
	shCmd := exec.Command("sh", "-c", string(captured)+" fakehost true")
	shCmd.Env = append(os.Environ(), "PATH="+sshShimDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if out, runErr := shCmd.CombinedOutput(); runErr != nil {
		t.Fatalf("sh -c %q failed: %v (output: %s)", string(captured), runErr, out)
	}

	if _, statErr := os.Stat(sshMarkerFile); statErr == nil {
		t.Fatal("shell command injection: a semicolon in the key path ran a second command")
	}

	argv, err := os.ReadFile(sshArgvFile)
	if err != nil {
		t.Fatalf("ssh was never invoked by the shell (cannot read its argv capture): %v", err)
	}
	gotArgv := strings.Split(strings.TrimSuffix(string(argv), "\n"), "\n")
	wantArgv := []string{"-i", keyFile, "fakehost", "true"}
	if !slicesEqual(gotArgv, wantArgv) {
		t.Errorf("ssh argv after shell re-parsing = %q, want %q (key path was split or otherwise mangled)", gotArgv, wantArgv)
	}
}

func TestCloneProcessSetsGitSSHCommandWithSpaceInKeyPath(t *testing.T) {
	const slug = "clone-ssh-key-space"
	const keyFile = "/home/tester/My Keys/id_ed25519"

	_, sshFile := setupGitShim(t)
	cmd := setupTest(t, "repository-clone-ssh-key-space", failIfCalled(t), false)
	primeRepositoryForClone(t, slug)
	setCloneProtocolForTest(t, "ssh")
	setCloneSSHKeyFileForTest(t, keyFile)

	if err := cloneProcess(cmd, []string{slug}); err != nil {
		t.Fatalf("cloneProcess() error = %v", err)
	}

	verifyGitSSHCommandSurvivesShell(t, sshFile, keyFile)
}

func TestCloneProcessSetsGitSSHCommandWithSemicolonInKeyPath(t *testing.T) {
	const slug = "clone-ssh-key-semicolon"
	// a key path containing ';' would, if interpolated unquoted into a string handed to /bin/sh,
	// let an attacker-controlled config value run an arbitrary second command.
	const keyFile = "/tmp/pwned;touch /tmp/clone-ssh-injection-marker"

	_, sshFile := setupGitShim(t)
	cmd := setupTest(t, "repository-clone-ssh-key-semicolon", failIfCalled(t), false)
	primeRepositoryForClone(t, slug)
	setCloneProtocolForTest(t, "ssh")
	setCloneSSHKeyFileForTest(t, keyFile)

	if err := cloneProcess(cmd, []string{slug}); err != nil {
		t.Fatalf("cloneProcess() error = %v", err)
	}

	verifyGitSSHCommandSurvivesShell(t, sshFile, keyFile)
}

func TestCloneProcessNoGitSSHCommandForHTTPS(t *testing.T) {
	const slug = "clone-https-no-ssh-command"
	const keyFile = "/home/tester/.ssh/id_ed25519"

	_, sshFile := setupGitShim(t)
	cmd := setupTest(t, "repository-clone-https-no-ssh-command", failIfCalled(t), false)
	primeRepositoryForClone(t, slug)
	setCloneProtocolForTest(t, "https")
	setCloneSSHKeyFileForTest(t, keyFile)

	if err := cloneProcess(cmd, []string{slug}); err != nil {
		t.Fatalf("cloneProcess() error = %v", err)
	}

	if _, err := os.Stat(sshFile); err == nil {
		t.Error("GIT_SSH_COMMAND capture file exists, want it absent for --protocol https")
	} else if !os.IsNotExist(err) {
		t.Fatalf("unexpected error checking ssh capture file: %v", err)
	}
}

func TestCloneProcessDestinationTrimsGitSuffix(t *testing.T) {
	const slug = "clone-destination-trim.git"
	const wantDestination = "clone-destination-trim"

	argvFile, _ := setupGitShim(t)
	cmd := setupTest(t, "repository-clone-destination-trim", failIfCalled(t), false)
	primeRepositoryForClone(t, slug)
	setCloneProtocolForTest(t, "")

	if err := cloneProcess(cmd, []string{slug}); err != nil {
		t.Fatalf("cloneProcess() error = %v", err)
	}

	gotArgv := readLines(t, argvFile)
	wantArgv := []string{"clone", "git@bitbucket.org:" + testWorkspaceSlug + "/" + slug + ".git", wantDestination}
	if !slicesEqual(gotArgv, wantArgv) {
		t.Errorf("git argv = %q, want %q", gotArgv, wantArgv)
	}
}

func TestCloneProcessNoGitSSHCommandWhenNoKeyFileConfigured(t *testing.T) {
	const slug = "clone-no-ssh-key"

	_, sshFile := setupGitShim(t)
	cmd := setupTest(t, "repository-clone-no-ssh-key", failIfCalled(t), false)
	primeRepositoryForClone(t, slug)
	setCloneProtocolForTest(t, "")
	setCloneSSHKeyFileForTest(t, "")

	if err := cloneProcess(cmd, []string{slug}); err != nil {
		t.Fatalf("cloneProcess() error = %v", err)
	}

	if _, err := os.Stat(sshFile); err == nil {
		t.Error("GIT_SSH_COMMAND capture file exists, want it absent when no ssh key file is configured")
	} else if !os.IsNotExist(err) {
		t.Fatalf("unexpected error checking ssh capture file: %v", err)
	}
}

func TestCloneProcessPropagatesGitFailure(t *testing.T) {
	const slug = "clone-git-failure"

	setupGitShim(t)
	t.Setenv("GIT_SHIM_EXIT_CODE", "7")
	cmd := setupTest(t, "repository-clone-git-failure", failIfCalled(t), false)
	primeRepositoryForClone(t, slug)
	setCloneProtocolForTest(t, "")

	err := cloneProcess(cmd, []string{slug})
	if err == nil {
		t.Fatal("cloneProcess() expected an error when git clone fails, got nil")
	}
	if !strings.Contains(err.Error(), "git clone failed") {
		t.Errorf("error = %q, want it to mention the git clone failure", err.Error())
	}
}

func TestCloneProcessDryRunDoesNotInvokeGit(t *testing.T) {
	const slug = "clone-dry-run"

	binDir := t.TempDir()
	argvFile := filepath.Join(t.TempDir(), "argv")
	t.Setenv("GIT_SHIM_ARGV_FILE", argvFile)
	// deliberately no git binary written into binDir, and PATH set to binDir ONLY (not appended
	// to the real PATH): system git must not be resolvable, so any exec("git", ...) attempt fails
	// with "executable file not found" rather than silently succeeding against the real binary.
	t.Setenv("PATH", binDir)

	cmd := setupTest(t, "repository-clone-dry-run", failIfCalled(t), true)
	primeRepositoryForClone(t, slug)
	setCloneProtocolForTest(t, "")

	if err := cloneProcess(cmd, []string{slug}); err != nil {
		t.Fatalf("cloneProcess() error = %v", err)
	}

	if _, err := os.Stat(argvFile); err == nil {
		t.Error("git argv capture file exists, want git to never have been invoked in dry-run mode")
	} else if !os.IsNotExist(err) {
		t.Fatalf("unexpected error checking argv capture file: %v", err)
	}
}

func TestCloneProcessNeverLogsUserinfo(t *testing.T) {
	const slug = "clone-userinfo"
	const cloneUser = "sekret-clone-user"

	logBuf := captureLog(t)

	setupGitShim(t)
	cmd := setupTest(t, "repository-clone-userinfo", failIfCalled(t), false)
	primeRepositoryForClone(t, slug)
	setCloneProtocolForTest(t, "https")

	profile.Current.CloneUser = cloneUser
	t.Cleanup(func() { profile.Current.CloneUser = "" })

	if err := cloneProcess(cmd, []string{slug}); err != nil {
		t.Fatalf("cloneProcess() error = %v", err)
	}

	logged := logBuf.String()
	if strings.Contains(logged, cloneUser) {
		t.Errorf("log output leaked clone userinfo: %q", logged)
	}
}

func TestBuildCloneURL(t *testing.T) {
	tests := []struct {
		name      string
		protocol  string
		cloneUser string
		wantURL   string
	}{
		{name: "git default", protocol: "git", wantURL: "git@bitbucket.org:ws/repo.git"},
		{name: "ssh", protocol: "ssh", wantURL: "ssh://git@bitbucket.org/ws/repo.git"},
		{name: "https no user", protocol: "https", wantURL: "https://bitbucket.org/ws/repo.git"},
		{name: "https with user", protocol: "https", cloneUser: "alice", wantURL: "https://alice@bitbucket.org/ws/repo.git"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildCloneURL(tt.protocol, "ws", "repo", tt.cloneUser)
			if got != tt.wantURL {
				t.Errorf("buildCloneURL(%q, %q) = %q, want %q", tt.protocol, tt.cloneUser, got, tt.wantURL)
			}
		})
	}
}

func TestResolveProtocol(t *testing.T) {
	tests := []struct {
		name            string
		flagValue       string
		profileProtocol string
		want            string
		wantErr         bool
	}{
		{name: "flag set wins", flagValue: "https", profileProtocol: "ssh", want: "https"},
		{name: "profile wins when flag unset", flagValue: "", profileProtocol: "ssh", want: "ssh"},
		{name: "default when neither set", flagValue: "", profileProtocol: "", want: "git"},
		{name: "flag set wins even over an invalid profile value", flagValue: "https", profileProtocol: "bogus", want: "https"},
		{name: "invalid profile value is rejected", flagValue: "", profileProtocol: "http", wantErr: true},
		{name: "typo'd profile value is rejected", flagValue: "", profileProtocol: "gti", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveProtocol(tt.flagValue, tt.profileProtocol)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("resolveProtocol(%q, %q) expected an error, got nil", tt.flagValue, tt.profileProtocol)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveProtocol(%q, %q) unexpected error: %v", tt.flagValue, tt.profileProtocol, err)
			}
			if got != tt.want {
				t.Errorf("resolveProtocol(%q, %q) = %q, want %q", tt.flagValue, tt.profileProtocol, got, tt.want)
			}
		})
	}
}

func TestShellQuoteSingle(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain", in: "id_ed25519", want: "'id_ed25519'"},
		{name: "embedded single quote", in: "it's-a-key", want: `'it'\''s-a-key'`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shellQuoteSingle(tt.in); got != tt.want {
				t.Errorf("shellQuoteSingle(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestResolveSSHKeyFilename(t *testing.T) {
	tests := []struct {
		name       string
		flagValue  string
		profileKey string
		want       string
	}{
		{name: "flag set wins", flagValue: "/flag/key", profileKey: "/profile/key", want: "/flag/key"},
		{name: "profile used when flag unset", flagValue: "", profileKey: "/profile/key", want: "/profile/key"},
		{name: "empty when neither set", flagValue: "", profileKey: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveSSHKeyFilename(tt.flagValue, tt.profileKey); got != tt.want {
				t.Errorf("resolveSSHKeyFilename(%q, %q) = %q, want %q", tt.flagValue, tt.profileKey, got, tt.want)
			}
		})
	}
}

func TestRepositoryWorkspaceSlug(t *testing.T) {
	t.Run("from embedded workspace", func(t *testing.T) {
		ws := workspace.Workspace{Slug: "embedded-ws"}
		repo := &Repository{Slug: "repo", FullName: "ignored/repo", Workspace: &ws}
		got, err := repositoryWorkspaceSlug(repo)
		if err != nil {
			t.Fatalf("repositoryWorkspaceSlug() error = %v", err)
		}
		if got != ws.Slug {
			t.Errorf("repositoryWorkspaceSlug() = %q, want %q", got, ws.Slug)
		}
	})

	t.Run("from full name when workspace missing", func(t *testing.T) {
		repo := &Repository{Slug: "repo", FullName: "acme/repo"}
		got, err := repositoryWorkspaceSlug(repo)
		if err != nil {
			t.Fatalf("repositoryWorkspaceSlug() error = %v", err)
		}
		if got != "acme" {
			t.Errorf("repositoryWorkspaceSlug() = %q, want %q", got, "acme")
		}
	})

	t.Run("error when neither available", func(t *testing.T) {
		repo := &Repository{Slug: "repo"}
		if _, err := repositoryWorkspaceSlug(repo); err == nil {
			t.Error("repositoryWorkspaceSlug() expected an error, got nil")
		}
	})
}

// readLines reads path and splits it into lines, dropping a single trailing empty line produced
// by the shim's final "\n".
func readLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read %s: %v", path, err)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	return lines
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
