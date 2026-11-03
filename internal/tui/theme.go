package tui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// Tachyon Design System & Color Tokens
// Technical Graphite palette with a single Sky Blue accent and semantic status colors.
var (
	// Primary & Secondary Accents
	ColorCyanElectric = tcell.NewRGBColor(56, 189, 248)  // #38bdf8 - Primary Accent (Sky 400)
	ColorSkyCyber     = tcell.NewRGBColor(56, 189, 248)  // #38bdf8 - Primary Accent
	ColorIndigoTach   = tcell.NewRGBColor(148, 163, 184) // #94a3b8 - Muted Slate Accent
	ColorVioletHolo   = tcell.NewRGBColor(56, 189, 248)  // #38bdf8 - Unified Accent
	ColorLilacHolo    = tcell.NewRGBColor(148, 163, 184) // #94a3b8 - Muted Slate

	// Graphite Surfaces & Backgrounds
	ColorBgSpace       = tcell.NewRGBColor(15, 15, 16)   // #0f0f10 - Base Canvas
	ColorBgCard        = tcell.NewRGBColor(20, 20, 21)   // #141415 - Raised Card/Modal Surface
	ColorBgInput       = tcell.NewRGBColor(25, 25, 27)   // #19191b - Inputs & Inactive Buttons
	ColorBgInputFocus  = tcell.NewRGBColor(2, 132, 199)  // #0284c7 - Focused Field/Button
	ColorBgActiveCyan  = tcell.NewRGBColor(56, 189, 248) // #38bdf8 - Active Accent
	ColorBorderNormal  = tcell.NewRGBColor(46, 46, 50)   // #2e2e32 - Hairline Border
	ColorBorderFocused = tcell.NewRGBColor(56, 189, 248) // #38bdf8 - Focused Border

	// Typography Hierarchy
	ColorTextPure      = tcell.NewRGBColor(255, 255, 255) // #ffffff - Pure White
	ColorTextPrimary   = tcell.NewRGBColor(237, 237, 237) // #ededed - Primary Text
	ColorTextSecondary = tcell.NewRGBColor(161, 161, 166) // #a1a1a6 - Secondary Body Text
	ColorTextMuted     = tcell.NewRGBColor(113, 113, 122) // #71717a - Muted Hints
	ColorTextDark      = tcell.NewRGBColor(82, 82, 91)    // #52525b - Subtle Dividers/Brackets

	// Semantic Status
	ColorStatusSuccess = tcell.NewRGBColor(76, 195, 138) // #4cc38a - Success Green
	ColorStatusWarning = tcell.NewRGBColor(234, 179, 8)  // #eab308 - Warning Amber
	ColorStatusError   = tcell.NewRGBColor(229, 72, 77)  // #e5484d - Error Red
)

// BBCode Color Tags for Rich Text Formatting
const (
	TagCyan       = "[#38bdf8]"
	TagCyanBold   = "[#38bdf8:b]"
	TagSky        = "[#38bdf8]"
	TagSkyBold    = "[#38bdf8:b]"
	TagViolet     = "[#38bdf8]"
	TagVioletBold = "[#38bdf8:b]"
	TagLilac      = "[#a1a1a6]"
	TagGreen      = "[#4cc38a]"
	TagGreenBold  = "[#4cc38a:b]"
	TagAmber      = "[#eab308]"
	TagAmberBold  = "[#eab308:b]"
	TagRed        = "[#e5484d]"
	TagRedBold    = "[#e5484d:b]"
	TagWhite      = "[#ffffff]"
	TagWhiteBold  = "[#ffffff:b]"
	TagText       = "[#ededed]"
	TagTextBold   = "[#ededed:b]"
	TagSubText    = "[#a1a1a6]"
	TagMuted      = "[#71717a]"
	TagDark       = "[#52525b]"
	TagReset      = "[-]"
)

// InitTheme applies the Tachyon Graphite theme to tview global styles.
func InitTheme() {
	// Canvas & Surfaces
	tview.Styles.PrimitiveBackgroundColor = ColorBgSpace
	tview.Styles.ContrastBackgroundColor = ColorBgInput
	tview.Styles.MoreContrastBackgroundColor = ColorBgCard

	// Borders & Visual Accents
	tview.Styles.BorderColor = ColorBorderNormal
	tview.Styles.TitleColor = ColorCyanElectric
	tview.Styles.GraphicsColor = ColorBorderNormal

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

// StyleList applies uniform Tachyon Graphite styling to interactive menu lists.
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
func FormatHotkey(key, action string) string {
	return fmt.Sprintf("%s[%s]%s %s%s%s", TagCyanBold, key, TagReset, TagSubText, action, TagReset)
}

// FormatVioletHotkey builds a secondary hotkey chip.
func FormatVioletHotkey(key, action string) string {
	return fmt.Sprintf("%s[%s]%s %s%s%s", TagCyanBold, key, TagReset, TagSubText, action, TagReset)
}

// FormatExitHotkey builds a red hotkey chip for exit/cancel actions.
func FormatExitHotkey(key, action string) string {
	return fmt.Sprintf("%s[%s]%s %s%s%s", TagRedBold, key, TagReset, TagSubText, action, TagReset)
}

// FormatStepTitle produces a standardized Tachyon header across all modals.
func FormatStepTitle(title string) string {
	return fmt.Sprintf(" %sTachyon%s · %s%s%s ", TagCyanBold, TagReset, TagText, strings.TrimSpace(title), TagReset)
}

// FormatModalTitle produces a standardized Tachyon utility header.
func FormatModalTitle(icon, title string) string {
	return fmt.Sprintf(" %sTachyon%s · %s%s%s ", TagCyanBold, TagReset, TagText, strings.TrimSpace(title), TagReset)
}
