package tui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// SolidFlex is a tview.Flex that fills its background completely,
// eliminating transparent gaps in the Zinc dark theme.
type SolidFlex struct {
	*tview.Flex
}

// NewSolidFlex creates a new SolidFlex.
func NewSolidFlex() *SolidFlex {
	return &SolidFlex{Flex: tview.NewFlex()}
}

// Draw renders the SolidFlex, filling the background before delegating to the inner Flex.
func (sf *SolidFlex) Draw(screen tcell.Screen) {
	if sf.GetBackgroundColor() != tcell.ColorDefault {
		x, y, w, h := sf.GetRect()
		style := tcell.StyleDefault.Background(sf.GetBackgroundColor())
		for row := y; row < y+h; row++ {
			for col := x; col < x+w; col++ {
				screen.SetContent(col, row, ' ', nil, style)
			}
		}
	}
	sf.Flex.Draw(screen)
}

// tviewWriter adapts tview.TextView to an io.Writer for streaming SSH output.
type tviewWriter struct {
	textView *tview.TextView
	app      *tview.Application
}

// Write implements io.Writer — strips ANSI codes and scrolls to end.
func (tw *tviewWriter) Write(p []byte) (n int, err error) {
	text := string(p)
	text = StripANSI(text)
	tw.app.QueueUpdateDraw(func() {
		tw.textView.Write([]byte(text))
		tw.textView.ScrollToEnd()
	})
	return len(p), nil
}

// ZincColor returns a tcell color from the Slate palette to match our theme.
func ZincColor(level int) tcell.Color {
	if level >= 70 {
		return tcell.NewRGBColor(51, 65, 85) // Slate 700
	}
	return tcell.NewRGBColor(15, 23, 42) // Slate 950
}

// InitTheme applies the Slate and Sky-blue dark theme to tview global styles.
func InitTheme() {
	// Backgrounds
	tview.Styles.PrimitiveBackgroundColor = tcell.NewRGBColor(15, 23, 42)    // Slate 950
	tview.Styles.ContrastBackgroundColor = tcell.NewRGBColor(30, 41, 59)     // Slate 800 (inputs, buttons)
	tview.Styles.MoreContrastBackgroundColor = tcell.NewRGBColor(15, 23, 42) // Slate 950 (popups)

	// Borders & Titles
	tview.Styles.BorderColor = tcell.NewRGBColor(51, 65, 85)     // Slate 700
	tview.Styles.TitleColor = tcell.NewRGBColor(56, 189, 248)    // Sky 400
	tview.Styles.GraphicsColor = tcell.NewRGBColor(56, 189, 248) // Sky 400

	// Text colors
	tview.Styles.PrimaryTextColor = tcell.NewRGBColor(241, 245, 249)           // Slate 100
	tview.Styles.SecondaryTextColor = tcell.NewRGBColor(203, 213, 225)         // Slate 300
	tview.Styles.TertiaryTextColor = tcell.NewRGBColor(148, 163, 184)          // Slate 400
	tview.Styles.InverseTextColor = tcell.NewRGBColor(255, 255, 255)           // White (text on focused blue buttons/options)
	tview.Styles.ContrastSecondaryTextColor = tcell.NewRGBColor(125, 211, 252) // Sky 300
}

// CreateWizardModal wraps a panel in a centered modal overlay with default dimensions.
func CreateWizardModal(panel tview.Primitive) *tview.Flex {
	return CreateWizardModalCustom(panel, 84, 24)
}

// CreateWizardModalCustom wraps a panel in a centered modal overlay with custom dimensions.
func CreateWizardModalCustom(panel tview.Primitive, width, height int) *tview.Flex {
	return tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			SetDirection(tview.FlexColumn).
			AddItem(nil, 0, 1, false).
			AddItem(panel, width, 1, true).
			AddItem(nil, 0, 1, false), height, 1, true).
		AddItem(nil, 0, 1, false)
}

// BuildConsoleLayout creates the main progress/console view layout
// with a header, a progress bar line, and a scrollable log area.
func BuildConsoleLayout(header tview.Primitive, progress tview.Primitive, console tview.Primitive) *tview.Flex {
	return tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(header, 1, 0, false).
		AddItem(progress, 6, 0, false).
		AddItem(console, 0, 1, true)
}

// textViewLayout wraps a TextView in a Flex with horizontal padding.
func textViewLayout(tv *tview.TextView) *tview.Flex {
	return tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(nil, 2, 0, false).
		AddItem(tv, 0, 1, false).
		AddItem(nil, 2, 0, false)
}

// formLayout wraps a Form in a Flex with horizontal padding.
func formLayout(form *tview.Form) *tview.Flex {
	return tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(nil, 2, 0, false).
		AddItem(form, 0, 1, true).
		AddItem(nil, 2, 0, false)
}
