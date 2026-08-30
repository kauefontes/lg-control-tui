package tui

import (
	"strings"
	"testing"

	"lg-control-tui/internal/ddc"
)

func TestRawValueString_GenericReadingOmitsFabricatedCode(t *testing.T) {
	r := ddc.FeatureReading{Readable: true, Generic: true, Label: "1164 hz"}
	got := rawValueString(r, true)
	if got != "1164 hz" {
		t.Errorf("rawValueString = %q, want %q (no fabricated hex code)", got, "1164 hz")
	}
	if strings.Contains(got, "0x00") {
		t.Errorf("rawValueString = %q must not contain a fabricated (0x00)", got)
	}
}

func TestRawValueString_KnownEnumKeepsCode(t *testing.T) {
	r := ddc.FeatureReading{Readable: true, Label: "6500 K", Current: 0x05}
	got := rawValueString(r, true)
	if got != "6500 K (0x05)" {
		t.Errorf("rawValueString = %q, want %q", got, "6500 K (0x05)")
	}
}

func TestRawValueString_NotProbed(t *testing.T) {
	got := rawValueString(ddc.FeatureReading{}, false)
	if !strings.Contains(got, "not probed") {
		t.Errorf("rawValueString = %q, want it to mention 'not probed'", got)
	}
}

func TestRawValueString_NotReadableAction(t *testing.T) {
	got := rawValueString(ddc.FeatureReading{Readable: false}, true)
	if !strings.Contains(got, "write-only") {
		t.Errorf("rawValueString = %q, want it to mention 'write-only'", got)
	}
}

func TestRenderRawTable_MarksFocusedRow(t *testing.T) {
	caps := &ddc.Capabilities{Features: []ddc.VCPFeature{
		{Code: 0x10, Name: "Brightness", Recognized: true},
		{Code: 0x12, Name: "Contrast", Recognized: true},
	}}

	got := renderRawTable(caps, map[uint8]ddc.FeatureReading{}, 1)
	lines := strings.Split(got, "\n")

	// Row order follows caps.Features (header takes the first two lines).
	if strings.Contains(lines[2], "▸") {
		t.Errorf("row 0 = %q, want no cursor marker", lines[2])
	}
	if !strings.Contains(lines[3], "▸") {
		t.Errorf("row 1 (focused) = %q, want the cursor marker", lines[3])
	}
}

func TestRawValueString_RawUnknownShowsBytes(t *testing.T) {
	r := ddc.FeatureReading{
		Readable: true, Current: 0x1f,
		Raw: &ddc.RawBytes{Mh: 0xff, Ml: 0xff, Sh: 0x00, Sl: 0x1f},
	}
	got := rawValueString(r, true)
	if !strings.Contains(got, "sl=1F") && !strings.Contains(got, "sl=1f") {
		t.Errorf("rawValueString = %q, want it to include the raw sl byte", got)
	}
}
