// Package tui provides an interactive worktree picker rendered as a scrollable,
// fuzzy-filterable table with an action submenu.
package tui

import (
	"fmt"
	"os"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/johnlotito/wt/internal/git"
	"github.com/johnlotito/wt/internal/terminal"
)

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

type view int

const (
	viewList view = iota
	viewActions
)

type action struct {
	key   string
	label string
	id    string
}

var actions = []action{
	{"c", "cd here (current shell)", "cd"},
	{"o", "Open in new terminal tab", "open"},
	{"s", "Show detailed status", "status"},
	{"r", "Remove worktree", "remove"},
}

// row is a worktree paired with its fuzzy-match metadata for the current
// filter query.
type row struct {
	wt      git.Worktree
	score   int   // fuzzy-match score; only meaningful when a filter is active
	matched []int // matched rune indices in the label, for highlighting
}

type model struct {
	dir          string
	all          []git.Worktree // full, unfiltered set
	terminalName string

	view      view
	searching bool   // true while in search-input mode (entered with '/')
	confirm   string // non-empty shows a y/n confirmation prompt for this action
	filter    string
	rows      []row // filtered view of `all`
	cursor    int   // index into rows
	offset    int   // scroll offset (first visible row)
	actionCur int

	width, height int

	message string
	isErr   bool
	quit    bool

	resultAction string
	resultWT     *git.Worktree
}

// Run launches the interactive picker.
//
// The TUI renders to and reads from the controlling terminal (/dev/tty) rather
// than stdin/stdout. This is essential because the shell wrapper invokes wt
// inside $(...) to capture the chosen path for `cd`; if the TUI drew to stdout
// it would be swallowed by that capture and appear to hang. Only the final
// selected path is written to real stdout, for the wrapper to consume.
func Run(dir string, wts []git.Worktree, terminalName string) error {
	m := &model{dir: dir, all: wts, terminalName: terminalName, width: 80, height: 24}
	m.recompute()

	opts := []tea.ProgramOption{tea.WithAltScreen()}
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err == nil {
		defer tty.Close()
		opts = append(opts, tea.WithInput(tty), tea.WithOutput(tty))
		// Detect color from the tty we actually draw to, not stdout (which the
		// shell wrapper captures via $(...)).
		initStyles(lipgloss.NewRenderer(tty))
	} else {
		// No controlling tty (e.g. piped): fall back to stderr for output so
		// stdout stays clean for the result path.
		opts = append(opts, tea.WithOutput(os.Stderr))
		initStyles(lipgloss.NewRenderer(os.Stderr))
	}

	res, err := tea.NewProgram(m, opts...).Run()
	if err != nil {
		return err
	}
	fm := res.(*model)
	if fm.resultWT == nil {
		return nil
	}
	switch fm.resultAction {
	case "cd":
		fmt.Println(fm.resultWT.Path)
	case "status":
		printStatus(*fm.resultWT)
	}
	return nil
}

func (m *model) Init() tea.Cmd { return nil }

// recompute rebuilds the filtered rows from the current filter. Rows keep
// git's native order (repo root first, as git lists it). When a filter is
// active, matches are sorted by fuzzy score (best match first).
func (m *model) recompute() {
	q := strings.ToLower(strings.TrimSpace(m.filter))
	m.rows = m.rows[:0]
	for _, w := range m.all {
		if q == "" {
			m.rows = append(m.rows, row{wt: w})
			continue
		}
		// Match against "branch  path" so either field can satisfy the query.
		hay := strings.ToLower(w.Label() + " " + w.Path)
		if idx, score, ok := fuzzyMatch(hay, q); ok {
			// Only keep highlight indices that fall within the label portion.
			var inLabel []int
			labelLen := len([]rune(w.Label()))
			for _, i := range idx {
				if i < labelLen {
					inLabel = append(inLabel, i)
				}
			}
			m.rows = append(m.rows, row{wt: w, score: score, matched: inLabel})
		}
	}
	// Rank matches best-first only while filtering; the unfiltered list keeps
	// git's native order. Stable sort preserves git order among equal scores.
	if q != "" {
		sort.SliceStable(m.rows, func(i, j int) bool {
			return m.rows[i].score > m.rows[j].score
		})
	}
	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.clampScroll()
}

// visibleRows is how many table rows fit given the terminal height, reserving
// space for title, header, filter line, and footer.
func (m *model) visibleRows() int {
	n := m.height - 6
	if n < 1 {
		n = 1
	}
	return n
}

func (m *model) clampScroll() {
	vis := m.visibleRows()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+vis {
		m.offset = m.cursor - vis + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m *model) selected() *git.Worktree {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return nil
	}
	return &m.rows[m.cursor].wt
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clampScroll()
		// Force a full repaint so shrinking the terminal can't leave stale
		// lines from the previous (taller/wider) frame on screen.
		return m, tea.ClearScreen
	case tea.KeyMsg:
		if m.confirm != "" {
			return m.updateConfirm(msg)
		}
		if m.view == viewActions {
			return m.updateActions(msg)
		}
		return m.updateList(msg)
	}
	return m, nil
}

// updateConfirm handles the y/n confirmation prompt.
func (m *model) updateConfirm(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Type == tea.KeyCtrlC {
		m.quit = true
		return m, tea.Quit
	}
	switch strings.ToLower(string(key.Runes)) {
	case "y":
		action := m.confirm
		m.confirm = ""
		if action == "remove" {
			return m.doRemove()
		}
	default:
		// Anything else (n, esc, etc.) cancels.
		m.confirm = ""
		m.setMsg("cancelled")
	}
	return m, nil
}

func (m *model) updateList(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.searching {
		return m.updateSearch(key)
	}
	return m.updateNormal(key)
}

// updateNormal handles keys in normal mode, where single letters are
// shortcuts and '/' enters search mode.
func (m *model) updateNormal(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.message = ""
	switch key.Type {
	case tea.KeyCtrlC:
		m.quit = true
		return m, tea.Quit
	case tea.KeyEsc:
		// First esc clears an active filter; otherwise quit.
		if m.filter != "" {
			m.filter = ""
			m.recompute()
			return m, nil
		}
		m.quit = true
		return m, tea.Quit
	case tea.KeyUp:
		m.moveCursor(-1)
		return m, nil
	case tea.KeyDown:
		m.moveCursor(1)
		return m, nil
	case tea.KeyPgUp:
		m.moveCursor(-m.visibleRows())
		return m, nil
	case tea.KeyPgDown:
		m.moveCursor(m.visibleRows())
		return m, nil
	case tea.KeyHome:
		m.cursor = 0
		m.clampScroll()
		return m, nil
	case tea.KeyEnd:
		m.cursor = len(m.rows) - 1
		m.clampScroll()
		return m, nil
	case tea.KeyEnter:
		if m.selected() != nil {
			return m.runAction("cd")
		}
		return m, nil
	case tea.KeyTab, tea.KeyRight:
		if m.selected() != nil {
			m.view = viewActions
			m.actionCur = 0
		}
		return m, nil
	case tea.KeyRunes:
		switch string(key.Runes) {
		case "/":
			m.searching = true
			return m, nil
		case "j":
			m.moveCursor(1)
		case "k":
			m.moveCursor(-1)
		case "g":
			m.cursor = 0
			m.clampScroll()
		case "G":
			m.cursor = len(m.rows) - 1
			m.clampScroll()
		case "o":
			return m.runAction("open")
		case "a":
			if m.selected() != nil {
				m.view = viewActions
				m.actionCur = 0
			}
		case "s":
			return m.runAction("status")
		case "r":
			return m.runAction("remove")
		case "q":
			m.quit = true
			return m, tea.Quit
		}
		return m, nil
	}
	return m, nil
}

// updateSearch handles keys while typing a filter query. Enter/esc leave
// search mode (keeping the current filter); a further esc in normal mode
// clears it.
func (m *model) updateSearch(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyCtrlC:
		m.quit = true
		return m, tea.Quit
	case tea.KeyEsc, tea.KeyEnter:
		m.searching = false
		return m, nil
	case tea.KeyUp:
		m.moveCursor(-1)
		return m, nil
	case tea.KeyDown:
		m.moveCursor(1)
		return m, nil
	case tea.KeyBackspace:
		if m.filter != "" {
			r := []rune(m.filter)
			m.filter = string(r[:len(r)-1])
			m.recompute()
		}
		return m, nil
	case tea.KeyRunes, tea.KeySpace:
		m.message = ""
		m.filter += string(key.Runes)
		if key.Type == tea.KeySpace {
			m.filter += " "
		}
		m.recompute()
		return m, nil
	}
	return m, nil
}

func (m *model) moveCursor(delta int) {
	m.message = ""
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor > len(m.rows)-1 {
		m.cursor = len(m.rows) - 1
	}
	m.clampScroll()
}

func (m *model) updateActions(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyCtrlC:
		m.quit = true
		return m, tea.Quit
	case tea.KeyEsc, tea.KeyLeft:
		m.view = viewList
		return m, nil
	case tea.KeyUp:
		if m.actionCur > 0 {
			m.actionCur--
		}
		return m, nil
	case tea.KeyDown:
		if m.actionCur < len(actions)-1 {
			m.actionCur++
		}
		return m, nil
	case tea.KeyEnter:
		return m.runAction(actions[m.actionCur].id)
	case tea.KeyRunes:
		for _, a := range actions {
			if string(key.Runes) == a.key {
				return m.runAction(a.id)
			}
		}
	}
	return m, nil
}

func (m *model) runAction(id string) (tea.Model, tea.Cmd) {
	wt := m.selected()
	if wt == nil {
		return m, nil
	}
	switch id {
	case "open":
		adapter, err := terminal.Resolve(m.terminalName)
		if err != nil {
			m.setErr(err.Error())
			m.view = viewList
			return m, nil
		}
		if err := adapter.OpenTab(wt.Path); err != nil {
			m.setErr(err.Error())
			m.view = viewList
			return m, nil
		}
		m.setMsg(fmt.Sprintf("opened %s in new %s tab", wt.Label(), adapter.Name()))
		m.view = viewList
	case "cd", "status":
		m.resultAction = id
		m.resultWT = wt
		return m, tea.Quit
	case "remove":
		// Ask for confirmation before a destructive removal (especially since
		// dirty worktrees are force-removed).
		m.view = viewList
		m.confirm = "remove"
		return m, nil
	}
	return m, nil
}

// doRemove performs the actual worktree removal after confirmation.
func (m *model) doRemove() (tea.Model, tea.Cmd) {
	wt := m.selected()
	if wt == nil {
		return m, nil
	}
	if wt.Current {
		m.setErr("refusing to remove the worktree you're currently in")
		return m, nil
	}
	force := wt.Dirty
	if err := git.Remove(m.dir, wt.Path, force); err != nil {
		m.setErr(err.Error())
		return m, nil
	}
	note := ""
	if force {
		note = " (forced, discarded changes)"
	}
	m.removeFromAll(wt.Path)
	m.setMsg(fmt.Sprintf("removed %s%s", wt.Label(), note))
	return m, nil
}

func (m *model) removeFromAll(path string) {
	for i := range m.all {
		if m.all[i].Path == path {
			m.all = append(m.all[:i], m.all[i+1:]...)
			break
		}
	}
	m.recompute()
}

func (m *model) setMsg(s string) { m.message, m.isErr = s, false }
func (m *model) setErr(s string) { m.message, m.isErr = s, true }

func (m *model) View() string {
	if m.quit {
		return ""
	}
	var raw string
	if m.view == viewActions {
		raw = m.viewActionsRender()
	} else {
		raw = m.viewListRender()
	}
	return m.clampFrame(raw)
}

// clampFrame guarantees the rendered frame never exceeds the terminal size:
// every line is hard-truncated to the terminal width, and the number of lines
// is capped to the terminal height. This prevents soft-wrapping (which
// desyncs Bubble Tea's line accounting and produces garbage frames on resize).
func (m *model) clampFrame(s string) string {
	lines := strings.Split(s, "\n")
	if m.height > 0 && len(lines) > m.height {
		lines = lines[:m.height]
	}
	if m.width > 0 {
		for i, ln := range lines {
			if lipgloss.Width(ln) > m.width {
				// ansi.Truncate respects escape sequences (won't cut mid-code).
				lines[i] = ansi.Truncate(ln, m.width, "")
			}
		}
	}
	return strings.Join(lines, "\n")
}

// layout describes the computed column widths for the current terminal size.
type layout struct {
	branchW  int
	statusW  int
	pathW    int  // 0 means the PATH column is dropped
	showPath bool
}

// Fixed spacing: "> " row prefix (2) + two 2-space column separators (4).
const rowOverhead = 2 + 2 + 2

// computeLayout allocates horizontal space with this priority:
//  1. STATUS is never truncated (short, known values).
//  2. PATH shrinks first, and is dropped entirely if too little room remains.
//  3. BRANCH is truncated only as a last resort, when STATUS + a minimal BRANCH
//     don't even fit.
func (m *model) computeLayout() layout {
	// Natural widths from content.
	branchNat := lipgloss.Width("BRANCH")
	statusNat := lipgloss.Width("STATUS")
	pathNat := lipgloss.Width("PATH")
	for _, r := range m.rows {
		if w := lipgloss.Width(r.wt.DisplayLabel()); w > branchNat {
			branchNat = w
		}
		if w := lipgloss.Width(plainStatus(r.wt)); w > statusNat {
			statusNat = w
		}
		if w := lipgloss.Width(abbrevHome(r.wt.Path)); w > pathNat {
			pathNat = w
		}
	}

	avail := m.width - rowOverhead
	if avail < 1 {
		avail = 1
	}

	// STATUS is protected from truncation relative to the other columns, but at
	// pathologically narrow widths (STATUS alone wider than the terminal) even
	// it must yield; reserve at least minBranch for the branch column.
	const minBranch = 6
	statusW := statusNat
	if statusW > avail-minBranch {
		statusW = max(1, avail-minBranch)
	}
	l := layout{statusW: statusW}

	// Space left for BRANCH + PATH after STATUS.
	rest := avail - statusW
	if rest < 0 {
		rest = 0
	}

	// Last resort: not even STATUS + a minimal branch fits -> truncate BRANCH.
	if rest < minBranch {
		l.branchW = max(1, rest)
		l.showPath = false
		return l
	}

	// Try to give BRANCH its natural width, capped so a single huge branch
	// can't starve PATH.
	branchW := branchNat
	if branchW > 40 {
		branchW = 40
	}
	if branchW > rest {
		branchW = rest
	}
	l.branchW = branchW

	// PATH gets whatever remains. Drop it if there's too little to be useful.
	const minPath = 12
	pathW := rest - branchW
	if pathW >= minPath {
		l.showPath = true
		l.pathW = min(pathW, pathNat)
	}
	return l
}

func (m *model) viewListRender() string {
	var b strings.Builder
	lay := m.computeLayout()

	// Title + count.
	title := fmt.Sprintf("git worktrees (%d)", len(m.all))
	if m.filter != "" {
		title += fmt.Sprintf("  —  %d match", len(m.rows))
	}
	b.WriteString(titleStyle.Render(title) + "\n")

	// Filter/search line.
	switch {
	case m.searching:
		// Show a cursor block while actively typing the query.
		b.WriteString(filterStyle.Render("/"+m.filter+"▏") + "\n")
	case m.filter != "":
		b.WriteString(filterStyle.Render("/"+m.filter) + helpStyle.Render("  (filtered)") + "\n")
	default:
		b.WriteString(helpStyle.Render("press / to search") + "\n")
	}

	// Header. Two leading spaces align with the "> " / "  " row prefix.
	header := "  " + pad(trunc("BRANCH", lay.branchW), lay.branchW) + "  " + pad("STATUS", lay.statusW)
	if lay.showPath {
		header += "  PATH"
	}
	b.WriteString(headerStyle.Render(header) + "\n")

	if len(m.rows) == 0 {
		b.WriteString(pathStyle.Render("  (no matches)") + "\n")
	}

	vis := m.visibleRows()
	end := m.offset + vis
	if end > len(m.rows) {
		end = len(m.rows)
	}
	for i := m.offset; i < end; i++ {
		b.WriteString(m.renderRow(m.rows[i], i == m.cursor, lay) + "\n")
	}

	// Scroll indicator.
	if len(m.rows) > vis {
		b.WriteString(helpStyle.Render(fmt.Sprintf("  [%d-%d of %d]", m.offset+1, end, len(m.rows))) + "\n")
	}

	// Confirmation prompt takes over the footer when active.
	if m.confirm == "remove" {
		wt := m.selected()
		name := ""
		if wt != nil {
			name = wt.Label()
		}
		warn := ""
		if wt != nil && wt.Dirty {
			warn = errorStyle.Render(" [has uncommitted changes!]")
		}
		b.WriteString("\n" + errorStyle.Render(fmt.Sprintf("remove %s?", name)) + warn +
			helpStyle.Render("  (y/n)"))
		return b.String()
	}

	var help string
	if m.searching {
		help = "type to filter · enter/esc done"
	} else {
		help = "j/k move · enter cd · o open · a actions · r remove · / search · q quit"
	}
	b.WriteString("\n" + m.footer(help))
	return b.String()
}

func (m *model) viewActionsRender() string {
	wt := m.selected()
	var b strings.Builder
	b.WriteString(titleStyle.Render("actions: "+wt.Label()) + "\n")
	b.WriteString(pathStyle.Render(wt.Path) + "\n\n")
	for i, a := range actions {
		label := fmt.Sprintf("  [%s] %s", a.key, a.label)
		if i == m.actionCur {
			label = selectedStyle.Render(fmt.Sprintf("> [%s] %s", a.key, a.label))
		}
		b.WriteString(label + "\n")
	}
	b.WriteString("\n" + m.footer("↑/↓ move · enter run · esc back"))
	return b.String()
}

func (m *model) footer(help string) string {
	var b strings.Builder
	if m.message != "" {
		if m.isErr {
			b.WriteString(errorStyle.Render("✗ "+m.message) + "\n")
		} else {
			b.WriteString(cleanStyle.Render("✓ "+m.message) + "\n")
		}
	}
	b.WriteString(helpStyle.Render(help))
	return b.String()
}

// renderRow renders one table row. The selected row is drawn as plain text
// (no per-cell color) wrapped in reverse video, which stays readable on any
// theme. Non-selected rows get per-cell semantic coloring. Both share the same
// widths from the layout, so columns always align and never overflow.
func (m *model) renderRow(r row, selected bool, lay layout) string {
	label := trunc(r.wt.DisplayLabel(), lay.branchW)
	status := trunc(plainStatus(r.wt), lay.statusW)
	labelW := lipgloss.Width(label)
	statusFits := status == plainStatus(r.wt)

	if selected {
		line := "> " + pad(label, lay.branchW) + "  " + pad(status, lay.statusW)
		if lay.showPath {
			line += "  " + truncPath(r.wt.Path, lay.pathW)
		}
		return selectedStyle.Render(line)
	}

	// Colored cells, padded to width using plain-text display width. When the
	// label was truncated we can't recolor by original match indices, so fall
	// back to plain (still current-colored) rendering. Colored status only when
	// it wasn't clipped (clipping breaks per-tag coloring).
	branchCell := colorLabelTrunc(r, label) + strings.Repeat(" ", max(0, lay.branchW-labelW))
	statusColored := coloredStatus(r.wt)
	if !statusFits {
		statusColored = status
	}
	statusCell := statusColored + strings.Repeat(" ", max(0, lay.statusW-lipgloss.Width(status)))
	out := "  " + branchCell + "  " + statusCell
	if lay.showPath {
		out += "  " + pathStyle.Render(truncPath(r.wt.Path, lay.pathW))
	}
	return out
}

// colorLabelTrunc colors a (possibly truncated) label: cyan for the current
// worktree, otherwise fuzzy-match highlighting when the label is intact.
func colorLabelTrunc(r row, truncated string) string {
	if r.wt.Current {
		return currentStyle.Render(truncated)
	}
	// If truncation changed the text, match indices no longer line up.
	if truncated != r.wt.DisplayLabel() {
		return truncated
	}
	return highlight(r.wt.Label(), r.matched)
}

// trunc right-truncates s to a display width, appending "…" when clipped.
func trunc(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r))+1 > width {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}

// abbrevHome replaces the user's home directory prefix with "~".
func abbrevHome(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if p == home {
		return "~"
	}
	if strings.HasPrefix(p, home+string(os.PathSeparator)) {
		return "~" + p[len(home):]
	}
	return p
}

// pad right-pads s with spaces to the given display width.
func pad(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

// plainStatus is the uncolored status string, used for width calculations.
func plainStatus(w git.Worktree) string { return w.StatusString() }

// coloredStatus renders status tags with per-tag colors.
func coloredStatus(w git.Worktree) string {
	tags := w.StatusTags()
	parts := make([]string, len(tags))
	for i, t := range tags {
		switch {
		case t == "dirty":
			parts[i] = dirtyStyle.Render(t)
		case t == "unmerged":
			parts[i] = unmergedStyle.Render(t)
		case t == "clean":
			parts[i] = cleanStyle.Render(t)
		default:
			parts[i] = pathStyle.Render(t)
		}
	}
	return strings.Join(parts, ", ")
}

// truncPath abbreviates $HOME to ~ and left-ellipsizes to fit width, keeping
// the tail (the worktree's own directory name) visible.
func truncPath(p string, width int) string {
	p = abbrevHome(p)
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(p) <= width {
		return p
	}
	r := []rune(p)
	for len(r) > 0 && lipgloss.Width(string(r))+1 > width {
		r = r[1:]
	}
	return "…" + string(r)
}

// highlight bolds the matched rune positions in s.
func highlight(s string, matched []int) string {
	if len(matched) == 0 {
		return s
	}
	set := make(map[int]bool, len(matched))
	for _, i := range matched {
		set[i] = true
	}
	var b strings.Builder
	for i, r := range []rune(s) {
		if set[i] {
			b.WriteString(matchStyle.Render(string(r)))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func printStatus(w git.Worktree) {
	fmt.Printf("%s\n  path:     %s\n", w.Label(), w.Path)
	if !w.Bare {
		fmt.Printf("  dirty:    %t\n  unmerged: %t\n", w.Dirty, w.Unmerged)
		if w.HasUpstream {
			fmt.Printf("  ahead:    %d\n  behind:   %d\n", w.Ahead, w.Behind)
		}
	}
}
