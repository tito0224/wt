// Package shell generates the shell integration wrapper and installs the
// activation line into the user's rc file.
package shell

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// marker delimits the block wt writes into an rc file so it can be detected
// (for idempotency) and later removed by hand if desired.
const marker = "# >>> wt shell integration >>>"
const markerEnd = "# <<< wt shell integration <<<"

// Supported returns the shells wt can generate integration for.
func Supported() []string { return []string{"zsh", "bash", "fish"} }

// IsSupported reports whether name is a shell wt can integrate with.
func IsSupported(name string) bool {
	for _, s := range Supported() {
		if s == name {
			return true
		}
	}
	return false
}

// Detect returns the user's login shell basename from $SHELL (e.g. "zsh").
func Detect() string {
	sh := os.Getenv("SHELL")
	if sh == "" {
		return ""
	}
	return filepath.Base(sh)
}

// Wrapper returns the shell function source for the given shell. The wrapper
// calls the real binary; when the binary prints an existing directory path it
// cd's the current shell into it, otherwise it passes output through.
func Wrapper(shell string) (string, error) {
	switch shell {
	case "zsh", "bash":
		return posixWrapper, nil
	case "fish":
		return fishWrapper, nil
	default:
		return "", fmt.Errorf("unsupported shell %q (supported: %s)", shell, strings.Join(Supported(), ", "))
	}
}

const posixWrapper = `wt() {
  # Interactive (no args) or a subcommand: capture stdout so a printed
  # worktree path can cd the current shell. Pass through other output.
  local __wt_out
  __wt_out="$(command wt "$@")"
  local __wt_rc=$?
  if [ $__wt_rc -ne 0 ]; then
    [ -n "$__wt_out" ] && printf '%s\n' "$__wt_out" >&2
    return $__wt_rc
  fi
  if [ -d "$__wt_out" ]; then
    cd "$__wt_out"
  elif [ -n "$__wt_out" ]; then
    printf '%s\n' "$__wt_out"
  fi
}
`

const fishWrapper = `function wt
    set -l __wt_out (command wt $argv)
    set -l __wt_rc $status
    if test $__wt_rc -ne 0
        test -n "$__wt_out"; and printf '%s\n' $__wt_out >&2
        return $__wt_rc
    end
    if test -d "$__wt_out"
        cd $__wt_out
    else if test -n "$__wt_out"
        printf '%s\n' $__wt_out
    end
end
`

// rcFile returns the conventional rc file path for a shell.
func rcFile(shell string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch shell {
	case "zsh":
		return filepath.Join(home, ".zshrc"), nil
	case "bash":
		// macOS terminal emulators start bash as a LOGIN shell, which reads
		// ~/.bash_profile and skips ~/.bashrc — installing into ~/.bashrc
		// there would write a file that is never sourced. On Linux, terminals
		// start interactive non-login shells, which read ~/.bashrc.
		if runtime.GOOS == "darwin" {
			return filepath.Join(home, ".bash_profile"), nil
		}
		return filepath.Join(home, ".bashrc"), nil
	case "fish":
		return filepath.Join(home, ".config", "fish", "config.fish"), nil
	default:
		return "", fmt.Errorf("unsupported shell %q", shell)
	}
}

// evalLine is the line users add to activate the integration.
func evalLine(shell string) string {
	if shell == "fish" {
		return fmt.Sprintf("%s\nwt shell %s | source\n%s\n", marker, shell, markerEnd)
	}
	return fmt.Sprintf("%s\neval \"$(wt shell %s)\"\n%s\n", marker, shell, markerEnd)
}

// InstallResult describes what an Install call did or would do.
type InstallResult struct {
	Shell         string
	RCFile        string
	Block         string
	AlreadyExists bool
	Wrote         bool
}

// Install appends the activation block to the shell's rc file. When dryRun is
// true it computes the change without writing. It is idempotent: if the marker
// block already exists it does nothing.
func Install(shell string, dryRun bool) (*InstallResult, error) {
	if !IsSupported(shell) {
		return nil, fmt.Errorf("unsupported shell %q (supported: %s)", shell, strings.Join(Supported(), ", "))
	}
	rc, err := rcFile(shell)
	if err != nil {
		return nil, err
	}
	block := evalLine(shell)
	res := &InstallResult{Shell: shell, RCFile: rc, Block: block}

	if existing, err := os.ReadFile(rc); err == nil {
		if strings.Contains(string(existing), marker) {
			res.AlreadyExists = true
			return res, nil
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	if dryRun {
		return res, nil
	}

	if err := os.MkdirAll(filepath.Dir(rc), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(rc, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	fmt.Fprintf(w, "\n%s", block)
	if err := w.Flush(); err != nil {
		return nil, err
	}
	res.Wrote = true
	return res, nil
}
