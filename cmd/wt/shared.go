package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/tito0224/wt/internal/git"
)

// Persistent flags shared across all commands, registered on the root command.
var (
	flagTerminal string
	flagDir      string
)

// repoDir returns the directory wt should operate in: the --dir/-C override if
// set, otherwise the current working directory.
func repoDir() string {
	if flagDir != "" {
		return flagDir
	}
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

// requireRepo returns the effective repo dir (see repoDir), erroring when it
// is not inside a git repository. Commands that operate on a repo call this
// first so they all fail with the same message.
func requireRepo() (string, error) {
	dir := repoDir()
	if !git.IsRepo(dir) {
		return "", fmt.Errorf("%s is not a git repository", dir)
	}
	return dir, nil
}

// loadWorktrees resolves the repo dir and returns its worktrees.
func loadWorktrees() (string, []git.Worktree, error) {
	dir, err := requireRepo()
	if err != nil {
		return "", nil, err
	}
	wts, err := git.ListWorktrees(dir)
	return dir, wts, err
}

// findWorktree matches a worktree by branch name or path substring.
func findWorktree(wts []git.Worktree, query string) (*git.Worktree, error) {
	var matches []*git.Worktree
	for i := range wts {
		w := &wts[i]
		if w.Branch == query || w.Path == query {
			return w, nil // exact match wins
		}
		if strings.Contains(w.Branch, query) || strings.Contains(w.Path, query) {
			matches = append(matches, w)
		}
	}
	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no worktree matching %q", query)
	case 1:
		return matches[0], nil
	default:
		names := make([]string, len(matches))
		for i, m := range matches {
			names[i] = m.Label()
		}
		return nil, fmt.Errorf("%q is ambiguous: %s", query, strings.Join(names, ", "))
	}
}

// matchRemoteBranch reports whether name exactly names a remote-tracking
// branch (e.g. "origin/foo") that has no local branch of the same short name.
// Used by `wt add` to do-what-I-mean: such a name asks for a local tracking
// branch, not a brand-new branch that happens to be called "origin/foo".
func matchRemoteBranch(repoRoot, name string) (git.Branch, bool) {
	branches, err := git.Branches(repoRoot)
	if err != nil {
		return git.Branch{}, false
	}
	local := make(map[string]bool)
	for _, b := range branches {
		if !b.IsRemote() {
			local[b.Name] = true
		}
	}
	for _, b := range branches {
		if b.IsRemote() && b.FullName() == name && !local[b.Name] {
			return b, true
		}
	}
	return git.Branch{}, false
}

// orNone renders an empty string as "(unset)" for display.
func orNone(s string) string {
	if s == "" {
		return "(unset)"
	}
	return s
}
