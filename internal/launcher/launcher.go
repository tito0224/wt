// Package launcher defines the interface for opening a worktree directory in an
// editor or AI agent (VS Code, Cursor, Claude, Opencode, ...), plus built-in
// launchers. Add support for a new app by implementing Launcher and registering
// it in an init().
//
// Launchers come in three kinds (see Kind), which the caller uses to decide how
// to run them: GUI apps and URLs are launched directly (fire-and-forget), while
// terminal agents are run in a new terminal tab/window via a terminal.Adapter.
package launcher

import (
	"fmt"

	"github.com/tito0224/wt/internal/browser"
	"github.com/tito0224/wt/internal/registry"
	"github.com/tito0224/wt/internal/terminal"
)

// Kind categorizes how a launcher runs.
type Kind int

const (
	// KindGUI is a standalone GUI application (e.g. VS Code) launched directly.
	KindGUI Kind = iota
	// KindTerminal is a terminal program (e.g. an AI agent CLI) that must run
	// inside a terminal; the caller opens a new terminal tab running it.
	KindTerminal
	// KindURL opens a URL in the browser.
	KindURL
)

// Launcher opens a worktree directory in a particular app or agent.
//
// Implement this interface and call Register in an init() function to add a new
// launcher.
type Launcher interface {
	// Name is the stable identifier used to select the launcher (e.g. "code").
	Name() string
	// Title is the human-readable label shown in the picker (e.g. "VS Code").
	Title() string
	// Icon is a single-glyph label for the picker (typically a Nerd Font
	// codepoint). May be empty; callers should tolerate glyphs the user's font
	// can't render.
	Icon() string
	// Kind reports how the launcher runs (GUI, terminal, or URL).
	Kind() Kind
	// Available reports whether the app/CLI is installed and usable.
	Available() bool

	// Command returns the argv to run in a new terminal for KindTerminal
	// launchers (e.g. {"claude"}). It is unused for other kinds.
	Command(dir string) []string
	// GUILaunch launches a KindGUI app for dir. Unused for other kinds.
	GUILaunch(dir string) error
	// URL returns the URL to open for KindURL launchers. Unused otherwise.
	URL(dir string) string
}

var launchers = registry.New[Launcher]("launcher", "launcher")

// Register adds a launcher to the global registry. Intended for use from
// init(). Panics on duplicate names to catch programming errors early.
func Register(l Launcher) { launchers.Register(l.Name(), l) }

// Names returns the sorted list of registered launcher names.
func Names() []string { return launchers.Names() }

// Get returns the launcher registered under name.
func Get(name string) (Launcher, bool) { return launchers.Get(name) }

// Available returns the registered launchers that are installed, in sorted
// (deterministic) name order.
func Available() []Launcher {
	var out []Launcher
	for _, l := range launchers.Values() {
		if l.Available() {
			out = append(out, l)
		}
	}
	return out
}

// Launch runs the launcher for dir. GUI and URL launchers run directly; a
// terminal launcher opens a new terminal tab (via term) running its command.
// term may be nil for non-terminal launchers.
func Launch(l Launcher, dir string, term terminal.Adapter) error {
	switch l.Kind() {
	case KindGUI:
		return l.GUILaunch(dir)
	case KindURL:
		return browser.Open(l.URL(dir))
	case KindTerminal:
		if term == nil {
			return fmt.Errorf("%s needs a terminal but none is available", l.Title())
		}
		return term.OpenTab(dir, l.Command(dir)...)
	default:
		return fmt.Errorf("unsupported launcher kind for %s", l.Name())
	}
}
