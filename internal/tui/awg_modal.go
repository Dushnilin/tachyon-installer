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
	panel := buildStepPanel("Управление AmneziaWG")

	infoView := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true)
	infoView.SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)

	initialText := "\n" +
		"  [#38bdf8:b]Генератор и настройка AmneziaWG 2.0 в Tachyon:[-]\n\n" +
		"  [#a1a1a6]• Создает секцию с обфускацией протокола WireGuard (Jc, S1–S4, H1–H4, CPS I1–I5, J1–J3)[-]\n" +
		"  [#a1a1a6]• Встроенный генератор профиля Cloudflare WARP с обфускацией заголовков[-]\n" +
		"  [#a1a1a6]• Автоматическое подключение списков маршрутизации (YouTube, Discord, RU-списки)[-]\n" +
		"  [#a1a1a6]• Проверка туннеля и доступности сервисов[-]\n\n" +
		"  [#71717a]Выберите действие ниже: сгенерируйте профиль WARP или вставьте свой .conf / vpn://[-]\n"

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
		sb.WriteString("  [#38bdf8:b]Параметры конфигурации AmneziaWG:[-]\n\n")
		sb.WriteString(fmt.Sprintf("  [#71717a]• Версия AWG:[-]       [#ededed]%s[-]\n", cfg.Version))
		sb.WriteString(fmt.Sprintf("  [#71717a]• Сервер (Endpoint):[-] [#4cc38a:b]%s:%d[-]\n", cfg.ServerAddress, cfg.ServerPort))
		sb.WriteString(fmt.Sprintf("  [#71717a]• Внутренний IP:[-]    [#ededed]%s[-]\n", cfg.Address))

		pubDisplay := cfg.PeerPublicKey
		if len(pubDisplay) > 30 {
			pubDisplay = pubDisplay[:28] + "..."
		}
		sb.WriteString(fmt.Sprintf("  [#71717a]• Публичный ключ:[-]   [#a1a1a6]%s[-]\n", pubDisplay))

		sb.WriteString(fmt.Sprintf("  [#71717a]• Мусорные пакеты:[-]  [#eab308]Jc=%d[-], [#eab308]Jmin=%d[-], [#eab308]Jmax=%d[-]\n",
			cfg.Jc, cfg.Jmin, cfg.Jmax))
		sb.WriteString(fmt.Sprintf("  [#71717a]• Сдвиги пакетов:[-]   [#eab308]S1=%d[-], [#eab308]S2=%d[-]\n",
			cfg.S1, cfg.S2))
		sb.WriteString(fmt.Sprintf("  [#71717a]• Заголовки H1–H4:[-]  [#38bdf8]H1=%s[-], [#38bdf8]H2=%s[-], [#38bdf8]H3=%s[-], [#38bdf8]H4=%s[-]\n",
			cfg.H1, cfg.H2, cfg.H3, cfg.H4))

		sb.WriteString("\n  [#71717a]Готово к применению. Нажмите «Применить секцию» для записи в Tachyon.[-]\n")
		infoView.SetText(sb.String())
	}

	// 1. Action: Generate WARP Config
	runGenerateWarp := func() {
		infoView.SetText("\n  [#38bdf8]Генерация профиля Cloudflare WARP и параметров AmneziaWG...[-]\n")

		go func() {
			cfg, err := routerpkg.GenerateWarpAWG(ctx.SSHClient, execFn)
			ctx.App.QueueUpdateDraw(func() {
				if err != nil {
					infoView.SetText(fmt.Sprintf("\n  [#e5484d:b]✗ Ошибка генерации:[-] %v\n\n  [#71717a]Проверьте подключение к интернету или вставьте готовый .conf[-]\n", err))
					return
				}
				currentCfg = cfg
				renderConfigPreview(cfg, "[#4cc38a:b]✓ Конфигурация Cloudflare WARP (AmneziaWG 2.0) создана[-]")
			})
		}()
	}

	// 2. Action: Apply Config to Router
	runApply := func() {
		if currentCfg == nil {
			infoView.SetText("\n  [#eab308:b]! Сначала создайте или введите конфигурацию[-]\n\n  [#a1a1a6]Нажмите «Сгенерировать WARP» или «Вставить .conf»[-]\n")
			return
		}

		infoView.SetText("\n  [#38bdf8]Сохранение конфигурации в UCI, обновление списков и перезапуск Tachyon...[-]\n")

		go func() {
			err := routerpkg.ApplyAWGSection(ctx.SSHClient, execFn, currentCfg, "awg", true)
			ctx.App.QueueUpdateDraw(func() {
				if err != nil {
					infoView.SetText(fmt.Sprintf("\n  [#e5484d:b]✗ Ошибка применения секции:[-] %v\n", err))
					return
				}

				msg := "\n  [#4cc38a:b]✓ Секция AmneziaWG применена и активирована[-]\n\n" +
					"  [#a1a1a6]• Служба Tachyon перезапущена[-]\n" +
					"  [#a1a1a6]• Списки маршрутизации (YouTube, Discord) подключены[-]\n\n" +
					"  [#38bdf8]Нажмите «Проверить работу» для тестирования соединения.[-]\n"
				infoView.SetText(msg)
			})
		}()
	}

	// 3. Action: Test Connectivity & Bypass
	runTest := func() {
		infoView.SetText("\n  [#38bdf8]Тестирование соединения через AmneziaWG...[-]\n  [#71717a]Проверка ядра прокси, DNS, YouTube, Discord и внешнего IP...[-]\n")

		go func() {
			rep, err := routerpkg.TestAWGSection(ctx.SSHClient, execFn, "awg")
			ctx.App.QueueUpdateDraw(func() {
				var sb strings.Builder
				sb.WriteString("\n")
				if err != nil && rep == nil {
					sb.WriteString(fmt.Sprintf("  [#e5484d:b]✗ Ошибка тестирования:[-] %v\n", err))
				} else if rep != nil {
					if rep.Success {
						sb.WriteString("  [#4cc38a:b]✓ Проверка AmneziaWG завершена успешно[-]\n\n")
					} else {
						sb.WriteString("  [#eab308:b]! Результаты проверки AmneziaWG:[-]\n\n")
					}
					sb.WriteString(rep.FormatSummary())
					sb.WriteString("\n")
					if rep.Success {
						sb.WriteString(fmt.Sprintf("  [#4cc38a:b]Статус:[-] %s\n", rep.Details))
					} else {
						sb.WriteString(fmt.Sprintf("  [#eab308:b]Примечание:[-] %s\n", rep.Details))
					}
				}
				infoView.SetText(sb.String())
			})
		}()
	}

	// 4. Action: Manual input dialog
	showManualModal := func() {
		inputPanel := buildStepPanel("Ввод конфигурации AmneziaWG")
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
				infoView.SetText(fmt.Sprintf("\n  [#e5484d:b]✗ Некорректный конфиг:[-] %v\n", err))
				closeManual()
				return
			}
			currentCfg = cfg
			renderConfigPreview(cfg, "[#4cc38a:b]✓ Конфигурация распознана и загружена[-]")
			closeManual()
		}

		inputForm.AddButton("Использовать", saveManual)
		inputForm.AddButton("Отмена", closeManual)
		inputForm.SetCancelFunc(closeManual)
		EnableFormArrowNavigation(inputForm)

		inputPanel.AddItem(inputArea, 12, 1, true)
		inputPanel.AddItem(formLayout(inputForm), 4, 1, false)

		subModal := CreateWizardModalCustom(inputPanel, 88, 22)
		ctx.Pages.AddPage("awg_manual_modal", subModal, true, true)
		ctx.App.SetFocus(inputArea)
	}

	form.AddButton("Сгенерировать WARP", runGenerateWarp)
	form.AddButton("Вставить .conf", showManualModal)
	form.AddButton("Применить секцию", runApply)
	form.AddButton("Проверить работу", runTest)
	form.AddButton("Закрыть", closeModal)
	form.SetCancelFunc(closeModal)
	EnableFormArrowNavigation(form)

	panel.AddItem(infoView, 13, 1, false)
	panel.AddItem(nil, 1, 0, false)
	panel.AddItem(formLayout(form), 4, 1, true)

	modal := CreateWizardModalCustom(panel, 88, 22)
	ctx.Pages.AddPage("awg_modal", modal, true, true)
	ctx.App.SetFocus(form)
}
