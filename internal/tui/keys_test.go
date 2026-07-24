package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
