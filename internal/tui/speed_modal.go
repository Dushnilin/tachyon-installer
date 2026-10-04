package tui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	gossh "golang.org/x/crypto/ssh"

	routerpkg "tachyon-installer/internal/router"
	sshpkg "tachyon-installer/internal/ssh"
)

// ShowSpeedDoctorModal displays the interactive Bufferbloat & Speed Doctor diagnostic modal.
func ShowSpeedDoctorModal(ctx *AppContext, returnPage string) {
	panel := buildStepPanel(" 🚀 BUFFERBLOAT & SPEED DOCTOR ")

	statusView := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(false)
	statusView.SetBackgroundColor(ColorBgSpace)
	statusView.SetText(fmt.Sprintf("  %s⚡ Измерение сетевых задержек, джиттера и нагрузки на CPU... Пожалуйста, подождите (5-10 сек)%s", TagCyanBold, TagReset))

	resultsView := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true)
	resultsView.SetBackgroundColor(ColorBgSpace)

	form := tview.NewForm()
	StyleForm(form)
	form.SetBorderPadding(0, 0, 0, 0)
	form.SetItemPadding(1)

	closeModal := func() {
		ctx.Pages.RemovePage("speed_doctor_modal")
		if returnPage != "" {
			ctx.Pages.SwitchToPage(returnPage)
			page := ctx.Pages.GetPage(returnPage)
			if page != nil {
				ctx.App.SetFocus(page)
			}
		}
	}

	form.AddButton("🚪 Назад (Esc)", closeModal)
	form.SetCancelFunc(closeModal)
	EnableFormArrowNavigation(form)

	panel.AddItem(statusView, 2, 0, false)
	panel.AddItem(resultsView, 0, 1, false)
	panel.AddItem(formLayout(form), 5, 0, true)

	modal := CreateWizardModalCustom(panel, 94, 28)
	ctx.Pages.AddPage("speed_doctor_modal", modal, true, true)
	ctx.App.SetFocus(form)

	reconnectFn := func() (*gossh.Client, error) {
		if ctx.Config == nil {
			return nil, fmt.Errorf("no router config")
		}
		return sshpkg.Connect(ctx.Config.RouterIP, ctx.Config.SSHPort, ctx.Config.Username, ctx.Config.Password)
	}

	execFn := func(_ *gossh.Client, cmd string) (string, error) {
		return sshpkg.Exec(ctx.SSHClient, cmd, reconnectFn)
	}

	var (
		initialReport  *routerpkg.SpeedDoctorReport
		isBenchmarking bool
	)

	renderReport := func(report *routerpkg.SpeedDoctorReport) {
		var sb strings.Builder
		sb.WriteString("\n")

		if report.Direct != nil {
			d := report.Direct
			gradeBadge := formatGradeBadge(d.Grade)

			sb.WriteString(fmt.Sprintf("  %s  %s%s%s\n\n",
				gradeBadge, TagTextBold, d.Verdict, TagReset))

			sb.WriteString(fmt.Sprintf("  %s⚡ Скорость скачивания (WAN):%s   %s%.1f Мбит/с%s\n",
				TagCyanBold, TagReset, TagTextBold, d.SpeedMbps, TagReset))
			sb.WriteString(fmt.Sprintf("  %s⏱️  Задержка в покое (Idle):%s     %s%.1f мс%s  %s(мин: %.1f, макс: %.1f, джиттер: %.1f мс)%s\n",
				TagSubText, TagReset, TagGreenBold, d.IdlePingAvg, TagReset, TagMuted, d.IdlePingMin, d.IdlePingMax, d.IdleJitter, TagReset))
			sb.WriteString(fmt.Sprintf("  %s🌊 Задержка под нагрузкой:%s       %s%.1f мс%s\n",
				TagSubText, TagReset, getLoadedLatencyColor(d.Grade), d.LoadedPingAvg, TagReset))
			sb.WriteString(fmt.Sprintf("  %s📊 Bufferbloat Оверхед:%s         %s+%.1f мс%s %s(%s)%s\n\n",
				TagSubText, TagReset, getLoadedLatencyColor(d.Grade), d.BufferbloatDelta, TagReset, TagMuted, d.Grade, TagReset))

			// CPU meter
			cpuBar := formatCPULoadBar(d.CPU.TotalPct)
			cpuWarning := ""
			if d.CPU.Throttled {
				cpuWarning = fmt.Sprintf(" %s⚠️ CPU Throttling!%s", TagRedBold, TagReset)
			}
			sb.WriteString(fmt.Sprintf("  %s⚙️  Загрузка CPU роутера:%s        %s %s%.0f%%%s (sys: %.0f%%, softirq: %.0f%%)%s\n",
				TagSubText, TagReset, cpuBar, TagTextBold, d.CPU.TotalPct, TagReset, d.CPU.SystemPct, d.CPU.SoftIRQPct, cpuWarning))
		}

		if report.Tunnel != nil {
			t := report.Tunnel
			sb.WriteString(fmt.Sprintf("\n  %s🟣 ТУННЕЛЬ TACHYON (PROXY):%s\n", TagVioletBold, TagReset))
			sb.WriteString(fmt.Sprintf("     Скорость: %s%.1f Мбит/с%s  •  Пинг: %s%.1f мс%s (Bufferbloat: +%.1f мс)  •  CPU: %s%.0f%%%s\n",
				TagTextBold, t.SpeedMbps, TagReset, TagSubText, t.IdlePingAvg, TagReset, t.BufferbloatDelta, TagText, t.CPU.TotalPct, TagReset))
		}

		// Comparison if tuning was applied
		if initialReport != nil && report.TuningApplied && initialReport.Direct != nil && report.Direct != nil {
			initDelta := initialReport.Direct.BufferbloatDelta
			currDelta := report.Direct.BufferbloatDelta
			if initDelta > currDelta && currDelta > 0 {
				ratio := initDelta / currDelta
				sb.WriteString(fmt.Sprintf("\n  %s🎉 РЕЗУЛЬТАТ ОПТИМИЗАЦИИ:%s Bufferbloat снижен с %s+%.0f мс%s до %s+%.0f мс%s (%sв %.1f раз быстрее!%s)\n",
					TagGreenBold, TagReset, TagRedBold, initDelta, TagReset, TagGreenBold, currDelta, TagReset, TagCyanBold, ratio, TagReset))
			}
		}

		// Recommendations
		if len(report.Recommendations) > 0 {
			sb.WriteString(fmt.Sprintf("\n  %s💡 Рекомендации сетевого доктора:%s\n", TagAmberBold, TagReset))
			for _, rec := range report.Recommendations {
				sb.WriteString(fmt.Sprintf("   • %s%s%s\n", TagSubText, rec, TagReset))
			}
		}

		resultsView.SetText(sb.String())
	}

	var runBenchmark func(isAfterTuning bool)

	runBenchmark = func(isAfterTuning bool) {
		if isBenchmarking {
			return
		}
		isBenchmarking = true

		statusView.SetText(fmt.Sprintf("  %s⚡ Измерение сетевых задержек, джиттера и нагрузки на CPU... Пожалуйста, подождите (5-10 сек)%s", TagCyanBold, TagReset))
		resultsView.SetText(fmt.Sprintf("\n  %sЗапуск сетевого тестирования... Потоковая загрузка тестовых пакетов без записи на диск...%s\n", TagSubText, TagReset))

		go func() {
			client := ctx.SSHClient
			if client == nil {
				var err error
				client, err = reconnectFn()
				if err != nil {
					ctx.App.QueueUpdateDraw(func() {
						isBenchmarking = false
						statusView.SetText(fmt.Sprintf("  %s❌ Ошибка SSH подключения:%s %v", TagRedBold, TagReset, err))
					})
					return
				}
			}

			report, err := routerpkg.RunSpeedDoctor(client, execFn, routerpkg.SpeedDoctorOptions{
				DurationSec: 5,
			})

			ctx.App.QueueUpdateDraw(func() {
				isBenchmarking = false
				if err != nil {
					statusView.SetText(fmt.Sprintf("  %s❌ Ошибка бенчмарка:%s %v", TagRedBold, TagReset, err))
					return
				}

				if initialReport == nil {
					initialReport = report
				}
				if isAfterTuning {
					report.TuningApplied = true
				}

				statusView.SetText(fmt.Sprintf("  %s✅ Анализ завершен!%s  [Enter] Повторить  [T] Оптимизировать сетевой стек", TagGreenBold, TagReset))
				renderReport(report)
			})
		}()
	}

	// Button: Run Benchmark
	form.Clear(true)
	form.AddButton("🚀 Повторить тест", func() {
		runBenchmark(false)
	})

	// Button: Apply Tuning
	form.AddButton("⚡ Оптимизировать (T)", func() {
		if isBenchmarking {
			return
		}
		statusView.SetText(fmt.Sprintf("  %s⚡ Применение сетевого тюнинга fq_codel, BBR и оптимизации сокетов...%s", TagCyanBold, TagReset))
		go func() {
			client := ctx.SSHClient
			if client == nil {
				var err error
				client, err = reconnectFn()
				if err != nil {
					ctx.App.QueueUpdateDraw(func() {
						statusView.SetText(fmt.Sprintf("  %s❌ Ошибка SSH подключения:%s %v", TagRedBold, TagReset, err))
					})
					return
				}
			}

			tuneRep := routerpkg.TuneNetwork(client, execFn)
			ctx.App.QueueUpdateDraw(func() {
				if tuneRep.Success {
					statusView.SetText(fmt.Sprintf("  %s✅ Тюнинг применен!%s Запуск контрольного теста задержки...", TagGreenBold, TagReset))
					runBenchmark(true)
				} else {
					statusView.SetText(fmt.Sprintf("  %s❌ Ошибка применения тюнинга:%s %s", TagRedBold, TagReset, tuneRep.Details))
				}
			})
		}()
	})

	form.AddButton("🚪 Назад (Esc)", closeModal)

	// Hotkey Handler for modal
	form.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEscape:
			closeModal()
			return nil
		case tcell.KeyRune:
			switch event.Rune() {
			case 't', 'T', 'е', 'Е':
				// Trigger optimization
				idx := 1
				if form.GetButtonCount() > 1 {
					btn := form.GetButton(idx)
					if btn != nil {
						// Invoke tuning
						form.SetFocus(idx)
					}
				}
			}
		}
		return event
	})

	// Initial run
	runBenchmark(false)
}

func formatGradeBadge(grade routerpkg.BufferbloatGrade) string {
	switch grade {
	case routerpkg.GradeAPlus:
		return fmt.Sprintf("[#ffffff:#10b981:b]  GRADE A+  [-]  %sИДЕАЛЬНО (0ms Lag)%s", TagGreenBold, TagReset)
	case routerpkg.GradeA:
		return fmt.Sprintf("[#ffffff:#10b981:b]  GRADE A   [-]  %sОТЛИЧНО (<15ms)%s", TagGreenBold, TagReset)
	case routerpkg.GradeB:
		return fmt.Sprintf("[#ffffff:#0284c7:b]  GRADE B   [-]  %sХОРОШО (<30ms)%s", TagCyanBold, TagReset)
	case routerpkg.GradeC:
		return fmt.Sprintf("[#ffffff:#d97706:b]  GRADE C   [-]  %sУДОВЛЕТВОРИТЕЛЬНО (<60ms)%s", TagAmberBold, TagReset)
	case routerpkg.GradeD:
		return fmt.Sprintf("[#ffffff:#ca8a04:b]  GRADE D   [-]  %sВЫСОКАЯ ЗАДЕРЖКА (<120ms)%s", TagAmberBold, TagReset)
	default:
		return fmt.Sprintf("[#ffffff:#ef4444:b]  GRADE F   [-]  %sКРИТИЧЕСКИЙ BUFFERBLOAT (>120ms)%s", TagRedBold, TagReset)
	}
}

func getLoadedLatencyColor(grade routerpkg.BufferbloatGrade) string {
	switch grade {
	case routerpkg.GradeAPlus, routerpkg.GradeA:
		return TagGreenBold
	case routerpkg.GradeB:
		return TagCyanBold
	case routerpkg.GradeC, routerpkg.GradeD:
		return TagAmberBold
	default:
		return TagRedBold
	}
}

func formatCPULoadBar(pct float64) string {
	totalBars := 12
	filled := int((pct / 100.0) * float64(totalBars))
	if filled < 0 {
		filled = 0
	}
	if filled > totalBars {
		filled = totalBars
	}

	color := TagGreen
	if pct >= 75.0 {
		color = TagAmber
	}
	if pct >= 90.0 {
		color = TagRedBold
	}

	barStr := strings.Repeat("█", filled) + strings.Repeat("░", totalBars-filled)
	return fmt.Sprintf("[%s%s%s]", color, barStr, TagReset)
}
