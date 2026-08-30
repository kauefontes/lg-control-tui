package ddc

import "testing"

// Captured from this project's own LG 29UM68 via `ddcutil --display 1
// capabilities --verbose`.
const sampleCapabilities = `Model: Not specified
MCCS version: 2.1
VCP Features:
   Feature: 02 (New control value)
   Feature: 04 (Restore factory defaults)
   Feature: 05 (Restore factory brightness/contrast defaults)
   Feature: 08 (Restore color defaults)
   Feature: 10 (Brightness)
   Feature: 12 (Contrast)
   Feature: 14 (Select color preset)
      Values (unparsed): 05 08 0B
      Values (  parsed):
         05: 6500 K
         08: 9300 K
         0b: User 1
   Feature: 16 (Video gain: Red)
   Feature: 18 (Video gain: Green)
   Feature: 1A (Video gain: Blue)
   Feature: 52 (Active control)
   Feature: 60 (Input Source)
      Values (unparsed):  11 12 0F
      Values (  parsed):
         11: HDMI-1
         12: HDMI-2
         0f: DisplayPort-1
   Feature: A4 (Turn the selected window operation on/off)
      Values (unparsed): 01 02 03
      Values (  parsed): 01 02 03 (interpretation unavailable)
   Feature: AC (Horizontal frequency)
   Feature: AE (Vertical frequency)
   Feature: B2 (Flat panel sub-pixel layout)
   Feature: B6 (Display technology type)
   Feature: C0 (Display usage time)
   Feature: C6 (Application enable key)
   Feature: C8 (Display controller type)
   Feature: C9 (Display firmware level)
   Feature: D6 (Power mode)
      Values (unparsed): 01 04
      Values (  parsed):
         01: DPM: On,  DPMS: Off
         04: DPM: Off, DPMS: Off
   Feature: DF (VCP Version)
   Feature: 62 (Audio speaker volume)
   Feature: 8D (Audio Mute)
   Feature: F4 (Manufacturer specific feature)
   Feature: F5 (Manufacturer specific feature)
      Values (unparsed): 00 01 02 03 04
      Values (  parsed): 00 01 02 03 04 (interpretation unavailable)
   Feature: F6 (Manufacturer specific feature)
      Values (unparsed): 00 01 02
      Values (  parsed): 00 01 02 (interpretation unavailable)
   Feature: 4D (Unrecognized feature)
   Feature: 4E (Unrecognized feature)
   Feature: 4F (Unrecognized feature)
   Feature: 15 (Unrecognized feature)
      Values (unparsed): 01 11 13 14 28 29 32 48
      Values (  parsed): 01 11 13 14 28 29 32 48 (interpretation unavailable)
   Feature: F7 (Manufacturer specific feature)
      Values (unparsed): 00 01 02 03
      Values (  parsed): 00 01 02 03 (interpretation unavailable)
   Feature: F8 (Manufacturer specific feature)
      Values (unparsed): 00 01
      Values (  parsed): 00 01 (interpretation unavailable)
   Feature: F9 (Manufacturer specific feature)
   Feature: FD (Manufacturer specific feature)
      Values (unparsed): 00 01
      Values (  parsed): 00 01 (interpretation unavailable)
   Feature: FE (Manufacturer specific feature)
      Values (unparsed): 00 01 02
      Values (  parsed): 00 01 02 (interpretation unavailable)
   Feature: FF (Manufacturer specific feature)
`

func TestParseCapabilities_HeaderFields(t *testing.T) {
	caps, err := ParseCapabilities(sampleCapabilities)
	if err != nil {
		t.Fatalf("ParseCapabilities: %v", err)
	}
	if caps.MCCSVersion != "2.1" {
		t.Errorf("MCCSVersion = %q, want 2.1", caps.MCCSVersion)
	}
}

func TestParseCapabilities_FeatureCount(t *testing.T) {
	caps, err := ParseCapabilities(sampleCapabilities)
	if err != nil {
		t.Fatalf("ParseCapabilities: %v", err)
	}
	const want = 38
	if got := len(caps.Features); got != want {
		t.Fatalf("len(Features) = %d, want %d", got, want)
	}
}

func TestParseCapabilities_KnownFeature(t *testing.T) {
	caps, _ := ParseCapabilities(sampleCapabilities)
	f, ok := caps.Feature(0x10)
	if !ok {
		t.Fatal("feature 0x10 (Brightness) not found")
	}
	if f.Name != "Brightness" || !f.Recognized || f.ManufacturerSpecific || f.HasValues() {
		t.Errorf("unexpected feature: %+v", f)
	}
}

func TestParseCapabilities_MultiLineEnum(t *testing.T) {
	caps, _ := ParseCapabilities(sampleCapabilities)
	f, ok := caps.Feature(0x60)
	if !ok {
		t.Fatal("feature 0x60 (Input Source) not found")
	}
	want := map[uint8]string{0x11: "HDMI-1", 0x12: "HDMI-2", 0x0f: "DisplayPort-1"}
	if len(f.Values) != len(want) {
		t.Fatalf("got %d values, want %d: %+v", len(f.Values), len(want), f.Values)
	}
	for code, name := range want {
		if got := f.ValueName(code); got != name {
			t.Errorf("ValueName(0x%02X) = %q, want %q", code, got, name)
		}
	}
}

func TestParseCapabilities_SingleLineUninterpretedEnum(t *testing.T) {
	caps, _ := ParseCapabilities(sampleCapabilities)
	f, ok := caps.Feature(0xA4)
	if !ok {
		t.Fatal("feature 0xA4 not found")
	}
	if len(f.Values) != 3 {
		t.Fatalf("got %d values, want 3: %+v", len(f.Values), f.Values)
	}
	for _, v := range f.Values {
		if v.Name != "" {
			t.Errorf("value 0x%02X: expected no interpretation, got %q", v.Code, v.Name)
		}
	}
}

func TestParseCapabilities_UnrecognizedFeaturesArePreserved(t *testing.T) {
	caps, _ := ParseCapabilities(sampleCapabilities)
	for _, code := range []uint8{0x4D, 0x4E, 0x4F, 0x15} {
		f, ok := caps.Feature(code)
		if !ok {
			t.Fatalf("unrecognized feature 0x%02X was dropped, not preserved", code)
		}
		if f.Recognized {
			t.Errorf("feature 0x%02X: Recognized = true, want false", code)
		}
	}
	// 0x15 additionally carries an unparsed enum — must still be kept.
	f, _ := caps.Feature(0x15)
	if len(f.Values) != 8 {
		t.Errorf("feature 0x15: got %d values, want 8: %+v", len(f.Values), f.Values)
	}
}

func TestParseCapabilities_ManufacturerSpecificFeaturesArePreserved(t *testing.T) {
	caps, _ := ParseCapabilities(sampleCapabilities)
	for _, code := range []uint8{0xF4, 0xF5, 0xF6, 0xF7, 0xF8, 0xF9, 0xFD, 0xFE, 0xFF} {
		f, ok := caps.Feature(code)
		if !ok {
			t.Fatalf("manufacturer-specific feature 0x%02X was dropped", code)
		}
		if !f.ManufacturerSpecific || f.Recognized {
			t.Errorf("feature 0x%02X: got %+v, want ManufacturerSpecific=true Recognized=false", code, f)
		}
	}
}
