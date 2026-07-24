package tui

import (
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

// newSpinner builds the bubbles spinner used for busy/loading states. MiniDot
// is the braille dot spinner; its style picks up the title color.
func newSpinner() spinner.Model {
	s := spinner.New()
	s.Spinner = spinner.MiniDot
	s.Style = titleStyle
	return s
}

// startBusy puts the model into the busy state with the given label and returns
// the command that begins animating the spinner.
func (m *model) startBusy(label string) tea.Cmd {
	m.spinner.active = true
	m.spinner.msg = label
	m.message = ""
	return m.spinner.model.Tick
}
