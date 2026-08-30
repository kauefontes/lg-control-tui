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
	out, err := exec.Command(
		"ddcutil", "--display", strconv.Itoa(displayNum),
		"setvcp", fmt.Sprintf("%02x", code), strconv.Itoa(value),
	).CombinedOutput()
	if err != nil {
		return fmt.Errorf("ddcutil setvcp %02x %d: %w: %s", code, value, err, strings.TrimSpace(string(out)))
	}
	return nil
}
