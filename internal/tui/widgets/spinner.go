package widgets

import (
	"fmt"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// LoadingSpinner is a custom tview.Primitive that displays an animated loading
// screen with a centered braille spinner and a checklist that fills in sequentially.
type LoadingSpinner struct {
	*tview.Box
	mu sync.Mutex

	title      string
	checklist  []CheckItem
	spinnerIdx int

	// Animation control
	tickCount   int
	running     bool
	app         *tview.Application
	initialized bool
}

// CheckItem represents a single line in the loading checklist.
type CheckItem struct {
	Text    string
	Checked bool
	DelayMs int // ms after which this item gets checked (0 = immediate)
}

// NewLoadingSpinner creates a new LoadingSpinner widget.
func NewLoadingSpinner(title string, items []CheckItem) *LoadingSpinner {
	return &LoadingSpinner{
		Box:       tview.NewBox(),
		title:     title,
		checklist: items,
	}
}

// StartAnimation begins the spinner and checklist animation.
func (ls *LoadingSpinner) StartAnimation(app *tview.Application) {
	ls.app = app
	ls.running = true

	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			if !ls.running {
				return
			}
			ls.mu.Lock()
			ls.tickCount++
			ls.spinnerIdx = (ls.spinnerIdx + 1) % len(spinnerFrames)

			elapsedMs := ls.tickCount * 100
			for i := range ls.checklist {
				if ls.checklist[i].DelayMs > 0 && elapsedMs >= ls.checklist[i].DelayMs && !ls.checklist[i].Checked {
					ls.checklist[i].Checked = true
				}
			}
			ls.mu.Unlock()

			app.QueueUpdateDraw(func() {})
		}
	}()
}

// StopAnimation stops the spinner goroutine.
func (ls *LoadingSpinner) StopAnimation() {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	ls.running = false
}

// AllDone returns true if all checklist items are checked.
func (ls *LoadingSpinner) AllDone() bool {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	for _, item := range ls.checklist {
		if !item.Checked {
			return false
		}
	}
	return true
}

// Draw renders the loading spinner and checklist.
func (ls *LoadingSpinner) Draw(screen tcell.Screen) {
	ls.mu.Lock()
	ls.initialized = true
	defer ls.mu.Unlock()

	x, y, w, h := ls.GetInnerRect()
	bg := tview.Styles.PrimitiveBackgroundColor
	bgStyle := tcell.StyleDefault.Background(bg)

	// Clear background.
	for row := y; row < y+h; row++ {
		for col := x; col < x+w; col++ {
			screen.SetContent(col, row, ' ', nil, bgStyle)
		}
	}

	if w < 20 || h < 5 {
		return
	}

	// Center the content.
	centerX := x + w/2
	contentY := y + (h-len(ls.checklist)-3)/2
	if contentY < y {
		contentY = y
	}

	// Draw title line.
	titleStr := fmt.Sprintf(" %s ", ls.title)
	titleStart := centerX - len(titleStr)/2
	for i, r := range titleStr {
		sx := titleStart + i
		if sx >= x && sx < x+w {
			screen.SetContent(sx, contentY, r, nil, tcell.StyleDefault.
				Foreground(spinnerColor).Background(bg))
		}
	}

	// Draw spinner on next line.
	spinnerLine := contentY + 1
	screen.SetContent(centerX-1, spinnerLine, spinnerFrames[ls.spinnerIdx], nil, tcell.StyleDefault.
		Foreground(spinnerColor).Background(bg))
	screen.SetContent(centerX, spinnerLine, spinnerFrames[(ls.spinnerIdx+1)%len(spinnerFrames)], nil, tcell.StyleDefault.
		Foreground(spinnerColor).Background(bg))

	// Draw separator.
	sepLine := contentY + 2
	sep := "────────────────────────────────"
	sepStart := centerX - len(sep)/2
	for i, r := range sep {
		sx := sepStart + i
		if sx >= x && sx < x+w {
			screen.SetContent(sx, sepLine, r, nil, tcell.StyleDefault.
				Foreground(tcell.NewRGBColor(63, 63, 70)).Background(bg))
		}
	}

	// Draw checklist items.
	for i, item := range ls.checklist {
		itemY := sepLine + 1 + i
		if itemY >= y+h {
			break
		}

		// Icon: spinner if not checked, checkmark if checked.
		var icon rune
		var iconColor tcell.Color
		if item.Checked {
			icon = '✓'
			iconColor = tcell.NewRGBColor(16, 185, 129) // emerald-500
		} else {
			// Use a subtle dot for pending items.
			icon = '○'
			iconColor = tcell.NewRGBColor(148, 163, 184) // slate-400
		}

		lineX := centerX - 18
		screen.SetContent(lineX, itemY, icon, nil, tcell.StyleDefault.
			Foreground(iconColor).Background(bg))

		// Draw item text.
		textStr := fmt.Sprintf("  %s", item.Text)
		for j, r := range textStr {
			sx := lineX + 2 + j
			if sx >= x && sx < x+w {
				var textColor tcell.Color
				if item.Checked {
					textColor = tcell.NewRGBColor(248, 250, 252) // slate-50
				} else {
					textColor = tcell.NewRGBColor(148, 163, 184) // slate-400
				}
				screen.SetContent(sx, itemY, r, nil, tcell.StyleDefault.
					Foreground(textColor).Background(bg))
			}
		}
	}

	// Draw bottom hint.
	bottomY := sepLine + 1 + len(ls.checklist) + 1
	if bottomY < y+h {
		hint := "Пожалуйста, подождите..."
		hintStart := centerX - len(hint)/2
		for i, r := range hint {
			sx := hintStart + i
			if sx >= x && sx < x+w {
				screen.SetContent(sx, bottomY, r, nil, tcell.StyleDefault.
					Foreground(tcell.NewRGBColor(113, 113, 122)).Background(bg))
			}
		}
	}
}
