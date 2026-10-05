package tui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestShowGuidedSetupModal(t *testing.T) {
	app := tview.NewApplication()
	pages := tview.NewPages()

	ctx := &AppContext{
		App:   app,
		Pages: pages,
	}

	ShowGuidedSetupModal(ctx, "")

	if !pages.HasPage("guided_modal") {
		t.Errorf("expected guided_modal to be registered in pages")
	}
}

func TestCycleSelectorItem_KeysAndCycle(t *testing.T) {
	options := []string{
		"1. Steer / Zapret",
		"2. VLESS Tunnel",
		"3. Hybrid Mode",
	}

	selectedIdx := 0
	item := newCycleSelectorItem("Режим: ", options, 0, func(idx int) {
		selectedIdx = idx
	})

	if item.GetLabel() != "Режим: " {
		t.Errorf("unexpected label: %s", item.GetLabel())
	}
	if item.GetFieldHeight() != 1 {
		t.Errorf("expected height 1, got %d", item.GetFieldHeight())
	}

	handler := item.InputHandler()
	if handler == nil {
		t.Fatalf("expected non-nil input handler")
	}

	// Press Right Arrow -> should switch to index 1
	handler(tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone), func(p tview.Primitive) {})
	if selectedIdx != 1 || item.selected != 1 {
		t.Errorf("expected index 1 after Right arrow, got %d", selectedIdx)
	}

	// Press Right Arrow again -> should switch to index 2
	handler(tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone), func(p tview.Primitive) {})
	if selectedIdx != 2 || item.selected != 2 {
		t.Errorf("expected index 2 after Right arrow, got %d", selectedIdx)
	}

	// Press Right Arrow again -> should wrap around to index 0
	handler(tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone), func(p tview.Primitive) {})
	if selectedIdx != 0 || item.selected != 0 {
		t.Errorf("expected index 0 after Right wrap, got %d", selectedIdx)
	}

	// Press Left Arrow -> should wrap backward to index 2
	handler(tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone), func(p tview.Primitive) {})
	if selectedIdx != 2 || item.selected != 2 {
		t.Errorf("expected index 2 after Left arrow, got %d", selectedIdx)
	}

	// Press Space -> should cycle forward to 0
	handler(tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone), func(p tview.Primitive) {})
	if selectedIdx != 0 || item.selected != 0 {
		t.Errorf("expected index 0 after Space, got %d", selectedIdx)
	}

	// Press '2' -> should select index 1 directly
	handler(tcell.NewEventKey(tcell.KeyRune, '2', tcell.ModNone), func(p tview.Primitive) {})
	if selectedIdx != 1 || item.selected != 1 {
		t.Errorf("expected index 1 after typing '2', got %d", selectedIdx)
	}
}

func TestCycleSelectorItem_FinishedCallback(t *testing.T) {
	options := []string{"Opt 1", "Opt 2"}
	item := newCycleSelectorItem("Test: ", options, 0, nil)

	var lastFinishedKey tcell.Key = -1
	item.SetFinishedFunc(func(key tcell.Key) {
		lastFinishedKey = key
	})

	handler := item.InputHandler()

	// Enter key triggers tab to next field
	handler(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(p tview.Primitive) {})
	if lastFinishedKey != tcell.KeyTab {
		t.Errorf("expected finished key KeyTab on Enter, got %v", lastFinishedKey)
	}

	// Down key triggers tab to next field
	handler(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone), func(p tview.Primitive) {})
	if lastFinishedKey != tcell.KeyTab {
		t.Errorf("expected finished key KeyTab on Down, got %v", lastFinishedKey)
	}

	// Up key triggers backtab
	handler(tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone), func(p tview.Primitive) {})
	if lastFinishedKey != tcell.KeyBacktab {
		t.Errorf("expected finished key KeyBacktab on Up, got %v", lastFinishedKey)
	}

	// Escape key triggers escape
	handler(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone), func(p tview.Primitive) {})
	if lastFinishedKey != tcell.KeyEscape {
		t.Errorf("expected finished key KeyEscape on Escape, got %v", lastFinishedKey)
	}
}

func TestCycleSelectorItem_Draw(t *testing.T) {
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	screen.SetSize(80, 24)

	options := []string{"Автономный (Steer/Zapret)", "VLESS Туннель"}
	item := newCycleSelectorItem("Режим работы: ", options, 0, nil)
	item.SetRect(2, 2, 70, 1)
	item.Draw(screen)
	screen.Show()

	cells, cw, _ := screen.GetContents()
	var row strings.Builder
	for x := 0; x < cw; x++ {
		c := cells[2*cw+x]
		if len(c.Runes) > 0 {
			row.WriteRune(c.Runes[0])
		}
	}
	output := row.String()
	if !strings.Contains(output, "Режим работы:") {
		t.Errorf("rendered row missing label: %s", output)
	}
	if !strings.Contains(output, "Автономный (Steer/Zapret)") {
		t.Errorf("rendered row missing selected option: %s", output)
	}
}

func TestGuidedSetupForm_AllControlsVisibleWithoutClipping(t *testing.T) {
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	screen.SetSize(92, 27)

	panel := buildStepPanel(" ТЕСТ АВТОПИЛОТА ")

	infoView := tview.NewTextView().SetDynamicColors(true)
	infoView.SetText("\n  Диагностика завершена.\n  Рекомендована стратегия FakeSplit.\n")

	form := tview.NewForm()
	StyleForm(form)
	form.SetBorderPadding(0, 0, 0, 0)
	form.SetItemPadding(1)

	modeOptions := []string{
		"1. VLESS / Sing-box подписка (Туннель)",
		"2. Автономный обход (Steer/Zapret — без серверов)",
		"3. Гибридный режим (Zapret + VLESS Туннель)",
	}
	modeItem := newCycleSelectorItem("Режим работы:        ", modeOptions, 0, nil)
	form.AddFormItem(modeItem)

	subInput := tview.NewInputField().
		SetLabel("Ссылка / ключ подписки:").
		SetFieldWidth(48)
	form.AddFormItem(subInput)

	cb := tview.NewCheckbox().
		SetLabel("Оптимизация сети:    ").
		SetChecked(true).
		SetCheckedString("[X] Включить TCP BBR, тюнинг буферов и защиту Netfilter").
		SetUncheckedString("[ ] Отключено (стандартные параметры ядра)")
	form.AddFormItem(cb)

	form.AddButton("🚀 Настроить всё автоматически (Enter)", nil)
	form.AddButton("Отмена (Esc)", nil)

	panel.AddItem(infoView, 0, 1, false)
	panel.AddItem(formLayout(form), 9, 0, true)

	modal := CreateWizardModalCustom(panel, 92, 27)
	modal.SetRect(0, 0, 92, 27)
	modal.Draw(screen)
	screen.Show()

	cells, cw, ch := screen.GetContents()
	var allText strings.Builder
	for y := 0; y < ch; y++ {
		for x := 0; x < cw; x++ {
			c := cells[y*cw+x]
			if len(c.Runes) > 0 {
				allText.WriteRune(c.Runes[0])
			}
		}
		allText.WriteByte('\n')
	}
	rendered := allText.String()

	// Verify all elements are simultaneously visible without scrolling:
	if !strings.Contains(rendered, "Режим работы:") {
		t.Errorf("missing 'Режим работы:' in modal render:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Ссылка / ключ подписки:") {
		t.Errorf("missing 'Ссылка / ключ подписки:' in modal render:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Оптимизация сети:") {
		t.Errorf("missing 'Оптимизация сети:' in modal render:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Настроить всё автоматически") {
		t.Errorf("missing 'Настроить всё автоматически' button in modal render:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Отмена (Esc)") {
		t.Errorf("missing 'Отмена (Esc)' button in modal render:\n%s", rendered)
	}
}
