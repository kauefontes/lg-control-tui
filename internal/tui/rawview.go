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
// guarantees no VCP code, understood or not, is ever inaccessible.
func renderRawTable(caps *ddc.Capabilities, byCode map[uint8]ddc.FeatureReading) string {
	var b strings.Builder

	header := fmt.Sprintf("%-4s %-34s %-12s %s", "Code", "Name", "Category", "Value")
	b.WriteString(dimStyle.Render(header) + "\n")
	b.WriteString(dimStyle.Render(strings.Repeat("─", 90)) + "\n")

	for _, f := range caps.Features {
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
		line := fmt.Sprintf("%-4s %-34s %s %s",
			fmt.Sprintf("%02X", f.Code),
			name,
			catStyle.Render(fmt.Sprintf("%-12s", category)),
			rawValueString(r, ok),
		)
		b.WriteString(line + "\n")
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
	header := title + "\n\n" +
		dimStyle.Render(fmt.Sprintf("Raw VCP — %d features declared (read-only)", len(m.caps.Features)))

	var body string
	switch {
	case m.rawLoading:
		body = dimStyle.Render("Scanning all VCP codes — this takes a few seconds...")
	case m.rawErr != nil:
		body = errStyle.Render("Error: " + m.rawErr.Error())
	default:
		body = m.raw.View()
	}

	help := dimStyle.Render("↑↓/pgup/pgdn scroll · r rescan · esc/v back · q quit")
	content := header + "\n\n" + body + "\n\n" + help
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
