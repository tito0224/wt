package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/johnlotito/wt/internal/git"
	"github.com/johnlotito/wt/internal/terminal"
	"github.com/johnlotito/wt/internal/tui"
	"github.com/spf13/cobra"
)

var (
	flagTerminal string
	flagDir      string
)

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

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "wt",
		Short: "Manage git worktrees",
		Long: "wt lists git worktrees, shows whether they have uncommitted changes " +
			"or unmerged commits, and can launch a new terminal tab in a worktree.\n\n" +
			"Run `wt` with no arguments to open the interactive picker.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInteractive()
		},
	}

	root.PersistentFlags().StringVar(&flagTerminal, "terminal", "",
		"terminal adapter to use (auto-detected if unset)")
	root.PersistentFlags().StringVarP(&flagDir, "dir", "C", "",
		"run as if wt was started in this directory")

	root.AddCommand(newListCmd())
	root.AddCommand(newOpenCmd())
	root.AddCommand(newStatusCmd())
	root.AddCommand(newRemoveCmd())
	root.AddCommand(newTerminalsCmd())
	root.AddCommand(newShellCmd())
	return root
}

func runInteractive() error {
	dir := repoDir()
	if !git.IsRepo(dir) {
		return fmt.Errorf("%s is not a git repository", dir)
	}
	wts, err := git.List(dir)
	if err != nil {
		return err
	}
	return tui.Run(dir, wts, flagTerminal)
}

func newTerminalsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "terminals",
		Short: "List available terminal adapters",
		RunE: func(cmd *cobra.Command, args []string) error {
			w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tDETECTED")
			for _, name := range terminal.Names() {
				a, _ := terminal.Get(name)
				det := ""
				if a.Detect() {
					det = "yes (current)"
				}
				fmt.Fprintf(w, "%s\t%s\n", name, det)
			}
			return w.Flush()
		},
	}
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "wt: "+err.Error())
		os.Exit(1)
	}
}
