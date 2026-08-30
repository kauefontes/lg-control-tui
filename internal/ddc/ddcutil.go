// Package ddc wraps the ddcutil CLI so the rest of the app never has to
// shell out or parse its output directly.
package ddc

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// Display represents one monitor found by `ddcutil detect`.
type Display struct {
	Number     int    // ddcutil's "Display N" index, used with --display
	I2CBus     string // e.g. /dev/i2c-2
	Connector  string // DRM connector name, e.g. card1-HDMI-A-1 (may be empty)
	MfgID      string
	Model      string
	VCPVersion string
}

var (
	displayRe   = regexp.MustCompile(`^Display (\d+)`)
	i2cRe       = regexp.MustCompile(`I2C bus:\s+(\S+)`)
	connectorRe = regexp.MustCompile(`DRM_connector:\s+(\S+)`)
	mfgRe       = regexp.MustCompile(`Mfg id:\s+(\S+)`)
	modelRe     = regexp.MustCompile(`Model:\s+(.*)`)
	vcpVerRe    = regexp.MustCompile(`VCP version:\s+(\S+)`)
)

// Detect runs `ddcutil detect` and returns every valid (non-laptop) display found.
func Detect() ([]Display, error) {
	out, err := exec.Command("ddcutil", "detect").CombinedOutput()
	if err != nil {
		// ddcutil can return non-zero even when it printed useful output
		// (e.g. warnings about invalid displays), so only bail if we got nothing.
		if len(out) == 0 {
			return nil, fmt.Errorf("ddcutil detect: %w", err)
		}
	}

	var displays []Display
	var cur *Display

	for _, line := range strings.Split(string(out), "\n") {
		trimmed := strings.TrimSpace(line)

		if m := displayRe.FindStringSubmatch(trimmed); m != nil {
			if cur != nil {
				displays = append(displays, *cur)
			}
			num, _ := strconv.Atoi(m[1])
			cur = &Display{Number: num}
			continue
		}
		if strings.HasPrefix(trimmed, "Invalid display") {
			if cur != nil {
				displays = append(displays, *cur) // flush whatever valid display we had
			}
			cur = nil // and start dropping the invalid one (laptop panel, etc.)
			continue
		}
		if cur == nil {
			continue
		}
		if m := i2cRe.FindStringSubmatch(trimmed); m != nil {
			cur.I2CBus = m[1]
		} else if m := connectorRe.FindStringSubmatch(trimmed); m != nil {
			cur.Connector = m[1]
		} else if m := mfgRe.FindStringSubmatch(trimmed); m != nil {
			cur.MfgID = m[1]
		} else if m := modelRe.FindStringSubmatch(trimmed); m != nil {
			cur.Model = strings.TrimSpace(m[1])
		} else if m := vcpVerRe.FindStringSubmatch(trimmed); m != nil {
			cur.VCPVersion = m[1]
		}
	}
	if cur != nil {
		displays = append(displays, *cur)
	}

	return displays, nil
}
