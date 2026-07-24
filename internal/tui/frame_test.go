package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestClampFrameNeverExceedsTerminal(t *testing.T) {
	initStyles(lipgloss.DefaultRenderer())
	for _, dim := range []struct{ w, h int }{
		{200, 50}, {120, 30}, {80, 24}, {50, 12}, {30, 8}, {15, 5}, {8, 3},
	} {
		m := &model{width: dim.w, height: dim.h, rows: rowsFixture()}
		frame := m.clampFrame(m.viewListRender())
		lines := strings.Split(frame, "\n")
		if len(lines) > dim.h {
			t.Errorf("w=%d h=%d: %d lines exceeds height", dim.w, dim.h, len(lines))
		}
		for i, ln := range lines {
			if w := lipgloss.Width(ln); w > dim.w {
				t.Errorf("w=%d h=%d: line %d width %d exceeds terminal width: %q", dim.w, dim.h, i, w, ln)
			}
		}
	}
}
