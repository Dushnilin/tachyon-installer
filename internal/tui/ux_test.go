package tui

import (
	"strings"
	"testing"

	"github.com/rivo/tview"
)

func TestValidateSSHInput(t *testing.T) {
	cases := []struct {
		host, port, user string
		field            int
		wantErr          bool
	}{
		{"192.168.1.1", "22", "root", 0, false},
		{"router.lan", "2222", "admin", 0, false},
		{"", "22", "root", 0, true},
		{"192.168 .1.1", "22", "root", 0, true},
		{"192.168.1.1", "abc", "root", 1, true},
		{"192.168.1.1", "0", "root", 1, true},
		{"192.168.1.1", "70000", "root", 1, true},
		{"192.168.1.1", "22", "  ", 2, true},
	}
	for _, c := range cases {
		field, msg := validateSSHInput(c.host, c.port, c.user)
		if (msg != "") != c.wantErr {
			t.Errorf("%+v: msg=%q, wantErr=%v", c, msg, c.wantErr)
			continue
		}
		if c.wantErr && field != c.field {
			t.Errorf("%+v: field=%d, want %d", c, field, c.field)
		}
	}
}

func TestRouterNoiseFilter(t *testing.T) {
	w := &tviewWriter{}

	noise := []string{
		`Command failed: ubus call service delete { "name": "steer" } (Not found)`,
		`Command failed: ubus call service delete { "name": "tachyon-steer-zapret" } (Not found)`,
	}
	for _, l := range noise {
		if out := w.filterLine(l); out != "" {
			t.Errorf("ubus noise not dropped: %q", out)
		}
	}

	// First nft error collapses into one hint and swallows the echoed rule + marker.
	first := w.filterLine("Error: Could not process rule: No such file or directory")
	if !strings.Contains(first, "fw4") {
		t.Fatalf("expected firewall hint, got %q", first)
	}
	if out := w.filterLine("insert rule inet fw4 forward meta mark 0x04000000 return"); out != "" {
		t.Errorf("rule echo not swallowed: %q", out)
	}
	if out := w.filterLine("            ^^^"); out != "" {
		t.Errorf("marker not swallowed: %q", out)
	}
	if !w.hinted {
		t.Error("hinted flag not set")
	}

	// The hint is shown once only.
	if out := w.filterLine("Error: Could not process rule: No such file or directory"); out != "" {
		t.Errorf("repeat hint shown: %q", out)
	}
	w.filterLine("rule")
	w.filterLine("^^^")

	// Real output must pass through, with tview tags escaped.
	out := w.filterLine("Installing tachyon [5/5]")
	if !strings.Contains(out, "Installing tachyon") || !strings.HasSuffix(out, "\n") {
		t.Errorf("normal line mangled: %q", out)
	}
}

func TestShowWelcomeWizard(t *testing.T) {
	app := tview.NewApplication()
	pages := tview.NewPages()
	ctx := &AppContext{
		App:   app,
		Pages: pages,
	}

	ShowWelcomeWizard(ctx)

	if !pages.HasPage("welcome_wizard") {
		t.Errorf("expected welcome_wizard page to be registered")
	}
	if !pages.HasPage("ssh_wizard") {
		t.Errorf("expected ssh_wizard page to be registered")
	}
	if !pages.HasPage("password_wizard") {
		t.Errorf("expected password_wizard page to be registered")
	}
}
