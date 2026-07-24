package main

import (
	"fmt"

	"github.com/johnlotito/wt/internal/shell"
	"github.com/spf13/cobra"
)

func newShellCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "shell [zsh|bash|fish]",
		Short: "Print the shell integration function (for eval)",
		Long: "Print the shell wrapper function that lets `wt` cd your current " +
			"shell into a selected worktree.\n\n" +
			"Add to your rc file, e.g.:\n" +
			"  eval \"$(wt shell zsh)\"    # zsh/bash\n" +
			"  wt shell fish | source     # fish\n\n" +
			"If the shell is omitted, it is detected from $SHELL.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sh := ""
			if len(args) == 1 {
				sh = args[0]
			} else {
				sh = shell.Detect()
			}
			if sh == "" {
				return fmt.Errorf("could not detect shell; pass one of: %v", shell.Supported())
			}
			src, err := shell.Wrapper(sh)
			if err != nil {
				return err
			}
			fmt.Print(src)
			return nil
		},
	}
	cmd.AddCommand(newShellInstallCmd())
	return cmd
}

func newShellInstallCmd() *cobra.Command {
	var shellName string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Add the shell integration line to your rc file",
		Long: "Append the activation line (e.g. eval \"$(wt shell zsh)\") to your " +
			"shell's rc file. Idempotent: does nothing if already present.\n\n" +
			"The shell is taken from --shell, otherwise detected from $SHELL.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			sh := shellName
			if sh == "" {
				sh = shell.Detect()
			}
			if sh == "" {
				return fmt.Errorf("could not detect shell; pass --shell (one of: %v)", shell.Supported())
			}
			res, err := shell.Install(sh, dryRun)
			if err != nil {
				return err
			}
			switch {
			case res.AlreadyExists:
				fmt.Printf("already installed in %s (nothing to do)\n", res.RCFile)
			case dryRun:
				fmt.Printf("would append to %s:\n\n%s\n", res.RCFile, res.Block)
			case res.Wrote:
				fmt.Printf("added wt shell integration to %s\n", res.RCFile)
				fmt.Printf("restart your shell or run: source %s\n", res.RCFile)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&shellName, "shell", "", "shell to target (zsh|bash|fish); detected from $SHELL if unset")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be written without modifying any file")
	return cmd
}
