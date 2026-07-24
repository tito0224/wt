// Package terminal defines the adapter interface for opening a new terminal
// tab/window in a given directory, plus built-in adapters. Add support for a
// new terminal by implementing Adapter and registering it.
package terminal

import (
	"fmt"
	"os"
	"sort"
	"strings"
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

var registry = map[string]Adapter{}

// Register adds an adapter to the global registry. Intended for use from
// init(). Panics on duplicate names to catch programming errors early.
func Register(a Adapter) {
	if _, dup := registry[a.Name()]; dup {
		panic("terminal: duplicate adapter " + a.Name())
	}
	registry[a.Name()] = a
}

// Names returns the sorted list of registered adapter names.
func Names() []string {
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Get returns the adapter registered under name.
func Get(name string) (Adapter, bool) {
	a, ok := registry[name]
	return a, ok
}

// Resolve selects an adapter. If name is non-empty it must match a registered
// adapter. Otherwise it prefers the WT_TERMINAL env var, then auto-detection.
func Resolve(name string) (Adapter, error) {
	if name != "" {
		if a, ok := registry[name]; ok {
			return a, nil
		}
		return nil, fmt.Errorf("unknown terminal %q (available: %s)", name, strings.Join(Names(), ", "))
	}
	if env := os.Getenv("WT_TERMINAL"); env != "" {
		if a, ok := registry[env]; ok {
			return a, nil
		}
		return nil, fmt.Errorf("WT_TERMINAL=%q is not a known terminal (available: %s)", env, strings.Join(Names(), ", "))
	}
	for _, a := range registry {
		if a.Detect() {
			return a, nil
		}
	}
	return nil, fmt.Errorf("could not detect a terminal; set WT_TERMINAL to one of: %s", strings.Join(Names(), ", "))
}
