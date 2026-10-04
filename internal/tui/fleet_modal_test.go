package tui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestShowFleetModal(t *testing.T) {
	app := tview.NewApplication()
	pages := tview.NewPages()

	ctx := &AppContext{
		App:   app,
		Pages: pages,
	}

	ShowFleetModal(ctx, "")

	if !pages.HasPage("fleet_modal") {
		t.Errorf("expected fleet_modal to be registered in pages")
	}
}

func TestFleetModal_Draw(t *testing.T) {
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	screen.SetSize(96, 28)

	panel := buildStepPanel(" ТЕСТ FLEET MANAGER ")

	statusView := tview.NewTextView().SetDynamicColors(true)
	statusView.SetText("  Найдено роутеров: 3 • Обновить: 1 • Чистые: 1 • Актуальны: 1")

	hintView := tview.NewTextView().SetDynamicColors(true)
	hintView.SetText("  [Space] Выбрать  [A] Все  [U] Устаревшие  [C] Чистые  [Enter] Запуск")

	table := tview.NewTable().SetBorders(false)
	table.SetCell(0, 0, tview.NewTableCell(" [✓] "))
	table.SetCell(0, 1, tview.NewTableCell("IP-адрес"))
	table.SetCell(0, 2, tview.NewTableCell("Модель"))
	table.SetCell(0, 3, tview.NewTableCell("Архитектура"))
	table.SetCell(0, 4, tview.NewTableCell("ОЗУ"))
	table.SetCell(0, 5, tview.NewTableCell("Статус"))

	table.SetCell(1, 0, tview.NewTableCell(" [✓] "))
	table.SetCell(1, 1, tview.NewTableCell("192.168.1.1"))
	table.SetCell(1, 2, tview.NewTableCell("Xiaomi AX3000T"))
	table.SetCell(1, 3, tview.NewTableCell("arm64"))
	table.SetCell(1, 4, tview.NewTableCell("233 МБ"))
	table.SetCell(1, 5, tview.NewTableCell("Обновить (v1.5.0 → v1.6.2)"))

	form := tview.NewForm()
	StyleForm(form)
	form.AddButton("🚀 Установить на выбранные (Enter)", nil)
	form.AddButton("✕ Закрыть (Esc)", nil)

	panel.AddItem(statusView, 2, 0, false)
	panel.AddItem(hintView, 1, 0, false)
	panel.AddItem(nil, 1, 0, false)
	panel.AddItem(table, 0, 1, true)
	panel.AddItem(nil, 1, 0, false)
	panel.AddItem(formLayout(form), 5, 0, false)

	modal := CreateWizardModalCustom(panel, 96, 28)
	modal.SetRect(0, 0, 96, 28)
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

	if !strings.Contains(rendered, "Xiaomi AX3000T") {
		t.Errorf("missing router model in rendered fleet modal:\n%s", rendered)
	}
	if !strings.Contains(rendered, "arm64") {
		t.Errorf("missing arch in rendered fleet modal:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Установить на выбранные") {
		t.Errorf("missing button in rendered fleet modal:\n%s", rendered)
	}
}
