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
)

// initStyles builds all styles from the given renderer. Because the TUI draws
// to /dev/tty (not stdout, which the shell wrapper captures), color support
// must be detected from that same tty — otherwise lipgloss profiles the
// captured stdout, sees no terminal, and strips all color. Passing a renderer
// bound to the tty fixes that.
func initStyles(r *lipgloss.Renderer) {
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
}
