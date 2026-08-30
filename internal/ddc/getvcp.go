package ddc

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// RawBytes holds the four raw VCP reply bytes ddcutil prints when it has no
// feature definition to interpret them with (unrecognized or
// manufacturer-specific codes).
type RawBytes struct {
	Mh, Ml, Sh, Sl uint8
}

// FeatureReading is the result of reading one VCP feature's live value via
// `ddcutil getvcp`.
type FeatureReading struct {
	Code     uint8
	Name     string // ddcutil's name for the code, as reported by getvcp
	Readable bool   // false for write-only/action features ("is not readable")

	Continuous bool
	Current    uint16
	Max        uint16 // only meaningful when Continuous

	Label string    // parsed label for a known non-continuous value, e.g. "6500 K"
	Raw   *RawBytes // set only for the raw mh/ml/sh/sl form (unknown codes)

	// Generic marks a reading that came from the catch-all fallback format
	// (frequencies, VCP version, firmware level, ...) rather than a shape
	// with an actual value code attached. Current is always its zero value
	// here — it was never parsed, not genuinely 0 — so callers must not
	// print Current/format it as a real value for these.
	Generic bool
}

var (
	notReadableRe   = regexp.MustCompile(`Feature ([0-9A-Fa-f]{2}) \(([^)]*)\) is not readable`)
	continuousRe    = regexp.MustCompile(`VCP code 0x([0-9A-Fa-f]{2}) \(([^)]*)\): current value =\s*(\d+), max value =\s*(\d+)`)
	rawValueRe      = regexp.MustCompile(`VCP code 0x([0-9A-Fa-f]{2}) \(([^)]*)\): mh=0x([0-9A-Fa-f]{2}), ml=0x([0-9A-Fa-f]{2}), sh=0x([0-9A-Fa-f]{2}), sl=0x([0-9A-Fa-f]{2})`)
	nonContinuousRe = regexp.MustCompile(`VCP code 0x([0-9A-Fa-f]{2}) \(([^)]*)\): (.+) \(sl=0x([0-9A-Fa-f]{2})\)`)
	genericReplyRe  = regexp.MustCompile(`VCP code 0x([0-9A-Fa-f]{2}) \(([^)]*)\): (.+)`)
)

// GetVCP runs `ddcutil getvcp <code>` for one feature and parses its reply.
// A write-only/action feature (e.g. "restore factory defaults") is not an
// error: it comes back as FeatureReading{Readable: false}.
func GetVCP(displayNum int, code uint8) (FeatureReading, error) {
	out, err := exec.Command("ddcutil", "--display", strconv.Itoa(displayNum), "getvcp", fmt.Sprintf("%02x", code)).CombinedOutput()
	return parseGetVCPReply(code, string(out), err)
}

// parseGetVCPReply is the pure part of GetVCP, split out so the shapes it
// has to handle can be exercised with fixture text instead of a real
// subprocess.
func parseGetVCPReply(code uint8, text string, err error) (FeatureReading, error) {
	if m := notReadableRe.FindStringSubmatch(text); m != nil {
		return FeatureReading{Code: code, Name: strings.TrimSpace(m[2]), Readable: false}, nil
	}
	if m := continuousRe.FindStringSubmatch(text); m != nil {
		cur, _ := strconv.Atoi(m[3])
		max, _ := strconv.Atoi(m[4])
		return FeatureReading{
			Code: code, Name: strings.TrimSpace(m[2]), Readable: true,
			Continuous: true, Current: uint16(cur), Max: uint16(max),
		}, nil
	}
	if m := rawValueRe.FindStringSubmatch(text); m != nil {
		mh, _ := strconv.ParseUint(m[3], 16, 8)
		ml, _ := strconv.ParseUint(m[4], 16, 8)
		sh, _ := strconv.ParseUint(m[5], 16, 8)
		sl, _ := strconv.ParseUint(m[6], 16, 8)
		return FeatureReading{
			Code: code, Name: strings.TrimSpace(m[2]), Readable: true,
			Current: uint16(sl), Raw: &RawBytes{Mh: uint8(mh), Ml: uint8(ml), Sh: uint8(sh), Sl: uint8(sl)},
		}, nil
	}
	if m := nonContinuousRe.FindStringSubmatch(text); m != nil {
		sl, _ := strconv.ParseUint(m[4], 16, 8)
		return FeatureReading{
			Code: code, Name: strings.TrimSpace(m[2]), Readable: true,
			Current: uint16(sl), Label: strings.TrimSpace(m[3]),
		}, nil
	}
	// A handful of features (VCP Version, Active control, frequencies,
	// firmware level, usage time, ...) get bespoke formatting from ddcutil
	// that matches none of the shapes above. Rather than treat a
	// successful read as an error, keep whatever text it printed — losing
	// the feature entirely would be worse than an unparsed label.
	if err == nil {
		if m := genericReplyRe.FindStringSubmatch(text); m != nil {
			return FeatureReading{
				Code: code, Name: strings.TrimSpace(m[2]), Readable: true,
				Label: strings.TrimSpace(m[3]), Generic: true,
			}, nil
		}
	}
	if err != nil {
		return FeatureReading{}, fmt.Errorf("ddcutil getvcp %02x: %w: %s", code, err, strings.TrimSpace(text))
	}
	return FeatureReading{}, fmt.Errorf("ddcutil getvcp %02x: unrecognized output: %s", code, strings.TrimSpace(text))
}

// ProbeResult is the outcome of reading every feature a monitor's
// capabilities string declared.
type ProbeResult struct {
	Readings []FeatureReading
	Errors   map[uint8]error
}

// ProbeAll calls GetVCP for each of the given feature codes. A code that
// fails to read is recorded in Errors rather than aborting the whole probe —
// one flaky/unsupported VCP code shouldn't hide every other reading.
func ProbeAll(displayNum int, codes []uint8) ProbeResult {
	res := ProbeResult{Errors: map[uint8]error{}}
	for _, code := range codes {
		reading, err := GetVCP(displayNum, code)
		if err != nil {
			res.Errors[code] = err
			continue
		}
		res.Readings = append(res.Readings, reading)
	}
	return res
}
