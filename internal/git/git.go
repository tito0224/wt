// Package git wraps the small subset of git plumbing/porcelain commands
// needed to enumerate worktrees and describe their state.
package git

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// Worktree describes a single git worktree and its computed status.
type Worktree struct {
	// Path is the absolute path to the worktree on disk.
	Path string
	// Branch is the short branch name (e.g. "main"). Empty when detached.
	Branch string
	// Head is the commit SHA currently checked out.
	Head string
	// Detached is true when the worktree is not on a branch.
	Detached bool
	// Bare is true for the bare repository entry (no working files).
	Bare bool
	// Locked is true when the worktree has been locked via `git worktree lock`.
	Locked bool
	// Prunable is true when git considers the worktree removable.
	Prunable bool

	// Dirty is true when there are uncommitted changes (staged, unstaged,
	// or untracked files) in the working tree.
	Dirty bool
	// Ahead is the number of commits this branch is ahead of its upstream.
	Ahead int
	// Behind is the number of commits this branch is behind its upstream.
	Behind int
	// HasUpstream reports whether the branch tracks a remote branch.
	HasUpstream bool
	// Unmerged is true when the branch has commits not merged into the
	// repository's main/default branch.
	Unmerged bool
	// Current is true for the worktree the command was invoked from.
	Current bool
}

// run executes a git command in the given directory and returns trimmed stdout.
func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(ee.Stderr)))
		}
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// IsRepo reports whether dir is inside a git working tree.
func IsRepo(dir string) bool {
	_, err := run(dir, "rev-parse", "--is-inside-work-tree")
	return err == nil
}

// TopLevel returns the absolute path to the root of the working tree
// containing dir.
func TopLevel(dir string) (string, error) {
	return run(dir, "rev-parse", "--show-toplevel")
}

// DefaultBranch returns the repository's primary branch, preferring the remote
// HEAD (e.g. "origin/main"), then the local "main"/"master". It returns an
// empty string when none can be determined.
func DefaultBranch(dir string) string {
	return defaultBranch(dir)
}

// defaultBranch attempts to determine the repository's primary branch,
// preferring the remote HEAD, then common names.
func defaultBranch(dir string) string {
	if out, err := run(dir, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD"); err == nil && out != "" {
		return out // e.g. "origin/main"
	}
	for _, cand := range []string{"main", "master"} {
		if _, err := run(dir, "rev-parse", "--verify", "--quiet", "refs/heads/"+cand); err == nil {
			return cand
		}
	}
	return ""
}

// ListWorktrees returns all worktrees for the repository containing dir, with
// their status fields populated.
func ListWorktrees(dir string) ([]Worktree, error) {
	out, err := run(dir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	wts := parsePorcelain(out)

	base := defaultBranch(dir)
	markCurrent(wts, dir)

	// Enrich worktrees concurrently: each enrich shells out to git several
	// times against an independent working directory, so they parallelize
	// cleanly. A bounded worker pool avoids spawning an unbounded number of
	// git processes at once.
	sem := make(chan struct{}, enrichConcurrency())
	var wg sync.WaitGroup
	for i := range wts {
		if wts[i].Bare {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(w *Worktree) {
			defer wg.Done()
			defer func() { <-sem }()
			enrich(w, base)
		}(&wts[i])
	}
	wg.Wait()
	return wts, nil
}

// markCurrent flags the worktree that dir resides in. It resolves the working
// tree's top-level directory so that invoking wt from any subdirectory still
// matches the right worktree.
func markCurrent(wts []Worktree, dir string) {
	top, err := run(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return
	}
	top = canonical(top)
	for i := range wts {
		if canonical(wts[i].Path) == top {
			wts[i].Current = true
			return
		}
	}
}

// canonical normalizes a path for comparison, resolving symlinks where
// possible (macOS /tmp vs /private/tmp, etc.).
func canonical(p string) string {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved
	}
	return filepath.Clean(p)
}

// enrichConcurrency caps how many git processes run at once. It scales with
// CPU count but stays bounded so we don't thrash on huge worktree sets.
func enrichConcurrency() int {
	n := runtime.NumCPU() * 2
	if n < 4 {
		n = 4
	}
	if n > 16 {
		n = 16
	}
	return n
}

// parsePorcelain parses the output of `git worktree list --porcelain`.
func parsePorcelain(out string) []Worktree {
	var wts []Worktree
	var cur *Worktree

	flush := func() {
		if cur != nil {
			wts = append(wts, *cur)
			cur = nil
		}
	}

	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			flush()
			continue
		}
		key, val, _ := strings.Cut(line, " ")
		switch key {
		case "worktree":
			flush()
			cur = &Worktree{Path: val}
		case "HEAD":
			if cur != nil {
				cur.Head = val
			}
		case "branch":
			if cur != nil {
				cur.Branch = strings.TrimPrefix(val, "refs/heads/")
			}
		case "detached":
			if cur != nil {
				cur.Detached = true
			}
		case "bare":
			if cur != nil {
				cur.Bare = true
			}
		case "locked":
			if cur != nil {
				cur.Locked = true
			}
		case "prunable":
			if cur != nil {
				cur.Prunable = true
			}
		}
	}
	flush()
	return wts
}

// enrich populates the dynamic status fields (dirty, ahead/behind, unmerged)
// for a single worktree by inspecting its working directory.
func enrich(w *Worktree, base string) {
	// Dirty: any porcelain status output means there are changes.
	if out, err := run(w.Path, "status", "--porcelain"); err == nil {
		w.Dirty = out != ""
	}

	// Ahead/behind vs upstream.
	if w.Branch != "" {
		if counts, err := run(w.Path, "rev-list", "--left-right", "--count", "@{upstream}...HEAD"); err == nil {
			fields := strings.Fields(counts)
			if len(fields) == 2 {
				w.HasUpstream = true
				w.Behind, _ = strconv.Atoi(fields[0])
				w.Ahead, _ = strconv.Atoi(fields[1])
			}
		}
	}

	// Unmerged: does this branch have commits not in the default branch?
	if base != "" && w.Branch != "" && w.Branch != stripRemote(base) {
		if out, err := run(w.Path, "rev-list", "--count", base+"..HEAD"); err == nil {
			if n, _ := strconv.Atoi(out); n > 0 {
				w.Unmerged = true
			}
		}
	}
}

// stripRemote turns "origin/main" into "main".
func stripRemote(ref string) string {
	if i := strings.IndexByte(ref, '/'); i >= 0 {
		return ref[i+1:]
	}
	return ref
}

// Remove removes a worktree. When force is true, uncommitted changes are
// discarded.
func Remove(dir, path string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, path)
	_, err := run(dir, args...)
	return err
}

// Add creates a new worktree at path checked out to a newly created branch,
// based on the given ref. When base is empty, git uses the current HEAD. The
// parent directory of path is created if necessary.
func Add(dir, path, branch, base string) error {
	if parent := filepath.Dir(path); parent != "" {
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", parent, err)
		}
	}
	args := []string{"worktree", "add", "-b", branch, path}
	if base != "" {
		args = append(args, base)
	}
	_, err := run(dir, args...)
	return err
}

// Prune removes worktree administrative files for missing worktrees.
func Prune(dir string) error {
	_, err := run(dir, "worktree", "prune")
	return err
}
