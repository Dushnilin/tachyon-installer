package tui

import (
	"fmt"
	"strings"

	"github.com/rivo/tview"
	gossh "golang.org/x/crypto/ssh"

	routerpkg "tachyon-installer/internal/router"
	sshpkg "tachyon-installer/internal/ssh"
)

// ShowAWGModal displays an interactive AmneziaWG management, generator, and diagnostics modal.
func ShowAWGModal(ctx *AppContext, returnPage string) {
	panel := buildStepPanel(" 🛡️ УПРАВЛЕНИЕ И ГЕНЕРАТОР AMNEZIA WG ")

	infoView := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true)
	infoView.SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)

	initialText := "\n" +
		"  [#38bdf8:b]Генератор и интеграция AmneziaWG (AWG) в Tachyon:[-]\n\n" +
		"  [#cbd5e1]• Создает секцию с защитой от блокировок протокола WireGuard (DPI Junk/Magic)[-]\n" +
		"  [#cbd5e1]• Авто-генератор Cloudflare WARP с обфускацией заголовков (Jc, S1, S2, H1-H4)[-]\n" +
		"  [#cbd5e1]• Подключение списков обхода (YouTube, Discord, RKN) под ключ[-]\n" +
		"  [#cbd5e1]• Встроенная сквозная проверка подключения и доступности сервисов[-]\n\n" +
		"  [#94a3b8]Выберите действие в меню ниже: сгенерируйте профиль или укажите свой .conf[-]\n"

	infoView.SetText(initialText)

	form := tview.NewForm()
	StyleForm(form)

	closeModal := func() {
		ctx.Pages.RemovePage("awg_modal")
		if returnPage != "" {
			ctx.Pages.SwitchToPage(returnPage)
			page := ctx.Pages.GetPage(returnPage)
			if page != nil {
				ctx.App.SetFocus(page)
			}
		}
	}

	reconnectFn := func() (*gossh.Client, error) {
		if ctx.Config == nil {
			return nil, fmt.Errorf("no router config")
		}
		return sshpkg.Connect(ctx.Config.RouterIP, ctx.Config.SSHPort, ctx.Config.Username, ctx.Config.Password)
	}

	execFn := func(_ *gossh.Client, cmd string) (string, error) {
		return sshpkg.Exec(ctx.SSHClient, cmd, reconnectFn)
	}

	var currentCfg *routerpkg.AWGConfig

	// Update text view with config parameters preview
	renderConfigPreview := func(cfg *routerpkg.AWGConfig, extraMsg string) {
		if cfg == nil {
			return
		}
		var sb strings.Builder
		sb.WriteString("\n")
		if extraMsg != "" {
			sb.WriteString("  " + extraMsg + "\n\n")
		}
		sb.WriteString("  [#38bdf8:b]Параметры сгенерированной конфигурации AmneziaWG:[-]\n\n")
		sb.WriteString(fmt.Sprintf("  [#94a3b8]• Версия AWG:[-]       [#f1f5f9]%s[-]\n", cfg.Version))
		sb.WriteString(fmt.Sprintf("  [#94a3b8]• Сервер (Endpoint):[-] [#22c55e:b]%s:%d[-]\n", cfg.ServerAddress, cfg.ServerPort))
		sb.WriteString(fmt.Sprintf("  [#94a3b8]• Внутренний IP:[-]    [#f1f5f9]%s[-]\n", cfg.Address))

		pubDisplay := cfg.PeerPublicKey
		if len(pubDisplay) > 30 {
			pubDisplay = pubDisplay[:28] + "..."
		}
		sb.WriteString(fmt.Sprintf("  [#94a3b8]• Публичный ключ:[-]   [#cbd5e1]%s[-]\n", pubDisplay))

		sb.WriteString(fmt.Sprintf("  [#94a3b8]• Мусорные пакеты:[-]  [#eab308]Jc=%d[-], [#eab308]Jmin=%d[-], [#eab308]Jmax=%d[-]\n",
			cfg.Jc, cfg.Jmin, cfg.Jmax))
		sb.WriteString(fmt.Sprintf("  [#94a3b8]• Сдвиги пакетов:[-]   [#eab308]S1=%d[-], [#eab308]S2=%d[-]\n",
			cfg.S1, cfg.S2))
		sb.WriteString(fmt.Sprintf("  [#94a3b8]• Магические хэши:[-]  [#a855f7]H1=%s[-], [#a855f7]H2=%s[-], [#a855f7]H3=%s[-], [#a855f7]H4=%s[-]\n",
			cfg.H1, cfg.H2, cfg.H3, cfg.H4))

		sb.WriteString("\n  [#94a3b8]Готово к применению! Нажмите «🚀 Применить секцию» для записи в Tachyon.[-]\n")
		infoView.SetText(sb.String())
	}

	// 1. Action: Generate WARP Config
	runGenerateWarp := func() {
		infoView.SetText("\n  [#38bdf8]⚡ Генерация учетной записи Cloudflare WARP и вычисление параметров AWG...[-]\n")

		go func() {
			cfg, err := routerpkg.GenerateWarpAWG(ctx.SSHClient, execFn)
			ctx.App.QueueUpdateDraw(func() {
				if err != nil {
					infoView.SetText(fmt.Sprintf("\n  [#ef5350:b]❌ Ошибка генерации:[-] %v\n\n  [#94a3b8]Проверьте интернет-соединение или используйте ручной ввод .conf[-]\n", err))
					return
				}
				currentCfg = cfg
				renderConfigPreview(cfg, "[#22c55e:b]✓ Конфигурация Cloudflare WARP (AmneziaWG 2.0) успешно создана![-]")
			})
		}()
	}

	// 2. Action: Apply Config to Router
	runApply := func() {
		if currentCfg == nil {
			infoView.SetText("\n  [#ef5350:b]⚠️ Сначала создайте или введите конфигурацию![-]\n\n  [#cbd5e1]Нажмите «⚡ Сгенерировать WARP» или «📋 Вставить .conf»[-]\n")
			return
		}

		infoView.SetText("\n  [#38bdf8]⚡ Сохранение конфигурации в UCI, скачивание списков и перезапуск Tachyon...[-]\n")

		go func() {
			err := routerpkg.ApplyAWGSection(ctx.SSHClient, execFn, currentCfg, "awg", true)
			ctx.App.QueueUpdateDraw(func() {
				if err != nil {
					infoView.SetText(fmt.Sprintf("\n  [#ef5350:b]❌ Ошибка применения секции:[-] %v\n", err))
					return
				}

				msg := "\n  [#22c55e:b]✓ Секция AmneziaWG успешно применена и активирована![-]\n\n" +
					"  [#cbd5e1]• Служба Tachyon перезапущена с ядром sing-box[-]\n" +
					"  [#cbd5e1]• Списки YouTube, Discord и РКН подключены[-]\n\n" +
					"  [#38bdf8]Нажмите «🔍 Проверить работу» для верификации обхода блокировок.[-]\n"
				infoView.SetText(msg)
			})
		}()
	}

	// 3. Action: Test Connectivity & Bypass
	runTest := func() {
		infoView.SetText("\n  [#38bdf8]⚡ Выполняется тестирование обхода блокировок через AmneziaWG...[-]\n  [#94a3b8]Проверка sing-box, Fake-IP DNS, YouTube, Discord и egress IP...[-]\n")

		go func() {
			rep, err := routerpkg.TestAWGSection(ctx.SSHClient, execFn, "awg")
			ctx.App.QueueUpdateDraw(func() {
				var sb strings.Builder
				sb.WriteString("\n")
				if err != nil && rep == nil {
					sb.WriteString(fmt.Sprintf("  [#ef5350:b]❌ Ошибка тестирования:[-] %v\n", err))
				} else if rep != nil {
					if rep.Success {
						sb.WriteString("  [#22c55e:b]✓ ТЕСТИРОВАНИЕ AMNEZIA WG ПРОШЛО УСПЕШНО![-]\n\n")
					} else {
						sb.WriteString("  [#eab308:b]⚠️ РЕЗУЛЬТАТЫ ПРОВЕРКИ AMNEZIA WG:[-]\n\n")
					}
					sb.WriteString(rep.FormatSummary())
					sb.WriteString("\n")
					if rep.Success {
						sb.WriteString(fmt.Sprintf("  [#22c55e:b]Статус:[-] %s\n", rep.Details))
					} else {
						sb.WriteString(fmt.Sprintf("  [#eab308:b]Замечание:[-] %s\n", rep.Details))
					}
				}
				infoView.SetText(sb.String())
			})
		}()
	}

	// 4. Action: Manual input dialog
	showManualModal := func() {
		inputPanel := buildStepPanel(" 📋 ВВОД КОНФИГУРАЦИИ AMNEZIA WG ")
		inputForm := tview.NewForm()
		StyleForm(inputForm)

		var inputStr string
		inputArea := tview.NewTextArea().
			SetPlaceholder("[Interface]\nPrivateKey = ...\nAddress = 10.77.0.2/32\nJc = 4\n...\n[Peer]\nPublicKey = ...\nEndpoint = ip:port\n\nИли ссылка vpn://...")
		inputArea.SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)
		inputArea.SetBorder(true).SetTitle(" Текст .conf или vpn:// ")

		closeManual := func() {
			ctx.Pages.RemovePage("awg_manual_modal")
			ctx.Pages.SwitchToPage("awg_modal")
			ctx.App.SetFocus(form)
		}

		saveManual := func() {
			inputStr = inputArea.GetText()
			cfg, err := routerpkg.ParseAWGConfig(inputStr)
			if err != nil {
				infoView.SetText(fmt.Sprintf("\n  [#ef5350:b]❌ Некорректный конфиг:[-] %v\n", err))
				closeManual()
				return
			}
			currentCfg = cfg
			renderConfigPreview(cfg, "[#22c55e:b]✓ Конфигурация успешно распознана и загружена![-]")
			closeManual()
		}

		inputForm.AddButton("✅ Использовать", saveManual)
		inputForm.AddButton("Отмена", closeManual)
		inputForm.SetCancelFunc(closeManual)
		EnableFormArrowNavigation(inputForm)

		inputPanel.AddItem(inputArea, 12, 1, true)
		inputPanel.AddItem(formLayout(inputForm), 4, 1, false)

		subModal := CreateWizardModalCustom(inputPanel, 84, 20)
		ctx.Pages.AddPage("awg_manual_modal", subModal, true, true)
		ctx.App.SetFocus(inputArea)
	}

	form.AddButton("⚡ Сгенерировать WARP", runGenerateWarp)
	form.AddButton("📋 Вставить .conf", showManualModal)
	form.AddButton("🚀 Применить секцию", runApply)
	form.AddButton("🔍 Проверить работу", runTest)
	form.AddButton("🚪 Закрыть", closeModal)
	form.SetCancelFunc(closeModal)
	EnableFormArrowNavigation(form)

	panel.AddItem(infoView, 13, 1, false)
	panel.AddItem(nil, 1, 0, false)
	panel.AddItem(formLayout(form), 4, 1, true)

	modal := CreateWizardModalCustom(panel, 88, 21)
	ctx.Pages.AddPage("awg_modal", modal, true, true)
	ctx.App.SetFocus(form)
}
