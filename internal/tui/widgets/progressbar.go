// Package widgets provides custom tview.Primitive widgets for the Tachyon installer TUI.
package widgets

import (
	"fmt"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// Unicode block elements for sub-character precision (8 steps).
var blockChars = []rune{'▏', '▎', '▍', '▌', '▋', '▊', '▉', '█'}

// Braille spinner frames for animation.
var spinnerFrames = []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}

// Color palette.
var (
	gradientStart = tcell.NewRGBColor(14, 165, 233)  // #0ea5e9 sky-500
	gradientEnd   = tcell.NewRGBColor(99, 102, 241)  // #6366f1 indigo-500
	subGradStart  = tcell.NewRGBColor(99, 102, 241)  // #6366f1 indigo-500
	subGradEnd    = tcell.NewRGBColor(168, 85, 247)  // #a855f7 purple-500
	spinnerColor  = tcell.NewRGBColor(56, 189, 248)  // #38bdf8 sky-400
	subSpinColor  = tcell.NewRGBColor(167, 139, 250) // #a78bfa violet-400
	labelColor    = tcell.NewRGBColor(228, 228, 231) // zinc-200
	subLabelColor = tcell.NewRGBColor(196, 181, 253) // violet-300
	percentColor  = tcell.NewRGBColor(255, 255, 255) // white
	trackColor    = tcell.NewRGBColor(39, 39, 42)    // zinc-800
	sepColor      = tcell.NewRGBColor(51, 65, 85)    // slate-700
)

// ProgressBar renders an animated dual progress bar:
//   - Row 0-1: overall installation progress (steps)
//   - Row 2:   thin separator
//   - Row 3-4: per-subtask progress (current operation)
type ProgressBar struct {
	*tview.Box
	mu sync.Mutex

	// Overall progress
	currentStep     int
	totalSteps      int
	phaseLabel      string
	fraction        float64 // 0.0–1.0 within current step
	completed       bool
	displayFraction float64

	// Sub-task progress
	subLabel    string
	subFraction float64
	subActive   bool
	subDisplay  float64

	// Animation
	spinnerIdx  int
	stopChan    chan struct{}
	initialized bool
}

// NewProgressBar creates a new ProgressBar with the given total steps.
func NewProgressBar(totalSteps int) *ProgressBar {
	return &ProgressBar{
		Box:        tview.NewBox(),
		totalSteps: totalSteps,
		stopChan:   make(chan struct{}),
	}
}

// SetProgress updates the progress bar to a specific step and phase.
// fraction is 0.0–1.0 representing progress within the current step.
func (pb *ProgressBar) SetProgress(step int, total int, label string, fraction float64) {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	pb.currentStep = step
	pb.totalSteps = total
	pb.phaseLabel = label
	pb.fraction = fraction
	pb.completed = false
}

// MarkCompleted marks the overall progress bar as fully done.
func (pb *ProgressBar) MarkCompleted() {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	pb.currentStep = pb.totalSteps
	pb.fraction = 1.0
	pb.completed = true
	pb.phaseLabel = "Завершено"
	pb.subActive = false
}

// SetStep sets progress for a discrete step (0-based index).
func (pb *ProgressBar) SetStep(step int, label string) {
	pb.mu.Lock()
	pb.totalSteps = 7
	pb.currentStep = step + 1
	pb.fraction = 0.0
	pb.phaseLabel = label
	pb.completed = false
	pb.mu.Unlock()
}

// SetSubTask activates the sub-task bar with a label and fraction (0.0–1.0).
func (pb *ProgressBar) SetSubTask(label string, fraction float64) {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	pb.subLabel = label
	pb.subFraction = fraction
	pb.subActive = true
}

// ClearSubTask hides the sub-task bar.
func (pb *ProgressBar) ClearSubTask() {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	pb.subActive = false
	pb.subLabel = ""
	pb.subFraction = 0
	pb.subDisplay = 0
}

// AdvanceTick smoothly advances displayed fractions toward their targets.
// Call from an animation ticker.
func (pb *ProgressBar) AdvanceTick() {
	pb.mu.Lock()
	defer pb.mu.Unlock()

	pb.spinnerIdx = (pb.spinnerIdx + 1) % len(spinnerFrames)

	// Smooth overall fraction.
	target := float64(pb.currentStep-1) + pb.fraction
	if pb.totalSteps > 0 {
		target /= float64(pb.totalSteps)
	}
	if target > 1.0 {
		target = 1.0
	}
	diff := target - pb.displayFraction
	if diff > 0.005 {
		pb.displayFraction += diff * 0.08
	} else {
		pb.displayFraction = target
	}

	// Smooth sub-task fraction.
	if pb.subActive {
		subDiff := pb.subFraction - pb.subDisplay
		if subDiff > 0.005 {
			pb.subDisplay += subDiff * 0.10
		} else {
			pb.subDisplay = pb.subFraction
		}
	} else {
		pb.subDisplay = 0
	}
}

// StartAnimation begins the animation loop. Call once when showing the progress page.
func (pb *ProgressBar) StartAnimation(app *tview.Application) {
	go func() {
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-pb.stopChan:
				return
			case <-ticker.C:
				pb.AdvanceTick()
				app.QueueUpdateDraw(func() {})
			}
		}
	}()
}

// StopAnimation stops the animation goroutine.
func (pb *ProgressBar) StopAnimation() {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	select {
	case <-pb.stopChan:
		// already stopped
	default:
		close(pb.stopChan)
	}

	// Force the display to reach the target immediately when stopping.
	target := float64(pb.currentStep-1) + pb.fraction
	if pb.totalSteps > 0 {
		target /= float64(pb.totalSteps)
	}
	if target > 1.0 {
		target = 1.0
	}
	pb.displayFraction = target
	pb.subDisplay = pb.subFraction
}

// Draw renders the dual-bar progress widget.
func (pb *ProgressBar) Draw(screen tcell.Screen) {
	pb.mu.Lock()
	pb.initialized = true
	defer pb.mu.Unlock()

	x, y, w, h := pb.GetInnerRect()
	if w < 10 || h < 2 {
		return
	}

	bg := tview.Styles.PrimitiveBackgroundColor
	bgStyle := tcell.StyleDefault.Background(bg)

	// Clear background.
	for row := y; row < y+h; row++ {
		for col := x; col < x+w; col++ {
			screen.SetContent(col, row, ' ', nil, bgStyle)
		}
	}

	// ── Row 0: overall spinner + label + percentage ──────────────────────
	spinnerRune := spinnerFrames[pb.spinnerIdx]
	if pb.completed {
		spinnerRune = '✓'
	}
	screen.SetContent(x, y, spinnerRune, nil, tcell.StyleDefault.
		Foreground(spinnerColor).Background(bg))

	labelStr := fmt.Sprintf(" %s", pb.phaseLabel)
	for i, r := range labelStr {
		if x+1+i >= x+w-6 {
			break
		}
		screen.SetContent(x+1+i, y, r, nil, tcell.StyleDefault.
			Foreground(labelColor).Background(bg))
	}

	pct := int(pb.displayFraction * 100)
	if pct > 100 {
		pct = 100
	}
	pctStr := fmt.Sprintf("%3d%%", pct)
	for i, r := range pctStr {
		screen.SetContent(x+w-4+i, y, r, nil, tcell.StyleDefault.
			Foreground(percentColor).Background(bg))
	}

	// ── Row 1: overall gradient bar ──────────────────────────────────────
	if y+1 < y+h {
		drawProgressBar(screen, x, y+1, w, pb.displayFraction, gradientStart, gradientEnd, pb.completed, bg)
	}

	// ── Rows 2-4: sub-task (only when active and space permits) ──────────
	if !pb.subActive || h < 5 {
		return
	}

	// Row 2: separator line
	if y+2 < y+h {
		for col := x; col < x+w; col++ {
			screen.SetContent(col, y+2, '─', nil, tcell.StyleDefault.
				Foreground(sepColor).Background(bg))
		}
	}

	// Row 3: sub-task arrow + label + sub-pct
	if y+3 < y+h {
		screen.SetContent(x, y+3, '→', nil, tcell.StyleDefault.
			Foreground(subSpinColor).Background(bg))

		subStr := fmt.Sprintf(" %s", pb.subLabel)
		for i, r := range subStr {
			if x+1+i >= x+w-6 {
				break
			}
			screen.SetContent(x+1+i, y+3, r, nil, tcell.StyleDefault.
				Foreground(subLabelColor).Background(bg))
		}

		subPct := int(pb.subDisplay * 100)
		if subPct > 100 {
			subPct = 100
		}
		subPctStr := fmt.Sprintf("%3d%%", subPct)
		for i, r := range subPctStr {
			screen.SetContent(x+w-4+i, y+3, r, nil, tcell.StyleDefault.
				Foreground(percentColor).Background(bg))
		}
	}

	// Row 4: sub-task gradient bar
	if y+4 < y+h {
		drawProgressBar(screen, x, y+4, w, pb.subDisplay, subGradStart, subGradEnd, false, bg)
	}
}

// drawProgressBar renders a single gradient progress bar at (x, y) with width w.
func drawProgressBar(screen tcell.Screen, x, y, w int, fraction float64, start, end tcell.Color, completed bool, bg tcell.Color) {
	// Draw track.
	for col := x; col < x+w; col++ {
		screen.SetContent(col, y, ' ', nil, tcell.StyleDefault.Background(trackColor))
	}

	filledCells := fraction * float64(w)
	fullBlocks := int(filledCells)
	remainder := filledCells - float64(fullBlocks)

	for i := 0; i < fullBlocks && i < w; i++ {
		t := float64(i) / float64(w)
		color := lerpColor(start, end, t)
		screen.SetContent(x+i, y, '█', nil, tcell.StyleDefault.Background(color))
	}

	if fullBlocks < w && remainder > 0 {
		blockIdx := int(remainder * 8)
		if blockIdx >= 8 {
			blockIdx = 7
		}
		t := float64(fullBlocks) / float64(w)
		color := lerpColor(start, end, t)
		screen.SetContent(x+fullBlocks, y, blockChars[blockIdx], nil, tcell.StyleDefault.
			Foreground(color).Background(trackColor))
	}

	if completed && w > 2 {
		screen.SetContent(x+w-1, y, '█', nil, tcell.StyleDefault.Background(end))
	}
}

// lerpColor interpolates between two tcell colors.
func lerpColor(c1, c2 tcell.Color, t float64) tcell.Color {
	r1, g1, b1 := c1.RGB()
	r2, g2, b2 := c2.RGB()
	r := uint8(float64(r1) + (float64(r2)-float64(r1))*t)
	g := uint8(float64(g1) + (float64(g2)-float64(g1))*t)
	b := uint8(float64(b1) + (float64(b2)-float64(b1))*t)
	return tcell.NewRGBColor(int32(r), int32(g), int32(b))
}
