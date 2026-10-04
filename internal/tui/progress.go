package tui

import (
	"os"
	"path/filepath"
	"strings"

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
// It strips ANSI codes, collapses known harmless router noise into a short hint
// and keeps the full raw output in a log file.
type tviewWriter struct {
	textView *tview.TextView
	app      *tview.Application

	buf    string
	skip   int
	logf   *os.File
	path   string
	hinted bool // firewall-not-ready hint was shown
}

// newTviewWriter creates a filtering writer with a raw log in the temp directory.
func newTviewWriter(tv *tview.TextView, app *tview.Application) *tviewWriter {
	w := &tviewWriter{textView: tv, app: app}
	path := filepath.Join(os.TempDir(), "tachyon-installer.log")
	if f, err := os.Create(path); err == nil {
		w.logf = f
		w.path = path
	}
	return w
}

// Write implements io.Writer.
func (tw *tviewWriter) Write(p []byte) (n int, err error) {
	text := StripANSI(string(p))
	if tw.logf != nil {
		tw.logf.WriteString(text)
	}
	tw.buf += strings.ReplaceAll(text, "\r", "")
	var out strings.Builder
	for {
		i := strings.IndexByte(tw.buf, '\n')
		if i < 0 {
			break
		}
		line := tw.buf[:i]
		tw.buf = tw.buf[i+1:]
		out.WriteString(tw.filterLine(line))
	}
	tw.emit(out.String())
	return len(p), nil
}

// Flush prints any trailing partial line.
func (tw *tviewWriter) Flush() {
	if tw.buf != "" {
		rest := tw.buf
		tw.buf = ""
		tw.emit(tview.Escape(rest) + "\n")
	}
}

// Close releases the raw log file.
func (tw *tviewWriter) Close() {
	if tw.logf != nil {
		tw.logf.Close()
		tw.logf = nil
	}
}

func (tw *tviewWriter) filterLine(line string) string {
	if tw.skip > 0 {
		tw.skip--
		return ""
	}
	trimmed := strings.TrimSpace(line)
	switch {
	case strings.Contains(trimmed, "Command failed: ubus call service delete"):
		return "" // cleanup of a service that does not exist yet
	case strings.HasPrefix(trimmed, "Error: Could not process rule"):
		tw.skip = 2 // the echoed rule and its ^^^ marker
		if tw.hinted {
			return ""
		}
		tw.hinted = true
		return "[#eab308]⚠ Правила firewall fw4 пока не применены (таблица ещё не загружена). Некритично, подробности в конце.[-]\n"
	}
	return tview.Escape(line) + "\n"
}

func (tw *tviewWriter) emit(text string) {
	if text == "" {
		return
	}
	tw.app.QueueUpdateDraw(func() {
		tw.textView.Write([]byte(text))
		tw.textView.ScrollToEnd()
	})
}


// adaptiveModal centers its content and shrinks it to fit the terminal.
type adaptiveModal struct {
	*tview.Box
	content       tview.Primitive
	width, height int
}

// Draw sizes the content to min(requested, screen) and centers it.
func (m *adaptiveModal) Draw(screen tcell.Screen) {
	sw, sh := screen.Size()
	w, h := m.width, m.height
	if w > sw {
		w = sw
	}
	if h > sh {
		h = sh
	}
	x, y := (sw-w)/2, (sh-h)/2
	m.SetRect(x, y, w, h)
	m.content.SetRect(x, y, w, h)
	m.content.Draw(screen)
}

// Focus delegates focus to the wrapped content.
func (m *adaptiveModal) Focus(delegate func(p tview.Primitive)) { delegate(m.content) }

// HasFocus reports whether the wrapped content has focus.
func (m *adaptiveModal) HasFocus() bool { return m.content.HasFocus() }

// InputHandler forwards keyboard input to the wrapped content.
func (m *adaptiveModal) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return m.content.InputHandler()
}

// MouseHandler forwards mouse events to the wrapped content.
func (m *adaptiveModal) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, recipient tview.Primitive) {
	return func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, recipient tview.Primitive) {
		if m.content == nil {
			return false, nil
		}
		if handler := m.content.MouseHandler(); handler != nil {
			return handler(action, event, setFocus)
		}
		return false, nil
	}
}

// CreateWizardModal wraps a panel in a centered modal overlay with default dimensions.
func CreateWizardModal(panel tview.Primitive) tview.Primitive {
	return CreateWizardModalCustom(panel, 84, 24)
}

// CreateWizardModalCustom wraps a panel in a centered modal that adapts to small terminals.
func CreateWizardModalCustom(panel tview.Primitive, width, height int) tview.Primitive {
	return &adaptiveModal{Box: tview.NewBox(), content: panel, width: width, height: height}
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
