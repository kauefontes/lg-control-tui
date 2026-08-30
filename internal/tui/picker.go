package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"lg-control-tui/internal/ddc"
)

// selectDisplay commits to controlling m.displays[idx] and (re)starts the
// probe for it. Used both for the very first pick (multiple displays found
// at startup) and for switching displays later via 'D' — either way, every
// bit of state from whatever was previously being controlled has to be
// dropped, not just left to be silently overwritten by the new probe.
func (m Model) selectDisplay(idx int) (Model, tea.Cmd) {
	m.selected = idx
	m.displayChosen = true
	m.screen = screenControls
	m.probing = true
	m.caps = nil
	m.probeErr = nil
	m.sliders = nil
	m.selectors = nil
	m.actions = nil
	m.order = nil
	m.cursor = 0
	m.opErr = nil
	m.pending = nil
	m.confirming = false
	m.rawReady = false
	m.rawLoading = false
	m.rawErr = nil
	m.rawReadings = nil
	m.rawCursor = 0
	return m, probeCmd(m.displays[idx])
}

// currentDisplay returns whichever display is currently selected, falling
// back to the first one if the selection is out of range (e.g. a display
// vanished between refreshes).
func (m Model) currentDisplay() (ddc.Display, bool) {
	if len(m.displays) == 0 {
		return ddc.Display{}, false
	}
	idx := m.selected
	if idx < 0 || idx >= len(m.displays) {
		idx = 0
	}
	return m.displays[idx], true
}

func (m Model) pickerView(title string) string {
	body := dimStyle.Render(fmt.Sprintf("%d DDC/CI displays found — choose one to control:", len(m.displays))) + "\n\n"

	for i, d := range m.displays {
		label := d.MfgID
		if d.Model != "" {
			label += " " + d.Model
		}
		label += fmt.Sprintf(" (%s, VCP %s)", d.I2CBus, d.VCPVersion)

		cursor := "  "
		line := dimStyle.Render(label)
		if i == m.pickerCursor {
			cursor = "▸ "
			line = rawFocusedStyle.Render(label)
		}
		body += cursor + line + "\n"
	}

	help := dimStyle.Render("↑↓ navigate · enter select · q quit")
	content := title + "\n\n" + body + "\n" + help
	return boxStyle.Render(content)
}
