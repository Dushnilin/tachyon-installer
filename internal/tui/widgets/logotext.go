package widgets

import (
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// LogoColors defines the gradient colors for each row of the TACHYON logo.
var logoColors = []tcell.Color{
	tcell.NewRGBColor(14, 165, 233),  // #0ea5e9 sky-500
	tcell.NewRGBColor(14, 165, 233),  // #0ea5e9
	tcell.NewRGBColor(56, 189, 248),  // #38bdf8 sky-400
	tcell.NewRGBColor(56, 189, 248),  // #38bdf8
	tcell.NewRGBColor(125, 211, 252), // #7dd3fc sky-300
	tcell.NewRGBColor(125, 211, 252), // #7dd3fc
}

// flashColor is the bright white flash when a character is first typed.
var flashColor = tcell.NewRGBColor(255, 255, 255)

// LogoText is a custom tview.Primitive that renders the TACHYON ASCII logo
// with a typing animation and color wave effect.
type LogoText struct {
	*tview.Box
	mu sync.Mutex

	// The full logo as a slice of strings (each string is one row).
	logoLines []string

	// Animation state.
	charsRevealed int // total characters revealed so far
	totalChars    int
	flashDuration int // ticks a character stays white before fading

	// Callback when animation completes.
	OnComplete func()

	initialized bool
}

// tachyonLogo is the ASCII art for the TACHYON banner.
var tachyonLogo = []string{
	"████████╗ █████╗  ██████╗██╗  ██╗██╗  ██╗ ██████╗ ███╗   ██╗",
	"╚══██╔══╝██╔══██╗██╔════╝██║  ██║╚██╗██╔╝██╔═══██╗████╗  ██║",
	"   ██║   ███████║██║     ███████║ ╚███╔╝ ██║   ██║██╔██╗ ██║",
	"   ██║   ██╔══██║██║     ██╔══██║  ██╔╝  ██║   ██║██║╚██╗██║",
	"   ██║   ██║  ██║╚██████╗██║  ██║ ██╔╝   ╚██████╔╝██║ ╚████║",
	"   ╚═╝   ╚═╝  ╚═╝ ╚═════╝╚═╝  ╚═╝ ╚═╝     ╚═════╝ ╚═╝  ╚═══╝",
}

// NewLogoText creates a new LogoText widget ready for animation.
func NewLogoText() *LogoText {
	lt := &LogoText{
		Box:           tview.NewBox(),
		logoLines:     tachyonLogo,
		charsRevealed: 0,
		flashDuration: 3,
	}

	// Count total runes (not bytes — block chars are 3 bytes each in UTF-8).
	for _, line := range lt.logoLines {
		lt.totalChars += utf8.RuneCountInString(line)
	}

	return lt
}

// StartAnimation begins the typing animation. Call this once.
func (lt *LogoText) StartAnimation(app *tview.Application) {
	go func() {
		ticker := time.NewTicker(18 * time.Millisecond) // slightly faster for premium feel
		defer ticker.Stop()
		for range ticker.C {
			lt.mu.Lock()
			lt.charsRevealed += 4 // reveal 4 chars per tick → ~1.8s total
			done := lt.charsRevealed >= lt.totalChars
			lt.mu.Unlock()

			app.QueueUpdateDraw(func() {})

			if done {
				if lt.OnComplete != nil {
					lt.OnComplete()
				}
				return
			}
		}
	}()
}

// Draw renders the logo with typing animation and color wave.
func (lt *LogoText) Draw(screen tcell.Screen) {
	lt.mu.Lock()
	lt.initialized = true
	defer lt.mu.Unlock()

	x, y, w, h := lt.GetInnerRect()
	bg := tview.Styles.PrimitiveBackgroundColor

	// Calculate vertical centering for the logo.
	logoHeight := len(lt.logoLines)
	startY := y + (h-logoHeight)/2
	if startY < y {
		startY = y
	}

	charIdx := 0 // running rune index across all rows

	for row, line := range lt.logoLines {
		screenY := startY + row
		if screenY >= y+h {
			break
		}

		// Use rune count (not byte length) for correct horizontal centering.
		lineWidth := utf8.RuneCountInString(line)
		padding := (w - lineWidth) / 2
		if padding < 0 {
			padding = 0
		}

		colIdx := 0 // visual column index within this line (rune position, not byte offset)
		for _, r := range line {
			charIdx++
			screenX := x + padding + colIdx
			colIdx++ // advance by 1 cell per rune (block chars are single-width)
			if screenX >= x+w {
				break
			}

			if charIdx > lt.charsRevealed {
				// Not yet revealed — draw background.
				screen.SetContent(screenX, screenY, ' ', nil, tcell.StyleDefault.Background(bg))
				continue
			}

			// Flash white when recently typed, then settle into row gradient color.
			var color tcell.Color
			charsSinceType := lt.charsRevealed - charIdx
			if charsSinceType < lt.flashDuration {
				color = flashColor
			} else if row < len(logoColors) {
				color = logoColors[row]
			} else {
				color = logoColors[len(logoColors)-1]
			}

			screen.SetContent(screenX, screenY, r, nil, tcell.StyleDefault.
				Foreground(color).Background(bg))
		}
	}
}
