package tui

import (
	"strings"
	"testing"

	"github.com/rivo/tview"
	routerpkg "tachyon-installer/internal/router"
)

func TestShowAWGModal_Creation(t *testing.T) {
	app := tview.NewApplication()
	pages := tview.NewPages()

	ctx := &AppContext{
		App:   app,
		Pages: pages,
	}

	ShowAWGModal(ctx, "")

	if !pages.HasPage("awg_modal") {
		t.Fatalf("expected awg_modal to be registered in pages")
	}

	modalPage := pages.GetPage("awg_modal")
	if modalPage == nil {
		t.Fatalf("awg_modal page is nil")
	}
}

func TestAWGTestReport_FormatSummary(t *testing.T) {
	rep := &routerpkg.AWGTestReport{
		SectionName:    "warp",
		ServiceRunning: true,
		EngineName:     "sing-box",
		FakeIPActive:   true,
		ResolvedIP:     "198.18.0.55",
		EgressIP:       "104.28.192.1",
		EgressLocation: "Frankfurt, Germany",
		EgressISP:      "Cloudflare Inc.",
		IsWARP:         true,
		LatencyMs:      42,
		Success:        true,
		Details:        "Все тесты успешно пройдены",
		Probes: []routerpkg.TargetProbe{
			{Name: "YouTube", URL: "https://www.youtube.com/generate_204", HTTPStatus: 204, LatencyMs: 45, Success: true},
			{Name: "Discord", URL: "https://discord.com", HTTPStatus: 200, LatencyMs: 50, Success: true},
			{Name: "Rutracker", URL: "https://rutracker.org/forum/index.php", HTTPStatus: 200, LatencyMs: 65, Success: true},
		},
	}

	summary := rep.FormatSummary()
	if !strings.Contains(summary, "работает") {
		t.Errorf("expected summary to contain 'работает'")
	}
	if !strings.Contains(summary, "WARP Active") {
		t.Errorf("expected summary to mention 'WARP Active'")
	}
	if !strings.Contains(summary, "YouTube") || !strings.Contains(summary, "Discord") {
		t.Errorf("expected summary to contain probed site names")
	}
}
