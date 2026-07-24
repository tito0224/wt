package ops

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tito0224/wt/internal/git"
)

// ---------------------------------------------------------------------------
// Guard tests (pure — no git binary required).
// ---------------------------------------------------------------------------

func TestRemoveWorktreeRefusesBare(t *testing.T) {
	_, err := RemoveWorktree(t.TempDir(), git.Worktree{Bare: true}, RemoveOptions{})
	if !errors.Is(err, ErrBare) {
		t.Fatalf("err = %v, want ErrBare", err)
	}
}

func TestRemoveWorktreeRefusesCurrent(t *testing.T) {
	_, err := RemoveWorktree(t.TempDir(), git.Worktree{Current: true, Path: "/x"}, RemoveOptions{})
	if !errors.Is(err, ErrCurrent) {
		t.Fatalf("err = %v, want ErrCurrent", err)
	}
}

// ---------------------------------------------------------------------------
// Integration tests against real git repositories.
//
// These pin the policy this package exists to centralize: how config feeds
// worktree creation, and how removal outcomes are classified (deleted vs.
// left-unmerged vs. hard error).
// ---------------------------------------------------------------------------

// newTestRepo creates a hermetic git repository with a deterministic "main"
// default branch and one initial commit. Global/system git config and the
// user's global wt config are neutralized via environment so tests never pick
// up developer settings (AddWorktree calls config.Load, which reads
// $XDG_CONFIG_HOME/wt/config.toml).
func newTestRepo(t *testing.T) string {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	dir := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	mustGit(t, dir, "init", "-b", "main")
	mustGit(t, dir, "config", "user.name", "wt test")
	mustGit(t, dir, "config", "user.email", "wt@example.invalid")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, dir, "add", "README.md")
	mustGit(t, dir, "commit", "-m", "initial commit")
	return dir
}

// mustGit runs a git command in dir and fails the test on error.
func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// findWorktree returns the enumerated worktree on the given branch.
func findWorktree(t *testing.T, repo, branch string) git.Worktree {
	t.Helper()
	wts, err := git.ListWorktrees(repo)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range wts {
		if w.Branch == branch {
			return w
		}
	}
	t.Fatalf("no worktree on branch %q in %+v", branch, wts)
	return git.Worktree{}
}

func TestAddWorktreeUsesRepoConfig(t *testing.T) {
	repo := newTestRepo(t)
	// Repo-local config: prefixed branch names and a relative worktree dir.
	cfg := "[create]\nbranch_prefix = \"tst\"\nbranch_separator = \"-\"\ndir = \"wts\"\n"
	if err := os.WriteFile(filepath.Join(repo, ".wt.toml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	branch, path, err := AddWorktree(repo, "feat", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if branch != "tst-feat" {
		t.Errorf("branch = %q, want tst-feat (prefix+separator applied)", branch)
	}
	if want := filepath.Join(repo, "wts", "feat"); path != want {
		t.Errorf("path = %q, want %q (relative dir resolved against repo root)", path, want)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("worktree dir not created: %v", err)
	}
	// The worktree must actually be checked out on the new branch.
	if got := findWorktree(t, repo, "tst-feat").Path; got == "" {
		t.Error("new worktree missing from git worktree list")
	}
}

func TestAddWorktreeBaseOverride(t *testing.T) {
	repo := newTestRepo(t)
	// A base branch one commit ahead of main.
	mustGit(t, repo, "checkout", "-b", "base1")
	if err := os.WriteFile(filepath.Join(repo, "extra.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, repo, "add", "extra.txt")
	mustGit(t, repo, "commit", "-m", "extra")
	baseHead := mustGit(t, repo, "rev-parse", "base1")
	mustGit(t, repo, "checkout", "main")

	_, path, err := AddWorktree(repo, "from-base", "base1", "")
	if err != nil {
		t.Fatal(err)
	}
	if head := mustGit(t, path, "rev-parse", "HEAD"); head != baseHead {
		t.Errorf("new worktree HEAD = %s, want base1 head %s", head, baseHead)
	}
}

func TestAddExistingBranch(t *testing.T) {
	repo := newTestRepo(t)
	mustGit(t, repo, "branch", "existing")

	path, err := AddExistingBranch(repo, "existing")
	if err != nil {
		t.Fatal(err)
	}
	// Directory is derived from the branch name (no prefix applied).
	if got := filepath.Base(path); got != "existing" {
		t.Errorf("path base = %q, want existing", got)
	}
	if got := mustGit(t, path, "rev-parse", "--abbrev-ref", "HEAD"); got != "existing" {
		t.Errorf("checked-out branch = %q, want existing", got)
	}
}

// TestAddExistingBranchFlattensSlashes pins the directory-normalization rule:
// a slashed branch gets a single flat directory (feat/x -> feat-x), not a
// nested one, while the checked-out branch keeps its real name.
func TestAddExistingBranchFlattensSlashes(t *testing.T) {
	repo := newTestRepo(t)
	mustGit(t, repo, "branch", "feat/x")

	path, err := AddExistingBranch(repo, "feat/x")
	if err != nil {
		t.Fatal(err)
	}
	if got := filepath.Base(path); got != "feat-x" {
		t.Errorf("path base = %q, want feat-x (slash flattened)", got)
	}
	if got := mustGit(t, path, "rev-parse", "--abbrev-ref", "HEAD"); got != "feat/x" {
		t.Errorf("checked-out branch = %q, want feat/x (branch name untouched)", got)
	}
}

// TestAddWorktreeRejectsDegenerateNames ensures traversal-y and empty names
// fail up front, before any branch or directory is created.
func TestAddWorktreeRejectsDegenerateNames(t *testing.T) {
	repo := newTestRepo(t)
	for _, name := range []string{"", ".", ".."} {
		if _, _, err := AddWorktree(repo, name, "", ""); err == nil {
			t.Errorf("AddWorktree(%q) succeeded, want error", name)
		}
	}
	// Nothing may have been created next to the repo.
	parent := filepath.Join(filepath.Dir(repo), "repo-worktrees")
	if _, err := os.Stat(parent); !os.IsNotExist(err) {
		t.Errorf("worktree parent dir %s was created for a rejected name", parent)
	}
}

// TestAddWorktreeFlattensSlashedName: a name containing '/' still yields a
// slashed branch (prefix + name is git's namespace) but a flat directory.
func TestAddWorktreeFlattensSlashedName(t *testing.T) {
	repo := newTestRepo(t)
	branch, path, err := AddWorktree(repo, "team/feat", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if branch != "team/feat" { // no prefix configured in the hermetic repo
		t.Errorf("branch = %q, want team/feat", branch)
	}
	if got := filepath.Base(path); got != "team-feat" {
		t.Errorf("path base = %q, want team-feat (flattened)", got)
	}
	if got := mustGit(t, path, "rev-parse", "--abbrev-ref", "HEAD"); got != "team/feat" {
		t.Errorf("checked-out branch = %q, want team/feat", got)
	}
}

func TestRemoveWorktreeDeletesMergedBranch(t *testing.T) {
	repo := newTestRepo(t)
	if _, _, err := AddWorktree(repo, "merged", "", ""); err != nil {
		t.Fatal(err)
	}
	wt := findWorktree(t, repo, "merged")

	res, err := RemoveWorktree(repo, wt, RemoveOptions{DeleteBranch: true})
	if err != nil {
		t.Fatal(err)
	}
	if !res.BranchDeleted || res.BranchUnmerged {
		t.Errorf("result = %+v, want BranchDeleted=true BranchUnmerged=false", res)
	}
	if out := mustGit(t, repo, "branch", "--list", "merged"); out != "" {
		t.Errorf("branch still exists: %q", out)
	}
}

func TestRemoveWorktreeReportsUnmergedBranch(t *testing.T) {
	repo := newTestRepo(t)
	if _, path, err := AddWorktree(repo, "wip", "", ""); err != nil {
		t.Fatal(err)
	} else {
		// A commit on the branch that main doesn't have -> safe delete refused.
		if err := os.WriteFile(filepath.Join(path, "wip.txt"), []byte("w\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		mustGit(t, path, "add", "wip.txt")
		mustGit(t, path, "commit", "-m", "wip")
	}
	wt := findWorktree(t, repo, "wip")

	res, err := RemoveWorktree(repo, wt, RemoveOptions{DeleteBranch: true})
	if err != nil {
		t.Fatalf("unmerged refusal must not be a hard error, got %v", err)
	}
	if res.BranchDeleted || !res.BranchUnmerged {
		t.Errorf("result = %+v, want BranchDeleted=false BranchUnmerged=true", res)
	}
	// The worktree is gone but the branch survives — that's the contract the
	// TUI's force-delete follow-up prompt depends on.
	if _, err := os.Stat(wt.Path); !os.IsNotExist(err) {
		t.Errorf("worktree dir still exists at %s", wt.Path)
	}
	if out := mustGit(t, repo, "branch", "--list", "wip"); out == "" {
		t.Error("unmerged branch was deleted by a safe delete")
	}

	// The follow-up force delete then removes it.
	if err := DeleteBranch(repo, res.Branch, true); err != nil {
		t.Fatalf("force DeleteBranch: %v", err)
	}
	if out := mustGit(t, repo, "branch", "--list", "wip"); out != "" {
		t.Errorf("branch still exists after force delete: %q", out)
	}
}

func TestRemoveWorktreeForceBranch(t *testing.T) {
	repo := newTestRepo(t)
	if _, path, err := AddWorktree(repo, "wip2", "", ""); err != nil {
		t.Fatal(err)
	} else {
		if err := os.WriteFile(filepath.Join(path, "w.txt"), []byte("w\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		mustGit(t, path, "add", "w.txt")
		mustGit(t, path, "commit", "-m", "wip2")
	}
	wt := findWorktree(t, repo, "wip2")

	res, err := RemoveWorktree(repo, wt, RemoveOptions{ForceBranch: true})
	if err != nil {
		t.Fatal(err)
	}
	if !res.BranchDeleted {
		t.Errorf("result = %+v, want BranchDeleted=true (forced)", res)
	}
}

func TestRemoveWorktreeDirtyNeedsForce(t *testing.T) {
	repo := newTestRepo(t)
	_, path, err := AddWorktree(repo, "dirty", "", "")
	if err != nil {
		t.Fatal(err)
	}
	// An untracked file makes git refuse a plain `worktree remove`.
	if err := os.WriteFile(filepath.Join(path, "junk.txt"), []byte("j\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wt := findWorktree(t, repo, "dirty")

	if _, err := RemoveWorktree(repo, wt, RemoveOptions{}); err == nil {
		t.Fatal("removing a dirty worktree without Force succeeded, want error")
	}
	if _, err := RemoveWorktree(repo, wt, RemoveOptions{Force: true}); err != nil {
		t.Fatalf("forced removal failed: %v", err)
	}
	if _, err := os.Stat(wt.Path); !os.IsNotExist(err) {
		t.Errorf("worktree dir still exists at %s", wt.Path)
	}
}
