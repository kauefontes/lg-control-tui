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
