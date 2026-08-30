package components

import "fmt"

// Option is one named value a Selector can take, e.g. {Code: 0x11, Name:
// "HDMI-1"}.
type Option struct {
	Code uint8
	Name string
}

// Selector is a non-continuous VCP control with a monitor-declared, fully
// named set of values, e.g. Input Source or Color Preset.
type Selector struct {
	Code     uint8
	Name     string
	Options  []Option
	Selected uint8 // current raw value; may not be among Options — see indexOf
}

func NewSelector(code uint8, name string, options []Option, current uint8) Selector {
	return Selector{Code: code, Name: name, Options: options, Selected: current}
}

func (s Selector) indexOf(code uint8) int {
	for i, o := range s.Options {
		if o.Code == code {
			return i
		}
	}
	return -1
}

// NextOption returns the option code the selector would move to for the
// given direction (+1 or -1), wrapping around. If the monitor's current
// value isn't among the declared options — this happens for real, e.g.
// this project's LG panel reports Input Source as 0x00 sometimes, a value
// it never advertised — cycling lands on the first option rather than
// guessing an offset from an unknown position.
func (s Selector) NextOption(direction int) uint8 {
	if len(s.Options) == 0 {
		return s.Selected
	}
	idx := s.indexOf(s.Selected)
	if idx == -1 {
		return s.Options[0].Code
	}
	idx = (idx + direction + len(s.Options)) % len(s.Options)
	return s.Options[idx].Code
}

func (s Selector) currentName() string {
	if idx := s.indexOf(s.Selected); idx != -1 {
		return s.Options[idx].Name
	}
	return fmt.Sprintf("unknown (0x%02X)", s.Selected)
}

func (s Selector) View(focused bool) string {
	cursor := "  "
	name := nameStyle.Render(fmt.Sprintf("%-18s", s.Name))
	if focused {
		cursor = "▸ "
		name = focusedNameStyle.Render(fmt.Sprintf("%-18s", s.Name))
	}
	return fmt.Sprintf("%s%s ‹ %s ›", cursor, name, s.currentName())
}
