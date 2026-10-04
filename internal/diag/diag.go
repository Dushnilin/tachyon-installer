// Package diag runs built-in health checks against the router and the host PC.
package diag

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Status is the severity of a single check result.
type Status int

const (
	OK Status = iota
	Info
	Warn
	Fail
)

// Icon returns a short ASCII marker for plain-text reports.
func (s Status) Icon() string {
	switch s {
	case OK:
		return "[ OK ]"
	case Info:
		return "[ i  ]"
	case Warn:
		return "[ !  ]"
	default:
		return "[FAIL]"
	}
}

// Result is the outcome of one diagnostic check.
type Result struct {
	ID     string
	Title  string
	Status Status
	Detail string
	Hint   string
}

// Exec runs a shell command on the router and returns its combined output.
type Exec func(cmd string) (string, error)

// ProgressFn is called after each finished check.
type ProgressFn func(Result)

// Conflicting packages known to fight with Tachyon over DNS/firewall/routing.
var conflictPackages = []string{
	"passwall", "luci-app-passwall", "openclash", "luci-app-openclash",
	"shadowsocksr", "luci-app-ssr-plus", "podkop", "forkop", "netshift",
	"nextdns", "https-dns-proxy",
}

const (
	minFreeRAMWarn  = 64.0
	minFreeRAMFail  = 32.0
	minFlashWarnMB  = 12.0
	minFlashFailMB  = 8.0
	maxLogDetailLen = 600
)

// RunRouter executes all router-side checks sequentially.
func RunRouter(run Exec, progress ProgressFn) []Result {
	var results []Result
	emit := func(r Result) {
		results = append(results, r)
		if progress != nil {
			progress(r)
		}
	}

	var pkgs string
	pkgsLoaded := false
	loadPkgs := func() string {
		if !pkgsLoaded {
			pkgsLoaded = true
			out, _ := run(`if command -v apk >/dev/null 2>&1; then apk info 2>/dev/null; else opkg list-installed 2>/dev/null; fi`)
			pkgs = out
		}
		return pkgs
	}

	emit(checkSystem(run))
	emit(checkMemory(run))
	emit(checkFlash(run))
	emit(checkTime(run))
	emit(checkFirewall(run))
	emit(checkModules(loadPkgs()))
	emit(checkConflicts(loadPkgs()))
	emit(checkDNS(run))
	emit(checkInternet(run))
	emit(checkGitHub(run))
	installed := checkTachyon(loadPkgs(), run)
	emit(installed)
	if installed.Status != Fail {
		emit(checkService(run))
		emit(checkEngines(run))
		emit(checkLogs(run))
	}
	return results
}

// RunHost checks what the installer PC needs: reaching the router and GitHub.
func RunHost(host string, port int, progress ProgressFn) []Result {
	var results []Result
	emit := func(r Result) {
		results = append(results, r)
		if progress != nil {
			progress(r)
		}
	}

	addr := net.JoinHostPort(host, strconv.Itoa(port))
	if conn, err := net.DialTimeout("tcp", addr, 4*time.Second); err != nil {
		emit(Result{ID: "host_ssh", Title: "Доступ к роутеру по SSH", Status: Fail,
			Detail: addr + ": " + err.Error(),
			Hint:   "Проверьте IP, порт и что SSH включён (System → Administration → SSH Access)."})
	} else {
		conn.Close()
		emit(Result{ID: "host_ssh", Title: "Доступ к роутеру по SSH", Status: OK, Detail: addr + " отвечает"})
	}

	if addrs, err := net.LookupHost("github.com"); err != nil {
		emit(Result{ID: "host_dns", Title: "DNS на компьютере (github.com)", Status: Warn,
			Detail: err.Error(), Hint: "Установщик попробует зеркала GitHub."})
	} else {
		emit(Result{ID: "host_dns", Title: "DNS на компьютере (github.com)", Status: OK, Detail: strings.Join(addrs[:min(2, len(addrs))], ", ")})
	}

	client := &http.Client{Timeout: 6 * time.Second}
	if resp, err := client.Head("https://github.com"); err != nil {
		emit(Result{ID: "host_github", Title: "GitHub напрямую с компьютера", Status: Warn,
			Detail: err.Error(), Hint: "Это нормально при блокировках: загрузка пойдёт через зеркала."})
	} else {
		resp.Body.Close()
		emit(Result{ID: "host_github", Title: "GitHub напрямую с компьютера", Status: OK, Detail: resp.Status})
	}
	return results
}

func checkSystem(run Exec) Result {
	out, err := run(`cat /tmp/sysinfo/model 2>/dev/null; . /etc/openwrt_release 2>/dev/null; echo "REL:$DISTRIB_RELEASE ARCH:$DISTRIB_ARCH"; echo "UNAME:$(uname -m)"`)
	r := Result{ID: "system", Title: "Система", Status: Info}
	if err != nil && strings.TrimSpace(out) == "" {
		r.Status = Fail
		r.Detail = "не удалось выполнить команды на роутере: " + err.Error()
		return r
	}
	var model, rel, arch, uname string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "REL:"):
			f := strings.Fields(line)
			for _, p := range f {
				if v, ok := strings.CutPrefix(p, "REL:"); ok {
					rel = v
				}
				if v, ok := strings.CutPrefix(p, "ARCH:"); ok {
					arch = v
				}
			}
		case strings.HasPrefix(line, "UNAME:"):
			uname = strings.TrimPrefix(line, "UNAME:")
		case line != "" && model == "":
			model = line
		}
	}
	r.Detail = fmt.Sprintf("%s, OpenWrt %s, %s (%s)", orDash(model), orDash(rel), orDash(arch), orDash(uname))
	return r
}

func checkMemory(run Exec) Result {
	r := Result{ID: "memory", Title: "Оперативная память"}
	out, _ := run(`awk '/MemTotal/{t=$2} /MemAvailable/{a=$2} /MemFree/{f=$2} END{if(a=="")a=f; print t, a}' /proc/meminfo`)
	f := strings.Fields(out)
	if len(f) < 2 {
		r.Status, r.Detail = Warn, "не удалось прочитать /proc/meminfo"
		return r
	}
	total, _ := strconv.ParseFloat(f[0], 64)
	avail, _ := strconv.ParseFloat(f[1], 64)
	totalMB, availMB := total/1024, avail/1024
	r.Detail = fmt.Sprintf("свободно %.0f из %.0f МБ", availMB, totalMB)
	switch {
	case availMB < minFreeRAMFail:
		r.Status = Fail
		r.Hint = "Критически мало ОЗУ: ядро прокси может падать. Выберите steer или освободите память."
	case availMB < minFreeRAMWarn:
		r.Status = Warn
		r.Hint = "Мало свободной ОЗУ. Рекомендуется steer вместо sing-box."
	default:
		r.Status = OK
	}
	return r
}

func checkFlash(run Exec) Result {
	r := Result{ID: "flash", Title: "Свободное место (Flash)"}
	out, _ := run(`df -k /overlay 2>/dev/null | awk 'END{print $4}'`)
	kb, _ := strconv.ParseFloat(strings.TrimSpace(out), 64)
	if kb == 0 {
		out, _ = run(`df -k / 2>/dev/null | awk 'END{print $4}'`)
		kb, _ = strconv.ParseFloat(strings.TrimSpace(out), 64)
	}
	if kb == 0 {
		r.Status, r.Detail = Warn, "не удалось определить свободное место"
		return r
	}
	mb := kb / 1024
	r.Detail = fmt.Sprintf("свободно %.1f МБ", mb)
	switch {
	case mb < minFlashFailMB:
		r.Status = Fail
		r.Hint = "Недостаточно места для установки. Освободите место или подключите USB ExtRoot."
	case mb < minFlashWarnMB:
		r.Status = Warn
		r.Hint = "Места мало: установка возможна, но без запаса."
	default:
		r.Status = OK
	}
	return r
}

func checkTime(run Exec) Result {
	r := Result{ID: "time", Title: "Системное время"}
	out, _ := run(`date +%Y`)
	year, _ := strconv.Atoi(strings.TrimSpace(out))
	r.Detail = "год " + strings.TrimSpace(out)
	if year < 2024 {
		r.Status = Warn
		r.Hint = "Время сбито: TLS-соединения и подписки могут не работать. Включите NTP (System → System)."
	} else {
		r.Status = OK
	}
	return r
}

func checkFirewall(run Exec) Result {
	r := Result{ID: "firewall", Title: "Межсетевой экран (fw4 / nftables)"}
	out, _ := run(`nft list tables 2>/dev/null; echo "---"; command -v fw3 >/dev/null 2>&1 && echo FW3`)
	switch {
	case strings.Contains(out, "inet fw4"):
		r.Status, r.Detail = OK, "таблица inet fw4 загружена"
	case strings.Contains(out, "FW3"):
		r.Status = Warn
		r.Detail = "найден устаревший fw3 (iptables)"
		r.Hint = "Tachyon рассчитан на fw4. Обновите OpenWrt до 22.03+."
	default:
		r.Status = Fail
		r.Detail = "таблица inet fw4 не найдена"
		r.Hint = "Выполните: /etc/init.d/firewall restart (из-за этого Tachyon не может добавить правила)."
	}
	return r
}

func checkModules(pkgs string) Result {
	r := Result{ID: "modules", Title: "Модули ядра (TPROXY, TUN)"}
	if strings.TrimSpace(pkgs) == "" {
		r.Status, r.Detail = Warn, "не удалось получить список пакетов"
		return r
	}
	var missing []string
	for _, p := range []string{"kmod-nft-tproxy", "kmod-tun"} {
		if !hasPackage(pkgs, p) {
			missing = append(missing, p)
		}
	}
	if len(missing) == 0 {
		r.Status, r.Detail = OK, "kmod-nft-tproxy, kmod-tun установлены"
		return r
	}
	r.Status = Warn
	r.Detail = "не установлены: " + strings.Join(missing, ", ")
	r.Hint = "Установщик Tachyon докачает их автоматически при проверке службы."
	return r
}

func checkConflicts(pkgs string) Result {
	r := Result{ID: "conflicts", Title: "Конфликтующие пакеты"}
	var found []string
	for _, p := range conflictPackages {
		if hasPackage(pkgs, p) {
			found = append(found, p)
		}
	}
	if len(found) == 0 {
		r.Status, r.Detail = OK, "не обнаружены"
		return r
	}
	r.Status = Warn
	r.Detail = strings.Join(found, ", ")
	r.Hint = "Эти пакеты меняют DNS и firewall так же, как Tachyon. Удалите или отключите их."
	return r
}

func checkDNS(run Exec) Result {
	r := Result{ID: "dns", Title: "DNS на роутере"}
	out, _ := run(`if timeout 3 nslookup openwrt.org 127.0.0.1 >/dev/null 2>&1; then echo LOCAL; elif timeout 3 nslookup openwrt.org >/dev/null 2>&1; then echo SYSTEM; else echo FAIL; fi`)
	switch strings.TrimSpace(out) {
	case "LOCAL":
		r.Status, r.Detail = OK, "локальный dnsmasq отвечает"
	case "SYSTEM":
		r.Status, r.Detail = OK, "разрешение имён работает"
	default:
		r.Status = Fail
		r.Detail = "имена не разрешаются"
		r.Hint = "Проверьте WAN и DNS-серверы. Без DNS Tachyon не сможет обновлять подписки."
	}
	return r
}

func checkInternet(run Exec) Result {
	r := Result{ID: "internet", Title: "Интернет на роутере"}
	out, _ := run(`ping -c 1 -W 2 1.1.1.1 >/dev/null 2>&1 && echo OK || echo FAIL`)
	if strings.Contains(out, "OK") {
		r.Status, r.Detail = OK, "1.1.1.1 доступен"
	} else {
		r.Status = Warn
		r.Detail = "1.1.1.1 недоступен"
		r.Hint = "Для установки не критично: файлы заливаются с компьютера. Но подписки и обновления потребуют интернета."
	}
	return r
}

func checkGitHub(run Exec) Result {
	r := Result{ID: "github", Title: "GitHub с роутера"}
	out, _ := run(`if command -v curl >/dev/null 2>&1; then curl -sI -m 5 https://github.com >/dev/null 2>&1 && echo OK || echo FAIL; else wget -q -T 5 -O /dev/null --spider https://github.com 2>/dev/null && echo OK || echo FAIL; fi`)
	if strings.Contains(out, "OK") {
		r.Status, r.Detail = OK, "github.com доступен"
	} else {
		r.Status = Info
		r.Detail = "github.com напрямую недоступен"
		r.Hint = "Это ожидаемо при блокировках. Именно поэтому установщик скачивает файлы на ПК."
	}
	return r
}

func checkTachyon(pkgs string, run Exec) Result {
	r := Result{ID: "tachyon", Title: "Tachyon на роутере"}
	var lines []string
	for _, l := range strings.Split(pkgs, "\n") {
		if strings.Contains(strings.ToLower(l), "tachyon") {
			lines = append(lines, strings.TrimSpace(l))
		}
	}
	out, _ := run(`[ -x /usr/bin/tachyon ] && echo BIN`)
	if len(lines) == 0 && !strings.Contains(out, "BIN") {
		r.Status = Fail
		r.Detail = "не установлен"
		r.Hint = "Запустите установку из главного меню."
		return r
	}
	r.Status = OK
	if len(lines) > 0 {
		r.Detail = strings.Join(lines, "; ")
	} else {
		r.Detail = "найден /usr/bin/tachyon"
	}
	return r
}

func checkService(run Exec) Result {
	r := Result{ID: "service", Title: "Служба Tachyon"}
	out, _ := run(`[ -x /usr/bin/tachyon ] && /usr/bin/tachyon get_status 2>/dev/null | grep -Eq '"running"[[:space:]]*:[[:space:]]*(1|true)' && echo RUNNING || echo STOPPED`)
	if strings.Contains(out, "RUNNING") {
		r.Status, r.Detail = OK, "запущена"
		return r
	}
	r.Status = Warn
	r.Detail = "не запущена"
	r.Hint = "Если подписка уже добавлена: /etc/init.d/tachyon restart и смотрите логи ниже."
	return r
}

func checkEngines(run Exec) Result {
	r := Result{ID: "engines", Title: "Ядра прокси"}
	out, _ := run(`for b in sing-box steer; do p=$(command -v $b 2>/dev/null) && echo "$b=$p"; done`)
	out = strings.TrimSpace(out)
	if out == "" {
		r.Status = Warn
		r.Detail = "sing-box и steer не найдены"
		r.Hint = "Установите ядро через мастер или в веб-интерфейсе Tachyon."
		return r
	}
	r.Status = OK
	r.Detail = strings.ReplaceAll(out, "\n", "; ")
	return r
}

func checkLogs(run Exec) Result {
	r := Result{ID: "logs", Title: "Последние записи в системном логе"}
	out, _ := run(`logread 2>/dev/null | grep -iE 'tachyon|sing-box|steer' | tail -n 8`)
	out = strings.TrimSpace(out)
	if out == "" {
		r.Status, r.Detail = Info, "записей нет"
		return r
	}
	r.Status = Info
	low := strings.ToLower(out)
	if strings.Contains(low, "fatal") || strings.Contains(low, "error") || strings.Contains(low, "failed") {
		r.Status = Warn
		r.Hint = "В логе есть ошибки. Полный лог: logread | grep -i tachyon"
	}
	if len(out) > maxLogDetailLen {
		out = out[len(out)-maxLogDetailLen:]
	}
	r.Detail = out
	return r
}

// Summary counts results by status.
func Summary(results []Result) (ok, warn, fail int) {
	for _, r := range results {
		switch r.Status {
		case OK, Info:
			ok++
		case Warn:
			warn++
		case Fail:
			fail++
		}
	}
	return
}

// Format renders results as a plain-text report suitable for saving or pasting into an issue.
func Format(title string, results []Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n%s\n\n", title, time.Now().Format("2006-01-02 15:04:05"))
	for _, r := range results {
		fmt.Fprintf(&b, "%s %s\n", r.Status.Icon(), r.Title)
		if r.Detail != "" {
			for _, l := range strings.Split(r.Detail, "\n") {
				fmt.Fprintf(&b, "         %s\n", l)
			}
		}
		if r.Hint != "" {
			fmt.Fprintf(&b, "         -> %s\n", r.Hint)
		}
	}
	ok, warn, fail := Summary(results)
	fmt.Fprintf(&b, "\nИтого: OK %d, предупреждений %d, ошибок %d\n", ok, warn, fail)
	return b.String()
}

// ExportToFile formats the results and writes them to the specified path.
func ExportToFile(title string, results []Result, targetPath string) (string, error) {
	if targetPath == "" {
		targetPath = fmt.Sprintf("tachyon-diag-report-%s.txt", time.Now().Format("20060102-150405"))
	}
	content := Format(title, results)
	if dir := filepath.Dir(targetPath); dir != "." && dir != "" {
		_ = os.MkdirAll(dir, 0755)
	}
	if err := os.WriteFile(targetPath, []byte(content), 0644); err != nil {
		return "", err
	}
	return targetPath, nil
}

func hasPackage(pkgs, name string) bool {
	for _, l := range strings.Split(pkgs, "\n") {
		l = strings.TrimSpace(l)
		if l == name || strings.HasPrefix(l, name+" ") || strings.HasPrefix(l, name+"-") && !strings.HasPrefix(l, name+"-i18n") && isVersionTail(l[len(name)+1:]) {
			return true
		}
	}
	return false
}

// isVersionTail reports whether s looks like the start of a package version (apk style: name-1.2.3-r1).
func isVersionTail(s string) bool {
	return s != "" && s[0] >= '0' && s[0] <= '9'
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}
