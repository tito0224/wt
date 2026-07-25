package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tito0224/wt/internal/git"
)

func runeKey(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

func TestSearchModeToggle(t *testing.T) {
	initStyles(lipgloss.DefaultRenderer())
	m := &model{width: 80, height: 24, rows: rowsFixture()}

	// '/' enters search mode.
	m.updateNormal(runeKey('/'))
	if !m.list.searching {
		t.Fatal("expected searching=true after '/'")
	}
	// typing builds filter
	m.updateSearch(runeKey('f'))
	m.updateSearch(runeKey('i'))
	if m.list.filter != "fi" {
		t.Fatalf("filter=%q want fi", m.list.filter)
	}
	// esc exits search but keeps filter
	m.updateSearch(tea.KeyMsg{Type: tea.KeyEsc})
	if m.list.searching {
		t.Fatal("expected searching=false after esc")
	}
	if m.list.filter != "fi" {
		t.Fatalf("filter cleared unexpectedly: %q", m.list.filter)
	}
	// esc in normal mode clears filter
	m.updateNormal(tea.KeyMsg{Type: tea.KeyEsc})
	if m.list.filter != "" {
		t.Fatalf("expected filter cleared, got %q", m.list.filter)
	}
}

// TestSearchSpaceInsertsSingleSpace is a regression test: Bubble Tea delivers
// KeySpace with Runes=[]rune{' '}, and the old handler appended the runes AND
// a literal space, doubling every space typed into the filter.
func TestSearchSpaceInsertsSingleSpace(t *testing.T) {
	initStyles(lipgloss.DefaultRenderer())
	m := &model{width: 80, height: 24, rows: rowsFixture()}
	m.list.searching = true

	m.updateSearch(runeKey('a'))
	// A space press exactly as Bubble Tea's input parser delivers it.
	m.updateSearch(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	if m.list.filter != "a " {
		t.Fatalf("filter=%q, want %q (one space press must add exactly one space)", m.list.filter, "a ")
	}
	// Defensive: a hypothetical rune-less KeySpace must still add one space.
	m.updateSearch(tea.KeyMsg{Type: tea.KeySpace})
	if m.list.filter != "a  " {
		t.Fatalf("filter=%q, want %q after rune-less KeySpace", m.list.filter, "a  ")
	}
}

// TestRefreshClearsCacheAndReselects covers the R (refresh) flow: the detail
// cache is dropped and the busy spinner starts (the list stays visible — no
// full loading screen), and when the reload lands the cursor is restored to
// the previously-selected worktree even if its index shifted.
func TestRefreshClearsCacheAndReselects(t *testing.T) {
	initStyles(lipgloss.DefaultRenderer())
	m := &model{
		width: 80, height: 24,
		all: []git.Worktree{
			{Path: "/r/root", Branch: "root", Current: true},
			{Path: "/r/fix", Branch: "fix"},
		},
		details: map[string]detail{"/r/fix": {}},
	}
	m.spinner.model = newSpinner()
	m.recompute()
	m.selectByPath("/r/fix")

	m.updateNormal(runeKey('R'))
	if len(m.details) != 0 {
		t.Fatalf("details cache not cleared: %d entries", len(m.details))
	}
	if !m.spinner.active {
		t.Fatal("expected busy spinner active after R")
	}
	if m.spinner.loading {
		t.Fatal("refresh must not enter the full loading screen")
	}
	if m.pendingSelect != "/r/fix" {
		t.Fatalf("pendingSelect=%q, want /r/fix", m.pendingSelect)
	}

	// Reload arrives with a new worktree inserted before the remembered one,
	// shifting its index from 1 to 2.
	m.handleLoadResult(loadResultMsg{wts: []git.Worktree{
		{Path: "/r/root", Branch: "root", Current: true},
		{Path: "/r/new", Branch: "new"},
		{Path: "/r/fix", Branch: "fix"},
	}})
	if m.spinner.active {
		t.Fatal("expected busy spinner cleared after load result")
	}
	if m.spinner.msg != "" {
		t.Fatalf("spinner msg not cleared: %q", m.spinner.msg)
	}
	if got := m.selectedPath(); got != "/r/fix" {
		t.Fatalf("selected=%q, want /r/fix re-selected after refresh", got)
	}
	if m.pendingSelect != "" {
		t.Fatalf("pendingSelect not consumed: %q", m.pendingSelect)
	}
}

// TestYankCopiesSelectedPath pins the y key: it copies the selected
// worktree's path via the (stubbed) clipboard and reports success; in
// remove-confirm mode y still means "confirm" (handled before normal keys).
func TestYankCopiesSelectedPath(t *testing.T) {
	initStyles(lipgloss.DefaultRenderer())
	var copied string
	old := clipboardWrite
	clipboardWrite = func(s string) error { copied = s; return nil }
	defer func() { clipboardWrite = old }()

	m := &model{width: 80, height: 24, rows: rowsFixture()}
	m.updateNormal(runeKey('y'))
	if want := m.rows[0].wt.Path; copied != want {
		t.Fatalf("copied %q, want selected path %q", copied, want)
	}
	if m.isErr || m.message == "" {
		t.Fatalf("expected success message, got isErr=%v message=%q", m.isErr, m.message)
	}
}

// TestPruneBannerAndKey pins the prune surfacing: a banner appears when (and
// only when) stale entries exist, P starts the async prune, and P with
// nothing to prune is an inline no-op.
func TestPruneBannerAndKey(t *testing.T) {
	initStyles(lipgloss.DefaultRenderer())

	// No prunable entries: no banner, P is a no-op message.
	m := &model{width: 80, height: 24, rows: rowsFixture()}
	if frame := m.viewListRender(); strings.Contains(frame, "prunable entr") {
		t.Error("banner shown with nothing prunable")
	}
	m.updateNormal(runeKey('P'))
	if m.spinner.active {
		t.Error("P with nothing prunable must not start a prune")
	}

	// With a prunable entry: banner + P starts the busy prune.
	m = &model{width: 80, height: 24}
	m.all = []git.Worktree{
		{Path: "/r/main", Branch: "main", Current: true},
		{Path: "/r/gone", Branch: "gone", Prunable: true},
	}
	m.recompute()
	if frame := m.viewListRender(); !strings.Contains(frame, "1 prunable entry") {
		t.Errorf("banner missing:\n%s", frame)
	}
	_, cmd := m.updateNormal(runeKey('P'))
	if !m.spinner.active || cmd == nil {
		t.Errorf("P did not start the prune (busy=%v cmd=%v)", m.spinner.active, cmd)
	}
	if m.pendingMsg == "" {
		t.Error("expected a parked result message for after the reload")
	}
}

func TestNormalShortcutsNotFilter(t *testing.T) {
	initStyles(lipgloss.DefaultRenderer())
	m := &model{width: 80, height: 24, rows: rowsFixture()}
	// 'j' should move cursor, not add to filter.
	m.updateNormal(runeKey('j'))
	if m.list.filter != "" {
		t.Fatalf("'j' leaked into filter: %q", m.list.filter)
	}
	if m.selectedRow != 1 {
		t.Fatalf("cursor=%d want 1", m.selectedRow)
	}
	// 'r' triggers remove confirmation, not immediate removal.
	m.updateNormal(runeKey('r'))
	if m.list.confirm != "remove" {
		t.Fatalf("expected confirm=remove, got %q", m.list.confirm)
	}
	// 'n' cancels.
	m.updateConfirm(runeKey('n'))
	if m.list.confirm != "" {
		t.Fatal("expected confirm cleared after n")
	}
}
