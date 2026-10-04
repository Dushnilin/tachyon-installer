package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func renderOptions(t *testing.T, w, h int) string {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	screen.SetSize(w, h)

	InitTheme()
	panel := buildStepPanel(" ШАГ 4/4 ")
	panel.AddItem(NewOptionsSelector(ProfileData{Model: "Test Router", Version: "25.12", Arch: "arm64", Firewall: "fw4", RAMTotal: 512, RAMFree: 300, FlashFree: 100}), 0, 1, true)
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
	return sb.String()
}

var _ = tview.Styles

func TestOptionsAdaptToTerminalSize(t *testing.T) {
	for _, sz := range [][2]int{{120, 40}, {92, 28}, {80, 24}, {60, 18}, {40, 12}} {
		out := renderOptions(t, sz[0], sz[1])
		if !strings.Contains(out, "ЯДРО") {
			t.Errorf("%dx%d: engine section missing", sz[0], sz[1])
		}
		for _, line := range strings.Split(out, "\n") {
			if len([]rune(line)) > sz[0] {
				t.Errorf("%dx%d: line wider than screen", sz[0], sz[1])
			}
		}
	}
}

func TestModalMouseClickPropagation(t *testing.T) {
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	screen.SetSize(100, 30)

	clicked := false
	btn := tview.NewButton("ClickMe").SetSelectedFunc(func() {
		clicked = true
	})

	modal := CreateWizardModalCustom(btn, 40, 10)
	modal.Draw(screen)

	mouseHandler := modal.MouseHandler()
	if mouseHandler == nil {
		t.Fatalf("expected modal to provide MouseHandler")
	}

	// Click in the center of the screen where button is drawn (50, 15)
	ev := tcell.NewEventMouse(50, 15, tcell.Button1, 0)
	consumed, _ := mouseHandler(tview.MouseLeftClick, ev, func(p tview.Primitive) {})
	if !consumed {
		t.Errorf("expected mouse click to be consumed by inner button")
	}
	if !clicked {
		t.Errorf("expected button action to be triggered by mouse click")
	}
}

func TestAdaptiveModalsAcrossResolutions(t *testing.T) {
	resolutions := [][2]int{
		{140, 45}, // 4K / Ultrawide / Large terminal
		{100, 30}, // Standard desktop terminal
		{80, 24},  // Traditional 80x24 standard
		{70, 20},  // Compact split-pane
		{60, 18},  // Small mobile / embed terminal
	}

	InitTheme()

	for _, res := range resolutions {
		w, h := res[0], res[1]
		t.Run(fmt.Sprintf("%dx%d", w, h), func(t *testing.T) {
			screen := tcell.NewSimulationScreen("UTF-8")
			if err := screen.Init(); err != nil {
				t.Fatal(err)
			}
			defer screen.Fini()
			screen.SetSize(w, h)

			// 1. Test buildStepPanel
			panel := buildStepPanel(" ТЕСТ АДАПТИВНОСТИ ")
			panel.AddItem(tview.NewTextView().SetText("Test Content"), 0, 1, false)
			modal := CreateWizardModalCustom(panel, 90, 26)
			modal.SetRect(0, 0, w, h)
			modal.Draw(screen)
			screen.Show()

			cells, cw, _ := screen.GetContents()
			if cw != w {
				t.Errorf("%dx%d: expected width %d, got %d", w, h, w, cw)
			}
			if len(cells) != w*h {
				t.Errorf("%dx%d: cells count %d != w*h %d", w, h, len(cells), w*h)
			}
		})
	}
}

