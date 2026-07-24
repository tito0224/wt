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

	"github.com/johnlotito/wt/internal/config"
	"github.com/johnlotito/wt/internal/git"
)

// AddWorktree creates a new worktree named `name` in the repository containing
// repoRoot, using config defaults for the base branch, target directory, and
// branch naming. Non-empty base/dir override the corresponding config values.
// It returns the resolved branch name and absolute worktree path.
func AddWorktree(repoRoot, name, base, dir string) (branch, path string, err error) {
	cfg, err := config.Load(repoRoot)
	if err != nil {
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
	return branch, path, nil
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
