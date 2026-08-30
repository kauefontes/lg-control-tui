package ddc

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// SetVCP runs `ddcutil setvcp <code> <value>`. ddcutil reads the value back
// afterward to verify the write took effect, so a non-nil error here means
// the monitor didn't actually change (not just that the command failed to run).
func SetVCP(displayNum int, code uint8, value int) error {
	return setVCP(displayNum, code, value, false)
}

// SetVCPUnknown is SetVCP for a feature code ddcutil itself doesn't
// recognize (unrecognized or manufacturer-specific). ddcutil refuses to
// write those without --permit-unknown-feature as a safety guard against
// blindly poking undocumented registers; the Raw VCP screen's edit flow
// puts its own confirmation prompt in front of this for the same reason.
func SetVCPUnknown(displayNum int, code uint8, value int) error {
	return setVCP(displayNum, code, value, true)
}

func setVCP(displayNum int, code uint8, value int, permitUnknown bool) error {
	args := []string{"--display", strconv.Itoa(displayNum)}
	if permitUnknown {
		args = append(args, "--permit-unknown-feature")
	}
	args = append(args, "setvcp", fmt.Sprintf("%02x", code), strconv.Itoa(value))

	out, err := exec.Command("ddcutil", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("ddcutil setvcp %02x %d: %w: %s", code, value, err, strings.TrimSpace(string(out)))
	}
	return nil
}
