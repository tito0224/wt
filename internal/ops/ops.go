// Package ops implements the worktree operations (add, remove, branch delete)
// shared by both the CLI and the interactive TUI. It encodes the *policy* —
// guards, the sequence of git calls, and how outcomes are classified — as pure,
// synchronous functions returning typed results and errors.
//
// Each frontend keeps its own I/O style around these: the CLI calls them
// directly and prints; the TUI calls them inside tea.Cmd goroutines and maps
// the results to messages. Keeping the rules here prevents the two from
// drifting (e.g. applying different removal guards).
package ops

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/tito0224/wt/internal/config"
	"github.com/tito0224/wt/internal/git"
)

// HookFailedError reports that a worktree was created successfully but one of
// its post-create hook commands (config `create.run`) failed. The worktree is
// intentionally kept — the user's setup may be half-done, but discarding a
// freshly created worktree over a hook failure would be worse. Frontends test
// for it with errors.As and present "created, but…" rather than a hard
// failure.
type HookFailedError struct {
	// Command is the hook command line that failed.
	Command string
	// Err is the underlying execution error (typically an exit status).
	Err error
}

func (e *HookFailedError) Error() string {
	return fmt.Sprintf("post-create hook %q: %v", e.Command, e.Err)
}

func (e *HookFailedError) Unwrap() error { return e.Err }

// runCreateHooks executes the configured post-create commands inside the new
// worktree, in order, stopping at the first failure. Commands run via
// `sh -c` so shell syntax (redirection, &&, env expansion) works as users
// expect from hook-style config. Combined output is streamed to out (the CLI
// passes stderr for live feedback; the TUI captures into a buffer). Hooks see
// the worktree context via WT_WORKTREE, WT_BRANCH, and WT_REPO_ROOT.
func runCreateHooks(repoRoot, path, branch string, cmds []string, out io.Writer) error {
	if out == nil {
		out = io.Discard
	}
	for _, line := range cmds {
		fmt.Fprintf(out, "wt: run: %s\n", line)
		cmd := exec.Command("sh", "-c", line)
		cmd.Dir = path
		cmd.Env = append(os.Environ(),
			"WT_WORKTREE="+path,
			"WT_BRANCH="+branch,
			"WT_REPO_ROOT="+repoRoot,
		)
		cmd.Stdout = out
		cmd.Stderr = out
		if err := cmd.Run(); err != nil {
			return &HookFailedError{Command: line, Err: err}
		}
	}
	return nil
}

// AddWorktree creates a new worktree named `name` in the repository containing
// repoRoot, using config defaults for the base branch, target directory, and
// branch naming. Non-empty base/dir override the corresponding config values.
// It returns the resolved branch name and absolute worktree path.
//
// The worktree's directory is a single flat component derived from name (see
// config.DirName); degenerate names are rejected here, before any directory
// or branch is created.
//
// After creation, the configured post-create hooks (config `create.run`) are
// executed in the worktree with output streamed to hookOut (nil discards).
// When a hook fails, the returned branch and path are still valid — the
// worktree exists — and err is a *HookFailedError.
func AddWorktree(repoRoot, name, base, dir string, hookOut io.Writer) (branch, path string, err error) {
	cfg, err := config.Load(repoRoot)
	if err != nil {
		return "", "", err
	}
	// Validate the *bare* name — what branch/dir derivation actually uses.
	// This also catches a name that is nothing but the prefix+separator
	// ("jl/"), which strips to an empty bare name.
	if _, err := config.DirName(cfg.Create.BareName(name)); err != nil {
		return "", "", err
	}
	if base != "" {
		cfg.Create.BaseBranch = base
	}
	if dir != "" {
		cfg.Create.Dir = dir
	}
	if cfg.Create.BaseBranch == "" {
		cfg.Create.BaseBranch = git.DefaultBranch(repoRoot)
	}

	branch = cfg.Create.BranchName(name)
	path = cfg.Create.WorktreeDir(repoRoot, name)
	if err := git.Add(repoRoot, path, branch, cfg.Create.BaseBranch); err != nil {
		return "", "", err
	}
	return branch, path, runCreateHooks(repoRoot, path, branch, cfg.Create.Run, hookOut)
}

// AddExistingBranch creates a new worktree checked out to an existing branch
// in the repository containing repoRoot. The worktree directory is a single
// flat component derived from the branch name (slashes become '-'; see
// config.DirName) under the configured dir template — no branch prefix is
// applied, since the branch already exists. It returns the absolute worktree
// path.
//
// Post-create hooks run exactly as for AddWorktree: on hook failure the
// returned path is still valid and err is a *HookFailedError.
func AddExistingBranch(repoRoot, branch string, hookOut io.Writer) (path string, err error) {
	cfg, err := config.Load(repoRoot)
	if err != nil {
		return "", err
	}
	// Same bare-name validation as AddWorktree, for symmetry. (Existing git
	// branch names can't be degenerate, but the guard is cheap.)
	if _, err := config.DirName(cfg.Create.BareName(branch)); err != nil {
		return "", err
	}
	path = cfg.Create.WorktreeDir(repoRoot, branch)
	if err := git.AddExisting(repoRoot, path, branch); err != nil {
		return "", err
	}
	return path, runCreateHooks(repoRoot, path, branch, cfg.Create.Run, hookOut)
}

// AddRemoteBranch creates a worktree for a remote-tracking branch: a new
// local branch named after it (e.g. "feat/x" for origin's "feat/x"), set up
// to track the remote branch, checked out into a directory derived exactly as
// for the other add flows (bare name, prefix stripped, flattened). git itself
// refuses when a local branch of that name already exists — callers normally
// only offer remote branches without a local counterpart.
//
// Post-create hooks run exactly as for AddWorktree: on hook failure the
// returned path is still valid and err is a *HookFailedError.
func AddRemoteBranch(repoRoot, remote, branch string, hookOut io.Writer) (path string, err error) {
	cfg, err := config.Load(repoRoot)
	if err != nil {
		return "", err
	}
	if _, err := config.DirName(cfg.Create.BareName(branch)); err != nil {
		return "", err
	}
	path = cfg.Create.WorktreeDir(repoRoot, branch)
	if err := git.AddTracking(repoRoot, path, branch, remote+"/"+branch); err != nil {
		return "", err
	}
	return path, runCreateHooks(repoRoot, path, branch, cfg.Create.Run, hookOut)
}

// RemoveOptions controls a worktree removal.
type RemoveOptions struct {
	// Force discards uncommitted changes when removing the worktree.
	Force bool
	// DeleteBranch also deletes the worktree's branch after removal (safe: a
	// `-d` delete that refuses unmerged branches).
	DeleteBranch bool
	// ForceBranch, with DeleteBranch, force-deletes the branch even if unmerged.
	ForceBranch bool
}

// RemoveResult describes the outcome of a Remove call.
type RemoveResult struct {
	// Branch is the branch the removed worktree was on (empty when detached).
	Branch string
	// BranchDeleted is true when a branch deletion was requested and succeeded.
	BranchDeleted bool
	// BranchUnmerged is true when a safe branch delete was requested but git
	// refused because the branch is unmerged. The worktree was still removed;
	// the caller may offer a force delete (see ForceDeleteBranch).
	BranchUnmerged bool
}

// Errors returned by Remove for cases the frontends present specially.
var (
	// ErrBare is returned when asked to remove the bare repository entry.
	ErrBare = errors.New("refusing to remove the bare repository")
	// ErrCurrent is returned when asked to remove the worktree the process is
	// currently in.
	ErrCurrent = errors.New("refusing to remove the worktree you're currently in")
)

// RemoveWorktree removes the given worktree from the repository containing
// repoRoot, applying the standard guards, then optionally deletes its branch. A
// worktree that removed cleanly but whose (safe) branch delete was refused as
// unmerged is reported via RemoveResult.BranchUnmerged with a nil error — the
// removal itself succeeded.
func RemoveWorktree(repoRoot string, wt git.Worktree, opts RemoveOptions) (RemoveResult, error) {
	res := RemoveResult{Branch: wt.Branch}

	if wt.Bare {
		return res, ErrBare
	}
	if wt.Current {
		return res, ErrCurrent
	}

	if err := git.Remove(repoRoot, wt.Path, opts.Force); err != nil {
		return res, err
	}

	if (opts.DeleteBranch || opts.ForceBranch) && wt.Branch != "" {
		err := git.DeleteBranch(repoRoot, wt.Branch, opts.ForceBranch)
		switch {
		case err == nil:
			res.BranchDeleted = true
		case errors.Is(err, git.ErrBranchNotMerged) && !opts.ForceBranch:
			// Worktree removed; branch left in place, unmerged. Not a hard error.
			res.BranchUnmerged = true
		case err != nil:
			return res, fmt.Errorf("worktree removed, but branch %q not deleted: %w", wt.Branch, err)
		}
	}
	return res, nil
}

// DeleteBranch deletes a branch in the repository containing repoRoot. It is
// used for the force follow-up after a Remove reported BranchUnmerged.
func DeleteBranch(repoRoot, branch string, force bool) error {
	return git.DeleteBranch(repoRoot, branch, force)
}
