package tui

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// Tachyon Design System & Color Tokens
// Reflecting the authentic visual identity of Tachyon Core (Neon Cyan & Holographic Violet).
var (
	// Brand Accents
	ColorCyanElectric = tcell.NewRGBColor(0, 240, 255)   // #00f0ff - Tachyon Signature Neon Cyan
	ColorSkyCyber     = tcell.NewRGBColor(56, 189, 248)  // #38bdf8 - Cyber Sky
	ColorIndigoTach   = tcell.NewRGBColor(129, 140, 248) // #818cf8 - Tachyon Indigo
	ColorVioletHolo   = tcell.NewRGBColor(168, 85, 247)  // #a855f7 - Quantum Violet
	ColorLilacHolo    = tcell.NewRGBColor(192, 132, 252) // #c084fc - Holographic Lilac

	// Cosmic Surfaces & Backgrounds
	ColorBgSpace       = tcell.NewRGBColor(9, 13, 22)     // #090d16 - Cosmic Deep Space (Main Canvas)
	ColorBgCard        = tcell.NewRGBColor(15, 23, 42)    // #0f172a - Slate 950 (Card/Modal Surface)
	ColorBgInput       = tcell.NewRGBColor(30, 41, 59)    // #1e293b - Slate 800 (Inputs, Inactive Buttons)
	ColorBgInputFocus  = tcell.NewRGBColor(2, 132, 199)   // #0284c7 - Sky 600 (Focused Field/Button)
	ColorBgActiveCyan  = tcell.NewRGBColor(0, 240, 255)   // #00f0ff - Pure Cyan Accent
	ColorBorderNormal  = tcell.NewRGBColor(51, 65, 85)    // #334155 - Slate 700 (Subtle border)
	ColorBorderFocused = tcell.NewRGBColor(0, 240, 255)   // #00f0ff - Neon Cyan (Active border)

	// High-Contrast Typography
	ColorTextPure      = tcell.NewRGBColor(255, 255, 255) // #ffffff - Pure White
	ColorTextPrimary   = tcell.NewRGBColor(248, 250, 252) // #f8fafc - Slate 50 (High-Contrast White)
	ColorTextSecondary = tcell.NewRGBColor(226, 232, 240) // #e2e8f0 - Slate 200 (Clean, Readable Body)
	ColorTextMuted     = tcell.NewRGBColor(148, 163, 184) // #94a3b8 - Slate 400 (Secondary Hints)
	ColorTextDark      = tcell.NewRGBColor(100, 116, 139) // #64748b - Slate 500 (Subtle brackets/separators)

	// Semantic Status
	ColorStatusSuccess = tcell.NewRGBColor(16, 185, 129)  // #10b981 - Emerald Green (Active / Success)
	ColorStatusWarning = tcell.NewRGBColor(245, 158, 11)  // #f59e0b - Amber Gold (Warning / Outdated)
	ColorStatusError   = tcell.NewRGBColor(239, 68, 68)   // #ef4444 - Coral Red (Error / Conflict)
)

// BBCode Color Tags for Rich Text Formatting
const (
	TagCyan       = "[#00f0ff]"
	TagCyanBold   = "[#00f0ff:b]"
	TagSky        = "[#38bdf8]"
	TagSkyBold    = "[#38bdf8:b]"
	TagViolet     = "[#a855f7]"
	TagVioletBold = "[#a855f7:b]"
	TagLilac      = "[#c084fc]"
	TagGreen      = "[#10b981]"
	TagGreenBold  = "[#10b981:b]"
	TagAmber      = "[#f59e0b]"
	TagAmberBold  = "[#f59e0b:b]"
	TagRed        = "[#ef4444]"
	TagRedBold    = "[#ef4444:b]"
	TagWhite      = "[#ffffff]"
	TagWhiteBold  = "[#ffffff:b]"
	TagText       = "[#f8fafc]"
	TagTextBold   = "[#f8fafc:b]"
	TagSubText    = "[#e2e8f0]"
	TagMuted      = "[#94a3b8]"
	TagDark       = "[#64748b]"
	TagReset      = "[-]"
)

// InitTheme applies the authentic Tachyon Cyber theme to tview global styles.
func InitTheme() {
	// Canvas & Surfaces
	tview.Styles.PrimitiveBackgroundColor = ColorBgSpace
	tview.Styles.ContrastBackgroundColor = ColorBgInput
	tview.Styles.MoreContrastBackgroundColor = ColorBgCard

	// Borders & Visual Accents
	tview.Styles.BorderColor = ColorBorderNormal
	tview.Styles.TitleColor = ColorCyanElectric
	tview.Styles.GraphicsColor = ColorCyanElectric

	// Typography & Readability
	tview.Styles.PrimaryTextColor = ColorTextPrimary
	tview.Styles.SecondaryTextColor = ColorTextSecondary
	tview.Styles.TertiaryTextColor = ColorTextMuted
	tview.Styles.InverseTextColor = ColorTextPure
	tview.Styles.ContrastSecondaryTextColor = ColorSkyCyber
}

// ZincColor is kept for backward-compatibility with existing calls.
func ZincColor(level int) tcell.Color {
	if level >= 70 {
		return ColorBorderNormal
	}
	return ColorBgSpace
}

// StyleForm applies uniform Tachyon styling to form controls, inputs, and buttons.
func StyleForm(form *tview.Form) {
	form.SetBackgroundColor(ColorBgSpace)
	form.SetLabelColor(ColorTextSecondary)
	form.SetFieldTextColor(ColorTextPrimary)
	form.SetFieldBackgroundColor(ColorBgInput)
	form.SetButtonBackgroundColor(ColorBgInput)
	form.SetButtonTextColor(ColorTextPrimary)
	form.SetButtonActivatedStyle(tcell.StyleDefault.
		Background(ColorBgInputFocus).
		Foreground(ColorTextPure).
		Bold(true))
}

// StyleList applies uniform Tachyon Cyber styling to interactive menu lists.
func StyleList(list *tview.List) {
	list.SetBackgroundColor(ColorBgSpace)
	list.SetMainTextColor(ColorTextPrimary)
	list.SetSecondaryTextColor(ColorTextMuted)
	list.SetShortcutColor(ColorCyanElectric)
	list.SetSelectedBackgroundColor(ColorBgInputFocus)
	list.SetSelectedTextColor(ColorTextPure)
	list.SetHighlightFullLine(true)
}

// FormatHotkey builds a unified, high-contrast hotkey helper chip.
// Example: "[Enter] Выбор" -> "[#00f0ff:b][Enter][-] [#e2e8f0]Выбор[-]"
func FormatHotkey(key, action string) string {
	return fmt.Sprintf("%s[%s]%s %s%s%s", TagCyanBold, key, TagReset, TagSubText, action, TagReset)
}

// FormatVioletHotkey builds a secondary hotkey chip with violet emphasis (e.g. for Fleet / Special actions).
func FormatVioletHotkey(key, action string) string {
	return fmt.Sprintf("%s[%s]%s %s%s%s", TagVioletBold, key, TagReset, TagSubText, action, TagReset)
}

// FormatExitHotkey builds a red hotkey chip for exit/cancel actions.
func FormatExitHotkey(key, action string) string {
	return fmt.Sprintf("%s[%s]%s %s%s%s", TagRedBold, key, TagReset, TagSubText, action, TagReset)
}

// FormatStepTitle produces a standardized Tachyon setup header across all modals.
func FormatStepTitle(title string) string {
	return fmt.Sprintf(" ⚡ %sTACHYON SETUP:%s %s%s%s ", TagCyanBold, TagReset, TagSubText, title, TagReset)
}

// FormatModalTitle produces a standardized Tachyon utility header.
func FormatModalTitle(icon, title string) string {
	return fmt.Sprintf(" %s %sTACHYON:%s %s%s%s ", icon, TagCyanBold, TagReset, TagSubText, title, TagReset)
}
