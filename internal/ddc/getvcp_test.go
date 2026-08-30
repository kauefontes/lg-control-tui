package ddc

import "testing"

// The samples below are `ddcutil getvcp` replies captured against this
// project's own LG 29UM68, one per output shape the parser has to handle.
const (
	sampleContinuous           = "VCP code 0x10 (Brightness                    ): current value =   100, max value =   100\n"
	sampleNonContinuousKnown   = "VCP code 0x14 (Select color preset           ): 6500 K (sl=0x05)\n"
	sampleNonContinuousInvalid = "VCP code 0x60 (Input Source                  ): Invalid value (sl=0x00)\n"
	sampleRawUnrecognized      = "VCP code 0x4d (Unknown feature               ): mh=0xff, ml=0xff, sh=0x78, sl=0x33\n"
	sampleRawManufacturer      = "VCP code 0xf4 (Manufacturer Specific         ): mh=0xff, ml=0xff, sh=0x00, sl=0x1f\n"
	sampleNotReadable          = "Feature 04 (Restore factory defaults) is not readable\n"

	// ddcutil gives several features their own bespoke formatting that
	// matches none of the generic shapes above — these must fall back to
	// genericReplyRe instead of being treated as parse errors.
	sampleFrequency = "VCP code 0xac (Horizontal frequency          ): 1164 hz\n"
	sampleVersion   = "VCP code 0xdf (VCP Version                   ): 2.1\n"
	sampleHexValue  = "VCP code 0x52 (Active control                ): Value: 0x00\n"
	sampleUsageTime = "VCP code 0xc0 (Display usage time            ): Usage time (hours) = 25985 (0x006581) mh=0xff, ml=0xff, sh=0x65, sl=0x81\n"
)

func TestGetVCP_ParsesContinuous(t *testing.T) {
	m := continuousRe.FindStringSubmatch(sampleContinuous)
	if m == nil {
		t.Fatal("continuousRe did not match")
	}
	if m[1] != "10" || m[2] != "Brightness                    " || m[3] != "100" || m[4] != "100" {
		t.Errorf("unexpected groups: %#v", m)
	}
}

func TestGetVCP_ParsesKnownEnumValue(t *testing.T) {
	m := nonContinuousRe.FindStringSubmatch(sampleNonContinuousKnown)
	if m == nil {
		t.Fatal("nonContinuousRe did not match")
	}
	if m[3] != "6500 K" || m[4] != "05" {
		t.Errorf("unexpected groups: %#v", m)
	}
}

func TestGetVCP_ParsesInvalidEnumValue(t *testing.T) {
	m := nonContinuousRe.FindStringSubmatch(sampleNonContinuousInvalid)
	if m == nil {
		t.Fatal("nonContinuousRe did not match")
	}
	if m[3] != "Invalid value" || m[4] != "00" {
		t.Errorf("unexpected groups: %#v", m)
	}
}

func TestGetVCP_ParsesRawUnrecognized(t *testing.T) {
	m := rawValueRe.FindStringSubmatch(sampleRawUnrecognized)
	if m == nil {
		t.Fatal("rawValueRe did not match")
	}
	if m[1] != "4d" || m[3] != "ff" || m[4] != "ff" || m[5] != "78" || m[6] != "33" {
		t.Errorf("unexpected groups: %#v", m)
	}
	// Must not also be swallowed by the generic non-continuous pattern
	// (no parenthesized "(sl=..)" in the raw form).
	if nonContinuousRe.MatchString(sampleRawUnrecognized) {
		t.Error("nonContinuousRe unexpectedly matched a raw-form reply")
	}
}

func TestGetVCP_ParsesRawManufacturerSpecific(t *testing.T) {
	m := rawValueRe.FindStringSubmatch(sampleRawManufacturer)
	if m == nil {
		t.Fatal("rawValueRe did not match")
	}
	if m[1] != "f4" || m[6] != "1f" {
		t.Errorf("unexpected groups: %#v", m)
	}
}

func TestGetVCP_FallsBackToGenericReplyForBespokeFormats(t *testing.T) {
	cases := []struct {
		name, sample, wantCode, wantLabel string
	}{
		{"frequency", sampleFrequency, "ac", "1164 hz"},
		{"version", sampleVersion, "df", "2.1"},
		{"hex value", sampleHexValue, "52", "Value: 0x00"},
		{"usage time", sampleUsageTime, "c0", "Usage time (hours) = 25985 (0x006581) mh=0xff, ml=0xff, sh=0x65, sl=0x81"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// None of the specific patterns should claim these lines...
			if continuousRe.MatchString(c.sample) || rawValueRe.MatchString(c.sample) || nonContinuousRe.MatchString(c.sample) {
				t.Fatalf("a specific pattern unexpectedly matched %q", c.sample)
			}
			// ...but the generic fallback must, preserving the full text.
			m := genericReplyRe.FindStringSubmatch(c.sample)
			if m == nil {
				t.Fatalf("genericReplyRe did not match %q", c.sample)
			}
			if m[1] != c.wantCode || m[3] != c.wantLabel {
				t.Errorf("got code=%q label=%q, want code=%q label=%q", m[1], m[3], c.wantCode, c.wantLabel)
			}
		})
	}
}

func TestGetVCP_DetectsNotReadable(t *testing.T) {
	m := notReadableRe.FindStringSubmatch(sampleNotReadable)
	if m == nil {
		t.Fatal("notReadableRe did not match")
	}
	if m[1] != "04" || m[2] != "Restore factory defaults" {
		t.Errorf("unexpected groups: %#v", m)
	}
}

func TestParseGetVCPReply_GenericFallbackIsFlagged(t *testing.T) {
	// Readings that fall back to the catch-all format never get a real
	// value code parsed — Current must stay 0 *and* be recognizable as
	// "never parsed" rather than "genuinely zero", so a caller (like the
	// Raw VCP table) doesn't print a fabricated "(0x00)" next to it.
	r, err := parseGetVCPReply(0xac, sampleFrequency, nil)
	if err != nil {
		t.Fatalf("parseGetVCPReply: %v", err)
	}
	if !r.Generic {
		t.Error("expected Generic=true for a bespoke-format reply")
	}
	if r.Current != 0 {
		t.Errorf("Current = %d, want 0 (never parsed)", r.Current)
	}
	if r.Label != "1164 hz" {
		t.Errorf("Label = %q, want %q", r.Label, "1164 hz")
	}
}

func TestParseGetVCPReply_ContinuousIsNotFlaggedGeneric(t *testing.T) {
	r, err := parseGetVCPReply(0x10, sampleContinuous, nil)
	if err != nil {
		t.Fatalf("parseGetVCPReply: %v", err)
	}
	if r.Generic {
		t.Error("a continuous reading must not be marked Generic")
	}
}

func TestParseGetVCPReply_KnownEnumIsNotFlaggedGeneric(t *testing.T) {
	r, err := parseGetVCPReply(0x14, sampleNonContinuousKnown, nil)
	if err != nil {
		t.Fatalf("parseGetVCPReply: %v", err)
	}
	if r.Generic {
		t.Error("a known non-continuous reading must not be marked Generic")
	}
}
