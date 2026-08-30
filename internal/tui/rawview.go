package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"lg-control-tui/internal/ddc"
)

// screenKind selects which of the two screens is showing.
type screenKind int

const (
	screenControls screenKind = iota
	screenRaw
)

// rawTableHeaderLines is how many lines renderRawTable emits before the
// first feature row — used to translate a feature index into a line index
// when keeping the cursor visible in the viewport.
const rawTableHeaderLines = 2

type rawProbeMsg struct {
	readings []ddc.FeatureReading
	err      error
}

// rawProbeCmd reads every declared VCP code live, including the
// unrecognized/manufacturer-specific ones the normal startup scan skips.
// This is deliberately only triggered when the user opens the Raw VCP
// screen — it costs one i2c round-trip per code (~175ms on this project's
// monitor), so scanning all 38 takes a few seconds. That's an acceptable
// one-time cost for a screen the user opens on purpose, unlike the default
// startup path we optimized to avoid exactly this.
func rawProbeCmd(displayNum int, codes []uint8) tea.Cmd {
	return func() tea.Msg {
		result := ddc.ProbeAll(displayNum, codes)
		return rawProbeMsg{readings: result.Readings}
	}
}

// rawSingleProbeMsg carries a fresh reading for exactly one code, used to
// refresh a single row after a write from the Raw VCP edit flow instead of
// re-scanning everything.
type rawSingleProbeMsg struct {
	code    uint8
	reading ddc.FeatureReading
	err     error
}

func rawSingleProbeCmd(displayNum int, code uint8) tea.Cmd {
	return func() tea.Msg {
		reading, err := ddc.GetVCP(displayNum, code)
		return rawSingleProbeMsg{code: code, reading: reading, err: err}
	}
}

// rawSetMsg is the result of writing a value to a raw VCP code from the
// edit flow.
type rawSetMsg struct {
	code uint8
	err  error
}

// rawSetCmd writes value to code. Unrecognized/manufacturer-specific codes
// need --permit-unknown-feature, which is exactly why the edit flow forces
// its own confirmation prompt before ever reaching here — see
// ddc.SetVCPUnknown.
func rawSetCmd(displayNum int, code uint8, value int, permitUnknown bool) tea.Cmd {
	return func() tea.Msg {
		var err error
		if permitUnknown {
			err = ddc.SetVCPUnknown(displayNum, code, value)
		} else {
			err = ddc.SetVCP(displayNum, code, value)
		}
		return rawSetMsg{code: code, err: err}
	}
}

func allFeatureCodes(caps *ddc.Capabilities) []uint8 {
	codes := make([]uint8, len(caps.Features))
	for i, f := range caps.Features {
		codes[i] = f.Code
	}
	return codes
}

// renderRawTable formats every declared feature as one line: code, name,
// category (known/unknown/mfg-specific), and whatever the live reading
// looked like. Nothing gets left out here — this is the screen that
// guarantees no VCP code, understood or not, is ever inaccessible. cursor
// is the index into caps.Features currently focused; pass -1 to render
// with no row focused.
func renderRawTable(caps *ddc.Capabilities, byCode map[uint8]ddc.FeatureReading, cursor int) string {
	var b strings.Builder

	header := fmt.Sprintf("   %-4s %-34s %-12s %s", "Code", "Name", "Category", "Value")
	b.WriteString(dimStyle.Render(header) + "\n")
	b.WriteString(dimStyle.Render(strings.Repeat("─", 90)) + "\n")

	for i, f := range caps.Features {
		name := f.Name
		if len(name) > 34 {
			name = name[:31] + "..."
		}

		category := "known"
		catStyle := okStyle
		if !f.Recognized {
			catStyle = dimStyle
			if f.ManufacturerSpecific {
				category = "mfg-specific"
			} else {
				category = "unknown"
			}
		}

		r, ok := byCode[f.Code]
		row := fmt.Sprintf("%-4s %-34s %s %s",
			fmt.Sprintf("%02X", f.Code),
			name,
			catStyle.Render(fmt.Sprintf("%-12s", category)),
			rawValueString(r, ok),
		)

		cursorMark := "  "
		if i == cursor {
			cursorMark = "▸ "
			row = rawFocusedStyle.Render(row)
		}
		b.WriteString(cursorMark + row + "\n")
	}

	return b.String()
}

func rawValueString(r ddc.FeatureReading, ok bool) string {
	if !ok {
		return dimStyle.Render("(not probed)")
	}
	if !r.Readable {
		return dimStyle.Render("write-only (action)")
	}
	switch {
	case r.Continuous:
		return fmt.Sprintf("%d / %d", r.Current, r.Max)
	case r.Raw != nil:
		return fmt.Sprintf("0x%02X  (mh=%02X ml=%02X sh=%02X sl=%02X)",
			r.Current, r.Raw.Mh, r.Raw.Ml, r.Raw.Sh, r.Raw.Sl)
	case r.Generic:
		// No value code was ever parsed for these (VCP version, frequency,
		// firmware level, ...) — Current is just its zero value, not a real
		// reading, so showing it alongside the label would be a fabricated fact.
		return r.Label
	case r.Label != "":
		return fmt.Sprintf("%s (0x%02X)", r.Label, r.Current)
	default:
		return fmt.Sprintf("0x%02X", r.Current)
	}
}

func readingsByCode(readings []ddc.FeatureReading) map[uint8]ddc.FeatureReading {
	m := make(map[uint8]ddc.FeatureReading, len(readings))
	for _, r := range readings {
		m[r.Code] = r
	}
	return m
}

func (m Model) rawView(title string) string {
	if m.rawConfirming {
		return m.rawConfirmView(title)
	}
	if m.rawEditing {
		return m.rawEditView(title)
	}

	header := title + "\n\n" +
		dimStyle.Render(fmt.Sprintf("Raw VCP — %d features declared", len(m.caps.Features)))

	var body string
	switch {
	case m.rawLoading:
		body = dimStyle.Render("Scanning all VCP codes — this takes a few seconds...")
	case m.rawErr != nil:
		body = errStyle.Render("Error: " + m.rawErr.Error())
	default:
		body = m.raw.View()
		if m.rawWriting {
			body += "\n\n" + dimStyle.Render("Writing...")
		} else if m.rawWriteErr != nil {
			body += "\n\n" + errStyle.Render("Write failed: "+m.rawWriteErr.Error())
		}
	}

	help := dimStyle.Render("↑↓/j/k move · f/pgdn b/pgup page · e edit value · r rescan · esc/v back · q quit")
	content := header + "\n\n" + body + "\n\n" + help
	return boxStyle.Render(content)
}

// rawEditView renders the numeric-entry prompt for the currently selected
// row.
func (m Model) rawEditView(title string) string {
	f := m.caps.Features[m.rawCursor]

	kindNote := ""
	if !f.Recognized {
		kindNote = dimStyle.Render("  (unrecognized/manufacturer-specific — write needs --permit-unknown-feature)")
	}

	body := fmt.Sprintf("Set %02X (%s)%s\n\n", f.Code, f.Name, kindNote) +
		"New value: " + m.rawEditInput + "█"

	if m.rawEditErr != "" {
		body += "\n\n" + errStyle.Render(m.rawEditErr)
	}

	help := dimStyle.Render("0-9 enter digits · enter confirm · backspace delete · esc cancel")
	content := title + "\n\n" + body + "\n\n" + help
	return boxStyle.Render(content)
}

// rawConfirmView renders the y/N gate shown after a valid value is entered,
// before it's actually sent to the monitor.
func (m Model) rawConfirmView(title string) string {
	f := m.caps.Features[m.rawCursor]

	warn := fmt.Sprintf("This writes %d (0x%02X) to code %02X (%s) directly.\n",
		m.rawConfirmValue, m.rawConfirmValue, f.Code, f.Name)
	if !f.Recognized {
		warn += "ddcutil does not recognize this code — sending an arbitrary\n" +
			"value to it is undocumented behavior; it may do nothing, or it\n" +
			"may affect the monitor in a way ddcutil can't warn about.\n"
	}
	warn += "There is no undo from software once it's sent.\n\nProceed? [y/N]"

	body := errStyle.Render("⚠ Write raw VCP value") + "\n\n" + warn

	help := dimStyle.Render("y confirm · n/esc cancel")
	content := title + "\n\n" + body + "\n\n" + help
	return boxStyle.Render(content)
}

// newRawViewport builds a viewport sized to fit inside boxStyle's border
// and padding alongside this screen's own header/footer lines.
func newRawViewport(termWidth, termHeight int) viewport.Model {
	w := termWidth - 14
	if w < 20 {
		w = 20
	}
	h := termHeight - 12
	if h < 5 {
		h = 5
	}
	return viewport.New(w, h)
}
