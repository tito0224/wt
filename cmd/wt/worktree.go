package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/johnlotito/wt/internal/config"
	"github.com/johnlotito/wt/internal/git"
	"github.com/johnlotito/wt/internal/terminal"
	"github.com/spf13/cobra"
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
					fmt.Printf("  unmerged:  %t\n", w.Unmerged)
					if w.HasUpstream {
						fmt.Printf("  ahead:     %d\n", w.Ahead)
						fmt.Printf("  behind:    %d\n", w.Behind)
					} else {
						fmt.Printf("  upstream:  (none)\n")
					}
				}
				fmt.Println()
			}
			return nil
		},
	}
}

func newCreateCmd() *cobra.Command {
	var (
		base string
		dir  string
		tab  bool
	)
	cmd := &cobra.Command{
		Use:     "create <name>",
		Aliases: []string{"new", "add"},
		Short:   "Create a new worktree",
		Long: "Create a new worktree and branch for <name>.\n\n" +
			"Defaults for the base branch, target directory, and branch naming " +
			"come from the wt config file (see `wt config path`), and can be " +
			"overridden per-invocation with flags.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			repo := repoDir()
			if !git.IsRepo(repo) {
				return fmt.Errorf("%s is not a git repository", repo)
			}
			top, err := git.TopLevel(repo)
			if err != nil {
				return err
			}

			branch, path, err := config.CreateWorktree(top, name, base, dir)
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
	var force bool
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
			if wt.Bare {
				return fmt.Errorf("refusing to remove the bare repository")
			}
			if err := git.Remove(dir, wt.Path, force); err != nil {
				return err
			}
			fmt.Printf("removed %s (%s)\n", wt.Label(), wt.Path)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "discard uncommitted changes when removing")
	return cmd
}
