package ddc

import (
	"fmt"
	"os"
	"testing"
)

func TestMonitorCache_RoundTrip(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	want := MonitorCache{
		Capabilities: Capabilities{
			Model:       "Not specified",
			MCCSVersion: "2.1",
			Features: []VCPFeature{
				{Code: 0x10, Name: "Brightness", Recognized: true},
			},
		},
		Sliders: []CachedSlider{{Code: 0x10, Max: 100, Value: 80}},
		Selectors: []CachedSelector{
			{Code: 0x60, Selected: 0x11},
			{Code: 0x14, Selected: 0x05},
		},
	}

	if err := SaveMonitorCache("GSM", "LG ULTRAWIDE", want); err != nil {
		t.Fatalf("SaveMonitorCache: %v", err)
	}

	got, ok := LoadMonitorCache("GSM", "LG ULTRAWIDE")
	if !ok {
		t.Fatal("LoadMonitorCache: expected a hit after saving")
	}
	if got.Capabilities.MCCSVersion != want.Capabilities.MCCSVersion {
		t.Errorf("MCCSVersion = %q, want %q", got.Capabilities.MCCSVersion, want.Capabilities.MCCSVersion)
	}
	if len(got.Sliders) != 1 || got.Sliders[0].Code != 0x10 || got.Sliders[0].Max != 100 || got.Sliders[0].Value != 80 {
		t.Errorf("Sliders = %+v, want [{0x10 100 80}]", got.Sliders)
	}
	if len(got.Selectors) != 2 || got.Selectors[0].Selected != 0x11 {
		t.Errorf("Selectors = %+v, want 2 entries with the first selected 0x11", got.Selectors)
	}
}

func TestMonitorCache_VersionMismatchIsTreatedAsMiss(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	// SaveMonitorCache always stamps the current version, so write the file
	// directly to actually simulate a stale one.
	path, _ := cachePath("GSM", "LG ULTRAWIDE")
	staleJSON := fmt.Sprintf(`{"Version":%d,"Sliders":[{"Code":16,"Max":100,"Value":80}]}`, cacheVersion-1)
	if err := os.WriteFile(path, []byte(staleJSON), 0o644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}

	if _, ok := LoadMonitorCache("GSM", "LG ULTRAWIDE"); ok {
		t.Error("expected a version mismatch to be treated as a cache miss")
	}
}

func TestMonitorCache_MissWhenNotSaved(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	_, ok := LoadMonitorCache("NoOne", "Never Saved")
	if ok {
		t.Error("expected a miss for a monitor that was never cached")
	}
}

func TestMonitorCache_ClearForcesM(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	_ = SaveMonitorCache("GSM", "LG ULTRAWIDE", MonitorCache{})
	if _, ok := LoadMonitorCache("GSM", "LG ULTRAWIDE"); !ok {
		t.Fatal("setup: expected a hit before clearing")
	}

	if err := ClearMonitorCache("GSM", "LG ULTRAWIDE"); err != nil {
		t.Fatalf("ClearMonitorCache: %v", err)
	}
	if _, ok := LoadMonitorCache("GSM", "LG ULTRAWIDE"); ok {
		t.Error("expected a miss after clearing the cache")
	}
}

func TestMonitorCache_KeySanitizesSeparators(t *testing.T) {
	if got := cacheKey("GSM", "LG/ULTRAWIDE 29\""); got == "" {
		t.Error("cacheKey produced an empty string")
	}
	// Must not contain path separators that would escape the cache dir.
	if got := cacheKey("A/B", "C\\D"); got != "A_B-C_D" {
		t.Errorf("cacheKey(\"A/B\", \"C\\\\D\") = %q, want \"A_B-C_D\"", got)
	}
}
