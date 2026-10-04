package tui

import (
	"testing"

	"github.com/rivo/tview"

	appconfig "tachyon-installer/internal/config"
)

func TestRenderBar(t *testing.T) {
	bar0 := renderBar(0, 10)
	if bar0 != "░░░░░░░░░░" {
		t.Errorf("expected empty bar, got %s", bar0)
	}

	bar100 := renderBar(100, 10)
	if bar100 != "██████████" {
		t.Errorf("expected full bar, got %s", bar100)
	}

	bar50 := renderBar(50, 10)
	if bar50 != "█████░░░░░" {
		t.Errorf("expected half bar, got %s", bar50)
	}
}

func TestFormatBytes(t *testing.T) {
	cases := []struct {
		in  int64
		out string
	}{
		{500, "500 B"},
		{1024, "1.0 KB"},
		{10 * 1024 * 1024, "10.0 MB"},
		{2 * 1024 * 1024 * 1024, "2.0 GB"},
	}

	for _, c := range cases {
		res := formatBytes(c.in)
		if res != c.out {
			t.Errorf("for %d expected %s, got %s", c.in, c.out, res)
		}
	}
}

func TestToolsModalsCreation(t *testing.T) {
	app := tview.NewApplication()
	pages := tview.NewPages()
	ctx := &AppContext{
		App:    app,
		Pages:  pages,
		Config: appconfig.DefaultConfig(),
	}

	// Test creation of monitor modal
	ShowMonitorModal(ctx, "")
	if !pages.HasPage("monitor_modal") {
		t.Errorf("expected monitor_modal page to be created")
	}

	// Test creation of rescue modal
	ShowRescueModal(ctx, "")
	if !pages.HasPage("rescue_modal") {
		t.Errorf("expected rescue_modal page to be created")
	}

	// Test creation of conflict fix modal
	ShowConflictFixModal(ctx, "")
	if !pages.HasPage("conflicts_modal") {
		t.Errorf("expected conflicts_modal page to be created")
	}

	// Test creation of snapshot modal
	ShowSnapshotModal(ctx, "")
	if !pages.HasPage("snapshot_modal") {
		t.Errorf("expected snapshot_modal page to be created")
	}

	// Test creation of tune modal
	ShowNetworkTuneModal(ctx, "")
	if !pages.HasPage("tune_modal") {
		t.Errorf("expected tune_modal page to be created")
	}

	// Test creation of self update modal
	ShowSelfUpdateModal(ctx, "", "v1.6.0")
	if !pages.HasPage("self_update_modal") {
		t.Errorf("expected self_update_modal page to be created")
	}
}
