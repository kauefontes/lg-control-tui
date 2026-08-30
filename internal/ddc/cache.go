package ddc

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// cacheVersion guards against silently misreading an older cache whose
// schema doesn't match — encoding/json ignores unknown/missing fields by
// default, so a renamed field (like the Selectors rework below) would
// otherwise load "successfully" with data quietly missing instead of
// falling back to a fresh scan. Bump this whenever MonitorCache's shape
// changes.
const cacheVersion = 2

// CachedSlider records enough about a continuous feature to skip
// rediscovering it: its code, max value, and last known current value.
// Value is what gets shown immediately on launch, before a fresh read
// confirms (or corrects) it — see Model's progressive refresh.
type CachedSlider struct {
	Code  uint8
	Max   uint16
	Value uint16
}

// CachedSelector records a non-continuous feature's code and last known
// selection, shown immediately for the same reason as CachedSlider.Value.
type CachedSelector struct {
	Code     uint8
	Selected uint8
}

// MonitorCache is what we persist after the first full scan of a monitor:
// its capabilities (essentially static — it only changes on a firmware
// update), which codes turned out to actually behave like a slider,
// selector, or action once probed live (capabilities alone can't tell us
// that — see buildControls), and each control's last known value.
type MonitorCache struct {
	Version      int
	Capabilities Capabilities
	Sliders      []CachedSlider
	Selectors    []CachedSelector
	// ActionCodes is []int rather than []uint8 purely for JSON readability:
	// encoding/json renders []uint8 as a base64 string (it's a byte slice),
	// which makes the cache file opaque to a human debugging it.
	//
	// Actions are write-only, non-continuous features confirmed (by a live
	// "is not readable" reply) to be commands rather than data — e.g.
	// Restore factory defaults. They have no current value to refresh, so
	// unlike sliders/selectors they need no live re-probe at all once known.
	ActionCodes []int
}

func cacheDir() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir = filepath.Join(dir, "lg-control-tui")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// cacheKey identifies a monitor by manufacturer + model, which is what
// ddcutil detect gives us from the EDID. Two monitors of the same model
// sharing a cache entry is harmless — they have identical capabilities by
// definition.
func cacheKey(mfgID, model string) string {
	s := mfgID + "-" + model
	s = strings.Map(func(r rune) rune {
		switch r {
		case ' ', '/', '\\':
			return '_'
		}
		return r
	}, s)
	if s == "" || s == "-" {
		s = "unknown"
	}
	return s
}

func cachePath(mfgID, model string) (string, error) {
	dir, err := cacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "monitor-"+cacheKey(mfgID, model)+".json"), nil
}

// LoadMonitorCache returns the cached scan for a monitor, or ok=false if
// there isn't one, it can't be read/parsed, or it was written by an older
// (incompatible) version of this program — all treated the same as a
// miss, so anything short of a clean match just triggers a fresh scan.
func LoadMonitorCache(mfgID, model string) (MonitorCache, bool) {
	path, err := cachePath(mfgID, model)
	if err != nil {
		return MonitorCache{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return MonitorCache{}, false
	}
	var c MonitorCache
	if err := json.Unmarshal(data, &c); err != nil {
		return MonitorCache{}, false
	}
	if c.Version != cacheVersion {
		return MonitorCache{}, false
	}
	return c, true
}

// SaveMonitorCache persists a scan. Failure is not fatal to the caller —
// worst case, the next launch just scans again.
func SaveMonitorCache(mfgID, model string, c MonitorCache) error {
	path, err := cachePath(mfgID, model)
	if err != nil {
		return err
	}
	c.Version = cacheVersion
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// ClearMonitorCache removes a cached scan, forcing the next load to
// rediscover everything from scratch (used by a manual "rescan" action).
func ClearMonitorCache(mfgID, model string) error {
	path, err := cachePath(mfgID, model)
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
