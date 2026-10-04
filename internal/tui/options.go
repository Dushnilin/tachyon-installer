package tui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	routerpkg "tachyon-installer/internal/router"
)

// Section indices for navigation.
const (
	SectionEngine     = 0
	SectionVersion    = 1
	SectionMirror     = 2
	SectionLang       = 3
	SectionBtnInstall = 4
	SectionBtnBack    = 5
	totalSections     = 6
)

// EngineItem represents a selectable proxy engine option.
type EngineItem struct {
	Key   string
	Name  string
	Badge string
	Desc  string
}

// VersionItem represents a selectable Tachyon version option.
type VersionItem struct {
	Key   string
	Name  string
	Badge string
}

// MirrorItem represents a selectable download mirror option.
type MirrorItem struct {
	Key   string
	Name  string
	Badge string
}

// OptionsSelector is a custom tview.Primitive displaying all installation parameters
// on a single comprehensive dashboard with visual radio buttons and keyboard navigation.
type OptionsSelector struct {
	*tview.Box

	Profile ProfileData

	// 1. Engines
	engines        []EngineItem
	selectedEngine int
	engineCursor   int

	// 2. Versions
	versions          []VersionItem
	selectedVersion   int
	versionCursor     int
	customVersionText string

	// 3. Mirrors
	mirrors        []MirrorItem
	selectedMirror int
	mirrorCursor   int

	// 4. Localization & System Options
	installRussian bool
	installZRAM    bool
	langCursor     int

	// Active section focus (0..5)
	activeSection int

	// clipBottom is the first screen row that must not be drawn on.
	clipBottom int

	// clickTargets tracks clickable areas on screen for mouse support.
	clickTargets []optClickTarget

	// Callbacks
	OnSubmit               func(opts InstallOptions)
	OnBack                 func()
	OnRequestManualVersion func(current string, callback func(newVer string))
	OnDiagnostics          func()

	prefVersion string
}

type optClickTarget struct {
	x1, y1, x2, y2 int
	onClick        func()
}

// NewOptionsSelector initializes the OptionsSelector with default choices.
func NewOptionsSelector(profile ProfileData) *OptionsSelector {
	defaultZRAM := false
	if (profile.RAMTotal > 0 && profile.RAMTotal < 128) || (profile.RAMFree > 0 && profile.RAMFree < 40) {
		defaultZRAM = true
	}

	recEngine, _ := routerpkg.HardwareRecommendation(profile.RAMTotal, profile.RAMFree, profile.FlashFree)
	initialEngIdx := 0

	engines := []EngineItem{
		{Key: "sing-box-extended", Name: "sing-box-extended", Badge: "xHTTP", Desc: "xHTTP, Reality, ShadowTLS, gRPC, TUIC, Hy2"},
		{Key: "sing-box-extended-compressed", Name: "sing-box-ext-compressed", Badge: "xHTTP (сжат)", Desc: "sing-box-extended в сжатом бинарнике (минимум Flash)"},
		{Key: "sing-box-tiny", Name: "sing-box-tiny", Badge: "Tiny", Desc: "Минималистичная сборка OpenWrt (минимум RAM)"},
		{Key: "sing-box-lx", Name: "sing-box-lx", Badge: "Leadaxe", Desc: "Компактная сборка sing-box от Leadaxe"},
		{Key: "steer", Name: "steer", Badge: "C-движок", Desc: "Ультра-легковесный модульный C-движок (минимум RAM)"},
		{Key: "steer-extended", Name: "steer-extended", Badge: "C-движок+", Desc: "Модульный steer на C + xHTTP, оптимизация под нагрузку"},
		{Key: "skip", Name: "Без ядра", Badge: "Пропустить", Desc: "Настроить или загрузить ядро позже в LuCI"},
	}

	if recEngine != "" {
		for i, e := range engines {
			if e.Key == recEngine {
				initialEngIdx = i
				break
			}
		}
	}

	opt := &OptionsSelector{
		Box:            tview.NewBox(),
		Profile:        profile,
		engines:        engines,
		selectedEngine: initialEngIdx,
		engineCursor:   initialEngIdx,

		versions: []VersionItem{
			{Key: "latest", Name: "latest", Badge: "Стабильный"},
			{Key: "custom", Name: "Вручную...", Badge: ""},
		},
		selectedVersion:   0,
		versionCursor:     0,
		customVersionText: "latest",

		mirrors: []MirrorItem{
			{Key: "auto", Name: "Автовыбор ★", Badge: "быстрейшее"},
			{Key: "https://gh-proxy.com/", Name: "gh-proxy.com", Badge: "зеркало"},
			{Key: "https://ghfast.top/", Name: "ghfast.top", Badge: "зеркало"},
			{Key: "https://gh.ddlc.top/", Name: "gh.ddlc.top", Badge: "зеркало"},
			{Key: "https://gh-proxy.org/", Name: "gh-proxy.org", Badge: "зеркало"},
			{Key: "direct", Name: "github.com (прямо)", Badge: "напрямую"},
		},
		selectedMirror: 0,
		mirrorCursor:   0,

		installRussian: true,
		installZRAM:    defaultZRAM,
		langCursor:     0,
		activeSection:  SectionEngine,
	}

	opt.SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)
	return opt
}

// GetInstallOptions returns the assembled options for installation.
func (opt *OptionsSelector) GetInstallOptions() InstallOptions {
	engineKey := opt.engines[opt.selectedEngine].Key
	mirrorKey := opt.mirrors[opt.selectedMirror].Key

	ver := opt.versions[opt.selectedVersion].Key
	if opt.selectedVersion == len(opt.versions)-1 {
		ver = "latest"
		if t := strings.TrimSpace(opt.customVersionText); t != "" {
			ver = t
		}
	}

	return InstallOptions{
		TachyonVersion: ver,
		SelectedEngine: engineKey,
		SelectedMirror: mirrorKey,
		InstallI18n:    opt.installRussian,
		InstallZRAM:    opt.installZRAM,
	}
}

// Draw renders the OptionsSelector dashboard.
func (opt *OptionsSelector) Draw(screen tcell.Screen) {
	opt.Box.DrawForSubclass(screen, opt)
	x, y, width, height := opt.GetInnerRect()
	if width <= 0 || height <= 0 {
		return
	}
	opt.clickTargets = opt.clickTargets[:0]

	curY := y

	// Hardware summary (compact, fits narrow terminals)
	opt.clipBottom = y + height
	modelStr := []rune(opt.Profile.Model)
	if len(modelStr) > 22 {
		modelStr = append(modelStr[:20], '.', '.')
	}
	fw := "fw4"
	if opt.Profile.Firewall == "fw3" {
		fw = "fw3 (устар.)"
	}
	archStr := opt.Profile.Arch
	if opt.Profile.DistribArch != "" && opt.Profile.DistribArch != opt.Profile.Arch && width >= 84 {
		archStr = fmt.Sprintf("%s (%s)", opt.Profile.Arch, opt.Profile.DistribArch)
	}
	line1 := fmt.Sprintf("  [#94a3b8]Модель:[-] [#f1f5f9]%s[-]  [#94a3b8]ОС:[-] [#f1f5f9]%s[-]  [#94a3b8]Arch:[-] [#38bdf8]%s[-]  [#94a3b8]FW:[-] %s %s",
		string(modelStr), opt.Profile.Version, archStr, opt.Profile.FirewallDot(), fw)
	opt.printClip(screen, line1, x, curY, width, tview.AlignLeft, tcell.ColorDefault)
	curY++

	conf := "нет"
	if len(opt.Profile.Conflicts) > 0 {
		conf = strings.Join(opt.Profile.Conflicts, ", ")
	}
	line2 := fmt.Sprintf("  %s [#94a3b8]ОЗУ:[-] [#cbd5e1]%.0f/%.0f МБ[-]  %s [#94a3b8]Flash:[-] [#cbd5e1]%.0f МБ[-]  %s [#94a3b8]Конфликты:[-] [#cbd5e1]%s[-]",
		opt.Profile.RAMDot(), opt.Profile.RAMFree, opt.Profile.RAMTotal,
		opt.Profile.FlashDot(), opt.Profile.FlashFree,
		opt.Profile.ConflictDot(), conf)
	opt.printClip(screen, line2, x, curY, width, tview.AlignLeft, tcell.ColorDefault)
	curY++

	_, recHint := routerpkg.HardwareRecommendation(opt.Profile.RAMTotal, opt.Profile.RAMFree, opt.Profile.FlashFree)
	if recHint != "" {
		opt.printClip(screen, fmt.Sprintf("  [#eab308]💡 Совет:[-] [#cbd5e1]%s[-]", recHint),
			x, curY, width, tview.AlignLeft, tcell.ColorDefault)
		curY++
	}

	if opt.Profile.InstalledTachyonVer != "" {
		opt.printClip(screen, fmt.Sprintf("  [#38bdf8]●[-] [#94a3b8]Уже установлено:[-] [#22c55e]Tachyon v%s[-]", opt.Profile.InstalledTachyonVer),
			x, curY, width, tview.AlignLeft, tcell.ColorDefault)
		curY++
	}

	// Thin Divider
	drawOptionDivider(screen, x, curY, width)
	curY++

	// -------------------------------------------------------------
	// SECTION 1: PROXY ENGINE
	// -------------------------------------------------------------
	sec1Header := "  [#94a3b8]⚡ 1. ЯДРО ПРОКСИ:[-]"
	if opt.activeSection == SectionEngine {
		sec1Header = fmt.Sprintf("  [#38bdf8:b]▶ 1. ЯДРО ПРОКСИ[-]  [#64748b]↑↓ или 1-%d[-]", len(opt.engines))
	}
	opt.printClip(screen, sec1Header, x, curY, width, tview.AlignLeft, tcell.ColorDefault)
	curY++

	for i, eng := range opt.engines {
		isSelected := (opt.selectedEngine == i)
		isFocused := (opt.activeSection == SectionEngine && opt.engineCursor == i)

		radio := "[#64748b]( )[-]"
		if isSelected {
			radio = "[#38bdf8:b](•)[-]"
		}

		cursorPrefix := "    "
		highlightOpen := ""
		highlightClose := ""
		if isFocused {
			cursorPrefix = "  [#38bdf8]▶[-] "
			highlightOpen = "[#ffffff:#1e293b:b]"
			highlightClose = "[-]"
		}

		badgeColor := "#94a3b8"
		if strings.Contains(eng.Badge, "xHTTP") {
			badgeColor = "#38bdf8"
		} else if strings.Contains(eng.Badge, "C-движок") {
			badgeColor = "#10b981"
		} else if strings.Contains(eng.Badge, "Tiny") {
			badgeColor = "#22c55e"
		} else if strings.Contains(eng.Badge, "Leadaxe") {
			badgeColor = "#a855f7"
		}

		desc, badge := eng.Desc, eng.Badge
		if width < 100 {
			desc = ""
		}
		if width < 56 {
			badge = ""
		}
		line := fmt.Sprintf("%s%s[%d] %s [#f1f5f9]%-25s[-] [%s]%-15s[-] [#94a3b8]%s[-]%s",
			cursorPrefix,
			highlightOpen,
			i+1,
			radio,
			eng.Name,
			badgeColor,
			badge,
			desc,
			highlightClose,
		)
		opt.printClip(screen, line, x, curY, width, tview.AlignLeft, tcell.ColorDefault)
		engIdx := i
		opt.clickTargets = append(opt.clickTargets, optClickTarget{
			x1: x, y1: curY, x2: x + width, y2: curY,
			onClick: func() {
				opt.activeSection = SectionEngine
				opt.engineCursor = engIdx
				opt.selectedEngine = engIdx
			},
		})
		curY++
	}

	// Thin Divider
	drawOptionDivider(screen, x, curY, width)
	curY++

	// -------------------------------------------------------------
	// SECTION 2: TACHYON VERSION (Prominent latest + sub-grid)
	// -------------------------------------------------------------
	sec2Header := "  [#94a3b8]📦 2. ВЕРСИЯ TACHYON:[-]"
	if opt.activeSection == SectionVersion {
		sec2Header = "  [#38bdf8:b]▶ 2. ВЕРСИЯ TACHYON[-]  [#64748b]↑↓←→ выбор, Space/цифры 1-7, Enter на «Вручную»[-]"
	}
	opt.printClip(screen, sec2Header, x, curY, width, tview.AlignLeft, tcell.ColorDefault)
	curY++

	if len(opt.versions) > 0 {
		// Row 0: latest (prominently displayed)
		ver0 := opt.versions[0]
		isZeroSelected := (opt.selectedVersion == 0)
		isZeroFocused := (opt.activeSection == SectionVersion && opt.versionCursor == 0)

		radio0 := "[#64748b]( )[-]"
		if isZeroSelected {
			radio0 = "[#38bdf8:b](•)[-]"
		}

		cursor0 := "  "
		highlightOpen0 := ""
		highlightClose0 := ""
		if isZeroFocused {
			cursor0 = "[#ffffff:#1e293b:b]▶ "
			highlightOpen0 = "[#ffffff:#1e293b:b]"
			highlightClose0 = "[-]"
		}

		badge0 := ""
		if ver0.Badge != "" {
			badge0 = fmt.Sprintf(" [#38bdf8]%s[-]", ver0.Badge)
		} else if strings.Contains(ver0.Name, "★") {
			badge0 = " [#38bdf8]★ Рекомендуется[-]"
		}

		desc0 := ""
		if width >= 80 {
			desc0 = " [#64748b](актуальный стабильный релиз)[-]"
		}

		verName0 := strings.TrimSpace(strings.ReplaceAll(ver0.Name, "★", ""))

		line0 := fmt.Sprintf("%s[1] %s %s%s%s%s%s",
			cursor0,
			radio0,
			highlightOpen0,
			verName0,
			highlightClose0,
			badge0,
			desc0,
		)
		opt.printClip(screen, line0, x+2, curY, width-4, tview.AlignLeft, tcell.ColorDefault)
		opt.clickTargets = append(opt.clickTargets, optClickTarget{
			x1: x, y1: curY, x2: x + width, y2: curY,
			onClick: func() {
				opt.activeSection = SectionVersion
				opt.versionCursor = 0
				opt.selectedVersion = 0
			},
		})
		curY++

		// Sub-grid for remaining versions (older tags & manual entry)
		if len(opt.versions) > 1 {
			subCols := opt.versionGridCols(width)
			colW := (width - 4) / subCols
			for i := 1; i < len(opt.versions); i++ {
				ver := opt.versions[i]
				subIdx := i - 1
				row := subIdx / subCols
				col := subIdx % subCols

				isSelected := (opt.selectedVersion == i)
				isFocused := (opt.activeSection == SectionVersion && opt.versionCursor == i)

				radio := "[#64748b]( )[-]"
				if isSelected {
					radio = "[#38bdf8:b](•)[-]"
				}

				name := ver.Name
				if i == len(opt.versions)-1 && opt.customVersionText != "" && opt.customVersionText != "latest" {
					name = fmt.Sprintf("Вручную: %s", opt.customVersionText)
				}

				cellW := colW
				if col == subCols-1 {
					cellW = width - 4 - col*colW
				}

				var cell string
				if isFocused {
					cell = fmt.Sprintf("[#ffffff:#1e293b:b]▶ [%d] %s %s[-] ", i+1, radio, name)
				} else if isSelected {
					cell = fmt.Sprintf("  [%d] %s [#38bdf8:b]%s[-] ", i+1, radio, name)
				} else {
					cell = fmt.Sprintf("  [%d] %s [#94a3b8]%s[-] ", i+1, radio, name)
				}
				cellX := x + 2 + col*colW
				opt.printClip(screen, cell, cellX, curY+row, cellW, tview.AlignLeft, tcell.ColorDefault)
				vIdx := i
				opt.clickTargets = append(opt.clickTargets, optClickTarget{
					x1: cellX, y1: curY + row, x2: cellX + cellW, y2: curY + row,
					onClick: func() {
						opt.activeSection = SectionVersion
						opt.versionCursor = vIdx
						opt.selectedVersion = vIdx
						opt.checkManualVersion()
					},
				})
			}
			subRows := (len(opt.versions) - 1 + subCols - 1) / subCols
			curY += subRows
		}
	}

	// Thin Divider
	drawOptionDivider(screen, x, curY, width)
	curY++

	// -------------------------------------------------------------
	// SECTION 3: DOWNLOAD MIRRORS (6 options in 2 rows of 3)
	// -------------------------------------------------------------
	sec3Header := "  [#94a3b8]🌐 3. ЗЕРКАЛО ЗАГРУЗКИ GITHUB:[-]"
	if opt.activeSection == SectionMirror {
		sec3Header = "  [#38bdf8:b]▶ 3. ЗЕРКАЛО ЗАГРУЗКИ GITHUB[-]  [#64748b]стрелки или 1-6[-]"
	}
	opt.printClip(screen, sec3Header, x, curY, width, tview.AlignLeft, tcell.ColorDefault)
	curY++

	renderMirrorRow := func(startIdx, endIdx int) {
		colWidth := (width - 6) / 3
		if colWidth < 26 {
			colWidth = 26
		}
		for i := startIdx; i < endIdx && i < len(opt.mirrors); i++ {
			m := opt.mirrors[i]
			isSelected := (opt.selectedMirror == i)
			isFocused := (opt.activeSection == SectionMirror && opt.mirrorCursor == i)

			radio := "[#64748b]( )[-]"
			if isSelected {
				radio = "[#38bdf8:b](•)[-]"
			}

			cellX := x + 3 + (i-startIdx)*colWidth
			if isFocused {
				opt.printClip(screen, fmt.Sprintf("[#ffffff:#1e293b:b]▶ [%d] %s %s[-] ", i+1, radio, m.Name), cellX, curY, colWidth, tview.AlignLeft, tcell.ColorDefault)
			} else if isSelected {
				opt.printClip(screen, fmt.Sprintf("  [%d] %s [#38bdf8:b]%s[-] ", i+1, radio, m.Name), cellX, curY, colWidth, tview.AlignLeft, tcell.ColorDefault)
			} else {
				opt.printClip(screen, fmt.Sprintf("  [%d] %s [#94a3b8]%s[-] ", i+1, radio, m.Name), cellX, curY, colWidth, tview.AlignLeft, tcell.ColorDefault)
			}
			mIdx := i
			opt.clickTargets = append(opt.clickTargets, optClickTarget{
				x1: cellX, y1: curY, x2: cellX + colWidth, y2: curY,
				onClick: func() {
					opt.activeSection = SectionMirror
					opt.mirrorCursor = mIdx
					opt.selectedMirror = mIdx
				},
			})
		}
		curY++
	}

	renderMirrorRow(0, 3)
	renderMirrorRow(3, 6)

	// Thin Divider
	drawOptionDivider(screen, x, curY, width)
	curY++

	// -------------------------------------------------------------
	// SECTION 4: LOCALIZATION & SYSTEM
	// -------------------------------------------------------------
	sec4Header := "  [#94a3b8]⚙️ 4. СИСТЕМА И ЛОКАЛИЗАЦИЯ:[-]"
	if opt.activeSection == SectionLang {
		sec4Header = "  [#38bdf8:b]▶ 4. СИСТЕМА И ЛОКАЛИЗАЦИЯ[-]  [#64748b]↑↓ выбор, Space/Enter переключение[-]"
	}
	opt.printClip(screen, sec4Header, x, curY, width, tview.AlignLeft, tcell.ColorDefault)
	curY++

	// 4.1 Russian language
	ruCheck := "[#64748b][ ][-]"
	if opt.installRussian {
		ruCheck = "[#22c55e:b][✓][-]"
	}
	ruCursorPrefix := "    "
	ruHighlightOpen := ""
	ruHighlightClose := ""
	if opt.activeSection == SectionLang && opt.langCursor == 0 {
		ruCursorPrefix = "  [#38bdf8]▶[-] "
		ruHighlightOpen = "[#ffffff:#1e293b:b]"
		ruHighlightClose = "[-]"
	}
	opt.printClip(screen, fmt.Sprintf("%s%s%s [#cbd5e1]Установить русский языковой пакет LuCI (luci-i18n-tachyon-ru)[-]%s",
		ruCursorPrefix, ruHighlightOpen, ruCheck, ruHighlightClose), x, curY, width, tview.AlignLeft, tcell.ColorDefault)
	opt.clickTargets = append(opt.clickTargets, optClickTarget{
		x1: x, y1: curY, x2: x + width, y2: curY,
		onClick: func() {
			opt.activeSection = SectionLang
			opt.langCursor = 0
			opt.installRussian = !opt.installRussian
		},
	})
	curY++

	// 4.2 zRAM swap
	zramCheck := "[#64748b][ ][-]"
	if opt.installZRAM {
		zramCheck = "[#22c55e:b][✓][-]"
	}
	zramCursorPrefix := "    "
	zramHighlightOpen := ""
	zramHighlightClose := ""
	if opt.activeSection == SectionLang && opt.langCursor == 1 {
		zramCursorPrefix = "  [#38bdf8]▶[-] "
		zramHighlightOpen = "[#ffffff:#1e293b:b]"
		zramHighlightClose = "[-]"
	}
	opt.printClip(screen, fmt.Sprintf("%s%s%s [#cbd5e1]Включить zRAM-swap (сжатый SWAP в RAM для защиты от OOM)[-]%s",
		zramCursorPrefix, zramHighlightOpen, zramCheck, zramHighlightClose), x, curY, width, tview.AlignLeft, tcell.ColorDefault)
	opt.clickTargets = append(opt.clickTargets, optClickTarget{
		x1: x, y1: curY, x2: x + width, y2: curY,
		onClick: func() {
			opt.activeSection = SectionLang
			opt.langCursor = 1
			opt.installZRAM = !opt.installZRAM
		},
	})
	curY++

	// -------------------------------------------------------------
	// SECTION 5 & 6: ACTION BUTTONS
	// -------------------------------------------------------------
	btnInstall := "  [#38bdf8:#1e293b]  🚀  Начать установку  [-]  "
	if opt.activeSection == SectionBtnInstall {
		btnInstall = "  [#ffffff:#0284c7:b] ▶ 🚀  Начать установку (Enter)  [-]  "
	}
	btnBack := "  [#94a3b8:#1e293b]  ←  Назад к SSH  [-]  "
	if opt.activeSection == SectionBtnBack {
		btnBack = "  [#ffffff:#0284c7:b] ▶ ←  Назад к SSH (Enter)  [-]  "
	}

	buttonsLine := fmt.Sprintf("        %s        %s", btnInstall, btnBack)
	opt.printClip(screen, buttonsLine, x, curY, width, tview.AlignLeft, tcell.ColorDefault)
	midX := x + width/2
	opt.clickTargets = append(opt.clickTargets, optClickTarget{
		x1: x + 4, y1: curY, x2: midX - 2, y2: curY,
		onClick: func() {
			opt.activeSection = SectionBtnInstall
			opt.submit()
		},
	})
	opt.clickTargets = append(opt.clickTargets, optClickTarget{
		x1: midX + 2, y1: curY, x2: x + width - 4, y2: curY,
		onClick: func() {
			opt.activeSection = SectionBtnBack
			if opt.OnBack != nil {
				opt.OnBack()
			}
		},
	})
	curY++

	// Navigation Footer
	footer := "  [#64748b]↑↓←→ навигация · Enter/Space выбор · Клик мышью · D диагностика · Esc назад[-]"
	opt.printClip(screen, footer, x, curY, width, tview.AlignLeft, tcell.ColorDefault)
}

func drawOptionDivider(screen tcell.Screen, x, y, width int) {
	style := tcell.StyleDefault.Foreground(tcell.NewRGBColor(51, 65, 85)) // Slate 700
	for col := x + 1; col < x+width-1; col++ {
		screen.SetContent(col, y, '─', nil, style)
	}
}

// MouseHandler handles mouse clicks and scrolling for OptionsSelector.
func (opt *OptionsSelector) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, recipient tview.Primitive) {
	return opt.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, recipient tview.Primitive) {
		if !opt.InRect(event.Position()) {
			return false, nil
		}
		setFocus(opt)

		mx, my := event.Position()

		switch action {
		case tview.MouseLeftClick:
			for _, target := range opt.clickTargets {
				if my >= target.y1 && my <= target.y2 && mx >= target.x1 && mx <= target.x2 {
					target.onClick()
					return true, opt
				}
			}
			return true, opt

		case tview.MouseScrollDown:
			opt.activeSection = (opt.activeSection + 1) % totalSections
			return true, opt

		case tview.MouseScrollUp:
			opt.activeSection = (opt.activeSection + totalSections - 1) % totalSections
			return true, opt
		}

		return false, nil
	})
}

// InputHandler handles keyboard navigation.
func (opt *OptionsSelector) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return opt.WrapInputHandler(func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		switch event.Key() {
		case tcell.KeyTab:
			opt.activeSection = (opt.activeSection + 1) % totalSections
			return
		case tcell.KeyBacktab:
			opt.activeSection = (opt.activeSection + totalSections - 1) % totalSections
			return
		case tcell.KeyEscape:
			if opt.OnBack != nil {
				opt.OnBack()
			}
			return
		}

		if event.Key() == tcell.KeyRune {
			switch event.Rune() {
			case 'd', 'D', 'в', 'В':
				if opt.OnDiagnostics != nil {
					opt.OnDiagnostics()
				}
				return
			}
		}

		switch opt.activeSection {
		case SectionEngine:
			switch event.Key() {
			case tcell.KeyUp:
				if opt.engineCursor > 0 {
					opt.engineCursor--
					opt.selectedEngine = opt.engineCursor
				}
			case tcell.KeyDown:
				if opt.engineCursor < len(opt.engines)-1 {
					opt.engineCursor++
					opt.selectedEngine = opt.engineCursor
				} else {
					opt.activeSection = SectionVersion
				}
			case tcell.KeyRight:
				opt.activeSection = SectionVersion
			case tcell.KeyEnter:
				opt.selectedEngine = opt.engineCursor
				opt.activeSection = SectionVersion
			case tcell.KeyRune:
				r := event.Rune()
				if r >= '1' && r <= rune('0'+len(opt.engines)) {
					idx := int(r - '1')
					opt.engineCursor = idx
					opt.selectedEngine = idx
				} else if r == ' ' {
					opt.selectedEngine = opt.engineCursor
				}
			}

		case SectionVersion:
			_, _, innerW, _ := opt.GetInnerRect()
			subCols := opt.versionGridCols(innerW)
			n := len(opt.versions)

			switch event.Key() {
			case tcell.KeyLeft:
				if opt.versionCursor > 0 {
					opt.versionCursor--
					opt.selectedVersion = opt.versionCursor
				} else {
					opt.activeSection = SectionEngine
				}
			case tcell.KeyRight:
				if opt.versionCursor < n-1 {
					opt.versionCursor++
					opt.selectedVersion = opt.versionCursor
				} else {
					opt.activeSection = SectionMirror
				}
			case tcell.KeyUp:
				if opt.versionCursor == 0 {
					opt.activeSection = SectionEngine
					opt.engineCursor = len(opt.engines) - 1
					opt.selectedEngine = opt.engineCursor
				} else if opt.versionCursor <= subCols {
					opt.versionCursor = 0
					opt.selectedVersion = 0
				} else {
					opt.versionCursor -= subCols
					opt.selectedVersion = opt.versionCursor
				}
			case tcell.KeyDown:
				if opt.versionCursor == 0 {
					if n > 1 {
						opt.versionCursor = 1
						opt.selectedVersion = 1
					} else {
						opt.activeSection = SectionMirror
					}
				} else {
					next := opt.versionCursor + subCols
					if next < n {
						opt.versionCursor = next
						opt.selectedVersion = next
					} else {
						col := (opt.versionCursor - 1) % subCols
						if col > 2 {
							col = 2
						}
						opt.mirrorCursor = col
						opt.selectedMirror = col
						opt.activeSection = SectionMirror
					}
				}
			case tcell.KeyEnter:
				if opt.versionCursor == n-1 {
					opt.triggerManualVersionPrompt()
				} else {
					opt.selectedVersion = opt.versionCursor
					opt.activeSection = SectionMirror
				}
			case tcell.KeyRune:
				r := event.Rune()
				if r == ' ' {
					opt.selectedVersion = opt.versionCursor
					if opt.versionCursor == n-1 {
						opt.triggerManualVersionPrompt()
					}
				} else if r >= '1' && r <= '9' {
					idx := int(r - '1')
					if idx < n {
						opt.versionCursor = idx
						opt.selectedVersion = idx
						opt.checkManualVersion()
					}
				}
			}

		case SectionMirror:
			switch event.Key() {
			case tcell.KeyLeft:
				if opt.mirrorCursor > 0 {
					opt.mirrorCursor--
					opt.selectedMirror = opt.mirrorCursor
				} else {
					opt.activeSection = SectionVersion
				}
			case tcell.KeyRight:
				if opt.mirrorCursor < len(opt.mirrors)-1 {
					opt.mirrorCursor++
					opt.selectedMirror = opt.mirrorCursor
				} else {
					opt.activeSection = SectionLang
				}
			case tcell.KeyUp:
				if opt.mirrorCursor >= 3 {
					opt.mirrorCursor -= 3
					opt.selectedMirror = opt.mirrorCursor
				} else {
					opt.activeSection = SectionVersion
					_, _, innerW, _ := opt.GetInnerRect()
					subCols := opt.versionGridCols(innerW)
					n := len(opt.versions)
					if n > 1 {
						subRows := (n - 1 + subCols - 1) / subCols
						target := 1 + (subRows-1)*subCols + (opt.mirrorCursor % subCols)
						if target >= n {
							target = n - 1
						}
						opt.versionCursor = target
						opt.selectedVersion = target
					} else {
						opt.versionCursor = 0
						opt.selectedVersion = 0
					}
				}
			case tcell.KeyDown:
				if opt.mirrorCursor < 3 {
					opt.mirrorCursor += 3
					opt.selectedMirror = opt.mirrorCursor
				} else {
					opt.activeSection = SectionLang
					opt.langCursor = 0
				}
			case tcell.KeyEnter:
				opt.activeSection = SectionLang
				opt.langCursor = 0
			case tcell.KeyRune:
				r := event.Rune()
				if r >= '1' && r <= '6' {
					idx := int(r - '1')
					opt.mirrorCursor = idx
					opt.selectedMirror = idx
				} else if r == ' ' {
					opt.activeSection = SectionLang
					opt.langCursor = 0
				}
			}

		case SectionLang:
			switch event.Key() {
			case tcell.KeyUp:
				if opt.langCursor > 0 {
					opt.langCursor--
				} else {
					opt.activeSection = SectionMirror
				}
			case tcell.KeyDown:
				if opt.langCursor < 1 {
					opt.langCursor++
				} else {
					opt.activeSection = SectionBtnInstall
				}
			case tcell.KeyRight:
				opt.activeSection = SectionBtnInstall
			case tcell.KeyLeft:
				opt.activeSection = SectionMirror
			case tcell.KeyEnter:
				if opt.langCursor == 0 {
					opt.installRussian = !opt.installRussian
				} else {
					opt.installZRAM = !opt.installZRAM
				}
			case tcell.KeyRune:
				if event.Rune() == ' ' {
					if opt.langCursor == 0 {
						opt.installRussian = !opt.installRussian
					} else {
						opt.installZRAM = !opt.installZRAM
					}
				}
			}

		case SectionBtnInstall:
			switch event.Key() {
			case tcell.KeyUp:
				opt.activeSection = SectionLang
				opt.langCursor = 1
			case tcell.KeyRight:
				opt.activeSection = SectionBtnBack
			case tcell.KeyLeft:
				opt.activeSection = SectionBtnBack
			case tcell.KeyDown:
				opt.activeSection = SectionEngine
			case tcell.KeyEnter:
				opt.submit()
			case tcell.KeyRune:
				if event.Rune() == ' ' {
					opt.submit()
				}
			}

		case SectionBtnBack:
			switch event.Key() {
			case tcell.KeyUp:
				opt.activeSection = SectionLang
			case tcell.KeyLeft:
				opt.activeSection = SectionBtnInstall
			case tcell.KeyRight:
				opt.activeSection = SectionBtnInstall
			case tcell.KeyDown:
				opt.activeSection = SectionEngine
			case tcell.KeyEnter:
				if opt.OnBack != nil {
					opt.OnBack()
				}
			case tcell.KeyRune:
				if event.Rune() == ' ' {
					if opt.OnBack != nil {
						opt.OnBack()
					}
				}
			}
		}
	})
}

func (opt *OptionsSelector) checkManualVersion() {
	if opt.versionCursor == len(opt.versions)-1 && (opt.customVersionText == "" || opt.customVersionText == "latest") {
		opt.triggerManualVersionPrompt()
	}
}

func (opt *OptionsSelector) triggerManualVersionPrompt() {
	if opt.OnRequestManualVersion != nil {
		opt.OnRequestManualVersion(opt.customVersionText, func(newVer string) {
			opt.customVersionText = newVer
			opt.selectedVersion = len(opt.versions) - 1
		})
	}
}

func (opt *OptionsSelector) versionGridCols(width int) int {
	if width > 0 && width < 60 {
		return 2
	}
	return 3
}

// SetReleases replaces the version list with fetched release tags (newest first).
// Must be called from the UI goroutine.
func (opt *OptionsSelector) SetReleases(tags []string) {
	list := []VersionItem{{Key: "latest", Name: "latest", Badge: "★ Рекомендуется"}}
	if len(tags) > 0 {
		list[0].Name = fmt.Sprintf("latest (%s)", tags[0])
	}
	for _, t := range tags {
		list = append(list, VersionItem{Key: t, Name: t})
	}
	list = append(list, VersionItem{Key: "custom", Name: "Вручную..."})
	opt.versions = list
	opt.applyPrefVersion()
}

func (opt *OptionsSelector) submit() {
	if opt.OnSubmit != nil {
		opt.OnSubmit(opt.GetInstallOptions())
	}
}

// printClip prints like tview.Print but never below the widget's bottom edge.
func (opt *OptionsSelector) printClip(screen tcell.Screen, text string, x, y, maxWidth, align int, color tcell.Color) {
	if opt.clipBottom > 0 && y >= opt.clipBottom {
		return
	}
	tview.Print(screen, text, x, y, maxWidth, align, color)
}

// Preselect applies previously used values. Unknown values are ignored.
func (opt *OptionsSelector) Preselect(engine, mirror, version string, installRussian bool) {
	for i, e := range opt.engines {
		if e.Key == engine {
			opt.selectedEngine, opt.engineCursor = i, i
		}
	}
	for i, m := range opt.mirrors {
		if m.Key == mirror {
			opt.selectedMirror, opt.mirrorCursor = i, i
		}
	}
	opt.installRussian = installRussian
	opt.prefVersion = version
	opt.applyPrefVersion()
}

// applyPrefVersion selects the preferred version in the current list, or falls back
// to the manual entry so a previously typed tag is not lost.
func (opt *OptionsSelector) applyPrefVersion() {
	v := opt.prefVersion
	if v == "" || v == "latest" {
		opt.selectedVersion, opt.versionCursor = 0, 0
		return
	}
	for i, it := range opt.versions {
		if it.Key == v && it.Key != "custom" {
			opt.selectedVersion, opt.versionCursor = i, i
			return
		}
	}
	last := len(opt.versions) - 1
	opt.customVersionText = v
	opt.selectedVersion, opt.versionCursor = last, last
}
