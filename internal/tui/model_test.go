package tui

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"lg-control-tui/internal/ddc"
	"lg-control-tui/internal/tui/components"
)

func modelWithSlider(code uint8, value, max, step int) Model {
	sliders := []components.Slider{components.NewSlider(code, "Test", value, max)}
	return Model{
		sliders: sliders,
		order:   []ctrlRef{{kind: kindSlider, idx: 0}},
		cursor:  0,
	}
}

func modelWithSelector(code uint8, options []components.Option, current uint8) Model {
	selectors := []components.Selector{components.NewSelector(code, "Test", options, current)}
	return Model{
		selectors: selectors,
		order:     []ctrlRef{{kind: kindSelector, idx: 0}},
		cursor:    0,
	}
}

func modelWithAction(code uint8, name string) Model {
	actions := []components.Action{{Code: code, Name: name}}
	return Model{
		actions: actions,
		order:   []ctrlRef{{kind: kindAction, idx: 0}},
		cursor:  0,
	}
}

func keyMsg(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestUpdate_RightKey_IssuesSetCmdButDoesNotCommitYet(t *testing.T) {
	m := modelWithSlider(0x12, 70, 100, 5)

	next, cmd := m.Update(keyMsg("l"))
	got := next.(Model)

	if cmd == nil {
		t.Fatal("expected a setCmd to be returned")
	}
	if got.sliders[0].Value != 70 {
		t.Errorf("slider value changed optimistically before confirmation: got %d, want 70", got.sliders[0].Value)
	}
}

func TestUpdate_SetMsgSuccess_CommitsValue(t *testing.T) {
	m := modelWithSlider(0x12, 70, 100, 5)

	next, _ := m.Update(setMsg{code: 0x12, value: 75, err: nil})
	got := next.(Model)

	if got.sliders[0].Value != 75 {
		t.Errorf("value not committed after successful setMsg: got %d, want 75", got.sliders[0].Value)
	}
	if got.opErr != nil {
		t.Errorf("opErr should be nil on success, got %v", got.opErr)
	}
}

func TestUpdate_SetMsgFailure_LeavesValueUnchangedAndSurfacesError(t *testing.T) {
	// This mirrors what actually happens on the real monitor: writes to
	// Contrast/RGB gain are accepted by ddcutil but silently rejected by
	// the panel, so verification fails.
	m := modelWithSlider(0x12, 70, 100, 5)
	wantErr := errors.New("ddcutil setvcp 12 65: exit status 1: Verification failed for feature 12")

	next, _ := m.Update(setMsg{code: 0x12, value: 65, err: wantErr})
	got := next.(Model)

	if got.sliders[0].Value != 70 {
		t.Errorf("value must not change when the write failed: got %d, want 70 (unchanged)", got.sliders[0].Value)
	}
	if got.opErr == nil {
		t.Error("expected opErr to be set after a failed write")
	}
}

func TestUpdate_AtUpperBound_RightKeyIsNoop(t *testing.T) {
	m := modelWithSlider(0x10, 100, 100, 5)

	_, cmd := m.Update(keyMsg("l"))
	if cmd != nil {
		t.Error("expected no setCmd when already at max value")
	}
}

func TestUpdate_AtLowerBound_LeftKeyIsNoop(t *testing.T) {
	m := modelWithSlider(0x10, 0, 100, 5)

	_, cmd := m.Update(keyMsg("h"))
	if cmd != nil {
		t.Error("expected no setCmd when already at min value")
	}
}

func TestUpdate_CursorNavigationWraps(t *testing.T) {
	m := Model{
		sliders: []components.Slider{
			components.NewSlider(0x10, "A", 50, 100),
			components.NewSlider(0x12, "B", 50, 100),
		},
		order: []ctrlRef{
			{kind: kindSlider, idx: 0},
			{kind: kindSlider, idx: 1},
		},
		cursor: 0,
	}

	next, _ := m.Update(keyMsg("k")) // up from 0 wraps to last
	got := next.(Model)
	if got.cursor != 1 {
		t.Errorf("cursor should wrap to last slider, got %d", got.cursor)
	}

	next, _ = got.Update(keyMsg("j")) // down from last wraps to 0
	got = next.(Model)
	if got.cursor != 0 {
		t.Errorf("cursor should wrap to first slider, got %d", got.cursor)
	}
}

func TestUpdate_Selector_RightKey_CyclesToNextOption(t *testing.T) {
	options := []components.Option{
		{Code: 0x0f, Name: "DisplayPort-1"},
		{Code: 0x11, Name: "HDMI-1"},
		{Code: 0x12, Name: "HDMI-2"},
	}
	m := modelWithSelector(0x60, options, 0x0f)

	next, cmd := m.Update(keyMsg("l"))
	got := next.(Model)

	if cmd == nil {
		t.Fatal("expected a setCmd to be returned")
	}
	if got.selectors[0].Selected != 0x0f {
		t.Errorf("selector changed optimistically before confirmation: got 0x%02X, want 0x0F", got.selectors[0].Selected)
	}
}

func TestUpdate_Selector_SetMsgSuccess_CommitsSelection(t *testing.T) {
	options := []components.Option{
		{Code: 0x0f, Name: "DisplayPort-1"},
		{Code: 0x11, Name: "HDMI-1"},
	}
	m := modelWithSelector(0x60, options, 0x0f)

	next, _ := m.Update(setMsg{code: 0x60, value: 0x11, err: nil})
	got := next.(Model)

	if got.selectors[0].Selected != 0x11 {
		t.Errorf("selection not committed: got 0x%02X, want 0x11", got.selectors[0].Selected)
	}
}

func TestUpdate_Selector_SingleOption_IsNoop(t *testing.T) {
	options := []components.Option{{Code: 0x05, Name: "Only Option"}}
	m := modelWithSelector(0x14, options, 0x05)

	_, cmd := m.Update(keyMsg("l"))
	if cmd != nil {
		t.Error("expected no setCmd for a single-option selector")
	}
}

func TestUpdate_Action_EnterOpensConfirmation(t *testing.T) {
	m := modelWithAction(0x04, "Restore factory defaults")

	next, cmd := m.Update(keyMsg("enter"))
	got := next.(Model)

	if !got.confirming {
		t.Fatal("expected confirming=true after pressing enter on an action")
	}
	if cmd != nil {
		t.Error("opening the confirmation prompt should not itself issue a command")
	}
}

func TestUpdate_Action_LeftRightDoNothing(t *testing.T) {
	// Actions have no value to adjust — ←/→ must be no-ops, not crash or
	// open the confirmation prompt by accident.
	m := modelWithAction(0x04, "Restore factory defaults")

	_, cmd := m.Update(keyMsg("l"))
	if cmd != nil {
		t.Error("expected no command from adjusting an action")
	}
	_, cmd = m.Update(keyMsg("h"))
	if cmd != nil {
		t.Error("expected no command from adjusting an action")
	}
}

func TestUpdate_Confirming_YConfirmsAndIssuesActionCmd(t *testing.T) {
	m := modelWithAction(0x04, "Restore factory defaults")
	m.confirming = true
	m.confirmActionIdx = 0

	next, cmd := m.Update(keyMsg("y"))
	got := next.(Model)

	if got.confirming {
		t.Error("expected confirming=false after answering y")
	}
	if cmd == nil {
		t.Fatal("expected an actionCmd to be returned after confirming")
	}
}

func TestUpdate_Confirming_NCancelsWithoutIssuingCmd(t *testing.T) {
	m := modelWithAction(0x04, "Restore factory defaults")
	m.confirming = true
	m.confirmActionIdx = 0

	next, cmd := m.Update(keyMsg("n"))
	got := next.(Model)

	if got.confirming {
		t.Error("expected confirming=false after answering n")
	}
	if cmd != nil {
		t.Error("cancelling must not issue a command")
	}
}

func TestUpdate_Confirming_OtherKeysAreSwallowed(t *testing.T) {
	// Nothing but y/n/esc/quit should have any effect while a destructive
	// action is pending confirmation — no sneaking in a navigate/adjust.
	m := modelWithAction(0x04, "Restore factory defaults")
	m.confirming = true
	m.confirmActionIdx = 0

	next, cmd := m.Update(keyMsg("j"))
	got := next.(Model)

	if !got.confirming {
		t.Error("confirmation should remain open for an unrelated key")
	}
	if cmd != nil {
		t.Error("an unrelated key must not issue a command while confirming")
	}
}

func TestUpdate_ActionMsg_SurfacesError(t *testing.T) {
	m := modelWithAction(0x04, "Restore factory defaults")
	wantErr := errors.New("ddcutil setvcp 04 1: exit status 1")

	next, _ := m.Update(actionMsg{name: "Restore factory defaults", err: wantErr})
	got := next.(Model)

	if got.opErr == nil {
		t.Error("expected opErr to be set after a failed action")
	}
}

func TestUpdate_VKey_SwitchesToRawScreenAndTriggersProbe(t *testing.T) {
	m := New()
	m.caps = &ddc.Capabilities{Features: []ddc.VCPFeature{{Code: 0x10, Name: "Brightness", Recognized: true}}}

	next, cmd := m.Update(keyMsg("v"))
	got := next.(Model)

	if got.screen != screenRaw {
		t.Error("expected screen to switch to screenRaw")
	}
	if cmd == nil {
		t.Fatal("expected a rawProbeCmd to be issued when caps are available and raw hasn't been scanned yet")
	}
}

func TestUpdate_VKey_NoCapsYet_NoProbeCmd(t *testing.T) {
	m := New() // m.caps is nil (still loading/probing)

	next, cmd := m.Update(keyMsg("v"))
	got := next.(Model)

	if got.screen != screenRaw {
		t.Error("expected screen to switch to screenRaw even without caps yet")
	}
	if cmd != nil {
		t.Error("expected no rawProbeCmd when capabilities haven't loaded yet")
	}
}

func TestUpdate_VKey_AlreadyScanned_NoRedundantProbe(t *testing.T) {
	m := New()
	m.caps = &ddc.Capabilities{Features: []ddc.VCPFeature{{Code: 0x10, Name: "Brightness", Recognized: true}}}
	m.rawReady = true

	_, cmd := m.Update(keyMsg("v"))
	if cmd != nil {
		t.Error("expected no rawProbeCmd when the raw screen was already scanned")
	}
}

func TestUpdate_RawScreen_EscReturnsToControls(t *testing.T) {
	m := New()
	m.screen = screenRaw

	next, _ := m.Update(keyMsg("esc"))
	got := next.(Model)

	if got.screen != screenControls {
		t.Error("expected esc to switch back to screenControls")
	}
}

func TestUpdate_RawScreen_VKeyTogglesBack(t *testing.T) {
	m := New()
	m.screen = screenRaw

	next, _ := m.Update(keyMsg("v"))
	got := next.(Model)

	if got.screen != screenControls {
		t.Error("expected v to toggle back to screenControls from the raw screen")
	}
}

func TestUpdate_RawScreen_RTriggersRescan(t *testing.T) {
	m := New()
	m.screen = screenRaw
	m.caps = &ddc.Capabilities{Features: []ddc.VCPFeature{{Code: 0x10, Name: "Brightness", Recognized: true}}}
	m.rawReady = true // even if already scanned once, 'r' forces a fresh probe

	_, cmd := m.Update(keyMsg("r"))
	if cmd == nil {
		t.Fatal("expected 'r' to issue a rawProbeCmd on the raw screen")
	}
}

func TestUpdate_RawProbeMsg_PopulatesViewport(t *testing.T) {
	m := New()
	m.caps = &ddc.Capabilities{Features: []ddc.VCPFeature{{Code: 0x4D, Name: "Unrecognized feature", Recognized: false}}}
	m.rawLoading = true

	next, _ := m.Update(rawProbeMsg{readings: []ddc.FeatureReading{
		{Code: 0x4D, Readable: true, Raw: &ddc.RawBytes{Mh: 0xFF, Ml: 0xFF, Sh: 0x78, Sl: 0x33}},
	}})
	got := next.(Model)

	if got.rawLoading {
		t.Error("expected rawLoading=false after rawProbeMsg")
	}
	if !got.rawReady {
		t.Error("expected rawReady=true after a successful rawProbeMsg")
	}
}

func sampleCache() ddc.MonitorCache {
	return ddc.MonitorCache{
		Capabilities: ddc.Capabilities{
			Features: []ddc.VCPFeature{
				{Code: 0x10, Name: "Brightness", Recognized: true},
				{Code: 0x14, Name: "Select color preset", Recognized: true, Values: []ddc.VCPValue{
					{Code: 0x05, Name: "6500 K"}, {Code: 0x08, Name: "9300 K"},
				}},
				{Code: 0x04, Name: "Restore factory defaults", Recognized: true},
			},
		},
		Sliders:     []ddc.CachedSlider{{Code: 0x10, Max: 100, Value: 80}},
		Selectors:   []ddc.CachedSelector{{Code: 0x14, Selected: 0x05}},
		ActionCodes: []int{0x04},
	}
}

func TestBuildControlsFromCache_ReconstructsAllThreeKinds(t *testing.T) {
	sliders, selectors, actions, order := buildControlsFromCache(sampleCache())

	if len(sliders) != 1 || sliders[0].Value != 80 || sliders[0].Max != 100 {
		t.Errorf("sliders = %+v, want one slider at 80/100", sliders)
	}
	if len(selectors) != 1 || selectors[0].Selected != 0x05 || len(selectors[0].Options) != 2 {
		t.Errorf("selectors = %+v, want one selector selected 0x05 with 2 options", selectors)
	}
	if len(actions) != 1 || actions[0].Code != 0x04 {
		t.Errorf("actions = %+v, want one action for code 0x04", actions)
	}
	// Capabilities order is 0x10, 0x14, 0x04 — order must follow that, not
	// cache insertion order.
	if len(order) != 3 || order[0].kind != kindSlider || order[1].kind != kindSelector || order[2].kind != kindAction {
		t.Errorf("order = %+v, want [slider, selector, action]", order)
	}
}

func TestUpdate_CachedControlsMsg_MarksSlidersAndSelectorsPendingNotActions(t *testing.T) {
	m := New()
	sliders, selectors, actions, order := buildControlsFromCache(sampleCache())

	next, _ := m.Update(cachedControlsMsg{sliders: sliders, selectors: selectors, actions: actions, order: order})
	got := next.(Model)

	if !got.pending[0x10] {
		t.Error("expected the cached slider to be pending confirmation")
	}
	if !got.pending[0x14] {
		t.Error("expected the cached selector to be pending confirmation")
	}
	if got.pending[0x04] {
		t.Error("an action has no value to confirm — it must never be marked pending")
	}
}

func TestUpdate_LiveValueMsg_Success_UpdatesValueAndClearsPending(t *testing.T) {
	m := modelWithSlider(0x10, 80, 100, 5)
	m.pending = map[uint8]bool{0x10: true}

	next, _ := m.Update(liveValueMsg{code: 0x10, reading: ddc.FeatureReading{
		Readable: true, Continuous: true, Current: 95, Max: 100,
	}})
	got := next.(Model)

	if got.sliders[0].Value != 95 {
		t.Errorf("slider value = %d, want 95 (live value should replace the cached one)", got.sliders[0].Value)
	}
	if got.pending[0x10] {
		t.Error("expected pending[0x10] to be cleared after a successful liveValueMsg")
	}
}

func TestUpdate_LiveValueMsg_Error_KeepsCachedValueButClearsPending(t *testing.T) {
	m := modelWithSlider(0x10, 80, 100, 5)
	m.pending = map[uint8]bool{0x10: true}

	next, _ := m.Update(liveValueMsg{code: 0x10, err: errors.New("ddcutil getvcp 10: transient failure")})
	got := next.(Model)

	if got.sliders[0].Value != 80 {
		t.Errorf("slider value = %d, want 80 (a flaky read must not blank out the cached value)", got.sliders[0].Value)
	}
	if got.pending[0x10] {
		t.Error("expected pending[0x10] to be cleared even when the live read failed")
	}
}

func rawScreenModelWithTwoFeatures() Model {
	m := New()
	m.screen = screenRaw
	m.caps = &ddc.Capabilities{Features: []ddc.VCPFeature{
		{Code: 0x10, Name: "Brightness", Recognized: true},
		{Code: 0x4D, Name: "Unrecognized feature", Recognized: false},
	}}
	m.rawReady = true
	return m
}

func TestUpdate_RawScreen_DownMovesCursorAndWraps(t *testing.T) {
	m := rawScreenModelWithTwoFeatures()

	next, _ := m.Update(keyMsg("j"))
	got := next.(Model)
	if got.rawCursor != 1 {
		t.Fatalf("rawCursor = %d, want 1", got.rawCursor)
	}

	next, _ = got.Update(keyMsg("j"))
	got = next.(Model)
	if got.rawCursor != 0 {
		t.Errorf("rawCursor = %d, want wrap to 0", got.rawCursor)
	}
}

func TestUpdate_RawScreen_UpWrapsToLast(t *testing.T) {
	m := rawScreenModelWithTwoFeatures()

	next, _ := m.Update(keyMsg("k"))
	got := next.(Model)
	if got.rawCursor != 1 {
		t.Errorf("rawCursor = %d, want wrap to last (1)", got.rawCursor)
	}
}

func TestUpdate_RawScreen_EKeyEntersEditMode(t *testing.T) {
	m := rawScreenModelWithTwoFeatures()

	next, cmd := m.Update(keyMsg("e"))
	got := next.(Model)

	if !got.rawEditing {
		t.Fatal("expected rawEditing=true after pressing 'e'")
	}
	if cmd != nil {
		t.Error("entering edit mode should not itself issue a command")
	}
}

func TestUpdate_RawEditing_DigitsAppendToInput(t *testing.T) {
	m := rawScreenModelWithTwoFeatures()
	m.rawEditing = true

	next, _ := m.Update(keyMsg("7"))
	got := next.(Model)
	next, _ = got.Update(keyMsg("5"))
	got = next.(Model)

	if got.rawEditInput != "75" {
		t.Errorf("rawEditInput = %q, want %q", got.rawEditInput, "75")
	}
}

func TestUpdate_RawEditing_NonDigitsAreIgnored(t *testing.T) {
	m := rawScreenModelWithTwoFeatures()
	m.rawEditing = true

	next, _ := m.Update(keyMsg("x"))
	got := next.(Model)

	if got.rawEditInput != "" {
		t.Errorf("rawEditInput = %q, want empty (non-digit ignored)", got.rawEditInput)
	}
}

func TestUpdate_RawEditing_BackspaceDeletesLastDigit(t *testing.T) {
	m := rawScreenModelWithTwoFeatures()
	m.rawEditing = true
	m.rawEditInput = "12"

	next, _ := m.Update(keyMsg("backspace"))
	got := next.(Model)

	if got.rawEditInput != "1" {
		t.Errorf("rawEditInput = %q, want %q", got.rawEditInput, "1")
	}
}

func TestUpdate_RawEditing_EscCancels(t *testing.T) {
	m := rawScreenModelWithTwoFeatures()
	m.rawEditing = true
	m.rawEditInput = "42"

	next, _ := m.Update(keyMsg("esc"))
	got := next.(Model)

	if got.rawEditing {
		t.Error("expected rawEditing=false after esc")
	}
	if got.rawEditInput != "" {
		t.Errorf("expected rawEditInput cleared on cancel, got %q", got.rawEditInput)
	}
}

func TestUpdate_RawEditing_EnterWithValidValue_MovesToConfirming(t *testing.T) {
	m := rawScreenModelWithTwoFeatures()
	m.rawEditing = true
	m.rawEditInput = "42"

	next, cmd := m.Update(keyMsg("enter"))
	got := next.(Model)

	if got.rawEditing {
		t.Error("expected rawEditing=false once a valid value is entered")
	}
	if !got.rawConfirming {
		t.Fatal("expected rawConfirming=true after a valid value")
	}
	if got.rawConfirmValue != 42 {
		t.Errorf("rawConfirmValue = %d, want 42", got.rawConfirmValue)
	}
	if cmd != nil {
		t.Error("moving to the confirmation prompt should not itself issue a command")
	}
}

func TestUpdate_RawEditing_EnterWithEmptyInput_SetsError(t *testing.T) {
	m := rawScreenModelWithTwoFeatures()
	m.rawEditing = true

	next, _ := m.Update(keyMsg("enter"))
	got := next.(Model)

	if !got.rawEditing {
		t.Error("expected to remain in edit mode after an invalid value")
	}
	if got.rawEditErr == "" {
		t.Error("expected rawEditErr to be set for empty input")
	}
}

func TestUpdate_RawEditing_EnterWithOutOfRangeValue_SetsError(t *testing.T) {
	m := rawScreenModelWithTwoFeatures()
	m.rawEditing = true
	m.rawEditInput = "999999"

	next, _ := m.Update(keyMsg("enter"))
	got := next.(Model)

	if got.rawConfirming {
		t.Error("expected an out-of-range value to be rejected, not sent to confirmation")
	}
	if got.rawEditErr == "" {
		t.Error("expected rawEditErr to be set for an out-of-range value")
	}
}

func TestUpdate_RawConfirming_YIssuesRawSetCmd(t *testing.T) {
	m := rawScreenModelWithTwoFeatures()
	m.rawCursor = 0 // recognized code 0x10
	m.rawConfirming = true
	m.rawConfirmValue = 50

	next, cmd := m.Update(keyMsg("y"))
	got := next.(Model)

	if got.rawConfirming {
		t.Error("expected rawConfirming=false after answering y")
	}
	if !got.rawWriting {
		t.Error("expected rawWriting=true while the write is in flight")
	}
	if cmd == nil {
		t.Fatal("expected a rawSetCmd to be returned after confirming")
	}
}

func TestUpdate_RawConfirming_NCancelsWithoutIssuingCmd(t *testing.T) {
	m := rawScreenModelWithTwoFeatures()
	m.rawConfirming = true
	m.rawConfirmValue = 50

	next, cmd := m.Update(keyMsg("n"))
	got := next.(Model)

	if got.rawConfirming {
		t.Error("expected rawConfirming=false after answering n")
	}
	if cmd != nil {
		t.Error("cancelling must not issue a command")
	}
}

func TestUpdate_RawConfirming_OtherKeysAreSwallowed(t *testing.T) {
	m := rawScreenModelWithTwoFeatures()
	m.rawConfirming = true
	m.rawConfirmValue = 50

	next, cmd := m.Update(keyMsg("j"))
	got := next.(Model)

	if !got.rawConfirming {
		t.Error("confirmation should remain open for an unrelated key")
	}
	if cmd != nil {
		t.Error("an unrelated key must not issue a command while raw-confirming")
	}
}

func TestUpdate_RawSetMsg_Success_ClearsWritingAndTriggersRefresh(t *testing.T) {
	m := rawScreenModelWithTwoFeatures()
	m.rawWriting = true

	next, cmd := m.Update(rawSetMsg{code: 0x10, err: nil})
	got := next.(Model)

	if got.rawWriting {
		t.Error("expected rawWriting=false after rawSetMsg")
	}
	if got.rawWriteErr != nil {
		t.Errorf("expected rawWriteErr=nil on success, got %v", got.rawWriteErr)
	}
	if cmd == nil {
		t.Fatal("expected a follow-up single-code probe to refresh the row after a successful write")
	}
}

func TestUpdate_RawSetMsg_Failure_SurfacesError(t *testing.T) {
	m := rawScreenModelWithTwoFeatures()
	m.rawWriting = true
	wantErr := errors.New("ddcutil setvcp 10 50: exit status 1: Verification failed")

	next, _ := m.Update(rawSetMsg{code: 0x10, err: wantErr})
	got := next.(Model)

	if got.rawWriting {
		t.Error("expected rawWriting=false after a failed rawSetMsg")
	}
	if got.rawWriteErr == nil {
		t.Error("expected rawWriteErr to be set after a failed write")
	}
}

func TestUpdate_RawSingleProbeMsg_UpdatesReading(t *testing.T) {
	m := rawScreenModelWithTwoFeatures()

	next, _ := m.Update(rawSingleProbeMsg{
		code:    0x10,
		reading: ddc.FeatureReading{Code: 0x10, Readable: true, Continuous: true, Current: 55, Max: 100},
	})
	got := next.(Model)

	r, ok := got.rawReadings[0x10]
	if !ok {
		t.Fatal("expected rawReadings[0x10] to be populated")
	}
	if r.Current != 55 {
		t.Errorf("rawReadings[0x10].Current = %d, want 55", r.Current)
	}
}

func TestUpdate_ProbeMsg_FreshScanHasNoPending(t *testing.T) {
	m := New()
	m.pending = map[uint8]bool{0x10: true} // leftover from a previous cached render, shouldn't survive

	next, _ := m.Update(probeMsg{
		caps:    &ddc.Capabilities{Features: []ddc.VCPFeature{{Code: 0x10, Name: "Brightness", Recognized: true}}},
		sliders: []components.Slider{components.NewSlider(0x10, "Brightness", 80, 100)},
		order:   []ctrlRef{{kind: kindSlider, idx: 0}},
	})
	got := next.(Model)

	if len(got.pending) != 0 {
		t.Errorf("pending = %v, want empty after a fresh (non-cached) scan", got.pending)
	}
}
