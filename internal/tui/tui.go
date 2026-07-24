// Package tui provides an interactive worktree picker rendered as a scrollable,
// fuzzy-filterable table, with a per-worktree status bar and a full details
// screen.
//
// The code is organized by concern:
//
//   - tui.go            the model, message types, Run, and the Update/View dispatch
//   - styles.go         lipgloss styles
//   - spinner.go        the busy/loading spinner
//   - statusbar.go      the selection status bar + key bar
//   - detail.go         async loading of per-worktree detail (commits, PR)
//   - commands.go       async git effects and their result reducers
//   - render.go         shared rendering + layout helpers
//   - view_*.go         per-view update + render (list, detail, create, loading)
package tui

import (
	"fmt"
	"os"
	"runtime/debug"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/johnlotito/wt/internal/forge"
	"github.com/johnlotito/wt/internal/git"
	"github.com/johnlotito/wt/internal/ops"
)

type view int

const (
	viewList view = iota
	viewDetail
	viewCreate
	viewHelp
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

// removeResultMsg is delivered when an async worktree removal finishes. result
// carries the typed outcome (branch deleted / left unmerged); err is the
// removal error (nil on success).
type removeResultMsg struct {
	label  string
	path   string
	force  bool
	result ops.RemoveResult
	err    error
}

// branchDeleteResultMsg is delivered when a standalone (forced) branch deletion
// finishes — used for the force-delete follow-up after an unmerged refusal.
type branchDeleteResultMsg struct {
	branch string
	err    error
}

// detailDebounceMsg fires after a short delay following a selection change. It
// carries the generation it was scheduled for; if the selection has moved on
// (gen advanced) it is ignored, so rapid j/k scrolling only loads the row the
// user lands on.
type detailDebounceMsg struct {
	gen int
}

// detailResultMsg carries the loaded detail for a worktree. path identifies
// which worktree it belongs to (the cursor may have moved); gen guards against
// stale results.
type detailResultMsg struct {
	gen    int
	path   string
	detail detail
}

// detail holds the extra, lazily-loaded information shown for the highlighted
// worktree: a one-line summary in the status bar (last commit + PR) and, on the
// detail screen, the working-tree status and recent commit log.
type detail struct {
	commits   []git.Commit // most-recent first; commits[0] is HEAD
	commitErr error

	status    git.WorktreeStatus
	statusErr error

	pr        *forge.PR
	prChecked bool // true once a forge lookup completed (found or definitively none)
	// postMerge counts commits on the branch made after the PR merged (only
	// meaningful when pr.State == MERGED); >0 means genuinely-unmerged new work.
	postMerge int
}

// last returns the HEAD commit, or a zero Commit when the log is empty.
func (d detail) last() git.Commit {
	if len(d.commits) == 0 {
		return git.Commit{}
	}
	return d.commits[0]
}

// row is a worktree paired with its fuzzy-match metadata for the current
// filter query.
type row struct {
	wt      git.Worktree
	score   int   // fuzzy-match score; only meaningful when a filter is active
	matched []int // matched rune indices in the label, for highlighting
}

// listState is the list screen's own state: the fuzzy filter, whether the
// search input is active, and any pending destructive-action confirmation
// ("remove" for the initial y/b/n prompt, "force-branch" for the follow-up
// unmerged-branch force-delete prompt).
type listState struct {
	filter    string
	searching bool   // true while in search-input mode (entered with '/')
	confirm   string // non-empty shows a confirmation prompt for this action
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

	view       view
	helpReturn view  // view to restore when leaving the help overlay
	rows       []row // filtered view of `all`

	list    listState
	create  createState
	spinner spinnerState

	// pendingBranch is the branch awaiting a force-delete decision after a
	// worktree removal whose safe branch delete was refused (unmerged).
	pendingBranch string

	// details caches loaded status-bar info per worktree path. detailGen is
	// bumped on every selection change to debounce and discard stale async
	// loads (see detailDebounceMsg/detailResultMsg).
	details   map[string]detail
	detailGen int

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
		details:      map[string]detail{},
	}
	m.recompute()

	// Disable Bubble Tea's built-in panic catching so a panic (from the update
	// loop or any of our tea.Cmd goroutines) propagates. We recover it below to
	// restore the terminal and print the full stack to stderr, then re-panic so
	// the process crashes with a complete, visible trace.
	opts := []tea.ProgramOption{tea.WithAltScreen(), tea.WithoutCatchPanics()}
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

	p := tea.NewProgram(m, opts...)
	defer func() {
		if r := recover(); r != nil {
			// Best-effort: put the terminal back before dumping the trace so
			// the output is readable and the shell isn't left in raw mode.
			_ = p.RestoreTerminal()
			fmt.Fprintf(os.Stderr, "wt: panic: %v\n\n%s", r, debug.Stack())
			os.Exit(2)
		}
	}()

	res, err := p.Run()
	if err != nil {
		return err
	}
	fm := res.(*model)
	if fm.resultWT == nil {
		return nil
	}
	if fm.resultAction == "cd" {
		fmt.Println(fm.resultWT.Path)
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
	case branchDeleteResultMsg:
		return m.handleBranchDeleteResult(msg)
	case detailDebounceMsg:
		return m.handleDetailDebounce(msg)
	case detailResultMsg:
		return m.handleDetailResult(msg)
	case execDoneMsg:
		if msg.err != nil {
			m.setErr(msg.err.Error())
		}
		// Force a repaint: the external program scribbled over the alt-screen.
		return m, tea.ClearScreen
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
		if m.list.confirm != "" {
			return m.updateConfirm(msg)
		}
		if m.view == viewHelp {
			return m.updateHelp(msg)
		}
		if m.view == viewCreate {
			return m.updateCreate(msg)
		}
		if m.view == viewDetail {
			return m.updateDetail(msg)
		}
		// Track the selected worktree across the key handler so a changed
		// selection can (debounced) load its status-bar detail.
		before := m.selectedPath()
		newM, cmd := m.updateList(msg)
		if mm, ok := newM.(*model); ok && mm.selectedPath() != before {
			return mm, tea.Batch(cmd, mm.scheduleDetail())
		}
		return newM, cmd
	}
	return m, nil
}

// selectedPath returns the highlighted worktree's path, or "" when none.
func (m *model) selectedPath() string {
	if wt := m.selected(); wt != nil {
		return wt.Path
	}
	return ""
}

// selectedPR returns the loaded PR for the highlighted worktree, or nil when
// none is known (not loaded, no forge, or no PR).
func (m *model) selectedPR() *forge.PR {
	wt := m.selected()
	if wt == nil {
		return nil
	}
	if d, ok := m.details[wt.Path]; ok {
		return d.pr
	}
	return nil
}

// selectedDirty reports whether the highlighted worktree has uncommitted
// changes (so diff actions apply).
func (m *model) selectedDirty() bool {
	wt := m.selected()
	return wt != nil && wt.Dirty
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
	case viewDetail:
		raw = m.viewDetailRender()
	case viewCreate:
		raw = m.viewCreateRender()
	case viewHelp:
		raw = m.viewHelpRender()
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
// space for the surrounding chrome:
//
//	title(1) + filter(1) + header(1) + scroll(1)
//	+ statusbar(blank + up to 3 lines = 4) + footer(blank + keybar = 2)
//
// which is 10 lines. When the terminal is very short, at least one row is shown.
func (m *model) visibleRows() int {
	const chrome = 10
	n := m.height - chrome
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
