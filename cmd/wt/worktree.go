package main

import (
	"errors"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/tito0224/wt/internal/git"
	"github.com/tito0224/wt/internal/ops"
	"github.com/tito0224/wt/internal/terminal"
)

func newListCmd() *cobra.Command {
	var porcelain bool
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List worktrees and their status",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, wts, err := loadWorktrees()
			if err != nil {
				return err
			}
			if porcelain {
				for _, w := range wts {
					fmt.Printf("%s\t%s\t%s\n", w.Path, w.Label(), w.StatusString())
				}
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
			fmt.Fprintln(w, "BRANCH\tSTATUS\tPATH")
			for _, wt := range wts {
				fmt.Fprintf(w, "%s\t%s\t%s\n", wt.DisplayLabel(), wt.StatusString(), wt.Path)
			}
			return w.Flush()
		},
	}
	cmd.Flags().BoolVar(&porcelain, "porcelain", false, "machine-readable tab-separated output")
	return cmd
}

func newOpenCmd() *cobra.Command {
	var tab bool
	cmd := &cobra.Command{
		Use:   "open <branch-or-path>",
		Short: "Select a worktree (prints its path by default, for cd)",
		Long: "Resolve the worktree matching the query.\n\n" +
			"By default it prints the worktree's path so the shell wrapper can " +
			"cd the current shell into it. With --tab it opens a new terminal " +
			"tab in the worktree instead.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, wts, err := loadWorktrees()
			if err != nil {
				return err
			}
			wt, err := findWorktree(wts, args[0])
			if err != nil {
				return err
			}
			if tab {
				adapter, err := terminal.Resolve(flagTerminal)
				if err != nil {
					return err
				}
				return adapter.OpenTab(wt.Path)
			}
			fmt.Println(wt.Path)
			return nil
		},
	}
	cmd.Flags().BoolVar(&tab, "tab", false, "open a new terminal tab instead of printing the path")
	return cmd
}

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status [branch-or-path]",
		Short: "Show detailed status for one or all worktrees",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, wts, err := loadWorktrees()
			if err != nil {
				return err
			}
			var show []git.Worktree
			if len(args) == 1 {
				wt, err := findWorktree(wts, args[0])
				if err != nil {
					return err
				}
				show = []git.Worktree{*wt}
			} else {
				show = wts
			}
			for _, w := range show {
				fmt.Printf("%s\n", w.Label())
				fmt.Printf("  path:      %s\n", w.Path)
				if !w.Bare {
					fmt.Printf("  dirty:     %t\n", w.Dirty)
					if w.HasUpstream {
						fmt.Printf("  unpushed:  %d\n", w.Ahead)
						fmt.Printf("  behind:    %d\n", w.Behind)
					} else {
						fmt.Printf("  upstream:  (not pushed)\n")
					}
					if w.BaseBranch != "" {
						fmt.Printf("  ahead of %s: %d\n", w.BaseBranch, w.AheadOfBase)
					}
				}
				fmt.Println()
			}
			return nil
		},
	}
}

func newAddCmd() *cobra.Command {
	var (
		base string
		dir  string
		tab  bool
	)
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Add a new worktree",
		Long: "Add a new worktree and branch for <name>.\n\n" +
			"Defaults for the base branch, target directory, and branch naming " +
			"come from the wt config file (see `wt config path`), and can be " +
			"overridden per-invocation with flags.\n\n" +
			"When <name> matches a remote-tracking branch with no local " +
			"counterpart (e.g. `wt add origin/foo`), a local branch tracking " +
			"it is created instead of a new branch. Run `git fetch` first if " +
			"the branch is not known locally yet.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			repo, err := requireRepo()
			if err != nil {
				return err
			}
			top, err := git.TopLevel(repo)
			if err != nil {
				return err
			}

			// Hook output streams to stderr: with the shell wrapper, stdout is
			// captured by $(...) until wt exits, so stderr is the only way to
			// show hook progress live.
			var branch, path string
			// Remote DWIM only without --base: an explicit base means "create
			// a new branch from <base>", which a tracking add can't honor.
			if rb, ok := matchRemoteBranch(top, name); ok && base == "" {
				branch = rb.Name
				path, err = ops.AddRemoteBranch(top, rb.Remote, rb.Name, os.Stderr)
			} else {
				branch, path, err = ops.AddWorktree(top, name, base, dir, os.Stderr)
			}
			var hookErr *ops.HookFailedError
			if errors.As(err, &hookErr) {
				// The worktree exists; report the partial success, then the
				// hook failure as the command's error (non-zero exit).
				fmt.Printf("created worktree %s at %s\n", branch, path)
				return err
			}
			if err != nil {
				return err
			}
			fmt.Printf("created worktree %s at %s\n", branch, path)

			if tab {
				adapter, err := terminal.Resolve(flagTerminal)
				if err != nil {
					return err
				}
				return adapter.OpenTab(path)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&base, "base", "", "base branch/ref to create from (overrides config)")
	cmd.Flags().StringVar(&dir, "into", "", "directory to place the worktree in (overrides config)")
	cmd.Flags().BoolVar(&tab, "tab", false, "open a new terminal tab in the new worktree")
	return cmd
}

func newRemoveCmd() *cobra.Command {
	var (
		force        bool
		deleteBranch bool
		forceBranch  bool
	)
	cmd := &cobra.Command{
		Use:     "remove <branch-or-path>",
		Aliases: []string{"rm"},
		Short:   "Remove a worktree",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, wts, err := loadWorktrees()
			if err != nil {
				return err
			}
			wt, err := findWorktree(wts, args[0])
			if err != nil {
				return err
			}

			res, err := ops.RemoveWorktree(dir, *wt, ops.RemoveOptions{
				Force:        force,
				DeleteBranch: deleteBranch,
				ForceBranch:  forceBranch,
			})
			if err != nil {
				return err
			}
			fmt.Printf("removed %s (%s)\n", wt.Label(), wt.Path)

			switch {
			case res.BranchDeleted:
				fmt.Printf("deleted branch %s\n", res.Branch)
			case res.BranchUnmerged:
				fmt.Printf("branch %s is not fully merged; not deleted "+
					"(use -B to force-delete)\n", res.Branch)
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "discard uncommitted changes when removing")
	cmd.Flags().BoolVarP(&deleteBranch, "delete-branch", "b", false, "also delete the branch (safe; refuses if unmerged)")
	cmd.Flags().BoolVarP(&forceBranch, "force-delete-branch", "B", false, "also delete the branch, even if unmerged")
	return cmd
}
