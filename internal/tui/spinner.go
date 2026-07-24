package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// spinnerFrames is a simple braille spinner animated via tea.Tick.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

const spinnerInterval = 80 * time.Millisecond

// spinTickMsg advances the spinner animation.
type spinTickMsg struct{}

// spinTick schedules the next spinner animation frame.
func spinTick() tea.Cmd {
	return tea.Tick(spinnerInterval, func(time.Time) tea.Msg { return spinTickMsg{} })
}

// startBusy puts the model into the busy state with the given label and
// returns the command that begins animating the spinner.
func (m *model) startBusy(label string) tea.Cmd {
	m.spinner.active = true
	m.spinner.msg = label
	m.spinner.frame = 0
	m.message = ""
	return spinTick()
}
