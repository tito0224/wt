// Package git wraps the small subset of git plumbing/porcelain commands
// needed to enumerate worktrees and describe their state.
package git

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// ErrBranchNotMerged is returned by DeleteBranch when a safe (`-d`) delete is
// refused because the branch has commits not merged into its upstream/HEAD.
// Callers can offer a force delete (`-D`) in response. Test with errors.Is.
var ErrBranchNotMerged = errors.New("git: branch not fully merged")

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
	// Ahead is the number of commits this branch is ahead of its upstream
	// (i.e. local commits not yet pushed to its remote tracking branch).
	Ahead int
	// Behind is the number of commits this branch is behind its upstream.
	Behind int
	// HasUpstream reports whether the branch tracks a remote branch.
	HasUpstream bool
	// AheadOfBase is the number of commits reachable from this branch's HEAD
	// but not from the repository's default branch (BaseBranch). This is a raw
	// commit-reachability count: because squash/rebase merges rewrite history,
	// a nonzero value does NOT by itself mean the work is unmerged — the merge
	// status is resolved with forge (PR) data at a higher layer.
	AheadOfBase int
	// BaseBranch is the default branch AheadOfBase was measured against, with
	// any remote prefix stripped (e.g. "master"). Empty when undetermined.
	BaseBranch string
	// BaseRef is the full ref AheadOfBase was measured against (e.g.
	// "origin/master"), suitable for passing back to git. Empty when
	// undetermined.
	BaseRef string
	// Current is true for the worktree the command was invoked from.
	Current bool
}

// run executes a git command in the given directory and returns trimmed stdout.
func run(dir string, args ...string) (string, error) {
	out, err := runRaw(dir, args...)
	return strings.TrimSpace(out), err
}

// runRaw is like run but does not trim surrounding whitespace, which matters
// for output whose leading columns are significant (e.g. `status --porcelain`,
// where a leading space is part of the XY status code).
func runRaw(dir string, args ...string) (string, error) {
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
	return string(out), nil
}

// IsRepo reports whether dir is inside a git working tree.
func IsRepo(dir string) bool {
	_, err := run(dir, "rev-parse", "--is-inside-work-tree")
	return err == nil
}

// Commit describes the most recent commit on a worktree's HEAD.
type Commit struct {
	ShortHash string // abbreviated SHA (e.g. "a1b2c3d")
	Author    string // author name
	RelDate   string // human relative date (e.g. "3 days ago")
	Subject   string // first line of the commit message
}

// Branch describes a branch that a worktree could be created for.
type Branch struct {
	// Name is the short branch name (e.g. "main", "feat/x").
	Name string
	// CheckedOutAt is the worktree path where this branch is currently checked
	// out, or "" when it is not checked out anywhere. A branch checked out
	// elsewhere cannot be added to a new worktree.
	CheckedOutAt string
	// Remote is the remote name for a remote-tracking branch (e.g. "origin"),
	// empty for a local branch.
	Remote string
}

// IsRemote reports whether the branch is a remote-tracking branch.
func (b Branch) IsRemote() bool { return b.Remote != "" }

// InUse reports whether the branch is already checked out in some worktree.
func (b Branch) InUse() bool { return b.CheckedOutAt != "" }

// FullName returns the ref-style display name: "origin/foo" for a
// remote-tracking branch, the plain branch name for a local one.
func (b Branch) FullName() string {
	if b.Remote != "" {
		return b.Remote + "/" + b.Name
	}
	return b.Name
}

// Branches lists the repository's local branches followed by its
// remote-tracking branches, each group most-recently-committed first. Local
// branches are annotated with the worktree they are checked out in, if any.
// Remote entries reflect the refs git already has locally — a `git fetch` is
// the user's job if they want fresher ones. The symbolic <remote>/HEAD entry
// is excluded (it duplicates the remote's default branch).
func Branches(dir string) ([]Branch, error) {
	// Full refnames (not :short) so local and remote refs are distinguishable
	// unambiguously even when a local branch is literally named "origin/x".
	out, err := run(dir, "for-each-ref",
		"--sort=-committerdate",
		"--format=%(refname)%00%(worktreepath)",
		"refs/heads", "refs/remotes")
	if err != nil {
		return nil, err
	}
	var locals, remotes []Branch
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		ref, wtPath, _ := strings.Cut(line, "\x00")
		switch {
		case strings.HasPrefix(ref, "refs/heads/"):
			locals = append(locals, Branch{
				Name:         strings.TrimPrefix(ref, "refs/heads/"),
				CheckedOutAt: wtPath,
			})
		case strings.HasPrefix(ref, "refs/remotes/"):
			remote, name, ok := strings.Cut(strings.TrimPrefix(ref, "refs/remotes/"), "/")
			if !ok || name == "HEAD" {
				continue
			}
			remotes = append(remotes, Branch{Name: name, Remote: remote})
		}
	}
	// Locals first: they are the more likely pick, and a stable partition
	// keeps each group in recency order.
	return append(locals, remotes...), nil
}

// commitFormat requests a commit's fields separated by NUL bytes (%x00) so any
// field may contain arbitrary text. Records are separated by a record
// separator (%x1e) when logging multiple commits.
const commitFormat = "%h%x00%an%x00%cr%x00%s"

// parseCommit parses a single NUL-separated commit record.
func parseCommit(rec string) (Commit, bool) {
	parts := strings.Split(rec, "\x00")
	if len(parts) < 4 {
		return Commit{}, false
	}
	return Commit{
		ShortHash: parts[0],
		Author:    parts[1],
		RelDate:   parts[2],
		Subject:   parts[3],
	}, true
}

// FileChange describes one changed path from `git status --porcelain -z`.
type FileChange struct {
	// Code is the two-character XY status (e.g. " M", "A ", "??"), where X is
	// the staged (index) state and Y the unstaged (worktree) state.
	Code string
	// Path is the affected path, exactly as git reported it (never C-quoted,
	// thanks to -z). For rename/copy entries it is rendered as
	// "original -> new" for display.
	Path string
}

// WorktreeStatus summarizes the working-tree state of dir.
type WorktreeStatus struct {
	Staged    int // paths with an index change (X is not ' ' or '?')
	Modified  int // paths with an unstaged change (Y is not ' ' or '?')
	Untracked int // paths not tracked ("??")
	Files     []FileChange
}

// Clean reports whether there are no changes at all.
func (s WorktreeStatus) Clean() bool { return len(s.Files) == 0 }

// Status parses `git status --porcelain -z` for dir into per-category counts
// and the list of changed files (in git's order).
//
// The -z (NUL-separated) form is used because it is unambiguous: paths are
// never C-quoted (without -z, git quotes paths containing spaces or non-ASCII
// characters unless core.quotepath=off), and rename/copy entries carry the
// original path as a separate NUL-terminated field instead of an in-line
// " -> " arrow that a path could legally contain.
func Status(dir string) (WorktreeStatus, error) {
	// Use runRaw: the leading space in a status code (e.g. " M") is significant
	// and must not be trimmed away from the first entry.
	out, err := runRaw(dir, "status", "--porcelain", "-z")
	if err != nil {
		return WorktreeStatus{}, err
	}
	var s WorktreeStatus
	fields := strings.Split(out, "\x00")
	for i := 0; i < len(fields); i++ {
		entry := fields[i]
		// Each entry is "XY <path>": two status bytes, a separator space, and
		// a path of at least one byte. The trailing NUL makes the final Split
		// field empty; skip it (and anything else too short to be valid).
		if len(entry) < 4 {
			continue
		}
		code := entry[:2]
		path := entry[3:]
		// Rename/copy entries (X or Y is R or C) are followed by ONE extra
		// NUL-terminated field holding the ORIGINAL path (-z emits the new
		// path first, then the original). Consume it here so it is never
		// misread as an independent entry, and render the pair as
		// "orig -> new" for display.
		if strings.ContainsAny(code, "RC") && i+1 < len(fields) {
			i++
			path = fields[i] + " -> " + path
		}
		s.Files = append(s.Files, FileChange{Code: code, Path: path})

		switch code {
		case "??":
			s.Untracked++
		default:
			if code[0] != ' ' {
				s.Staged++
			}
			if code[1] != ' ' {
				s.Modified++
			}
		}
	}
	return s, nil
}

// CommitsAfter counts commits reachable from HEAD but not from base whose
// commit time is strictly after the given RFC3339 timestamp. It is used to
// detect commits pushed to a branch after its PR merged (which git's plain
// commit-reachability count cannot distinguish, since squash/rebase merges
// rewrite SHAs). A zero base skips the base exclusion.
func CommitsAfter(dir, base, sinceRFC3339 string) (int, error) {
	if sinceRFC3339 == "" {
		return 0, nil
	}
	rangeSpec := "HEAD"
	if base != "" {
		rangeSpec = base + "..HEAD"
	}
	// --since filters by commit date; count the resulting commits.
	out, err := run(dir, "rev-list", "--count", "--since="+sinceRFC3339, rangeSpec)
	if err != nil {
		return 0, err
	}
	n, _ := strconv.Atoi(out)
	return n, nil
}

// RecentCommits returns up to n commits from HEAD in dir, most recent first.
func RecentCommits(dir string, n int) ([]Commit, error) {
	out, err := run(dir, "log", fmt.Sprintf("-%d", n), "--no-color",
		"--format="+commitFormat+"%x1e")
	if err != nil {
		return nil, err
	}
	var commits []Commit
	for _, rec := range strings.Split(out, "\x1e") {
		rec = strings.Trim(rec, "\n")
		if rec == "" {
			continue
		}
		if c, ok := parseCommit(rec); ok {
			commits = append(commits, c)
		}
	}
	return commits, nil
}

// TopLevel returns the absolute path to the root of the working tree
// containing dir.
func TopLevel(dir string) (string, error) {
	return run(dir, "rev-parse", "--show-toplevel")
}

// RemoteURL returns the configured URL for the named remote (e.g. "origin"),
// or an empty string when the remote is not configured.
func RemoteURL(dir, name string) string {
	out, err := run(dir, "remote", "get-url", name)
	if err != nil {
		return ""
	}
	return out
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

	// Commits ahead of the default branch (raw reachability count). This does
	// not distinguish squash/rebase-merged work; callers combine it with PR
	// state to decide whether the branch is actually merged.
	if base != "" && w.Branch != "" && w.Branch != stripRemote(base) {
		w.BaseBranch = stripRemote(base)
		w.BaseRef = base
		if out, err := run(w.Path, "rev-list", "--count", base+"..HEAD"); err == nil {
			w.AheadOfBase, _ = strconv.Atoi(out)
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

// DeleteBranch deletes a local branch. When force is false it uses `git branch
// -d` (which refuses to delete branches not merged into their upstream/HEAD);
// when true it uses `-D` (unconditional). The command runs in dir, which must
// be a worktree other than the branch's own (git refuses to delete a branch
// that is checked out anywhere).
func DeleteBranch(dir, branch string, force bool) error {
	flag := "-d"
	if force {
		flag = "-D"
	}
	_, err := run(dir, "branch", flag, branch)
	if err != nil && strings.Contains(err.Error(), "not fully merged") {
		// Wrap so callers can detect the refusal with errors.Is while still
		// seeing git's original message via %w.
		return fmt.Errorf("%w: %v", ErrBranchNotMerged, err)
	}
	return err
}

// Add creates a new worktree at path checked out to a newly created branch,
// based on the given ref. When base is empty, git uses the current HEAD. The
// parent directory of path is created if necessary.
func Add(dir, path, branch, base string) error {
	if err := ensureParent(path); err != nil {
		return err
	}
	args := []string{"worktree", "add", "-b", branch, path}
	if base != "" {
		args = append(args, base)
	}
	_, err := run(dir, args...)
	return err
}

// AddExisting creates a worktree at path checked out to an existing branch
// (no new branch is created). The parent directory of path is created if
// necessary.
func AddExisting(dir, path, branch string) error {
	if err := ensureParent(path); err != nil {
		return err
	}
	_, err := run(dir, "worktree", "add", path, branch)
	return err
}

// AddTracking creates a worktree at path on a newly created local branch that
// tracks the given remote-tracking ref (e.g. branch "feat/x" tracking
// "origin/feat/x"). The parent directory of path is created if necessary.
func AddTracking(dir, path, branch, remoteRef string) error {
	if err := ensureParent(path); err != nil {
		return err
	}
	_, err := run(dir, "worktree", "add", "--track", "-b", branch, path, remoteRef)
	return err
}

// ensureParent creates the parent directory of path if needed.
func ensureParent(path string) error {
	if parent := filepath.Dir(path); parent != "" {
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", parent, err)
		}
	}
	return nil
}
