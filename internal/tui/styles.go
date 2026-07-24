package tui

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle    lipgloss.Style
	headerStyle   lipgloss.Style
	selectedStyle lipgloss.Style
	dirtyStyle    lipgloss.Style
	unmergedStyle lipgloss.Style
	cleanStyle    lipgloss.Style
	currentStyle  lipgloss.Style
	pathStyle     lipgloss.Style
	helpStyle     lipgloss.Style
	errorStyle    lipgloss.Style
	matchStyle    lipgloss.Style
	filterStyle   lipgloss.Style

	keycapStyle lipgloss.Style // key hint "caps" in the footer
	labelStyle  lipgloss.Style // dim labels next to keycaps
	prOpenStyle lipgloss.Style // PR state: open
	prMergedSty lipgloss.Style // PR state: merged
	prClosedSty lipgloss.Style // PR state: closed
)

// initStyles builds all styles from the given renderer. The TUI draws to
// stderr, so color support must be detected from stderr — passing a renderer
// bound to stderr. (Profiling stdout instead would be wrong: the shell wrapper
// captures stdout via $(...), where lipgloss sees no terminal and strips color.)
func initStyles(r *lipgloss.Renderer) {
	// Make this the process-wide default renderer so the Bubbles components
	// (textinput cursor, spinner) — which build their own styles via the
	// default renderer — also detect color from stderr. Without this they'd
	// profile stdout (captured by the shell wrapper) and render without color,
	// making the reverse-video text cursor invisible.
	lipgloss.SetDefaultRenderer(r)

	// Semantic colors use ANSI 0-7 palette slots, which resolve to whatever the
	// user's terminal theme defines for those slots. This is the most
	// theme-portable choice: "green" is the theme's green, etc. We avoid
	// hardcoded bright/256/truecolor values that could clash with a theme.
	titleStyle = r.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))   // cyan
	headerStyle = r.NewStyle().Bold(true)                                  // theme fg, bold
	selectedStyle = r.NewStyle().Reverse(true)                             // swap fg/bg: always readable
	dirtyStyle = r.NewStyle().Foreground(lipgloss.Color("3"))              // yellow
	unmergedStyle = r.NewStyle().Foreground(lipgloss.Color("1"))           // red
	cleanStyle = r.NewStyle().Foreground(lipgloss.Color("2"))              // green
	currentStyle = r.NewStyle().Foreground(lipgloss.Color("6")).Bold(true) // cyan
	pathStyle = r.NewStyle().Foreground(lipgloss.Color("8"))               // bright black / gray
	helpStyle = r.NewStyle().Foreground(lipgloss.Color("8"))
	errorStyle = r.NewStyle().Foreground(lipgloss.Color("1"))
	matchStyle = r.NewStyle().Foreground(lipgloss.Color("5")).Bold(true) // magenta
	filterStyle = r.NewStyle().Foreground(lipgloss.Color("6"))

	// Footer key hints: the key is a reverse-video "cap" that the eye locks
	// onto; the label is dimmed. This is far more scannable than a flat run of
	// "key verb · key verb".
	keycapStyle = r.NewStyle().Reverse(true).Bold(true)
	labelStyle = r.NewStyle().Foreground(lipgloss.Color("7")) // normal-ish fg

	// PR state colors (GitHub-like): open green, merged magenta, closed red.
	prOpenStyle = r.NewStyle().Foreground(lipgloss.Color("2"))
	prMergedSty = r.NewStyle().Foreground(lipgloss.Color("5"))
	prClosedSty = r.NewStyle().Foreground(lipgloss.Color("1"))
}
