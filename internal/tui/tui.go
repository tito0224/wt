// Package tui provides an interactive worktree picker rendered as a scrollable,
// fuzzy-filterable table with an action submenu.
//
// The code is organized by concern:
//
//   - tui.go            the model, message types, Run, and the Update/View dispatch
//   - styles.go         lipgloss styles
//   - spinner.go        the busy/loading spinner
//   - commands.go       async git effects and their result reducers
//   - render.go         shared rendering + layout helpers
//   - view_*.go         per-view update + render (list, actions, create, loading)
package tui

import (
	"fmt"
	"os"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/johnlotito/wt/internal/git"
)

type view int

const (
	viewList view = iota
	viewActions
	viewCreate
)

// loadResultMsg is delivered when the initial (or a refreshed) worktree
// enumeration finishes.
type loadResultMsg struct {
	wts []git.Worktree
	err error
}

// createResultMsg is delivered when an async worktree creation finishes. On
// success it carries the refreshed worktree set and the new worktree's path.
type createResultMsg struct {
	branch string
	path   string
	wts    []git.Worktree
	err    error
}

// removeResultMsg is delivered when an async worktree removal finishes.
type removeResultMsg struct {
	label string
	path  string
	force bool
	err   error
}

// row is a worktree paired with its fuzzy-match metadata for the current
// filter query.
type row struct {
	wt      git.Worktree
	score   int   // fuzzy-match score; only meaningful when a filter is active
	matched []int // matched rune indices in the label, for highlighting
}

// listState is the list screen's own state: the fuzzy filter and whether the
// search input is active.
type listState struct {
	filter    string
	searching bool // true while in search-input mode (entered with '/')
}

// menuState is the action submenu's state: the highlighted action and, when
// non-empty, an active y/n confirmation prompt for that action.
type menuState struct {
	cursor  int
	confirm string // non-empty shows a y/n confirmation prompt for this action
}

// createState is the create screen's state: the worktree name being typed.
type createState struct {
	buf string
}

// spinnerState is the cross-cutting busy/loading indicator state. active is
// true during an async git operation (create/remove); loading is true during
// the initial worktree enumeration. Either animates the spinner; msg labels an
// active operation.
type spinnerState struct {
	active  bool
	loading bool
	msg     string
	frame   int
}

type model struct {
	dir          string
	all          []git.Worktree // full, unfiltered set
	terminalName string

	view view
	rows []row // filtered view of `all`

	list    listState
	menu    menuState
	create  createState
	spinner spinnerState

	cursor int // index into rows
	offset int // scroll offset (first visible row)

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
func Run(dir string, terminalName string) error {
	// Start empty and in the loading state; Init kicks off async enumeration.
	m := &model{
		dir:          dir,
		terminalName: terminalName,
		width:        80,
		height:       24,
		spinner:      spinnerState{loading: true},
	}
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

func (m *model) Init() tea.Cmd {
	// Begin animating the spinner and enumerating worktrees concurrently.
	return tea.Batch(spinTick(), loadCmd(m.dir))
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clampScroll()
		// Force a full repaint so shrinking the terminal can't leave stale
		// lines from the previous (taller/wider) frame on screen.
		return m, tea.ClearScreen
	case spinTickMsg:
		// Keep animating only while some git operation is still in flight.
		if !m.spinner.active && !m.spinner.loading {
			return m, nil
		}
		m.spinner.frame = (m.spinner.frame + 1) % len(spinnerFrames)
		return m, spinTick()
	case loadResultMsg:
		return m.handleLoadResult(msg)
	case createResultMsg:
		return m.handleCreateResult(msg)
	case removeResultMsg:
		return m.handleRemoveResult(msg)
	case tea.KeyMsg:
		// While loading or a git operation is running, swallow input except
		// Ctrl-C so the spinner isn't interrupted and no conflicting action
		// starts.
		if m.spinner.active || m.spinner.loading {
			if msg.Type == tea.KeyCtrlC {
				m.quit = true
				return m, tea.Quit
			}
			return m, nil
		}
		if m.menu.confirm != "" {
			return m.updateConfirm(msg)
		}
		if m.view == viewCreate {
			return m.updateCreate(msg)
		}
		if m.view == viewActions {
			return m.updateActions(msg)
		}
		return m.updateList(msg)
	}
	return m, nil
}

func (m *model) View() string {
	if m.quit {
		return ""
	}
	if m.spinner.loading {
		return m.clampFrame(m.viewLoadingRender())
	}
	var raw string
	switch m.view {
	case viewActions:
		raw = m.viewActionsRender()
	case viewCreate:
		raw = m.viewCreateRender()
	default:
		raw = m.viewListRender()
	}
	return m.clampFrame(raw)
}

// recompute rebuilds the filtered rows from the current filter. Rows keep
// git's native order (repo root first, as git lists it). When a filter is
// active, matches are sorted by fuzzy score (best match first).
func (m *model) recompute() {
	q := strings.ToLower(strings.TrimSpace(m.list.filter))
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

// selectByPath moves the cursor to the row whose worktree path matches, if
// present, scrolling it into view.
func (m *model) selectByPath(path string) {
	for i := range m.rows {
		if m.rows[i].wt.Path == path {
			m.cursor = i
			m.clampScroll()
			return
		}
	}
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
