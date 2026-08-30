package components

import "fmt"

// Action is a write-only, non-continuous VCP command — there's nothing to
// read back, just a code to trigger (e.g. "Restore factory defaults").
type Action struct {
	Code uint8
	Name string
}

func (a Action) View(focused bool) string {
	cursor := "  "
	label := fmt.Sprintf("[ %s ]", a.Name)
	if focused {
		cursor = "▸ "
		label = focusedNameStyle.Render(label)
	} else {
		label = nameStyle.Render(label)
	}
	return cursor + label
}
