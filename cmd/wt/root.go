package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/johnlotito/wt/internal/config"
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
	root.AddCommand(newCreateCmd())
	root.AddCommand(newOpenCmd())
	root.AddCommand(newStatusCmd())
	root.AddCommand(newRemoveCmd())
	root.AddCommand(newTerminalsCmd())
	root.AddCommand(newConfigCmd())
	root.AddCommand(newShellCmd())
	return root
}

func runInteractive() error {
	dir := repoDir()
	if !git.IsRepo(dir) {
		return fmt.Errorf("%s is not a git repository", dir)
	}
	// Worktrees are loaded asynchronously inside the TUI (with a spinner), so
	// the picker appears instantly even when enumerating many worktrees is slow.
	return tui.Run(dir, flagTerminal)
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

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect wt configuration",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "path",
		Short: "Print the global config file path",
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := config.GlobalPath()
			if err != nil {
				return err
			}
			fmt.Println(p)
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Print the effective configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := repoDir()
			root := dir
			if top, err := git.TopLevel(dir); err == nil {
				root = top
			}
			cfg, err := config.Load(root)
			if err != nil {
				return err
			}
			c := cfg.Create
			w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
			fmt.Fprintf(w, "base_branch\t%s\n", orNone(c.BaseBranch))
			fmt.Fprintf(w, "dir\t%s\n", orNone(c.Dir))
			fmt.Fprintf(w, "branch_prefix\t%s\n", orNone(c.BranchPrefix))
			fmt.Fprintf(w, "branch_separator\t%s\n", orNone(c.BranchSeparator))
			return w.Flush()
		},
	})

	cmd.AddCommand(newConfigInitCmd())

	return cmd
}

func newConfigInitCmd() *cobra.Command {
	var (
		global bool
		repo   bool
		yes    bool
		force  bool
	)
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create a config file interactively",
		Long: "Create a wt config file.\n\n" +
			"By default the file is written to the repository's .wt.toml when run " +
			"inside a git repository, otherwise to the global config file. Use " +
			"--global or --repo to choose explicitly.\n\n" +
			"Prompts for each value unless --yes is given, in which case a commented " +
			"template with defaults is written.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if global && repo {
				return fmt.Errorf("--global and --repo are mutually exclusive")
			}

			// Determine repo root (if any).
			root := ""
			if top, err := git.TopLevel(repoDir()); err == nil {
				root = top
			}

			// Choose target path.
			var path string
			switch {
			case repo:
				if root == "" {
					return fmt.Errorf("--repo requires being inside a git repository")
				}
				path = config.RepoPath(root)
			case global:
				p, err := config.GlobalPath()
				if err != nil {
					return err
				}
				path = p
			case root != "":
				path = config.RepoPath(root)
			default:
				p, err := config.GlobalPath()
				if err != nil {
					return err
				}
				path = p
			}

			// Seed values from the currently effective config so init is
			// idempotent-ish and reflects existing overrides.
			cfg, err := config.Load(root)
			if err != nil {
				return err
			}

			if !yes {
				if err := promptConfig(&cfg); err != nil {
					return err
				}
			}

			content, err := config.Render(cfg)
			if err != nil {
				return err
			}
			if err := config.Write(path, content, force); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "wrote %s\n", path)
			return nil
		},
	}
	cmd.Flags().BoolVar(&global, "global", false, "write the global config file")
	cmd.Flags().BoolVar(&repo, "repo", false, "write the repo-local .wt.toml")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip prompts; write a template with defaults")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "overwrite an existing file")
	return cmd
}

// promptConfig interactively fills cfg.Create, showing current values as
// defaults (accepted by pressing enter).
//
// Prompts read from and write to the controlling terminal (/dev/tty) rather
// than stdin/stdout. This is essential because the shell wrapper invokes wt
// inside $(...) to capture stdout; if prompts used stdout they would be
// swallowed by that capture and appear to hang. When there is no controlling
// tty (e.g. piped input), it falls back to stdin for reading and stderr for
// the prompt labels so stdout stays clean.
func promptConfig(cfg *config.Config) error {
	c := &cfg.Create

	var out io.Writer = os.Stderr
	var in io.Reader = os.Stdin
	if tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0); err == nil {
		defer tty.Close()
		out = tty
		in = tty
	}
	r := bufio.NewReader(in)

	sep := c.BranchSeparator
	if sep == "" {
		sep = "/"
	}

	c.BaseBranch = prompt(out, r, "Base branch/ref (empty = repo default)", c.BaseBranch)
	c.Dir = prompt(out, r, "Worktree dir (empty = ../<repo>-worktrees, supports {repo})", c.Dir)
	c.BranchPrefix = prompt(out, r, "Branch prefix (empty = none)", c.BranchPrefix)
	c.BranchSeparator = prompt(out, r, "Branch separator", sep)
	return nil
}

// prompt reads a single line, returning def when the input is empty. The
// prompt label is written to out (the controlling tty, or stderr) so it stays
// visible even when stdout is captured by the shell integration wrapper.
func prompt(out io.Writer, r *bufio.Reader, label, def string) string {
	if def != "" {
		fmt.Fprintf(out, "%s [%s]: ", label, def)
	} else {
		fmt.Fprintf(out, "%s: ", label)
	}
	line, err := r.ReadString('\n')
	if err != nil && line == "" {
		return def
	}
	line = strings.TrimRight(line, "\r\n")
	if strings.TrimSpace(line) == "" {
		return def
	}
	return line
}

func orNone(s string) string {
	if s == "" {
		return "(unset)"
	}
	return s
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "wt: "+err.Error())
		os.Exit(1)
	}
}
