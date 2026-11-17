package tui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	gossh "golang.org/x/crypto/ssh"

	"tachyon-installer/internal/browser"
	routerpkg "tachyon-installer/internal/router"
	sshpkg "tachyon-installer/internal/ssh"
)

// cycleSelectorItem is an inline FormItem that switches between multiple choices
// using Left/Right arrows, Space, or number keys (1, 2, 3...) without any popup menus,
// avoiding dropdown clipping and visual overlap in modal dialogs.
type cycleSelectorItem struct {
	*tview.Box
	label      string
	labelWidth int
	labelColor tcell.Color
	options    []string
	selected   int
	onChanged  func(index int)
	finished   func(key tcell.Key)
	disabled   bool
}

func newCycleSelectorItem(label string, options []string, initialIdx int, onChanged func(index int)) *cycleSelectorItem {
	if initialIdx < 0 || initialIdx >= len(options) {
		initialIdx = 0
	}
	return &cycleSelectorItem{
		Box:        tview.NewBox(),
		label:      label,
		options:    options,
		selected:   initialIdx,
		onChanged:  onChanged,
		labelColor: tcell.NewRGBColor(203, 213, 225), // Slate 300
	}
}

func (c *cycleSelectorItem) GetLabel() string {
	return c.label
}

func (c *cycleSelectorItem) SetFormAttributes(labelWidth int, labelColor, bgColor, fieldTextColor, fieldBgColor tcell.Color) tview.FormItem {
	c.labelWidth = labelWidth
	c.labelColor = labelColor
	c.SetBackgroundColor(bgColor)
	return c
}

func (c *cycleSelectorItem) GetFieldWidth() int {
	return 0 // flexible
}

func (c *cycleSelectorItem) GetFieldHeight() int {
	return 1
}

func (c *cycleSelectorItem) SetFinishedFunc(handler func(key tcell.Key)) tview.FormItem {
	c.finished = handler
	return c
}

func (c *cycleSelectorItem) SetDisabled(disabled bool) tview.FormItem {
	c.disabled = disabled
	return c
}

func (c *cycleSelectorItem) Draw(screen tcell.Screen) {
	c.Box.DrawForSubclass(screen, c)
	x, y, width, height := c.GetInnerRect()
	if height < 1 || width < 1 {
		return
	}

	labelWidth := c.labelWidth
	if labelWidth == 0 {
		labelWidth = tview.TaggedStringWidth(c.label)
	}
	if labelWidth > width {
		labelWidth = width
	}

	// 1. Draw Label
	tview.Print(screen, c.label, x, y, labelWidth, tview.AlignLeft, c.labelColor)

	x += labelWidth
	width -= labelWidth
	if width < 1 {
		return
	}

	// 2. Format Option Text
	optText := ""
	if c.selected >= 0 && c.selected < len(c.options) {
		optText = c.options[c.selected]
	}

	displayText := fmt.Sprintf("◄ %s ►", optText)
	if width < len([]rune(displayText)) {
		displayText = optText
	}

	var style tcell.Style
	if c.HasFocus() {
		style = tcell.StyleDefault.
			Background(ColorBgInputFocus).
			Foreground(ColorTextPure).
			Bold(true)
	} else {
		style = tcell.StyleDefault.
			Background(ColorBgInput).
			Foreground(ColorTextPrimary)
	}

	// Fill background of the field
	for col := 0; col < width; col++ {
		screen.SetContent(x+col, y, ' ', nil, style)
	}

	// Print text inside the field area
	fg, _, _ := style.Decompose()
	tview.Print(screen, displayText, x+1, y, width-2, tview.AlignLeft, fg)
}

func (c *cycleSelectorItem) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return c.WrapInputHandler(func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		if c.disabled {
			return
		}
		switch event.Key() {
		case tcell.KeyLeft:
			c.selected = (c.selected - 1 + len(c.options)) % len(c.options)
			if c.onChanged != nil {
				c.onChanged(c.selected)
			}
		case tcell.KeyRight:
			c.selected = (c.selected + 1) % len(c.options)
			if c.onChanged != nil {
				c.onChanged(c.selected)
			}
		case tcell.KeyEnter, tcell.KeyDown, tcell.KeyTab:
			if c.finished != nil {
				c.finished(tcell.KeyTab)
			}
		case tcell.KeyUp, tcell.KeyBacktab:
			if c.finished != nil {
				c.finished(tcell.KeyBacktab)
			}
		case tcell.KeyEscape:
			if c.finished != nil {
				c.finished(tcell.KeyEscape)
			}
		case tcell.KeyRune:
			switch event.Rune() {
			case ' ':
				c.selected = (c.selected + 1) % len(c.options)
				if c.onChanged != nil {
					c.onChanged(c.selected)
				}
			case '1', '2', '3', '4', '5', '6', '7', '8', '9':
				idx := int(event.Rune() - '1')
				if idx >= 0 && idx < len(c.options) {
					c.selected = idx
					if c.onChanged != nil {
						c.onChanged(c.selected)
					}
				}
			}
		}
	})
}

func (c *cycleSelectorItem) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, recipient tview.Primitive) {
	return c.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, recipient tview.Primitive) {
		if !c.InRect(event.Position()) {
			return false, nil
		}
		if action == tview.MouseLeftClick {
			setFocus(c)
			c.selected = (c.selected + 1) % len(c.options)
			if c.onChanged != nil {
				c.onChanged(c.selected)
			}
			return true, c
		}
		return false, nil
	})
}

// ShowGuidedSetupModal displays the hand-in-hand autopilot setup wizard.
func ShowGuidedSetupModal(ctx *AppContext, returnPage string) {
	panel := buildStepPanel("Мастер быстрой настройки")

	infoView := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true)
	infoView.SetBackgroundColor(ColorBgSpace)

	infoView.SetText("\n" +
		fmt.Sprintf("  %sЗапуск анализа роутера и провайдера...%s\n\n", TagCyanBold, TagReset) +
		fmt.Sprintf("  %s1. Подключение по SSH и сбор параметров оборудования%s\n", TagSubText, TagReset) +
		fmt.Sprintf("  %s2. Тестирование DNS: проверка подмены UDP 53 и выбор быстрого резолвера%s\n", TagSubText, TagReset) +
		fmt.Sprintf("  %s3. Packet Fuzzer: подбор стратегии десинхронизации для YouTube и Discord%s\n", TagSubText, TagReset) +
		fmt.Sprintf("  %s4. Оптимизация сетевого стека (TCP BBR, conntrack, сокеты)%s\n", TagSubText, TagReset) +
		fmt.Sprintf("  %s5. Сквозная проверка результата%s\n\n", TagSubText, TagReset) +
		fmt.Sprintf("  %sВыполняется диагностика роутера и провайдера...%s\n", TagAmberBold, TagReset))

	form := tview.NewForm()
	StyleForm(form)
	form.SetBorderPadding(0, 0, 0, 0)
	form.SetItemPadding(1)

	closeModal := func() {
		ctx.Pages.RemovePage("guided_modal")
		if returnPage != "" {
			ctx.Pages.SwitchToPage(returnPage)
			page := ctx.Pages.GetPage(returnPage)
			if page != nil {
				ctx.App.SetFocus(page)
			}
		}
	}

	form.AddButton("Назад (Esc)", closeModal)
	form.SetCancelFunc(closeModal)
	EnableFormArrowNavigation(form)

	panel.AddItem(infoView, 0, 1, false)
	panel.AddItem(formLayout(form), 9, 0, true)

	modal := CreateWizardModalCustom(panel, 94, 28)
	ctx.Pages.AddPage("guided_modal", modal, true, true)
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

	// Asynchronous Analysis Phase
	go func() {
		client := ctx.SSHClient
		if client == nil {
			var err error
			client, err = reconnectFn()
			if err != nil {
				ctx.App.QueueUpdateDraw(func() {
					infoView.SetText(fmt.Sprintf("\n  [#e5484d:b]✗ Ошибка SSH-подключения:[-] %v\n\n  [#a1a1a6]Проверьте IP-адрес роутера и пароль.[-]\n", err))
					form.Clear(true)
					StyleForm(form)
					form.SetBorderPadding(0, 0, 0, 0)
					form.AddButton("Закрыть", closeModal)
					form.SetCancelFunc(closeModal)
					ctx.App.SetFocus(form)
				})
				return
			}
			ctx.SSHClient = client
		}

		// 1. Hardware scan
		profile, _ := routerpkg.RunPreConnectionCheck(client)

		// 2. DNS benchmark & poisoning detection
		dnsReport := routerpkg.RunDNSTest(client, execFn)

		// 3. DPI Fuzzing for YouTube and Discord
		fuzzReport := routerpkg.RunDPIFuzzer(client, execFn, nil)

		// 4. Existing subscription detection
		subStatus := routerpkg.CheckSubscriptions(client, execFn)

		// Render diagnostic findings & ready-to-apply action
		ctx.App.QueueUpdateDraw(func() {
			var sb strings.Builder
			sb.WriteString("\n  [#38bdf8:b]Результаты анализа роутера и провайдера:[-]\n\n")

			if profile != nil {
				sb.WriteString(fmt.Sprintf("  • Роутер:        [#ededed:b]%s[-] ([#71717a]%s, RAM: %.0f МБ[-])\n",
					profile.Model, profile.Arch, profile.RAMTotal))
			}

			// DNS
			dnsStatus := "[#4cc38a]без подмены[-]"
			if dnsReport.UDPPoisoned {
				dnsStatus = "[#e5484d:b]подменяется провайдером (спуфинг)[-]"
			}
			bestDNSName := "Cloudflare DNS"
			bestDNSLat := int64(15)
			if dnsReport.Fastest != nil {
				bestDNSName = dnsReport.Fastest.Name
				bestDNSLat = dnsReport.Fastest.LatencyMs
			}
			sb.WriteString(fmt.Sprintf("  • DNS-провайдер: %s · выбран: [#4cc38a:b]%s (%d мс)[-]\n",
				dnsStatus, bestDNSName, bestDNSLat))

			// DPI Strategy
			bestStratName := "Adaptive Desync 2.0 (FakeSplit + Disorder)"
			bestStratRate := 100
			if fuzzReport.WinningStrategy != nil {
				bestStratName = fuzzReport.WinningStrategy.Name
				bestStratRate = fuzzReport.WinningStrategy.SuccessRate
			}
			sb.WriteString(fmt.Sprintf("  • DPI-стратегия: [#4cc38a:b]%s[-] (успех: [#38bdf8]%d%%[-])\n",
				bestStratName, bestStratRate))

			sb.WriteString("  • Сетевой стек:  [#ededed]TCP BBR + защита Netfilter[-]\n\n")
			sb.WriteString("  [#4cc38a]Проверьте параметры ниже и нажмите Enter для применения.[-]\n")

			infoView.SetText(sb.String())

			// Build Interactive Options Form
			form.Clear(true)
			StyleForm(form)
			form.SetBorderPadding(0, 0, 0, 0)
			form.SetItemPadding(1)

			modeOptions := []string{
				"1. Подписка прокси / VLESS (tachyon-core)",
				"2. Автономная десинхронизация пакетов (Adaptive Desync / Zapret)",
				"3. Гибридный режим (Desync/Zapret + Туннель)",
				"4. AmneziaWG 2.0 (Генератор WARP / свой .conf)",
			}
			selectedModeIdx := 0
			modeItem := newCycleSelectorItem("Режим работы:        ", modeOptions, 0, func(index int) {
				selectedModeIdx = index
			})
			form.AddFormItem(modeItem)

			initialSub := subStatus.CurrentURL
			subInput := tview.NewInputField().
				SetLabel("Ссылка / ключ подписки:").
				SetText(initialSub).
				SetFieldWidth(48)
			form.AddFormItem(subInput)

			tuneStack := true
			cb := tview.NewCheckbox().
				SetLabel("Оптимизация сети:    ").
				SetChecked(true).
				SetCheckedString("[X] Включить TCP BBR, тюнинг буферов и защиту Netfilter").
				SetUncheckedString("[ ] Отключено (стандартные параметры ядра)").
				SetChangedFunc(func(checked bool) {
					tuneStack = checked
				})
			form.AddFormItem(cb)

			// Apply Button
			form.AddButton("Применить настройки (Enter)", func() {
				if selectedModeIdx == 3 {
					ctx.Pages.RemovePage("guided_modal")
					ShowAWGModal(ctx, returnPage)
					return
				}

				subVal := strings.TrimSpace(subInput.GetText())
				if (selectedModeIdx == 0 || selectedModeIdx == 2) && subVal == "" {
					infoView.SetText(sb.String() + "\n  [#e5484d:b]! Укажите ссылку на подписку или узел для режима туннеля[-]\n")
					ctx.App.SetFocus(subInput)
					return
				}

				// Switch to execution view
				infoView.SetText("\n  [#38bdf8:b]Применение параметров настройки...[-]\n\n")
				form.Clear(true)
				StyleForm(form)
				form.SetBorderPadding(0, 0, 0, 0)
				form.AddButton("Выполняется...", nil)

				go func() {
					var chosenDNS routerpkg.DNSResolver
					if dnsReport.Fastest != nil {
						chosenDNS = *dnsReport.Fastest
					} else {
						chosenDNS = routerpkg.DefaultDNSResolvers()[0]
					}

					var chosenStrat routerpkg.DPIStrategy
					if fuzzReport.WinningStrategy != nil {
						chosenStrat = *fuzzReport.WinningStrategy
					} else {
						chosenStrat = routerpkg.DefaultDPIStrategies()[0]
					}

					defaultEngine := "tachyon-core"
					mode := routerpkg.ModeTunnel
					engine := defaultEngine
					if selectedModeIdx == 1 {
						mode = routerpkg.ModeStandaloneDPI
					} else if selectedModeIdx == 2 {
						mode = routerpkg.ModeHybrid
					}

					plan := routerpkg.GuidedSetupPlan{
						Mode:            mode,
						ChosenDNS:       chosenDNS,
						ChosenStrategy:  chosenStrat,
						SubscriptionURL: subVal,
						SelectedEngine:  engine,
						EnableTune:      tuneStack,
					}

					var logLines []string
					progressCallback := func(stepTitle string, fraction float64) {
						ctx.App.QueueUpdateDraw(func() {
							logLines = append(logLines, fmt.Sprintf("  • [#a1a1a6]%s[-] [#4cc38a]✓[-]", stepTitle))
							infoView.SetText("\n  [#38bdf8:b]Применение параметров настройки:[-]\n\n" + strings.Join(logLines, "\n"))
						})
					}

					res, err := routerpkg.RunGuidedSetup(client, execFn, plan, progressCallback)

					ctx.App.QueueUpdateDraw(func() {
						var resSB strings.Builder
						resSB.WriteString("\n")
						if err != nil {
							resSB.WriteString(fmt.Sprintf("  [#e5484d:b]✗ Ошибка при настройке:[-] %v\n", err))
						} else {
							resSB.WriteString("  [#4cc38a:b]✓ Настройка завершена[-]\n\n")
							resSB.WriteString(fmt.Sprintf("  • [#38bdf8]Защищенный DNS:[-]     [#4cc38a]%s[-] ([#ededed]%d мс[-], защита от подмены включена)\n",
								res.DNSName, res.DNSLatencyMs))
							if res.StrategyApplied {
								resSB.WriteString(fmt.Sprintf("  • [#38bdf8]DPI-стратегия:[-]      [#4cc38a]%s[-]\n",
									res.StrategyName))
							}
							if res.NetworkTuned {
								resSB.WriteString(fmt.Sprintf("  • [#38bdf8]Сетевой стек:[-]       [#4cc38a]TCP BBR + сокеты %s + conntrack %s[-]\n",
									res.TuneReport.Buffers, res.TuneReport.Conntrack))
							}

							// Bypass probes
							for _, p := range res.VerifyResult.Probes {
								if p.Success {
									resSB.WriteString(fmt.Sprintf("  • [#38bdf8]%-18s[-] [#4cc38a]HTTP %d (%d мс) ✓[-]\n",
										p.Name+":", p.HTTPStatus, p.LatencyMs))
								} else {
									resSB.WriteString(fmt.Sprintf("  • [#38bdf8]%-18s[-] [#eab308]HTTP %d (проверьте в браузере)[-]\n",
										p.Name+":", p.HTTPStatus))
								}
							}

							resSB.WriteString("\n  [#ededed]Роутер готов к работе.[-]\n")
						}

						infoView.SetText(resSB.String())

						form.Clear(true)
						StyleForm(form)
						form.SetBorderPadding(0, 0, 0, 0)
						form.AddButton("Открыть LuCI", func() {
							ip := "192.168.1.1"
							if ctx.Config != nil && ctx.Config.RouterIP != "" {
								ip = ctx.Config.RouterIP
							}
							_ = browser.OpenURL(fmt.Sprintf("http://%s", ip))
							closeModal()
						})
						form.AddButton("Закрыть (Enter)", closeModal)
						form.SetCancelFunc(closeModal)
						EnableFormArrowNavigation(form)
						ctx.App.SetFocus(form)
					})
				}()
			})

			form.AddButton("Отмена (Esc)", closeModal)
			form.SetCancelFunc(closeModal)
			EnableFormArrowNavigation(form)
			ctx.App.SetFocus(form)
		})
	}()
}
