package tui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func renderOptionsWithReleases(t *testing.T, w, h int, tags []string) (string, *OptionsSelector) {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	screen.SetSize(w, h)

	InitTheme()
	opt := NewOptionsSelector(ProfileData{
		Model:     "QEMU KVM Virtual Machine",
		Version:   "25.12.5",
		Arch:      "arm64",
		Firewall:  "fw4",
		RAMTotal:  727,
		RAMFree:   452,
		FlashFree: 3768,
	})
	if len(tags) > 0 {
		opt.SetReleases(tags)
	}

	panel := buildStepPanel(" ШАГ 4/4 ")
	panel.AddItem(opt, 0, 1, true)
	modal := CreateWizardModalCustom(panel, 92, 28)
	modal.SetRect(0, 0, w, h)
	modal.Draw(screen)
	screen.Show()

	cells, cw, ch := screen.GetContents()
	var sb strings.Builder
	for y := 0; y < ch; y++ {
		for x := 0; x < cw; x++ {
			c := cells[y*cw+x]
			if len(c.Runes) > 0 {
				sb.WriteRune(c.Runes[0])
			}
		}
		sb.WriteByte('\n')
	}
	return sb.String(), opt
}

func TestLatestVersionDisplayNotTruncated(t *testing.T) {
	tags := []string{"1.4.9", "1.4.8", "1.4.7", "1.4.6", "1.4.5"}

	for _, sz := range [][2]int{{120, 40}, {92, 28}, {80, 24}} {
		out, _ := renderOptionsWithReleases(t, sz[0], sz[1], tags)

		// Ensure the full latest version string is present and not cut off like "latest (1.4"
		if !strings.Contains(out, "latest (1.4.9)") {
			t.Errorf("%dx%d: expected 'latest (1.4.9)' to be rendered completely, got output:\n%s", sz[0], sz[1], out)
		}
		if strings.Contains(out, "latest (1.4 ") || strings.Contains(out, "latest (1.4  ") {
			t.Errorf("%dx%d: detected truncated latest version string in output:\n%s", sz[0], sz[1], out)
		}
		if !strings.Contains(out, "Рекомендуется") {
			t.Errorf("%dx%d: expected 'Рекомендуется' badge for latest, got output:\n%s", sz[0], sz[1], out)
		}

		// Verify all other versions are also rendered
		for _, tag := range tags {
			if !strings.Contains(out, tag) {
				t.Errorf("%dx%d: expected tag %s in rendered output", sz[0], sz[1], tag)
			}
		}
		if !strings.Contains(out, "Вручную...") {
			t.Errorf("%dx%d: expected 'Вручную...' in rendered output", sz[0], sz[1])
		}
	}
}

func TestVersionNavigation2D(t *testing.T) {
	tags := []string{"1.4.9", "1.4.8", "1.4.7", "1.4.6", "1.4.5"}
	opt := NewOptionsSelector(ProfileData{RAMTotal: 512, RAMFree: 300})
	opt.SetReleases(tags)
	opt.SetRect(0, 0, 92, 28)

	// Switch active section to SectionVersion
	opt.activeSection = SectionVersion
	opt.versionCursor = 0
	opt.selectedVersion = 0

	handler := opt.InputHandler()
	noopFocus := func(p any) {}
	_ = noopFocus

	sendKey := func(k tcell.Key) {
		handler(tcell.NewEventKey(k, 0, tcell.ModNone), nil)
	}
	sendRune := func(r rune) {
		handler(tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone), nil)
	}

	// 1. Initial: at 0 (latest)
	if opt.versionCursor != 0 {
		t.Fatalf("expected cursor at 0, got %d", opt.versionCursor)
	}

	// 2. Down from latest: should move to row 1, col 0 (index 1: "1.4.9")
	sendKey(tcell.KeyDown)
	if opt.versionCursor != 1 {
		t.Errorf("expected cursor at 1 after Down, got %d", opt.versionCursor)
	}
	if opt.activeSection != SectionVersion {
		t.Errorf("expected activeSection to still be SectionVersion, got %d", opt.activeSection)
	}

	// 3. Down from index 1: with 3 sub-columns, 1 + 3 = 4 (index 4: "1.4.6")
	sendKey(tcell.KeyDown)
	if opt.versionCursor != 4 {
		t.Errorf("expected cursor at 4 after Down, got %d", opt.versionCursor)
	}

	// 4. Down from index 4 (bottom row): should move to SectionMirror
	sendKey(tcell.KeyDown)
	if opt.activeSection != SectionMirror {
		t.Errorf("expected transition to SectionMirror after Down on bottom row, got %d", opt.activeSection)
	}

	// 5. Up from SectionMirror: should return to SectionVersion on corresponding bottom row element (index 4)
	sendKey(tcell.KeyUp)
	if opt.activeSection != SectionVersion {
		t.Errorf("expected transition back to SectionVersion after Up, got %d", opt.activeSection)
	}
	if opt.versionCursor != 4 {
		t.Errorf("expected cursor at bottom row (4) after returning from mirror, got %d", opt.versionCursor)
	}

	// 6. Up from index 4: moves back to index 1
	sendKey(tcell.KeyUp)
	if opt.versionCursor != 1 {
		t.Errorf("expected cursor at 1 after Up from row 2, got %d", opt.versionCursor)
	}

	// 7. Up from index 1: moves to row 0 (latest, index 0)
	sendKey(tcell.KeyUp)
	if opt.versionCursor != 0 {
		t.Errorf("expected cursor at 0 after Up from row 1, got %d", opt.versionCursor)
	}

	// 8. Up from index 0: moves to SectionEngine
	sendKey(tcell.KeyUp)
	if opt.activeSection != SectionEngine {
		t.Errorf("expected transition to SectionEngine after Up from latest, got %d", opt.activeSection)
	}

	// 9. Back down to SectionVersion: test Left / Right
	opt.activeSection = SectionVersion
	opt.versionCursor = 0
	sendKey(tcell.KeyRight)
	if opt.versionCursor != 1 {
		t.Errorf("expected cursor at 1 after Right, got %d", opt.versionCursor)
	}
	sendKey(tcell.KeyLeft)
	if opt.versionCursor != 0 {
		t.Errorf("expected cursor at 0 after Left, got %d", opt.versionCursor)
	}

	// 10. Number selection: '3' should select index 2 ("1.4.8")
	sendRune('3')
	if opt.versionCursor != 2 || opt.selectedVersion != 2 {
		t.Errorf("expected cursor and selectedVersion at 2 after '3', got cursor=%d, selected=%d",
			opt.versionCursor, opt.selectedVersion)
	}

	// 11. Space key selection: move cursor to 3 ("1.4.7") without selecting, then press Space
	opt.versionCursor = 3
	opt.selectedVersion = 2
	sendRune(' ')
	if opt.selectedVersion != 3 {
		t.Errorf("expected selectedVersion to update to 3 on Space, got %d", opt.selectedVersion)
	}
}

func TestEngineOptionsConfigurationAndSteerC(t *testing.T) {
	opt := NewOptionsSelector(ProfileData{RAMTotal: 512, RAMFree: 300})

	foundTiny := false
	foundExtCompressed := false

	for _, eng := range opt.engines {
		// 1. User requirement: "никакой не рекомендуется" (no engine should have "Рекомендуется")
		if strings.Contains(eng.Badge, "Рекомендуется") || strings.Contains(eng.Desc, "Рекомендуется") {
			t.Errorf("engine %s should NOT have 'Рекомендуется': badge=%q desc=%q", eng.Key, eng.Badge, eng.Desc)
		}

		// 2. User requirement: Steer is in C, NOT Go
		if eng.Key == "steer" || eng.Key == "steer-extended" {
			if strings.Contains(strings.ToLower(eng.Desc), "go") || strings.Contains(strings.ToLower(eng.Badge), "go") {
				t.Errorf("engine %s incorrectly mentions Go: badge=%q desc=%q", eng.Key, eng.Badge, eng.Desc)
			}
			if !strings.Contains(eng.Badge, "C-движок") && !strings.Contains(eng.Desc, "C") {
				t.Errorf("engine %s should state C-engine: badge=%q desc=%q", eng.Key, eng.Badge, eng.Desc)
			}
		}

		if eng.Key == "sing-box-tiny" {
			foundTiny = true
		}
		if eng.Key == "sing-box-extended-compressed" {
			foundExtCompressed = true
		}
	}

	if !foundTiny {
		t.Errorf("expected sing-box-tiny in engine options")
	}
	if !foundExtCompressed {
		t.Errorf("expected sing-box-extended-compressed in engine options")
	}

	// 3. Test numeric shortcuts 1..7 for engines
	handler := opt.InputHandler()
	opt.activeSection = SectionEngine
	handler(tcell.NewEventKey(tcell.KeyRune, '3', tcell.ModNone), nil) // sing-box-tiny
	if opt.selectedEngine != 2 || opt.engines[opt.selectedEngine].Key != "sing-box-tiny" {
		t.Errorf("expected '3' to select sing-box-tiny (index 2), got index=%d key=%s",
			opt.selectedEngine, opt.engines[opt.selectedEngine].Key)
	}

	handler(tcell.NewEventKey(tcell.KeyRune, '5', tcell.ModNone), nil) // steer
	if opt.selectedEngine != 4 || opt.engines[opt.selectedEngine].Key != "steer" {
		t.Errorf("expected '5' to select steer (index 4), got index=%d key=%s",
			opt.selectedEngine, opt.engines[opt.selectedEngine].Key)
	}
}

func TestOptionsSelectorMouseClicksInModal(t *testing.T) {
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	screen.SetSize(120, 40)

	opt := NewOptionsSelector(ProfileData{Model: "Router", RAMTotal: 512, RAMFree: 300})
	panel := buildStepPanel(" ШАГ 4/4 ")
	panel.AddItem(opt, 0, 1, true)
	modal := CreateWizardModalCustom(panel, 94, 30)

	modal.SetRect(0, 0, 120, 40)
	modal.Draw(screen)

	// In Draw, opt.clickTargets are registered.
	// Click on targets directly via modal.MouseHandler()
	mouseHandler := modal.MouseHandler()
	if mouseHandler == nil {
		t.Fatalf("expected modal to provide MouseHandler")
	}

	// Verify click targets were populated
	if len(opt.clickTargets) == 0 {
		t.Fatalf("expected click targets to be registered in Draw")
	}

	// Target 2 should be the 3rd engine ("sing-box-tiny")
	targetTiny := opt.clickTargets[2]
	ev := tcell.NewEventMouse((targetTiny.x1+targetTiny.x2)/2, targetTiny.y1, tcell.Button1, 0)
	consumed, _ := mouseHandler(tview.MouseLeftClick, ev, func(p tview.Primitive) {})
	if !consumed {
		t.Errorf("expected mouse click on sing-box-tiny to be consumed")
	}
	if opt.selectedEngine != 2 || opt.engines[opt.selectedEngine].Key != "sing-box-tiny" {
		t.Errorf("expected sing-box-tiny (index 2) to be selected after mouse click, got %d key=%s",
			opt.selectedEngine, opt.engines[opt.selectedEngine].Key)
	}
}

func TestZRAMOptionAndHardwareAdvice(t *testing.T) {
	// Constrained router (<128MB RAM)
	optLow := NewOptionsSelector(ProfileData{
		Model:     "GL-MT300N",
		RAMTotal:  64,
		RAMFree:   20,
		FlashFree: 10,
	})

	optsLow := optLow.GetInstallOptions()
	if !optsLow.InstallZRAM {
		t.Errorf("expected InstallZRAM to be true by default for 64MB RAM router")
	}
	if optsLow.SelectedEngine != "steer" {
		t.Errorf("expected steer engine to be preselected for 64MB router, got %s", optsLow.SelectedEngine)
	}

	// High-end router (>512MB RAM)
	optHigh := NewOptionsSelector(ProfileData{
		Model:     "x86_64 Gateway",
		RAMTotal:  1024,
		RAMFree:   800,
		FlashFree: 500,
	})
	optsHigh := optHigh.GetInstallOptions()
	if optsHigh.InstallZRAM {
		t.Errorf("expected InstallZRAM to be false by default for 1024MB RAM router")
	}
}
