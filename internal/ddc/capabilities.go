package ddc

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// VCPValue is one enum value a feature can take, as declared by the monitor's
// capabilities string. Name is empty when the monitor reports the code exists
// but ddcutil has no interpretation for it ("interpretation unavailable").
type VCPValue struct {
	Code uint8
	Name string
}

// VCPFeature describes one VCP feature code exposed by the monitor.
//
// Recognized and ManufacturerSpecific reflect what ddcutil could tell about
// the code itself, not whether we know how to *use* it — an unrecognized or
// manufacturer-specific feature is still tracked and still controllable, we
// just don't have a friendly name/meaning for it yet.
type VCPFeature struct {
	Code                 uint8
	Name                 string
	Recognized           bool
	ManufacturerSpecific bool
	Values               []VCPValue
}

// HasValues reports whether the monitor declared a fixed set of values for
// this feature (i.e. it's an enum, not a continuous 0..max control).
func (f VCPFeature) HasValues() bool {
	return len(f.Values) > 0
}

// ValueName returns the parsed name for a value code, or "" if unknown.
func (f VCPFeature) ValueName(code uint8) string {
	for _, v := range f.Values {
		if v.Code == code {
			return v.Name
		}
	}
	return ""
}

// Capabilities is the parsed result of `ddcutil capabilities`.
type Capabilities struct {
	Model       string
	MCCSVersion string
	Features    []VCPFeature
}

// Feature looks up a feature by code.
func (c *Capabilities) Feature(code uint8) (VCPFeature, bool) {
	for _, f := range c.Features {
		if f.Code == code {
			return f, true
		}
	}
	return VCPFeature{}, false
}

var (
	featureRe      = regexp.MustCompile(`^Feature:\s+([0-9A-Fa-f]{2})\s+\((.+)\)$`)
	parsedHeaderRe = regexp.MustCompile(`^Values\s+\(\s*parsed\):\s*(.*)$`)
	parsedEntryRe  = regexp.MustCompile(`^([0-9A-Fa-f]{2}):\s+(.*)$`)
)

// GetCapabilities runs `ddcutil --display N capabilities --verbose` and
// parses the result.
func GetCapabilities(displayNum int) (*Capabilities, error) {
	out, err := exec.Command("ddcutil", "--display", strconv.Itoa(displayNum), "capabilities", "--verbose").CombinedOutput()
	if err != nil && len(out) == 0 {
		return nil, fmt.Errorf("ddcutil capabilities: %w", err)
	}
	return ParseCapabilities(string(out))
}

// ParseCapabilities parses the text output of `ddcutil capabilities --verbose`.
//
// It deliberately keeps every feature code the monitor reports, including
// ones ddcutil labels "Unrecognized feature" or "Manufacturer specific
// feature" — those are exactly the codes a naive parser would throw away.
func ParseCapabilities(output string) (*Capabilities, error) {
	caps := &Capabilities{}
	var cur *VCPFeature
	inParsedBlock := false

	flush := func() {
		if cur != nil {
			caps.Features = append(caps.Features, *cur)
			cur = nil
		}
	}

	for _, rawLine := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(rawLine)

		switch {
		case strings.HasPrefix(trimmed, "Model:"):
			flush()
			caps.Model = strings.TrimSpace(strings.TrimPrefix(trimmed, "Model:"))
			inParsedBlock = false
			continue

		case strings.HasPrefix(trimmed, "MCCS version:"):
			flush()
			caps.MCCSVersion = strings.TrimSpace(strings.TrimPrefix(trimmed, "MCCS version:"))
			inParsedBlock = false
			continue
		}

		if m := featureRe.FindStringSubmatch(trimmed); m != nil {
			flush()
			code, err := strconv.ParseUint(m[1], 16, 8)
			if err != nil {
				continue
			}
			name := m[2]
			manufacturerSpecific := name == "Manufacturer specific feature"
			cur = &VCPFeature{
				Code:                 uint8(code),
				Name:                 name,
				Recognized:           name != "Unrecognized feature" && !manufacturerSpecific,
				ManufacturerSpecific: manufacturerSpecific,
			}
			inParsedBlock = false
			continue
		}

		if cur == nil {
			continue
		}

		if m := parsedHeaderRe.FindStringSubmatch(trimmed); m != nil {
			rest := strings.TrimSpace(m[1])
			if rest == "" {
				// Values follow on their own indented lines below.
				inParsedBlock = true
				continue
			}
			// Single-line form, e.g. "01 02 03 (interpretation unavailable)".
			for _, tok := range strings.Fields(rest) {
				if strings.HasPrefix(tok, "(") {
					break
				}
				code, err := strconv.ParseUint(tok, 16, 8)
				if err != nil {
					continue
				}
				cur.Values = append(cur.Values, VCPValue{Code: uint8(code)})
			}
			inParsedBlock = false
			continue
		}

		if inParsedBlock {
			if m := parsedEntryRe.FindStringSubmatch(trimmed); m != nil {
				code, err := strconv.ParseUint(m[1], 16, 8)
				if err == nil {
					cur.Values = append(cur.Values, VCPValue{Code: uint8(code), Name: strings.TrimSpace(m[2])})
				}
				continue
			}
			inParsedBlock = false
		}
	}
	flush()

	return caps, nil
}
