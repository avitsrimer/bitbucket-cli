package common

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/go-pkgz/lgr"
	"gopkg.in/ini.v1"
)

// OpenGitConfig opens the .git/config file in the current folder or one of its parents
func OpenGitConfig(_ context.Context) (io.ReadCloser, error) {
	folder, err := filepath.Abs(".")
	if err != nil {
		folder = "."
	}
	last := folder + "dummy"

	for {
		if folder == last {
			return nil, fmt.Errorf("git config file not found starting from %s", folder)
		}

		// If .git is a file (e.g. worktree), read the actual git dir from there (field gitdir):
		// resolvedDir is then already the git directory itself (its "config" lives directly under
		// it), unlike the ordinary case where folder is the repository root and "config" lives
		// under folder/.git -- joining ".git/config" onto resolvedDir unconditionally in both
		// cases would double the ".git" segment for a worktree and silently fail to find it,
		// falling back to a parent repository's own config instead.
		resolvedDir := resolveWorktreeGitDir(folder, filepath.Join(folder, ".git"))
		filename := filepath.Join(resolvedDir, ".git", "config")
		if resolvedDir != folder {
			filename = filepath.Join(resolvedDir, "config")
		}
		lgr.Printf("[DEBUG] opening %s", filename)
		file, err := os.Open(filename) //nolint:gosec // filename is built by walking up from the process's own working directory, not from external input
		if err == nil {
			return file, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("runtime error: %w", err)
		}
		if folder == "/" {
			return nil, errors.New("not a git repository")
		}
		last = folder
		folder = filepath.Dir(folder)
	}
}

// resolveWorktreeGitDir returns the real git directory for folder: when gitPath is a plain file
// (git worktrees do this) it contains a "gitdir: <path>" line pointing at the actual git directory;
// otherwise folder is returned unchanged
func resolveWorktreeGitDir(folder, gitPath string) string {
	info, err := os.Stat(gitPath)
	if err != nil || info.IsDir() {
		return folder
	}
	lgr.Printf("[DEBUG] .git is a file, reading gitdir from there")
	content, err := os.ReadFile(gitPath) //nolint:gosec // gitPath is derived from the process's own working directory, not from external input
	if err != nil {
		return folder
	}
	const prefix = "gitdir: "
	for line := range strings.SplitSeq(string(content), "\n") {
		if len(line) > len(prefix) && line[:len(prefix)] == prefix {
			gitDir := line[len(prefix):]
			lgr.Printf("[DEBUG] found gitdir: %s", gitDir)
			if !filepath.IsAbs(gitDir) {
				return filepath.Join(folder, gitDir)
			}
			return gitDir
		}
	}
	return folder
}

// GetGitSection returns the INI section from the git config file
func GetGitSection(ctx context.Context, reader io.Reader, name string) (section *ini.Section, err error) {
	data, err := getINIContent(ctx, reader)
	if err != nil {
		return nil, err
	}
	// ini.File.Section fabricates and returns an empty section when name doesn't exist (it never
	// returns nil), so a nil check against it is always false, dead code; GetSection is the
	// method that actually reports a missing section as an error.
	section, err = data.GetSection(name)
	if err != nil {
		return nil, fmt.Errorf("git config section %s not found", name)
	}
	return section, nil
}

// GetGitSectionsMatching returns the INI sections from the git config file matching the given regex
func GetGitSectionsMatching(ctx context.Context, reader io.Reader, rex *regexp.Regexp) (sections []*ini.Section, err error) {
	data, err := getINIContent(ctx, reader)
	if err != nil {
		return nil, err
	}
	for _, section := range data.Sections() {
		if rex.MatchString(section.Name()) {
			sections = append(sections, section)
		}
	}
	return
}

// getINIContent reads the INI content from a reader and returns it as an ini.File
func getINIContent(_ context.Context, reader io.Reader) (data *ini.File, err error) {
	payload, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("cannot read git config: %w", err)
	}
	data, err = ini.Load(payload)
	if err != nil {
		return nil, fmt.Errorf("cannot parse git config: %w", err)
	}
	return data, nil
}
