// Package terminal defines the adapter interface for opening a new terminal
// tab/window in a given directory, plus built-in adapters. Add support for a
// new terminal by implementing Adapter and registering it.
package terminal

import (
	"fmt"
	"os"
	"strings"

	"github.com/johnlotito/wt/internal/registry"
)

// Adapter knows how to launch a new terminal tab/window in a directory.
//
// Implement this interface and call Register in an init() function to add
// support for a new terminal emulator.
type Adapter interface {
	// Name is the stable identifier used to select the adapter (e.g. "iterm").
	Name() string
	// Detect reports whether this adapter is appropriate for the current
	// environment (typically by inspecting env vars like $TERM_PROGRAM).
	Detect() bool
	// OpenTab opens a new tab/window in dir. When command is non-empty, the tab
	// runs that command (argv-style: command[0] with command[1:] as args)
	// instead of an interactive shell. Implementations should not block waiting
	// on the spawned terminal.
	OpenTab(dir string, command ...string) error
}

var adapters = registry.New[Adapter]("terminal", "adapter")

// Register adds an adapter to the global registry. Intended for use from
// init(). Panics on duplicate names to catch programming errors early.
func Register(a Adapter) { adapters.Register(a.Name(), a) }

// Names returns the sorted list of registered adapter names.
func Names() []string { return adapters.Names() }

// Get returns the adapter registered under name.
func Get(name string) (Adapter, bool) { return adapters.Get(name) }

// Resolve selects an adapter. If name is non-empty it must match a registered
// adapter. Otherwise it prefers the WT_TERMINAL env var, then auto-detection.
func Resolve(name string) (Adapter, error) {
	if name != "" {
		if a, ok := adapters.Get(name); ok {
			return a, nil
		}
		return nil, fmt.Errorf("unknown terminal %q (available: %s)", name, strings.Join(Names(), ", "))
	}
	if env := os.Getenv("WT_TERMINAL"); env != "" {
		if a, ok := adapters.Get(env); ok {
			return a, nil
		}
		return nil, fmt.Errorf("WT_TERMINAL=%q is not a known terminal (available: %s)", env, strings.Join(Names(), ", "))
	}
	// Auto-detect in an explicit, deterministic order. Several adapters can
	// detect at once — running tmux inside iTerm2 sets both $TMUX and
	// $TERM_PROGRAM=iTerm.app — so iterating a map here would pick a winner at
	// random. tmux is checked first because it is the innermost environment:
	// when the user is inside tmux, opening a tab in the host terminal from
	// within the session is almost never what they want. The remaining
	// adapters are tried in sorted-name order for determinism.
	if a, ok := adapters.Get("tmux"); ok && a.Detect() {
		return a, nil
	}
	for _, name := range adapters.Names() {
		if name == "tmux" {
			continue // already tried above
		}
		a, _ := adapters.Get(name)
		if a.Detect() {
			return a, nil
		}
	}
	return nil, fmt.Errorf("could not detect a terminal; set WT_TERMINAL to one of: %s", strings.Join(Names(), ", "))
}
