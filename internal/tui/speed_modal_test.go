package tui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	routerpkg "tachyon-installer/internal/router"
)

func TestShowSpeedDoctorModal(t *testing.T) {
	app := tview.NewApplication()
	pages := tview.NewPages()

	ctx := &AppContext{
		App:   app,
		Pages: pages,
	}

	ShowSpeedDoctorModal(ctx, "")

	if !pages.HasPage("speed_doctor_modal") {
		t.Errorf("expected speed_doctor_modal to be registered in pages")
	}
}

func TestFormatGradeBadge(t *testing.T) {
	tests := []struct {
		grade   routerpkg.BufferbloatGrade
		contain string
	}{
		{routerpkg.GradeAPlus, "GRADE A+"},
		{routerpkg.GradeA, "GRADE A"},
		{routerpkg.GradeB, "GRADE B"},
		{routerpkg.GradeC, "GRADE C"},
		{routerpkg.GradeD, "GRADE D"},
		{routerpkg.GradeF, "GRADE F"},
	}

	for _, tt := range tests {
		badge := formatGradeBadge(tt.grade)
		if !strings.Contains(badge, tt.contain) {
			t.Errorf("expected badge to contain %q, got: %s", tt.contain, badge)
		}
	}
}

func TestFormatCPULoadBar(t *testing.T) {
	bar0 := formatCPULoadBar(0)
	if !strings.Contains(bar0, "░░░░░░░░░░░░") {
		t.Errorf("expected 12 empty blocks at 0%%, got: %s", bar0)
	}

	bar100 := formatCPULoadBar(100)
	if !strings.Contains(bar100, "████████████") {
		t.Errorf("expected 12 full blocks at 100%%, got: %s", bar100)
	}

	bar50 := formatCPULoadBar(50)
	if !strings.Contains(bar50, "██████░░░░░░") {
		t.Errorf("expected 6 full and 6 empty blocks at 50%%, got: %s", bar50)
	}
}

func TestGetLoadedLatencyColor(t *testing.T) {
	cAPlus := getLoadedLatencyColor(routerpkg.GradeAPlus)
	if cAPlus != TagGreenBold {
		t.Errorf("expected TagGreenBold for Grade A+, got %s", cAPlus)
	}

	cB := getLoadedLatencyColor(routerpkg.GradeB)
	if cB != TagCyanBold {
		t.Errorf("expected TagCyanBold for Grade B, got %s", cB)
	}

	cC := getLoadedLatencyColor(routerpkg.GradeC)
	if cC != TagAmberBold {
		t.Errorf("expected TagAmberBold for Grade C, got %s", cC)
	}

	cF := getLoadedLatencyColor(routerpkg.GradeF)
	if cF != TagRedBold {
		t.Errorf("expected TagRedBold for Grade F, got %s", cF)
	}
}

func TestSpeedDoctorModal_Draw(t *testing.T) {
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	screen.SetSize(94, 28)

	panel := buildStepPanel(" 🚀 BUFFERBLOAT & SPEED DOCTOR ")

	statusView := tview.NewTextView().SetDynamicColors(true)
	statusView.SetText("  ⚡ Анализ завершен!  [Enter] Повторить  [T] Оптимизировать сетевой стек")

	resultsView := tview.NewTextView().SetDynamicColors(true)
	report := &routerpkg.SpeedDoctorReport{
		Direct: &routerpkg.BenchmarkRun{
			Mode:             "direct",
			SpeedMbps:        84.5,
			IdlePingAvg:      14.2,
			IdlePingMin:      13.5,
			IdlePingMax:      15.8,
			IdleJitter:       0.9,
			LoadedPingAvg:    21.4,
			BufferbloatDelta: 7.2,
			Grade:            routerpkg.GradeA,
			Verdict:          "Отличная сеть, лаги и задержки в играх и звонках маловероятны",
			CPU: routerpkg.CPUSample{
				TotalPct:   18.5,
				SystemPct:  12.0,
				SoftIRQPct: 6.5,
				Throttled:  false,
			},
		},
		Recommendations: []string{
			"Сеть работает эффективно. Для дальнейшей стабилизации рекомендуется алгоритм fq_codel.",
		},
	}

	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(formatGradeBadge(report.Direct.Grade))
	sb.WriteString("\n  Скорость скачивания (WAN): 84.5 Мбит/с\n")
	sb.WriteString("  Задержка в покое (Idle): 14.2 мс\n")
	resultsView.SetText(sb.String())

	form := tview.NewForm()
	StyleForm(form)
	form.AddButton("🚀 Повторить тест", nil)
	form.AddButton("⚡ Оптимизировать (T)", nil)
	form.AddButton("🚪 Назад (Esc)", nil)

	panel.AddItem(statusView, 2, 0, false)
	panel.AddItem(resultsView, 0, 1, false)
	panel.AddItem(formLayout(form), 5, 0, true)

	modal := CreateWizardModalCustom(panel, 94, 28)
	modal.SetRect(0, 0, 94, 28)
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

	if !strings.Contains(rendered, "BUFFERBLOAT & SPEED DOCTOR") {
		t.Errorf("missing header in rendered modal:\n%s", rendered)
	}
	if !strings.Contains(rendered, "GRADE A") {
		t.Errorf("missing grade in rendered modal:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Скорость скачивания") {
		t.Errorf("missing speed metric in rendered modal:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Оптимизировать (T)") {
		t.Errorf("missing button in rendered modal:\n%s", rendered)
	}
}
