// Package components holds small reusable pieces of the TUI that aren't
// covered by Bubbles (a DDC/CI brightness-style slider isn't really a
// "progress" bar, so we roll our own).
package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const barWidth = 20

var (
	filledStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
	emptyStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	nameStyle        = lipgloss.NewStyle()
	focusedNameStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
)

// Slider is a continuous (0..Max) VCP control rendered as a bar, e.g.
// Brightness, Contrast, RGB gain, Volume.
type Slider struct {
	Code  uint8
	Name  string
	Value int
	Max   int
	Step  int
}

// NewSlider builds a slider with a step size scaled to its range.
func NewSlider(code uint8, name string, value, max int) Slider {
	step := 5
	if max <= 20 {
		step = 1
	}
	return Slider{Code: code, Name: name, Value: value, Max: max, Step: step}
}

func (s Slider) View(focused bool) string {
	filled := 0
	if s.Max > 0 {
		filled = s.Value * barWidth / s.Max
	}
	if filled > barWidth {
		filled = barWidth
	}
	bar := filledStyle.Render(strings.Repeat("█", filled)) +
		emptyStyle.Render(strings.Repeat("░", barWidth-filled))

	cursor := "  "
	name := nameStyle.Render(fmt.Sprintf("%-18s", s.Name))
	if focused {
		cursor = "▸ "
		name = focusedNameStyle.Render(fmt.Sprintf("%-18s", s.Name))
	}

	return fmt.Sprintf("%s%s [%s] %3d", cursor, name, bar, s.Value)
}
