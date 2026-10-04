package router

import (
	"fmt"
	"strings"

	gossh "golang.org/x/crypto/ssh"
	sshutil "tachyon-installer/internal/ssh"
)

// KnownConflictingServices lists services known to bind DNS (port 53) or TProxy/redirect ports.
var KnownConflictingServices = []string{
	"passwall",
	"openclash",
	"shadowsocksr",
	"podkop",
	"forkop",
	"zapret",
	"v2ray",
	"xray",
	"clash",
}

// ConflictFixReport summarizes the outcome of disabling conflicting packages.
type ConflictFixReport struct {
	Success       bool
	DisabledCount int
	DisabledList  []string
	Details       string
}

// DisableConflicts safely stops and disables autostart for all known conflicting proxy/dns services.
func DisableConflicts(client *gossh.Client, execFn sshutil.ExecFunc, targetServices []string) ConflictFixReport {
	report := ConflictFixReport{
		DisabledList: []string{},
	}

	if len(targetServices) == 0 {
		targetServices = KnownConflictingServices
	}

	var sb strings.Builder
	sb.WriteString("#!/bin/sh\n")
	sb.WriteString("DISABLED=\"\"\n")

	for _, svc := range targetServices {
		svc = strings.TrimSpace(svc)
		if svc == "" {
			continue
		}
		sb.WriteString(fmt.Sprintf(`
if [ -x "/etc/init.d/%s" ]; then
    /etc/init.d/%s stop >/dev/null 2>&1 || true
    /etc/init.d/%s disable >/dev/null 2>&1 || true
    killall -9 %s 2>/dev/null || true
    DISABLED="$DISABLED %s"
fi
`, svc, svc, svc, svc, svc))
	}

	sb.WriteString(`
echo "DISABLED_SERVICES:$DISABLED"
`)

	out, err := execFn(client, sb.String())
	if err != nil {
		report.Success = false
		report.Details = fmt.Sprintf("Ошибка остановки конфликтующих служб: %v", err)
		return report
	}

	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "DISABLED_SERVICES:") {
			raw := strings.TrimPrefix(line, "DISABLED_SERVICES:")
			for _, item := range strings.Fields(raw) {
				report.DisabledList = append(report.DisabledList, item)
			}
		}
	}

	report.DisabledCount = len(report.DisabledList)
	report.Success = true

	if report.DisabledCount > 0 {
		report.Details = fmt.Sprintf("Успешно остановлены и отключены службы: %s", strings.Join(report.DisabledList, ", "))
	} else {
		report.Details = "Конфликтующих запущенных служб не обнаружено."
	}

	return report
}
