package diag

import (
	"os"
	"strings"
	"testing"
)

type fake struct {
	rules []struct{ contains, out string }
}

func (f *fake) on(contains, out string) *fake {
	f.rules = append(f.rules, struct{ contains, out string }{contains, out})
	return f
}

func (f *fake) run(cmd string) (string, error) {
	for _, r := range f.rules {
		if strings.Contains(cmd, r.contains) {
			return r.out, nil
		}
	}
	return "", nil
}

func find(t *testing.T, rs []Result, id string) Result {
	t.Helper()
	for _, r := range rs {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("check %q not found", id)
	return Result{}
}

func healthy() *fake {
	return (&fake{}).
		on("/tmp/sysinfo/model", "Test Router\nREL:25.12.5 ARCH:aarch64_generic\nUNAME:aarch64\n").
		on("MemTotal", "512000 300000").
		on("df -k /overlay", "100000").
		on("date +%Y", "2026").
		on("nft list tables", "table inet fw4\n---\n").
		on("apk info", "kmod-nft-tproxy-6.12\nkmod-tun-6.12\ntachyon-1.4.9-r1\nluci-app-tachyon-1.4.9\n").
		on("nslookup openwrt.org 127.0.0.1", "LOCAL").
		on("ping -c 1", "OK").
		on("github.com", "OK").
		on("[ -x /usr/bin/tachyon ] && /usr/bin/tachyon get_status", "RUNNING").
		on("[ -x /usr/bin/tachyon ] && echo BIN", "BIN").
		on("command -v $b", "sing-box=/usr/bin/sing-box").
		on("logread", "tachyon: started")
}

func TestHealthyRouter(t *testing.T) {
	rs := RunRouter(healthy().run, nil)
	_, warn, fail := Summary(rs)
	if warn != 0 || fail != 0 {
		t.Fatalf("healthy router reported warn=%d fail=%d:\n%s", warn, fail, Format("t", rs))
	}
	if d := find(t, rs, "system").Detail; !strings.Contains(d, "Test Router") || !strings.Contains(d, "25.12.5") {
		t.Errorf("system detail: %q", d)
	}
}

func TestMissingFirewallIsFailWithHint(t *testing.T) {
	f := healthy()
	f.rules[4].out = "---\n" // no inet fw4
	r := find(t, RunRouter(f.run, nil), "firewall")
	if r.Status != Fail || !strings.Contains(r.Hint, "firewall restart") {
		t.Errorf("got %+v", r)
	}
}

func TestLowMemoryAndFlashThresholds(t *testing.T) {
	f := healthy()
	f.rules[1].out = "128000 20000" // 20 MB free
	f.rules[2].out = "6000"         // ~5.8 MB
	rs := RunRouter(f.run, nil)
	if find(t, rs, "memory").Status != Fail {
		t.Error("20 MB free RAM must be Fail")
	}
	if find(t, rs, "flash").Status != Fail {
		t.Error("<8 MB flash must be Fail")
	}
}

func TestConflictsDetectedButNotSubstrings(t *testing.T) {
	f := healthy()
	f.rules[5].out = "luci-app-passwall-1.0\nkmod-nft-tproxy-6\nkmod-tun-6\npodkop-0.7.1\ntachyon-1.4.9\nnextdnsfoo-1\n"
	r := find(t, RunRouter(f.run, nil), "conflicts")
	if r.Status != Warn || !strings.Contains(r.Detail, "podkop") || !strings.Contains(r.Detail, "luci-app-passwall") {
		t.Errorf("got %+v", r)
	}
	if strings.Contains(r.Detail, "nextdns") {
		t.Errorf("nextdnsfoo must not match nextdns: %q", r.Detail)
	}
}

func TestMissingModulesWarn(t *testing.T) {
	f := healthy()
	f.rules[5].out = "tachyon-1.4.9\n"
	r := find(t, RunRouter(f.run, nil), "modules")
	if r.Status != Warn || !strings.Contains(r.Detail, "kmod-nft-tproxy") {
		t.Errorf("got %+v", r)
	}
}

func TestNotInstalledSkipsServiceChecks(t *testing.T) {
	f := healthy()
	f.rules[5].out = "kmod-nft-tproxy-6\nkmod-tun-6\n"
	f.rules[10].out = "" // no /usr/bin/tachyon
	rs := RunRouter(f.run, nil)
	if find(t, rs, "tachyon").Status != Fail {
		t.Error("tachyon must be Fail when not installed")
	}
	for _, r := range rs {
		if r.ID == "service" || r.ID == "engines" || r.ID == "logs" {
			t.Errorf("check %s must be skipped when not installed", r.ID)
		}
	}
}

func TestLogErrorsEscalateToWarn(t *testing.T) {
	f := healthy()
	f.rules[len(f.rules)-1].out = "sing-box: FATAL start service: bad config"
	if r := find(t, RunRouter(f.run, nil), "logs"); r.Status != Warn {
		t.Errorf("got %+v", r)
	}
}

func TestFormatContainsHintsAndSummary(t *testing.T) {
	out := Format("Отчёт", []Result{
		{Title: "A", Status: OK, Detail: "fine"},
		{Title: "B", Status: Fail, Detail: "bad", Hint: "fix it"},
	})
	for _, want := range []string{"[ OK ] A", "[FAIL] B", "-> fix it", "ошибок 1"} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q:\n%s", want, out)
		}
	}
}

func TestHasPackage(t *testing.T) {
	pk := "tachyon-1.4.9-r1\nluci-app-tachyon-1.4.9\nluci-i18n-tachyon-ru-1.4.9\nkmod-tun - 6.12 - \n"
	if !hasPackage(pk, "tachyon") {
		t.Error("apk-style tachyon")
	}
	if !hasPackage(pk, "kmod-tun") {
		t.Error("opkg-style kmod-tun")
	}
	if hasPackage(pk, "kmod-nft-tproxy") {
		t.Error("false positive")
	}
	if hasPackage("tachyon-helper-2\n", "tachyon") {
		t.Error("tachyon-helper must not count as tachyon")
	}
}

func TestExportToFile(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := tmpDir + "/subdir/test-report.txt"
	rs := []Result{
		{ID: "c1", Title: "Check 1", Status: OK, Detail: "all good"},
		{ID: "c2", Title: "Check 2", Status: Fail, Detail: "broken", Hint: "fix here"},
	}
	savedPath, err := ExportToFile("Test Diagnostics", rs, outPath)
	if err != nil {
		t.Fatalf("ExportToFile failed: %v", err)
	}
	if savedPath != outPath {
		t.Errorf("ExportToFile returned path %q, want %q", savedPath, outPath)
	}

	content, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read exported file failed: %v", err)
	}
	str := string(content)
	if !strings.Contains(str, "Test Diagnostics") || !strings.Contains(str, "[ OK ] Check 1") || !strings.Contains(str, "[FAIL] Check 2") {
		t.Errorf("unexpected file content: %s", str)
	}
}
