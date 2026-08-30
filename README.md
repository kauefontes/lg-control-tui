# lg-control-tui

A terminal UI for controlling DDC/CI monitors over `ddcutil` — built and
tested against an LG 29UM68 ultrawide, but not hardcoded to it.

```
╭──────────────────────────────────────────────────╮
│  LG CONTROL TUI                                   │
│                                                    │
│  ● GSM LG ULTRAWIDE (/dev/i2c-2, VCP 2.1)         │
│                                                    │
│  ● MCCS 2.1                                       │
│  38 VCP features declared · 12 shown as controls  │
│  · 13 unrecognized/mfg-specific                   │
│                                                    │
│  ▸ [ Restore factory defaults ]                   │
│    [ Restore factory brightness/contrast defaults]│
│    [ Restore color defaults ]                     │
│    Brightness         [████████████████████] 100  │
│    Contrast           [██████████████░░░░░░]  70  │
│    Select color preset ‹ 6500 K ›                 │
│    Video gain: Red    [██████████░░░░░░░░░░]  50  │
│    Video gain: Green  [██████████░░░░░░░░░░]  50  │
│    Video gain: Blue   [██████████░░░░░░░░░░]  50  │
│    Input Source       ‹ HDMI-1 ›                  │
│    Power mode         ‹ DPM: On,  DPMS: Off ›     │
│    Audio speaker volume [████████████████████] 100│
│                                                    │
│  ↑↓ navigate · ←→ adjust · enter run action ·     │
│  v raw VCP · r refresh · R rescan · q quit         │
╰──────────────────────────────────────────────────╯
```

## Why this exists

`ddcutil` already does all the hard DDC/CI work. This project's job is
narrower: turn `ddcutil capabilities` / `getvcp` / `setvcp` into something
you can actually navigate with arrow keys, without ever throwing away a
VCP feature code just because we don't recognize it.

The governing rule of the whole codebase:

> **Everything the monitor exposes must be representable, even the codes
> we don't understand.** An "Unrecognized feature" or "Manufacturer
> specific feature" is still tracked, still visible (in the Raw VCP
> screen), and never silently dropped.

## Requirements

- [`ddcutil`](https://www.ddcutil.com/) installed and able to talk to your
  monitor (`ddcutil detect` should list it).
- Your user in the `i2c` group (`groups` should show `i2c`) — otherwise
  every `ddcutil` call needs `sudo`, which this project doesn't shell out
  with.
- Go 1.27+.

## Build & run

```bash
go build -o lg-control-tui .
./lg-control-tui
```

or just `go run .` during development.

## Keybindings

| Key | Action |
| --- | --- |
| `↑`/`k`, `↓`/`j` | Move focus between controls |
| `←`/`h`, `→`/`l` | Adjust the focused slider or cycle the focused selector |
| `enter` | Run the focused action (opens a confirmation prompt first) |
| `y` / `n` / `esc` | Confirm / cancel a pending action |
| `v` | Toggle the Raw VCP screen |
| `r` | Refresh (re-detect + re-read current values) |
| `R` | Full rescan — drops the on-disk cache and rediscovers everything from scratch |
| `D` | Switch which display is being controlled (only shown/active with more than one detected) |
| `q` / `ctrl+c` | Quit |

Inside the Raw VCP screen: `↑↓`/`j`/`k` move the focused row, `f`/`pgdn` and
`b`/`pgup` page, `e` edits the focused row's value, `r` rescans the whole
screen, `esc`/`v` goes back.

Editing a row types a new decimal value, then goes through a `y`/`n`
confirmation before anything is sent — writing to a code `ddcutil` doesn't
recognize (unrecognized/manufacturer-specific) additionally requires
`--permit-unknown-feature`, which the confirmation prompt calls out
explicitly since it's undocumented behavior on ddcutil's part, not this
project's.

### Multiple displays

If `ddcutil detect` finds more than one DDC/CI-capable display, a picker
screen appears before the controls screen — pick one with `↑↓`/`enter`.
With exactly one display, the picker is skipped entirely and behavior is
unchanged from before. Press `D` from the controls screen at any point to
reopen the picker and switch to a different display (each display has its
own on-disk cache, keyed by manufacturer+model, so switching back and
forth doesn't re-scan a display you've already controlled). `r`/`R`
refreshes stay on whichever display was last picked instead of bouncing
back to the picker.

## How it works

```
TUI (Bubble Tea)
 │
 ▼
internal/tui   — Model/Update/View, screens, components (slider/selector/action)
 │
 ▼
internal/ddc   — wraps `ddcutil` as subprocess calls; owns all text parsing
 │
 ▼
ddcutil → /dev/i2c-N → monitor
```

### Discovery pipeline

1. **`ddcutil detect`** finds the display and its `/dev/i2c-N` bus.
2. **`ddcutil capabilities --verbose`** is parsed into a generic model —
   every `VCPFeature` keeps its code, name, and declared values, whether
   or not `ddcutil` could interpret them:

   ```go
   type VCPFeature struct {
       Code                 uint8
       Name                 string
       Recognized           bool // false for "Unrecognized"/"Manufacturer specific"
       ManufacturerSpecific bool
       Values               []VCPValue
   }
   ```

3. **Capabilities is not trusted blindly.** A feature it declares can
   still turn out to be write-only, unreadable, or formatted in a way
   `ddcutil` has no shape for. Every feature that becomes a control gets
   a live `getvcp` read first, and its actual reply shape (continuous /
   named-enum / raw bytes / not-readable / bespoke-format) decides what
   it becomes on screen — not the capabilities text alone.

4. Only fully-understood features become **sliders** (continuous),
   **selectors** (enum with every value named), or **actions**
   (write-only, e.g. "Restore factory defaults"). Everything else —
   unrecognized codes, manufacturer-specific codes, partially-named enums
   — is left for the **Raw VCP** screen (`v`), never dropped.

### Writes are optimistic-free, not optimistic

A slider/selector only updates on screen after `ddcutil setvcp` reads the
value *back* and confirms it stuck. This turned out to matter in
practice: this project's own monitor was, at one point, silently
rejecting writes to Contrast/RGB gain/Color Preset — accepting the
command but never actually changing — while advertising them as fully
settable. Restoring factory defaults made them writable again, so it
wasn't a fixed hardware limit; some hidden runtime state (likely a locked
picture mode) was blocking them, and neither `capabilities` nor a plain
`getvcp` on the feature itself revealed that. The UI surfacing "Failed:
..." instead of silently trusting the write is what caught this.

### Caching, and loading progressively

A full scan costs one `ddcutil getvcp` per feature — on this project's
monitor that's ~175ms of i2c round-trip *each*, so scanning all 38
declared codes takes several seconds. Most of the shape (which codes are
sliders, which are selectors, their names/options/max values) never
changes between launches — only current values do. So after the first
scan of a monitor, the result — including each control's last known value
— is cached to `$XDG_CACHE_HOME/lg-control-tui/monitor-<mfg>-<model>.json`
(typically `~/.cache/lg-control-tui/`).

On a cached launch the screen doesn't even wait for fresh reads: it
renders immediately with each control's last known value (marked with a
dim `…` until confirmed), then updates each one in place as its own
`getvcp` comes back — a batch of independent reads running concurrently
rather than one blocking scan. A control that fails its live re-read
just keeps showing the cached value instead of blanking out.

Measured on this project's monitor: **~7.4s** with no cache → **~4.9s**
on the first scan of a new monitor (skips unrecognized/mfg-specific codes
up front) → **~1.9s** of background confirmation on every launch after
that, with the screen itself usable from frame one.

Press `R` to force a full rescan (deletes the cache and starts over) —
useful after a firmware update or if you suspect the cached shape is
stale. The cache format is versioned (`cacheVersion` in
`internal/ddc/cache.go`); a schema change just falls back to a fresh scan
instead of silently misreading old data.

## Project layout

```
lg-control-tui/
├── main.go
└── internal/
    ├── ddc/                  # everything that talks to ddcutil
    │   ├── ddcutil.go        # `ddcutil detect` → []Display
    │   ├── capabilities.go   # `ddcutil capabilities --verbose` → *Capabilities
    │   ├── getvcp.go         # `ddcutil getvcp` → FeatureReading (4 reply shapes + fallback)
    │   ├── setvcp.go         # `ddcutil setvcp`, verified by ddcutil's own readback
    │   └── cache.go          # on-disk, versioned MonitorCache (see Caching, above)
    │
    └── tui/
        ├── model.go          # Bubble Tea Model/Update/View, controls screen
        ├── picker.go         # multi-display picker screen
        ├── rawview.go        # Raw VCP screen (bubbles/viewport) + value editing
        ├── styles.go         # Lip Gloss styles
        └── components/
            ├── slider.go     # continuous control
            ├── selector.go   # named-enum control
            └── action.go     # write-only command control
```

## Testing

```bash
go test ./...
```

Tests are unit-level and don't require a monitor to be attached — the
`ddc` package tests parse fixture text captured from real `ddcutil`
output (including this project's own monitor's actual `capabilities` and
`getvcp` replies), and the `tui` package tests drive `Model.Update`
directly with synthetic messages rather than a real terminal or
subprocess. Every quirk mentioned above (the bespoke-format fallback, the
write-rejection-without-error case, the "current value outside declared
options" case, the progressive-load pending/confirm lifecycle) has a
regression test built from what was actually observed on hardware, not a
hypothetical.

## What's deliberately not here yet

- **Other monitors/vendors.** Nothing here is LG-specific — the whole
  point of the generic `VCPFeature` model is that it doesn't need to be —
  but it's only been exercised against one panel so far.
- **A native DDC/CI backend.** `ddcutil` is the backend by design for
  now; swapping it for direct `/dev/i2c-*` access later wouldn't need to
  touch `internal/tui` at all.
