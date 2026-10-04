package tui

import (
	"testing"

	"github.com/rivo/tview"
)

func TestShowGuidedSetupModal(t *testing.T) {
	app := tview.NewApplication()
	pages := tview.NewPages()

	ctx := &AppContext{
		App:   app,
		Pages: pages,
	}

	ShowGuidedSetupModal(ctx, "")

	if !pages.HasPage("guided_modal") {
		t.Errorf("expected guided_modal to be registered in pages")
	}
}
