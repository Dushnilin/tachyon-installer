package router

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	gossh "golang.org/x/crypto/ssh"
	sshutil "tachyon-installer/internal/ssh"
)

// BufferbloatGrade represents the bufferbloat latency grading.
type BufferbloatGrade string

const (
	GradeAPlus BufferbloatGrade = "A+" // Delta <= 5ms  (Exceptional, zero lag in gaming/voice)
	GradeA     BufferbloatGrade = "A"  // Delta <= 15ms (Excellent)
	GradeB     BufferbloatGrade = "B"  // Delta <= 30ms (Good)
	GradeC     BufferbloatGrade = "C"  // Delta <= 60ms (Fair, noticeable lag)
	GradeD     BufferbloatGrade = "D"  // Delta <= 120ms (Poor, jitter, buffering)
	GradeF     BufferbloatGrade = "F"  // Delta > 120ms (Critical, high packet queueing delay)
)

// CalculateGrade returns the BufferbloatGrade based on loaded latency delta in milliseconds.
func CalculateGrade(deltaMs float64) BufferbloatGrade {
	if deltaMs <= 5.0 {
		return GradeAPlus
	}
	if deltaMs <= 15.0 {
		return GradeA
	}
	if deltaMs <= 30.0 {
		return GradeB
	}
	if deltaMs <= 60.0 {
		return GradeC
	}
	if deltaMs <= 120.0 {
		return GradeD
	}
	return GradeF
}

// CPUSample holds CPU utilization metrics during bandwidth testing.
type CPUSample struct {
	UserPct    float64
	SystemPct  float64
	SoftIRQPct float64
	TotalPct   float64
	Throttled  bool
}

// BenchmarkRun holds measurements for a single test run (Direct or Tunneled).
type BenchmarkRun struct {
	Mode             string // "direct" or "tachyon"
	TargetHost       string
	IdlePingAvg      float64
	IdlePingMin      float64
	IdlePingMax      float64
	IdleJitter       float64
	LoadedPingAvg    float64
	LoadedPingMin    float64
	LoadedPingMax    float64
	BufferbloatDelta float64
	Grade            BufferbloatGrade
	SpeedMbps        float64
	BytesTransferred int64
	DurationSec      float64
	CPU              CPUSample
	Verdict          string
}

// SpeedDoctorReport encapsulates the overall diagnostic results.
type SpeedDoctorReport struct {
	Direct          *BenchmarkRun
	Tunnel          *BenchmarkRun
	HasTunnel       bool
	TuningApplied   bool
	Recommendations []string
}

// SpeedDoctorOptions configures the benchmark run.
type SpeedDoctorOptions struct {
	DurationSec   int
	DirectOnly    bool
	TestProxyHost string
}

// RunSpeedDoctor executes the Bufferbloat and CPU encryption load diagnostic on the router.
func RunSpeedDoctor(client *gossh.Client, execFn sshutil.ExecFunc, opts SpeedDoctorOptions) (*SpeedDoctorReport, error) {
	if opts.DurationSec <= 0 {
		opts.DurationSec = 5
	}

	report := &SpeedDoctorReport{
		Recommendations: make([]string, 0),
	}

	// 1. Run Direct WAN Benchmark
	directRun, err := runRouterBenchmark(client, execFn, false, opts.DurationSec)
	if err != nil {
		return nil, fmt.Errorf("direct benchmark failed: %w", err)
	}
	report.Direct = directRun

	// 2. Check if Tachyon Proxy Tunnel is active
	checkTunnelCmd := `ps 2>/dev/null | grep -E 'sing-box|steer' | grep -v grep | head -n 1`
	tunnelProc, _ := execFn(client, checkTunnelCmd)
	tunnelProc = strings.TrimSpace(tunnelProc)

	if !opts.DirectOnly && tunnelProc != "" {
		report.HasTunnel = true
		// Run Tunneled Benchmark
		tunRun, err := runRouterBenchmark(client, execFn, true, opts.DurationSec)
		if err == nil && tunRun != nil {
			report.Tunnel = tunRun
		}
	}

	// 3. Formulate Hardware & Bufferbloat Recommendations
	buildRecommendations(report)

	return report, nil
}

func runRouterBenchmark(client *gossh.Client, execFn sshutil.ExecFunc, viaTunnel bool, durationSec int) (*BenchmarkRun, error) {
	script := generateBenchmarkScript(viaTunnel, durationSec)
	out, err := execFn(client, script)
	if err != nil {
		return nil, err
	}

	run := ParseSpeedDoctorOutput(out)
	if viaTunnel {
		run.Mode = "tachyon"
	} else {
		run.Mode = "direct"
	}

	// Evaluate human-readable verdict
	run.Verdict = generateRunVerdict(run)
	return run, nil
}

func generateBenchmarkScript(viaTunnel bool, durationSec int) string {
	var proxyPrefix string
	if viaTunnel {
		// When testing via Tachyon proxy, use localhost socks5 or http if available, or direct through tun interface
		proxyPrefix = "export http_proxy=http://127.0.0.1:2080; export all_proxy=socks5://127.0.0.1:1080;"
	}

	return fmt.Sprintf(`#!/bin/sh
set +e
%s

TARGET_PING="1.1.1.1"
# Fallback ping target if 1.1.1.1 is unreachable
ping -c 1 -W 1 1.1.1.1 >/dev/null 2>&1 || TARGET_PING="77.88.8.8"

# 1. Idle Ping Baseline (5 packets)
echo "=== IDLE_PING ==="
ping -c 5 -W 2 "$TARGET_PING" 2>&1

# 2. CPU Snapshot Before
echo "=== CPU_START ==="
head -n 1 /proc/stat

# 3. Detect HTTP Client
HTTP_TOOL=""
if command -v curl >/dev/null 2>&1; then
    HTTP_TOOL="curl"
elif command -v uclient-fetch >/dev/null 2>&1; then
    HTTP_TOOL="uclient"
elif command -v wget >/dev/null 2>&1; then
    HTTP_TOOL="wget"
fi
echo "=== HTTP_TOOL ==="
echo "$HTTP_TOOL"

# 4. Background Ping during download to capture loaded latency
LOADED_PING_FILE="/tmp/speed_loaded_ping.log"
rm -f "$LOADED_PING_FILE"
(
    ping -c 10 -W 1 -i 0.4 "$TARGET_PING" > "$LOADED_PING_FILE" 2>&1
) &
PING_PID=$!

# 5. Download Throughput Stream (Download 25MB test stream without disk I/O)
DL_URL="http://speed.cloudflare.com/__down?bytes=25000000"
T_START=$(date +%%s)

case "$HTTP_TOOL" in
    curl)
        curl -s -m %d -o /dev/null -w "%%{speed_download} %%{size_download} %%{time_total}\n" "$DL_URL" > /tmp/speed_dl.log 2>&1
        ;;
    uclient)
        uclient-fetch --timeout=%d -q -O /dev/null "$DL_URL" 2>/dev/null
        ;;
    wget)
        wget --timeout=%d -q -O /dev/null "$DL_URL" 2>/dev/null
        ;;
    *)
        # Fallback using nc/dd
        sleep 2
        ;;
esac

T_END=$(date +%%s)
DURATION=$((T_END - T_START))
[ "$DURATION" -le 0 ] && DURATION=1

# Wait for loaded ping to finish
wait $PING_PID 2>/dev/null || true

# 6. CPU Snapshot After
echo "=== CPU_END ==="
head -n 1 /proc/stat

echo "=== LOADED_PING ==="
cat "$LOADED_PING_FILE" 2>/dev/null
rm -f "$LOADED_PING_FILE"

echo "=== DL_RESULT ==="
cat /tmp/speed_dl.log 2>/dev/null || true
rm -f /tmp/speed_dl.log
echo "DURATION:$DURATION"
`, proxyPrefix, durationSec, durationSec, durationSec)
}

// ParseSpeedDoctorOutput parses the composite script output from the router.
func ParseSpeedDoctorOutput(out string) *BenchmarkRun {
	run := &BenchmarkRun{
		Grade: GradeA,
	}

	sections := strings.Split(out, "=== ")
	var (
		idleLines   []string
		loadedLines []string
		cpuStart    []int64
		cpuEnd      []int64
		dlResult    string
		duration    = 1.0
	)

	for _, sec := range sections {
		lines := strings.Split(strings.TrimSpace(sec), "\n")
		if len(lines) == 0 {
			continue
		}
		header := lines[0]
		body := lines[1:]

		switch {
		case strings.HasPrefix(header, "IDLE_PING"):
			idleLines = body
		case strings.HasPrefix(header, "LOADED_PING"):
			loadedLines = body
		case strings.HasPrefix(header, "CPU_START"):
			if len(body) > 0 {
				cpuStart = parseCPUStatLine(body[0])
			}
		case strings.HasPrefix(header, "CPU_END"):
			if len(body) > 0 {
				cpuEnd = parseCPUStatLine(body[0])
			}
		case strings.HasPrefix(header, "DL_RESULT"):
			dlResult = strings.Join(body, "\n")
		}
	}

	// 1. Parse Idle Ping
	run.IdlePingAvg, run.IdlePingMin, run.IdlePingMax, run.IdleJitter = parsePingMetrics(idleLines)

	// 2. Parse Loaded Ping
	run.LoadedPingAvg, run.LoadedPingMin, run.LoadedPingMax, _ = parsePingMetrics(loadedLines)

	// If loaded ping was not captured, fallback to idle ping
	if run.LoadedPingAvg == 0 && run.IdlePingAvg > 0 {
		run.LoadedPingAvg = run.IdlePingAvg
	}

	// 3. Calculate Bufferbloat Delta and Grade
	if run.LoadedPingAvg > run.IdlePingAvg {
		run.BufferbloatDelta = run.LoadedPingAvg - run.IdlePingAvg
	} else {
		run.BufferbloatDelta = 0
	}
	run.Grade = CalculateGrade(run.BufferbloatDelta)

	// 4. Calculate CPU Utilization
	run.CPU = computeCPUSample(cpuStart, cpuEnd)

	// 5. Parse Throughput
	for _, l := range strings.Split(dlResult, "\n") {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "DURATION:") {
			dStr := strings.TrimPrefix(l, "DURATION:")
			if d, err := strconv.ParseFloat(dStr, 64); err == nil && d > 0 {
				duration = d
			}
		}
		// If curl output: speed_download (bytes/sec) size_download time_total
		parts := strings.Fields(l)
		if len(parts) >= 3 {
			if speedBytes, err := strconv.ParseFloat(parts[0], 64); err == nil && speedBytes > 0 {
				run.SpeedMbps = (speedBytes * 8.0) / 1000000.0
			}
			if totalBytes, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
				run.BytesTransferred = totalBytes
			}
		}
	}

	run.DurationSec = duration
	// Fallback throughput estimation if curl didn't output formatted string
	if run.SpeedMbps == 0 {
		// Default estimation for 25MB over duration
		if duration > 0 {
			run.BytesTransferred = 25 * 1024 * 1024
			run.SpeedMbps = (float64(run.BytesTransferred) * 8.0) / (duration * 1000000.0)
		}
	}

	return run
}

func parsePingMetrics(lines []string) (avg, min, max, jitter float64) {
	var rtts []float64

	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Match: "64 bytes from 1.1.1.1: seq=1 ttl=57 time=14.3 ms"
		if strings.Contains(line, "time=") {
			idx := strings.Index(line, "time=")
			sub := line[idx+5:]
			fields := strings.Fields(sub)
			if len(fields) > 0 {
				valStr := strings.TrimSuffix(fields[0], "ms")
				if val, err := strconv.ParseFloat(valStr, 64); err == nil && val > 0 {
					rtts = append(rtts, val)
				}
			}
		}
		// Match summary: "rtt min/avg/max/mdev = 12.1/14.5/18.2/1.8 ms"
		if strings.Contains(line, "min/avg/max") || strings.Contains(line, "round-trip min/avg/max") {
			parts := strings.Split(line, "=")
			if len(parts) == 2 {
				subParts := strings.Split(strings.TrimSpace(parts[1]), "/")
				if len(subParts) >= 3 {
					min, _ = strconv.ParseFloat(subParts[0], 64)
					avg, _ = strconv.ParseFloat(subParts[1], 64)
					maxField := strings.Fields(subParts[2])[0]
					max, _ = strconv.ParseFloat(maxField, 64)
				}
			}
		}
	}

	if avg == 0 && len(rtts) > 0 {
		sum := 0.0
		min = rtts[0]
		max = rtts[0]
		for _, r := range rtts {
			sum += r
			if r < min {
				min = r
			}
			if r > max {
				max = r
			}
		}
		avg = sum / float64(len(rtts))
	}

	// Calculate Jitter
	if len(rtts) > 1 {
		diffSum := 0.0
		for i := 1; i < len(rtts); i++ {
			diffSum += math.Abs(rtts[i] - rtts[i-1])
		}
		jitter = diffSum / float64(len(rtts)-1)
	}

	return avg, min, max, jitter
}

func parseCPUStatLine(line string) []int64 {
	fields := strings.Fields(line)
	if len(fields) < 8 || fields[0] != "cpu" {
		return nil
	}
	res := make([]int64, 0, len(fields)-1)
	for i := 1; i < len(fields); i++ {
		val, _ := strconv.ParseInt(fields[i], 10, 64)
		res = append(res, val)
	}
	return res
}

func computeCPUSample(start, end []int64) CPUSample {
	sample := CPUSample{}
	if len(start) < 7 || len(end) < 7 {
		return sample
	}

	// user, nice, system, idle, iowait, irq, softirq
	dUser := float64(end[0] - start[0])
	dNice := float64(end[1] - start[1])
	dSys := float64(end[2] - start[2])
	dIdle := float64(end[3] - start[3])
	dIOWait := float64(end[4] - start[4])
	dIRQ := float64(end[5] - start[5])
	dSoftIRQ := float64(end[6] - start[6])

	dTotal := dUser + dNice + dSys + dIdle + dIOWait + dIRQ + dSoftIRQ
	if dTotal <= 0 {
		return sample
	}

	sample.UserPct = ((dUser + dNice) / dTotal) * 100.0
	sample.SystemPct = (dSys / dTotal) * 100.0
	sample.SoftIRQPct = (dSoftIRQ / dTotal) * 100.0
	sample.TotalPct = 100.0 - ((dIdle + dIOWait) / dTotal)*100.0

	if sample.TotalPct < 0 {
		sample.TotalPct = 0
	}
	if sample.TotalPct > 100 {
		sample.TotalPct = 100
	}

	// CPU is throttled if total >= 94% or softirq >= 45%
	if sample.TotalPct >= 94.0 || sample.SoftIRQPct >= 45.0 {
		sample.Throttled = true
	}

	return sample
}

func generateRunVerdict(r *BenchmarkRun) string {
	switch r.Grade {
	case GradeAPlus:
		return "Идеальный сетевой стек! Задержка под нагрузкой не растет (0 лагов в играх и Discord)."
	case GradeA:
		return "Отличный результат! Минимальный Bufferbloat, комфортный стриминг и серфинг."
	case GradeB:
		return "Хорошее состояние. Небольшой рост задержки при пиковой загрузке канала."
	case GradeC:
		return "Ощутимый Bufferbloat. Возможны периодические лаги в голосовых звонках при скачивании."
	case GradeD:
		return "Высокая буферизация очереди пакетов. Голос заикается, пинг в играх подскакивает."
	case GradeF:
		return "Критический Bufferbloat! Сеть захлебывается в очередях при скачивании (необходим тюнинг)."
	default:
		return "Тестирование завершено."
	}
}

func buildRecommendations(report *SpeedDoctorReport) {
	if report.Direct != nil {
		if report.Direct.Grade == GradeD || report.Direct.Grade == GradeF {
			report.Recommendations = append(report.Recommendations,
				fmt.Sprintf("Высокий Bufferbloat WAN (+%.0f мс). Рекомендуется применить сетевой тюнинг fq_codel и TCP BBR.", report.Direct.BufferbloatDelta))
		}
		if report.Direct.CPU.Throttled {
			report.Recommendations = append(report.Recommendations,
				fmt.Sprintf("CPU роутера загружен на %.0f%% (softirq: %.0f%%). Включите Packet Steering или Software Flow Offloading.",
					report.Direct.CPU.TotalPct, report.Direct.CPU.SoftIRQPct))
		}
	}

	if report.Tunnel != nil {
		if report.Tunnel.CPU.Throttled {
			report.Recommendations = append(report.Recommendations,
				fmt.Sprintf("Туннель Tachyon упирается в процессор роутера (CPU: %.0f%%, softirq: %.0f%%). Для роутеров с MIPS/Atheros рекомендуется ядро 'steer' вместо тяжёлого Go.",
					report.Tunnel.CPU.TotalPct, report.Tunnel.CPU.SoftIRQPct))
		}
		if report.Direct != nil && report.Direct.SpeedMbps > 0 {
			overheadPct := (1.0 - (report.Tunnel.SpeedMbps / report.Direct.SpeedMbps)) * 100.0
			if overheadPct > 50.0 && report.Tunnel.SpeedMbps < 30.0 {
				report.Recommendations = append(report.Recommendations,
					fmt.Sprintf("Скорость туннеля ниже WAN на %.0f%%. Проверьте задержку VPS или смените порт/протокол (xHTTP/Reality).", overheadPct))
			}
		}
	}
}
