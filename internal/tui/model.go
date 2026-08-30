package tui

import (
	"fmt"
	"strconv"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"lg-control-tui/internal/ddc"
	"lg-control-tui/internal/tui/components"
)

type detectMsg struct {
	displays []ddc.Display
	err      error
}

// detectCmd runs `ddcutil detect` off the UI goroutine and reports back.
func detectCmd() tea.Msg {
	displays, err := ddc.Detect()
	return detectMsg{displays: displays, err: err}
}

type probeMsg struct {
	caps      *ddc.Capabilities
	sliders   []components.Slider
	selectors []components.Selector
	actions   []components.Action
	order     []ctrlRef
	err       error
}

// cachedControlsMsg is the immediate render built purely from a cached
// scan — no i2c round-trip yet, just whatever value was last known. It's
// sent alongside a batch of liveValueCmds that each confirm (or correct)
// one control's value shortly after.
type cachedControlsMsg struct {
	caps      *ddc.Capabilities
	sliders   []components.Slider
	selectors []components.Selector
	actions   []components.Action
	order     []ctrlRef
}

type liveValueMsg struct {
	code    uint8
	reading ddc.FeatureReading
	err     error
}

// probeCmd discovers a display's controls, preferring a cached scan.
//
// A full scan means one `ddcutil getvcp` per feature — on this project's
// own monitor that's ~175ms of i2c round-trip *each*, so scanning all 38
// declared features takes ~7 seconds. Most of those codes never become a
// control (see buildControls), so once we've scanned a monitor once, we
// remember exactly which codes did and only re-read *those* live next
// time. Sliders/selectors need a fresh value each launch; actions
// (write-only, nothing to read) don't need re-probing at all once we know
// they're actions.
//
// On a cache hit, the screen doesn't wait for those live reads either: it
// renders immediately with each control's last known value, then updates
// each one in place as its own read comes back — a batch of independent
// commands rather than one command that blocks until all ~9 finish.
func probeCmd(display ddc.Display) tea.Cmd {
	if cache, ok := ddc.LoadMonitorCache(display.MfgID, display.Model); ok {
		caps := cache.Capabilities
		sliders, selectors, actions, order := buildControlsFromCache(cache)

		cmds := make([]tea.Cmd, 0, 1+len(cache.Sliders)+len(cache.Selectors))
		cmds = append(cmds, func() tea.Msg {
			return cachedControlsMsg{caps: &caps, sliders: sliders, selectors: selectors, actions: actions, order: order}
		})
		for _, s := range cache.Sliders {
			cmds = append(cmds, liveValueCmd(display.Number, s.Code))
		}
		for _, s := range cache.Selectors {
			cmds = append(cmds, liveValueCmd(display.Number, s.Code))
		}
		return tea.Batch(cmds...)
	}

	return func() tea.Msg {
		caps, err := ddc.GetCapabilities(display.Number)
		if err != nil {
			return probeMsg{err: err}
		}
		// Only recognized features can ever become a control (see
		// buildControls) — no point spending an i2c round-trip on the
		// unrecognized/manufacturer-specific codes just to discover
		// that. Those stay unscanned until a future Raw VCP screen asks.
		var codes []uint8
		for _, f := range caps.Features {
			if f.Recognized {
				codes = append(codes, f.Code)
			}
		}
		result := ddc.ProbeAll(display.Number, codes)
		sliders, selectors, actions, order := buildControls(caps, result.Readings, nil)

		// Best-effort: a failed save just means the next launch scans again.
		_ = ddc.SaveMonitorCache(display.MfgID, display.Model, toMonitorCache(caps, sliders, selectors, actions))

		return probeMsg{caps: caps, sliders: sliders, selectors: selectors, actions: actions, order: order}
	}
}

// liveValueCmd reads one VCP code's live value, independent of any other
// code — this is what lets a cache-hit launch fan out N i2c reads
// concurrently instead of running them one after another behind a single
// loading screen.
func liveValueCmd(displayNum int, code uint8) tea.Cmd {
	return func() tea.Msg {
		reading, err := ddc.GetVCP(displayNum, code)
		return liveValueMsg{code: code, reading: reading, err: err}
	}
}

// buildControlsFromCache rebuilds the friendly controls straight from a
// cached scan, with no live reading involved — every code in it was
// already confirmed to behave like a slider/selector/action on a previous
// full scan, so there's nothing left to (re)classify, only values to trust
// provisionally until liveValueCmd confirms them.
func buildControlsFromCache(cache ddc.MonitorCache) ([]components.Slider, []components.Selector, []components.Action, []ctrlRef) {
	sliderByCode := make(map[uint8]ddc.CachedSlider, len(cache.Sliders))
	for _, s := range cache.Sliders {
		sliderByCode[s.Code] = s
	}
	selectorByCode := make(map[uint8]ddc.CachedSelector, len(cache.Selectors))
	for _, s := range cache.Selectors {
		selectorByCode[s.Code] = s
	}
	actionCodes := make(map[uint8]bool, len(cache.ActionCodes))
	for _, c := range cache.ActionCodes {
		actionCodes[uint8(c)] = true
	}

	var sliders []components.Slider
	var selectors []components.Selector
	var actions []components.Action
	var order []ctrlRef

	// Walk capabilities' own order so this matches what a fresh scan would
	// have produced, not cache-file insertion order.
	for _, f := range cache.Capabilities.Features {
		if cs, ok := sliderByCode[f.Code]; ok {
			sliders = append(sliders, components.NewSlider(cs.Code, f.Name, int(cs.Value), int(cs.Max)))
			order = append(order, ctrlRef{kind: kindSlider, idx: len(sliders) - 1})
			continue
		}
		if cs, ok := selectorByCode[f.Code]; ok {
			opts := make([]components.Option, len(f.Values))
			for i, v := range f.Values {
				opts[i] = components.Option{Code: v.Code, Name: v.Name}
			}
			selectors = append(selectors, components.NewSelector(cs.Code, f.Name, opts, cs.Selected))
			order = append(order, ctrlRef{kind: kindSelector, idx: len(selectors) - 1})
			continue
		}
		if actionCodes[f.Code] {
			actions = append(actions, components.Action{Code: f.Code, Name: f.Name})
			order = append(order, ctrlRef{kind: kindAction, idx: len(actions) - 1})
		}
	}
	return sliders, selectors, actions, order
}

func toMonitorCache(caps *ddc.Capabilities, sliders []components.Slider, selectors []components.Selector, actions []components.Action) ddc.MonitorCache {
	cachedSliders := make([]ddc.CachedSlider, len(sliders))
	for i, s := range sliders {
		cachedSliders[i] = ddc.CachedSlider{Code: s.Code, Max: uint16(s.Max), Value: uint16(s.Value)}
	}
	cachedSelectors := make([]ddc.CachedSelector, len(selectors))
	for i, s := range selectors {
		cachedSelectors[i] = ddc.CachedSelector{Code: s.Code, Selected: s.Selected}
	}
	actionCodes := make([]int, len(actions))
	for i, a := range actions {
		actionCodes[i] = int(a.Code)
	}
	return ddc.MonitorCache{
		Capabilities: *caps,
		Sliders:      cachedSliders,
		Selectors:    cachedSelectors,
		ActionCodes:  actionCodes,
	}
}

type setMsg struct {
	code  uint8
	value int
	err   error
}

// setCmd writes one VCP value. ddcutil verifies the write by reading it
// back, so a non-nil err here means the monitor didn't actually change —
// and on this LG panel that turns out to happen for real: writes to
// Contrast/RGB gain/Color Preset were silently rejected at one point even
// though the monitor advertises them as settable. Restoring factory
// defaults made them writable again, so it wasn't a fixed hardware limit —
// some hidden runtime state (likely a locked picture mode) blocked them,
// and neither capabilities nor a plain getvcp reveals that state. The UI
// only commits a control's new value once a write comes back clean,
// instead of assuming it took effect — which is exactly what surfaced
// this in the first place.
func setCmd(displayNum int, code uint8, value int) tea.Cmd {
	return func() tea.Msg {
		return setMsg{code: code, value: value, err: ddc.SetVCP(displayNum, code, value)}
	}
}

type actionMsg struct {
	name string
	err  error
}

// actionCmd triggers a write-only action feature (e.g. Restore factory
// defaults). Per MCCS convention for Write-Only Non-Continuous features,
// the value written doesn't carry data — it just needs to be non-zero to
// fire the command, so we always send 1.
func actionCmd(displayNum int, code uint8, name string) tea.Cmd {
	return func() tea.Msg {
		return actionMsg{name: name, err: ddc.SetVCP(displayNum, code, 1)}
	}
}

// ctrlKind distinguishes the three kinds of control a probe result can
// surface. See ctrlRef and buildControls.
type ctrlKind int

const (
	kindSlider ctrlKind = iota
	kindSelector
	kindAction
)

// ctrlRef points at one entry in m.sliders, m.selectors, or m.actions, in
// the order they should be interleaved for navigation/rendering (the order
// their VCP codes appear in capabilities).
type ctrlRef struct {
	kind ctrlKind
	idx  int
}

// buildControls turns a probe result into the three kinds of friendly
// control this screen understands:
//   - continuous features (Brightness, Contrast, RGB gain, Volume, ...) become sliders
//   - non-continuous features whose every declared value has a name
//     (Input Source, Color Preset, Power mode, ...) become selectors
//   - write-only features with nothing to read back (Restore factory
//     defaults, ...) become actions
//
// Everything else — unrecognized codes, manufacturer-specific codes, and
// enums with unnamed/"interpretation unavailable" values — is deliberately
// left out. That's not data loss: it belongs on the future Raw VCP screen,
// not mixed into a "friendly" view that implies we understand it.
//
// knownActionCodes lets a code be classified as an action without a live
// reading in hand — used when restoring from a cache that never re-probes
// actions (they have no value to refresh). Pass nil when every recognized
// code was actually probed this run.
func buildControls(caps *ddc.Capabilities, readings []ddc.FeatureReading, knownActionCodes map[uint8]bool) ([]components.Slider, []components.Selector, []components.Action, []ctrlRef) {
	byCode := make(map[uint8]ddc.FeatureReading, len(readings))
	for _, r := range readings {
		byCode[r.Code] = r
	}

	var sliders []components.Slider
	var selectors []components.Selector
	var actions []components.Action
	var order []ctrlRef

	addAction := func(f ddc.VCPFeature) {
		actions = append(actions, components.Action{Code: f.Code, Name: f.Name})
		order = append(order, ctrlRef{kind: kindAction, idx: len(actions) - 1})
	}

	for _, f := range caps.Features {
		if !f.Recognized {
			continue
		}

		r, ok := byCode[f.Code]
		if !ok {
			if knownActionCodes[f.Code] {
				addAction(f)
			}
			continue
		}

		if !r.Readable {
			addAction(f)
			continue
		}

		switch {
		case r.Continuous:
			sliders = append(sliders, components.NewSlider(f.Code, f.Name, int(r.Current), int(r.Max)))
			order = append(order, ctrlRef{kind: kindSlider, idx: len(sliders) - 1})

		case f.HasValues() && allValuesNamed(f.Values):
			opts := make([]components.Option, len(f.Values))
			for i, v := range f.Values {
				opts[i] = components.Option{Code: v.Code, Name: v.Name}
			}
			selectors = append(selectors, components.NewSelector(f.Code, f.Name, opts, uint8(r.Current)))
			order = append(order, ctrlRef{kind: kindSelector, idx: len(selectors) - 1})
		}
	}
	return sliders, selectors, actions, order
}

func allValuesNamed(values []ddc.VCPValue) bool {
	for _, v := range values {
		if v.Name == "" {
			return false
		}
	}
	return true
}

type Model struct {
	loading  bool
	displays []ddc.Display
	err      error

	// selected indexes m.displays for whichever one is currently being
	// controlled. displayChosen stays false until either a single display
	// was auto-picked or the picker screen (for more than one) has been
	// answered — it's what keeps a later refresh from re-showing the
	// picker and bouncing the user back to display 0.
	selected      int
	displayChosen bool
	pickerCursor  int

	probing  bool
	caps     *ddc.Capabilities
	probeErr error

	sliders   []components.Slider
	selectors []components.Selector
	actions   []components.Action
	order     []ctrlRef
	cursor    int
	opErr     error

	// pending marks controls whose value came from cache and hasn't been
	// confirmed by a live read yet — see liveValueMsg. Empty after a fresh
	// (non-cached) scan, since there's nothing left to confirm by then.
	pending map[uint8]bool

	// confirming is set while a destructive action is awaiting a y/n
	// answer. Every other key is swallowed until it's answered.
	confirming       bool
	confirmActionIdx int

	// Raw VCP screen: a view of every declared feature, including the
	// unrecognized/manufacturer-specific ones the friendly screen never
	// shows. Probed lazily, only when the user opens it.
	screen      screenKind
	raw         viewport.Model
	rawReady    bool
	rawLoading  bool
	rawErr      error
	rawReadings map[uint8]ddc.FeatureReading
	rawCursor   int // index into m.caps.Features of the focused row

	// Raw VCP editing: typing a new value for the row at rawCursor. Only
	// entered via 'e'; the value isn't sent until rawConfirming passes.
	rawEditing   bool
	rawEditInput string
	rawEditErr   string

	// rawConfirming gates the actual write behind a y/n prompt — writing an
	// arbitrary value to an unrecognized/manufacturer-specific code is
	// undocumented behavior, per ddcutil's own --permit-unknown-feature
	// caution, so this never fires silently.
	rawConfirming   bool
	rawConfirmValue int
	rawWriting      bool
	rawWriteErr     error

	winW, winH int
}

func New() Model {
	return Model{loading: true, raw: newRawViewport(80, 24)}
}

func (m Model) Init() tea.Cmd {
	return detectCmd
}

func (m Model) refresh() (Model, tea.Cmd) {
	m.loading = true
	m.err = nil
	m.probing = false
	m.caps = nil
	m.probeErr = nil
	m.sliders = nil
	m.selectors = nil
	m.actions = nil
	m.order = nil
	m.cursor = 0
	m.opErr = nil
	m.pending = nil
	m.confirming = false
	m.screen = screenControls
	m.rawReady = false
	m.rawLoading = false
	m.rawErr = nil
	m.rawReadings = nil
	m.rawCursor = 0
	m.rawEditing = false
	m.rawEditInput = ""
	m.rawEditErr = ""
	m.rawConfirming = false
	m.rawWriting = false
	m.rawWriteErr = nil
	return m, detectCmd
}

// refreshRawContent re-renders the raw table from the current readings and
// cursor and pushes it into the viewport, keeping the focused row visible.
func (m *Model) refreshRawContent() {
	m.raw.SetContent(renderRawTable(m.caps, m.rawReadings, m.rawCursor))
	line := m.rawCursor + rawTableHeaderLines
	if line < m.raw.YOffset {
		m.raw.SetYOffset(line)
	} else if line >= m.raw.YOffset+m.raw.Height {
		m.raw.SetYOffset(line - m.raw.Height + 1)
	}
}

func (m Model) displayNum() int {
	d, ok := m.currentDisplay()
	if !ok {
		return 0
	}
	return d.Number
}

// adjust computes the command to move the focused slider/selector one step
// in the given direction (-1 or +1), or nil if there's nothing to adjust,
// the focused control is an action, or it's already at that end (a slider
// at its bound, a single-option selector). It never mutates the model —
// the value only changes once setMsg confirms the write actually took
// effect.
func (m Model) adjust(direction int) tea.Cmd {
	if len(m.order) == 0 {
		return nil
	}
	ref := m.order[m.cursor]

	switch ref.kind {
	case kindSlider:
		s := m.sliders[ref.idx]
		newValue := s.Value + direction*s.Step
		if newValue < 0 {
			newValue = 0
		}
		if newValue > s.Max {
			newValue = s.Max
		}
		if newValue == s.Value {
			return nil
		}
		return setCmd(m.displayNum(), s.Code, newValue)

	case kindSelector:
		sel := m.selectors[ref.idx]
		nextCode := sel.NextOption(direction)
		if nextCode == sel.Selected {
			return nil
		}
		return setCmd(m.displayNum(), sel.Code, int(nextCode))
	}

	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.winW, m.winH = msg.Width, msg.Height
		sized := newRawViewport(msg.Width, msg.Height)
		m.raw.Width, m.raw.Height = sized.Width, sized.Height

	case tea.KeyMsg:
		if m.confirming {
			switch msg.String() {
			case "y", "Y":
				a := m.actions[m.confirmActionIdx]
				m.confirming = false
				m.opErr = nil
				return m, actionCmd(m.displayNum(), a.Code, a.Name)
			case "n", "N", "esc":
				m.confirming = false
			case "q", "ctrl+c":
				return m, tea.Quit
			}
			return m, nil // swallow anything else while a destructive action is pending
		}

		if m.rawConfirming {
			switch msg.String() {
			case "y", "Y":
				f := m.caps.Features[m.rawCursor]
				m.rawConfirming = false
				m.rawWriting = true
				m.rawWriteErr = nil
				return m, rawSetCmd(m.displayNum(), f.Code, m.rawConfirmValue, !f.Recognized)
			case "n", "N", "esc":
				m.rawConfirming = false
			case "q", "ctrl+c":
				return m, tea.Quit
			}
			return m, nil // swallow anything else while a raw write is pending confirmation
		}

		if m.rawEditing {
			switch msg.String() {
			case "esc":
				m.rawEditing = false
				m.rawEditInput = ""
				m.rawEditErr = ""
			case "enter":
				v, err := strconv.Atoi(m.rawEditInput)
				switch {
				case m.rawEditInput == "" || err != nil:
					m.rawEditErr = "Enter a whole number."
				case v < 0 || v > 65535:
					m.rawEditErr = "Value must be between 0 and 65535."
				default:
					m.rawEditing = false
					m.rawEditErr = ""
					m.rawConfirming = true
					m.rawConfirmValue = v
				}
			case "backspace":
				if len(m.rawEditInput) > 0 {
					m.rawEditInput = m.rawEditInput[:len(m.rawEditInput)-1]
				}
			case "ctrl+c":
				return m, tea.Quit
			default:
				s := msg.String()
				if len(s) == 1 && s[0] >= '0' && s[0] <= '9' && len(m.rawEditInput) < 5 {
					m.rawEditInput += s
					m.rawEditErr = ""
				}
			}
			return m, nil // swallow anything else while entering a raw value
		}

		if m.screen == screenPicker {
			switch msg.String() {
			case "q", "ctrl+c":
				return m, tea.Quit
			case "esc":
				if m.displayChosen {
					m.screen = screenControls
				}
				return m, nil
			case "up", "k":
				if len(m.displays) > 0 {
					m.pickerCursor--
					if m.pickerCursor < 0 {
						m.pickerCursor = len(m.displays) - 1
					}
				}
			case "down", "j":
				if len(m.displays) > 0 {
					m.pickerCursor = (m.pickerCursor + 1) % len(m.displays)
				}
			case "enter":
				if len(m.displays) > 0 {
					return m.selectDisplay(m.pickerCursor)
				}
			}
			return m, nil
		}

		if m.screen == screenRaw {
			switch msg.String() {
			case "q", "ctrl+c":
				return m, tea.Quit
			case "esc", "v":
				m.screen = screenControls
				return m, nil
			case "r":
				if m.caps != nil {
					m.rawLoading = true
					return m, rawProbeCmd(m.displayNum(), allFeatureCodes(m.caps))
				}
				return m, nil
			case "up", "k":
				if m.caps != nil && len(m.caps.Features) > 0 {
					m.rawCursor--
					if m.rawCursor < 0 {
						m.rawCursor = len(m.caps.Features) - 1
					}
					m.refreshRawContent()
				}
				return m, nil
			case "down", "j":
				if m.caps != nil && len(m.caps.Features) > 0 {
					m.rawCursor = (m.rawCursor + 1) % len(m.caps.Features)
					m.refreshRawContent()
				}
				return m, nil
			case "e":
				if m.caps != nil && len(m.caps.Features) > 0 && !m.rawLoading {
					m.rawEditing = true
					m.rawEditInput = ""
					m.rawEditErr = ""
					m.rawWriteErr = nil
				}
				return m, nil
			}
			var cmd tea.Cmd
			m.raw, cmd = m.raw.Update(msg)
			return m, cmd
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "r":
			return m.refresh()
		case "R":
			// Full rescan: drop the cached shape and start over as if this
			// were the first time we've seen this monitor.
			if d, ok := m.currentDisplay(); ok {
				_ = ddc.ClearMonitorCache(d.MfgID, d.Model)
			}
			return m.refresh()
		case "D":
			if len(m.displays) > 1 {
				m.screen = screenPicker
				m.pickerCursor = m.selected
			}
			return m, nil
		case "v":
			m.screen = screenRaw
			if !m.rawReady && !m.rawLoading && m.caps != nil {
				m.rawLoading = true
				return m, rawProbeCmd(m.displayNum(), allFeatureCodes(m.caps))
			}

		case "up", "k":
			if len(m.order) > 0 {
				m.cursor--
				if m.cursor < 0 {
					m.cursor = len(m.order) - 1
				}
			}
		case "down", "j":
			if len(m.order) > 0 {
				m.cursor = (m.cursor + 1) % len(m.order)
			}
		case "left", "h":
			if cmd := m.adjust(-1); cmd != nil {
				m.opErr = nil
				return m, cmd
			}
		case "right", "l":
			if cmd := m.adjust(1); cmd != nil {
				m.opErr = nil
				return m, cmd
			}
		case "enter":
			if len(m.order) > 0 && m.order[m.cursor].kind == kindAction {
				m.confirming = true
				m.confirmActionIdx = m.order[m.cursor].idx
				m.opErr = nil
			}
		}

	case detectMsg:
		m.loading = false
		m.displays = msg.displays
		m.err = msg.err
		if m.err != nil || len(m.displays) == 0 {
			break
		}
		if !m.displayChosen && len(m.displays) > 1 {
			// More than one display and nothing chosen yet (first launch,
			// or the previously-selected one vanished on refresh) — ask
			// instead of silently guessing which one the user wants.
			m.screen = screenPicker
			m.pickerCursor = 0
			break
		}
		if m.selected >= len(m.displays) {
			m.selected = 0
		}
		m.displayChosen = true
		m.probing = true
		return m, probeCmd(m.displays[m.selected])

	case probeMsg:
		m.probing = false
		m.caps = msg.caps
		m.probeErr = msg.err
		if m.probeErr == nil && m.caps != nil {
			m.sliders = msg.sliders
			m.selectors = msg.selectors
			m.actions = msg.actions
			m.order = msg.order
			m.cursor = 0
			m.pending = nil // a fresh scan's values are already live, nothing to confirm
		}

	case cachedControlsMsg:
		m.probing = false
		m.caps = msg.caps
		m.probeErr = nil
		m.sliders = msg.sliders
		m.selectors = msg.selectors
		m.actions = msg.actions
		m.order = msg.order
		m.cursor = 0
		m.pending = make(map[uint8]bool, len(m.sliders)+len(m.selectors))
		for _, s := range m.sliders {
			m.pending[s.Code] = true
		}
		for _, s := range m.selectors {
			m.pending[s.Code] = true
		}

	case liveValueMsg:
		delete(m.pending, msg.code) // resolved either way — see below on error
		if msg.err == nil && msg.reading.Readable {
			r := msg.reading
			for i := range m.sliders {
				if m.sliders[i].Code == msg.code {
					if r.Continuous {
						m.sliders[i].Value = int(r.Current)
						m.sliders[i].Max = int(r.Max)
					}
					break
				}
			}
			for i := range m.selectors {
				if m.selectors[i].Code == msg.code {
					m.selectors[i].Selected = uint8(r.Current)
					break
				}
			}
		}
		// On error, the cached value just keeps showing rather than
		// blanking the control — a single flaky read isn't worth an error
		// banner when we already have a reasonable last-known value.

	case setMsg:
		m.opErr = msg.err
		if msg.err == nil {
			for i := range m.sliders {
				if m.sliders[i].Code == msg.code {
					m.sliders[i].Value = msg.value
					break
				}
			}
			for i := range m.selectors {
				if m.selectors[i].Code == msg.code {
					m.selectors[i].Selected = uint8(msg.value)
					break
				}
			}
		}

	case actionMsg:
		m.opErr = msg.err

	case rawProbeMsg:
		m.rawLoading = false
		m.rawErr = msg.err
		if msg.err == nil {
			m.rawReady = true
			m.rawReadings = readingsByCode(msg.readings)
			m.rawCursor = 0
			m.refreshRawContent()
			m.raw.GotoTop()
		}

	case rawSingleProbeMsg:
		// Best-effort refresh of one row after a write — if the re-read
		// itself fails, the row just keeps showing its last known value,
		// same as the controls screen's liveValueMsg handling.
		if msg.err == nil {
			if m.rawReadings == nil {
				m.rawReadings = map[uint8]ddc.FeatureReading{}
			}
			m.rawReadings[msg.code] = msg.reading
			m.refreshRawContent()
		}

	case rawSetMsg:
		m.rawWriting = false
		m.rawWriteErr = msg.err
		if msg.err == nil {
			return m, rawSingleProbeCmd(m.displayNum(), msg.code)
		}
	}

	return m, nil
}

func (m Model) View() string {
	title := titleStyle.Render("LG CONTROL TUI")

	if m.screen == screenPicker {
		return m.pickerView(title)
	}

	if m.screen == screenRaw {
		return m.rawView(title)
	}

	if m.confirming {
		return m.confirmView(title)
	}

	var body string
	switch {
	case m.loading:
		body = dimStyle.Render("Detecting monitors via ddcutil...")
	case m.err != nil:
		body = errStyle.Render("Error: " + m.err.Error())
	case len(m.displays) == 0:
		body = errStyle.Render("No DDC/CI capable displays found.")
	default:
		multi := len(m.displays) > 1
		for i, d := range m.displays {
			prefix := ""
			if multi {
				if i == m.selected {
					prefix = "▸ "
				} else {
					prefix = "  "
				}
			}
			line := prefix + okStyle.Render("● ") + d.MfgID
			if d.Model != "" {
				line += " " + d.Model
			}
			line += dimStyle.Render(" (" + d.I2CBus + ", VCP " + d.VCPVersion + ")")
			body += line + "\n"
		}

		body += "\n"
		switch {
		case m.probing:
			body += dimStyle.Render("Reading VCP features...")
		case m.probeErr != nil:
			body += errStyle.Render("Probe error: " + m.probeErr.Error())
		case m.caps != nil:
			unknown := 0
			for _, f := range m.caps.Features {
				if !f.Recognized {
					unknown++
				}
			}
			shown := len(m.sliders) + len(m.selectors) + len(m.actions)

			body += okStyle.Render("● ") + "MCCS " + m.caps.MCCSVersion + "\n"
			body += dimStyle.Render(summaryLine(len(m.caps.Features), shown, unknown))
			body += "\n\n"

			for i, ref := range m.order {
				focused := i == m.cursor
				var line string
				var code uint8
				switch ref.kind {
				case kindSlider:
					line, code = m.sliders[ref.idx].View(focused), m.sliders[ref.idx].Code
				case kindSelector:
					line, code = m.selectors[ref.idx].View(focused), m.selectors[ref.idx].Code
				case kindAction:
					line = m.actions[ref.idx].View(focused)
				}
				if m.pending[code] {
					line += dimStyle.Render(" …")
				}
				body += line + "\n"
			}

			if m.opErr != nil {
				body += "\n" + errStyle.Render("Failed: "+m.opErr.Error())
			}
		}
	}

	helpText := "↑↓ navigate · ←→ adjust · enter run action · v raw VCP · r refresh · R rescan"
	if len(m.displays) > 1 {
		helpText += " · D switch display"
	}
	helpText += " · q quit"
	help := dimStyle.Render(helpText)

	content := title + "\n\n" + body + "\n\n" + help
	return boxStyle.Render(content)
}

func (m Model) confirmView(title string) string {
	a := m.actions[m.confirmActionIdx]

	body := errStyle.Render("⚠ "+a.Name) + "\n\n" +
		fmt.Sprintf("This writes %q to the monitor directly.\nThere is no undo from software once it's sent.\n\n", a.Name) +
		"Proceed? [y/N]"

	help := dimStyle.Render("y confirm · n/esc cancel")

	content := title + "\n\n" + body + "\n\n" + help
	return boxStyle.Render(content)
}

func summaryLine(total, shown, unknown int) string {
	return strconv.Itoa(total) + " VCP features declared · " +
		strconv.Itoa(shown) + " shown as controls · " +
		strconv.Itoa(unknown) + " unrecognized/mfg-specific"
}
