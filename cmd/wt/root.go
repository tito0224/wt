package main

import (
	"github.com/spf13/cobra"
	"github.com/tito0224/wt/internal/config"
	"github.com/tito0224/wt/internal/git"
	"github.com/tito0224/wt/internal/launcher"
	"github.com/tito0224/wt/internal/tui"
)

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "wt",
		Short: "Manage git worktrees",
		Long: "wt lists git worktrees, shows whether they have uncommitted changes " +
			"or unmerged commits, and can launch a new terminal tab in a worktree.\n\n" +
			"Run `wt` with no arguments to open the interactive picker.",
		// Version enables cobra's built-in --version flag.
		Version:       versionString(),
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
	root.AddCommand(newAddCmd())
	root.AddCommand(newOpenCmd())
	root.AddCommand(newStatusCmd())
	root.AddCommand(newRemoveCmd())
	root.AddCommand(newPruneCmd())
	root.AddCommand(newTerminalsCmd())
	root.AddCommand(newConfigCmd())
	root.AddCommand(newShellCmd())
	return root
}

func runInteractive() error {
	dir, err := requireRepo()
	if err != nil {
		return err
	}
	// Resolve launcher settings from config. Best-effort: an unreadable config
	// just means defaults.
	var defaultLauncher string
	if top, err := git.MainDir(dir); err == nil {
		if cfg, err := config.Load(top); err == nil {
			defaultLauncher = cfg.Launch.Default
			launcher.SetOpencodeWebOrigin(cfg.Launch.OpencodeWebURL)
		}
	}
	// Worktrees are loaded asynchronously inside the TUI (with a spinner), so
	// the picker appears instantly even when enumerating many worktrees is slow.
	return tui.Run(dir, flagTerminal, defaultLauncher)
}
