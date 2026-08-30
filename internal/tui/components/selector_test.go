package components

import "testing"

func inputSourceSelector(current uint8) Selector {
	return NewSelector(0x60, "Input Source", []Option{
		{Code: 0x0f, Name: "DisplayPort-1"},
		{Code: 0x11, Name: "HDMI-1"},
		{Code: 0x12, Name: "HDMI-2"},
	}, current)
}

func TestSelector_NextOption_WrapsForward(t *testing.T) {
	s := inputSourceSelector(0x12) // HDMI-2, last option
	if got := s.NextOption(1); got != 0x0f {
		t.Errorf("NextOption(1) = 0x%02X, want 0x0F (wrap to first)", got)
	}
}

func TestSelector_NextOption_WrapsBackward(t *testing.T) {
	s := inputSourceSelector(0x0f) // DisplayPort-1, first option
	if got := s.NextOption(-1); got != 0x12 {
		t.Errorf("NextOption(-1) = 0x%02X, want 0x12 (wrap to last)", got)
	}
}

func TestSelector_NextOption_UnknownCurrentValueLandsOnFirst(t *testing.T) {
	// The monitor can (and on this project's LG panel, does) report a
	// current value outside the set it advertised in capabilities.
	s := inputSourceSelector(0x00)
	if got := s.NextOption(1); got != 0x0f {
		t.Errorf("NextOption(1) from unknown value = 0x%02X, want 0x0F", got)
	}
	if got := s.NextOption(-1); got != 0x0f {
		t.Errorf("NextOption(-1) from unknown value = 0x%02X, want 0x0F", got)
	}
}

func TestSelector_CurrentName_UnknownValueIsLabeled(t *testing.T) {
	s := inputSourceSelector(0x00)
	if got := s.currentName(); got != "unknown (0x00)" {
		t.Errorf("currentName() = %q, want %q", got, "unknown (0x00)")
	}
}

func TestSelector_CurrentName_KnownValue(t *testing.T) {
	s := inputSourceSelector(0x11)
	if got := s.currentName(); got != "HDMI-1" {
		t.Errorf("currentName() = %q, want HDMI-1", got)
	}
}
