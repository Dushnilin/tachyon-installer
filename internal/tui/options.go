package tui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
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

	// 4. Localization
	installRussian bool

	// Active section focus (0..5)
	activeSection int

	// Callbacks
	OnSubmit               func(opts InstallOptions)
	OnBack                 func()
	OnRequestManualVersion func(current string, callback func(newVer string))
}

// NewOptionsSelector initializes the OptionsSelector with default choices.
func NewOptionsSelector(profile ProfileData) *OptionsSelector {
	opt := &OptionsSelector{
		Box:     tview.NewBox(),
		Profile: profile,
		engines: []EngineItem{
			{Key: "sing-box-extended", Name: "sing-box-extended", Badge: "★ Рекомендуется", Desc: "xHTTP, Reality, ShadowTLS, gRPC, TUIC, Hy2"},
			{Key: "steer-extended", Name: "steer-extended", Badge: "⚡ Скорость", Desc: "steer + xHTTP, оптимизация под высокую нагрузку"},
			{Key: "steer", Name: "steer", Badge: "🍃 Легковес", Desc: "Ультра-легковесный Go-движок (минимум RAM)"},
			{Key: "sing-box-lx", Name: "sing-box-lx", Badge: "📦 Leadaxe", Desc: "Компактная сборка sing-box от Leadaxe"},
			{Key: "skip", Name: "Без ядра", Badge: "⚙️ Пропустить", Desc: "Настроить или загрузить ядро позже в LuCI"},
		},
		selectedEngine: 0,
		engineCursor:   0,

		versions: []VersionItem{
			{Key: "latest", Name: "latest ★", Badge: ""},
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
	}
}

// Draw renders the OptionsSelector dashboard.
func (opt *OptionsSelector) Draw(screen tcell.Screen) {
	opt.Box.DrawForSubclass(screen, opt)
	x, y, width, height := opt.GetInnerRect()
	if width <= 0 || height <= 0 {
		return
	}

	curY := y

	// 1. Hardware Summary Line 1
	modelStr := opt.Profile.Model
	if len(modelStr) > 26 {
		modelStr = modelStr[:24] + ".."
	}
	summaryLine1 := fmt.Sprintf("  [#38bdf8]●[-] [#94a3b8]Модель:[-] [#f1f5f9]%-26s[-]  [#38bdf8]●[-] [#94a3b8]ОС:[-] [#f1f5f9]%s (%s)[-]   %s [#94a3b8]FW:[-] %s",
		modelStr,
		opt.Profile.Version, opt.Profile.Arch,
		opt.Profile.FirewallDot(), opt.Profile.FirewallStatus(),
	)
	tview.Print(screen, summaryLine1, x, curY, width, tview.AlignLeft, tcell.ColorDefault)
	curY++

	// 2. Hardware Summary Line 2
	installedBadge := ""
	if opt.Profile.InstalledTachyonVer != "" {
		installedBadge = fmt.Sprintf("  [#38bdf8]●[-] [#cbd5e1]Установлен:[-] [#22c55e]Tachyon v%s[-]", opt.Profile.InstalledTachyonVer)
	}
	summaryLine2 := fmt.Sprintf("  %s [#94a3b8]ОЗУ:[-] [#cbd5e1]%.0f МБ (своб. %.0f МБ)[-]  %s [#94a3b8]Flash:[-] %s  %s [#94a3b8]Конфликты:[-] %s%s",
		opt.Profile.RAMDot(), opt.Profile.RAMTotal, opt.Profile.RAMFree,
		opt.Profile.FlashDot(), opt.Profile.FlashStatus(),
		opt.Profile.ConflictDot(), opt.Profile.ConflictStatus(),
		installedBadge,
	)
	tview.Print(screen, summaryLine2, x, curY, width, tview.AlignLeft, tcell.ColorDefault)
	curY++

	// Thin Divider
	drawOptionDivider(screen, x, curY, width)
	curY++

	// -------------------------------------------------------------
	// SECTION 1: PROXY ENGINE (5 options)
	// -------------------------------------------------------------
	sec1Header := "  [#94a3b8]⚡ 1. ЯДРО ПРОКСИ:[-]"
	if opt.activeSection == SectionEngine {
		sec1Header = "  [#38bdf8:b]▶ 1. ЯДРО ПРОКСИ (выберите движок клавишами [1-5] или ↑↓):[-]"
	}
	tview.Print(screen, sec1Header, x, curY, width, tview.AlignLeft, tcell.ColorDefault)
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

		badgeColor := "#38bdf8"
		if strings.Contains(eng.Badge, "Рекомендуется") {
			badgeColor = "#38bdf8"
		} else if strings.Contains(eng.Badge, "Скорость") {
			badgeColor = "#eab308"
		} else if strings.Contains(eng.Badge, "Легковес") {
			badgeColor = "#22c55e"
		} else if strings.Contains(eng.Badge, "Leadaxe") {
			badgeColor = "#a855f7"
		} else {
			badgeColor = "#94a3b8"
		}

		line := fmt.Sprintf("%s%s[%d] %s [#f1f5f9]%-18s[-] [%s]%-17s[-] [#94a3b8]%s[-]%s",
			cursorPrefix,
			highlightOpen,
			i+1,
			radio,
			eng.Name,
			badgeColor,
			eng.Badge,
			eng.Desc,
			highlightClose,
		)
		tview.Print(screen, line, x, curY, width, tview.AlignLeft, tcell.ColorDefault)
		curY++
	}

	// Thin Divider
	drawOptionDivider(screen, x, curY, width)
	curY++

	// -------------------------------------------------------------
	// SECTION 2: TACHYON VERSION (4 options inline)
	// -------------------------------------------------------------
	sec2Header := "  [#94a3b8]📦 2. ВЕРСИЯ TACHYON:[-]"
	if opt.activeSection == SectionVersion {
		sec2Header = "  [#38bdf8:b]▶ 2. ВЕРСИЯ TACHYON (клавиши цифр или ←→):[-]"
	}
	tview.Print(screen, sec2Header, x, curY, width, tview.AlignLeft, tcell.ColorDefault)
	curY++

	const verPerRow = 4
	colW := (width - 4) / verPerRow
	for i, ver := range opt.versions {
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

		var cell string
		if isFocused {
			cell = fmt.Sprintf("[#ffffff:#1e293b:b]▶ [%d] %s %s[-] ", i+1, radio, name)
		} else if isSelected {
			cell = fmt.Sprintf("  [%d] %s [#38bdf8:b]%s[-] ", i+1, radio, name)
		} else {
			cell = fmt.Sprintf("  [%d] %s [#94a3b8]%s[-] ", i+1, radio, name)
		}
		tview.Print(screen, cell, x+2+(i%verPerRow)*colW, curY+i/verPerRow, colW, tview.AlignLeft, tcell.ColorDefault)
	}
	curY += (len(opt.versions) + verPerRow - 1) / verPerRow

	// Thin Divider
	drawOptionDivider(screen, x, curY, width)
	curY++

	// -------------------------------------------------------------
	// SECTION 3: DOWNLOAD MIRRORS (6 options in 2 rows of 3)
	// -------------------------------------------------------------
	sec3Header := "  [#94a3b8]🌐 3. ЗЕРКАЛО ЗАГРУЗКИ GITHUB:[-]"
	if opt.activeSection == SectionMirror {
		sec3Header = "  [#38bdf8:b]▶ 3. ЗЕРКАЛО ЗАГРУЗКИ GITHUB (клавиши [1-6] или стрелки):[-]"
	}
	tview.Print(screen, sec3Header, x, curY, width, tview.AlignLeft, tcell.ColorDefault)
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
				tview.Print(screen, fmt.Sprintf("[#ffffff:#1e293b:b]▶ [%d] %s %s[-] ", i+1, radio, m.Name), cellX, curY, colWidth, tview.AlignLeft, tcell.ColorDefault)
			} else if isSelected {
				tview.Print(screen, fmt.Sprintf("  [%d] %s [#38bdf8:b]%s[-] ", i+1, radio, m.Name), cellX, curY, colWidth, tview.AlignLeft, tcell.ColorDefault)
			} else {
				tview.Print(screen, fmt.Sprintf("  [%d] %s [#94a3b8]%s[-] ", i+1, radio, m.Name), cellX, curY, colWidth, tview.AlignLeft, tcell.ColorDefault)
			}
		}
		curY++
	}

	renderMirrorRow(0, 3)
	renderMirrorRow(3, 6)

	// Thin Divider
	drawOptionDivider(screen, x, curY, width)
	curY++

	// -------------------------------------------------------------
	// SECTION 4: LOCALIZATION
	// -------------------------------------------------------------
	checkIcon := "[#64748b][ ][-]"
	if opt.installRussian {
		checkIcon = "[#22c55e:b][✓][-]"
	}
	sec4Header := "  [#94a3b8]🌍 4. ЛОКАЛИЗАЦИЯ:[-]"
	if opt.activeSection == SectionLang {
		sec4Header = "  [#38bdf8:b]▶ 4. ЛОКАЛИЗАЦИЯ (Space/Enter переключить):[-]"
		tview.Print(screen, fmt.Sprintf("%s [#ffffff:#1e293b:b]▶ %s Установить русский языковой пакет LuCI (luci-i18n-tachyon-ru)[-] ", sec4Header, checkIcon), x, curY, width, tview.AlignLeft, tcell.ColorDefault)
	} else {
		tview.Print(screen, fmt.Sprintf("%s   %s [#cbd5e1]Установить русский языковой пакет LuCI (luci-i18n-tachyon-ru)[-] ", sec4Header, checkIcon), x, curY, width, tview.AlignLeft, tcell.ColorDefault)
	}
	curY++
	curY++ // breathing room

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
	tview.Print(screen, buttonsLine, x, curY, width, tview.AlignLeft, tcell.ColorDefault)
	curY++
	curY++

	// Navigation Footer
	footer := "  [#64748b]Навигация: [Tab/Shift+Tab] Разделы | [↑↓/←→] Выбор | [1-6] Цифры | [Space] Переключить | [Enter] Пуск | [Esc] Назад[-]"
	tview.Print(screen, footer, x, curY, width, tview.AlignLeft, tcell.ColorDefault)
}

func drawOptionDivider(screen tcell.Screen, x, y, width int) {
	style := tcell.StyleDefault.Foreground(tcell.NewRGBColor(51, 65, 85)) // Slate 700
	for col := x + 1; col < x+width-1; col++ {
		screen.SetContent(col, y, '─', nil, style)
	}
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
			case tcell.KeyEnter:
				opt.selectedEngine = opt.engineCursor
				opt.activeSection = SectionVersion
			case tcell.KeyRune:
				r := event.Rune()
				if r >= '1' && r <= '5' {
					idx := int(r - '1')
					opt.engineCursor = idx
					opt.selectedEngine = idx
				} else if r == ' ' {
					opt.selectedEngine = opt.engineCursor
				}
			}

		case SectionVersion:
			switch event.Key() {
			case tcell.KeyLeft:
				if opt.versionCursor > 0 {
					opt.versionCursor--
					opt.selectedVersion = opt.versionCursor
				}
			case tcell.KeyRight:
				if opt.versionCursor < len(opt.versions)-1 {
					opt.versionCursor++
					opt.selectedVersion = opt.versionCursor
				}
			case tcell.KeyUp:
				opt.activeSection = SectionEngine
			case tcell.KeyDown:
				opt.activeSection = SectionMirror
			case tcell.KeyEnter:
				if opt.versionCursor == len(opt.versions)-1 {
					opt.triggerManualVersionPrompt()
				} else {
					opt.activeSection = SectionMirror
				}
			case tcell.KeyRune:
				r := event.Rune()
				if idx := int(r - '1'); r >= '1' && r <= '9' && idx < len(opt.versions) {
					opt.versionCursor = idx
					opt.selectedVersion = idx
					opt.checkManualVersion()
				}
			}

		case SectionMirror:
			switch event.Key() {
			case tcell.KeyLeft:
				if opt.mirrorCursor > 0 {
					opt.mirrorCursor--
					opt.selectedMirror = opt.mirrorCursor
				}
			case tcell.KeyRight:
				if opt.mirrorCursor < len(opt.mirrors)-1 {
					opt.mirrorCursor++
					opt.selectedMirror = opt.mirrorCursor
				}
			case tcell.KeyUp:
				if opt.mirrorCursor >= 3 {
					opt.mirrorCursor -= 3
					opt.selectedMirror = opt.mirrorCursor
				} else {
					opt.activeSection = SectionVersion
				}
			case tcell.KeyDown:
				if opt.mirrorCursor < 3 {
					opt.mirrorCursor += 3
					opt.selectedMirror = opt.mirrorCursor
				} else {
					opt.activeSection = SectionLang
				}
			case tcell.KeyEnter:
				opt.activeSection = SectionLang
			case tcell.KeyRune:
				r := event.Rune()
				if r >= '1' && r <= '6' {
					idx := int(r - '1')
					opt.mirrorCursor = idx
					opt.selectedMirror = idx
				}
			}

		case SectionLang:
			switch event.Key() {
			case tcell.KeyUp:
				opt.activeSection = SectionMirror
			case tcell.KeyDown:
				opt.activeSection = SectionBtnInstall
			case tcell.KeyEnter:
				opt.installRussian = !opt.installRussian
			case tcell.KeyRune:
				if event.Rune() == ' ' {
					opt.installRussian = !opt.installRussian
				}
			}

		case SectionBtnInstall:
			switch event.Key() {
			case tcell.KeyUp:
				opt.activeSection = SectionLang
			case tcell.KeyRight:
				opt.activeSection = SectionBtnBack
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

// SetReleases replaces the version list with fetched release tags (newest first).
// Must be called from the UI goroutine.
func (opt *OptionsSelector) SetReleases(tags []string) {
	list := []VersionItem{{Key: "latest", Name: "latest ★"}}
	if len(tags) > 0 {
		list[0].Name = "latest (" + tags[0] + ") ★"
	}
	for _, t := range tags {
		list = append(list, VersionItem{Key: t, Name: t})
	}
	list = append(list, VersionItem{Key: "custom", Name: "Вручную..."})
	opt.versions = list
	opt.selectedVersion, opt.versionCursor = 0, 0
}

func (opt *OptionsSelector) submit() {
	if opt.OnSubmit != nil {
		opt.OnSubmit(opt.GetInstallOptions())
	}
}
