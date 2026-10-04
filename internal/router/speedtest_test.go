package router

import (
	"strings"
	"testing"
)

func TestCalculateGrade(t *testing.T) {
	tests := []struct {
		delta float64
		want  BufferbloatGrade
	}{
		{2.1, GradeAPlus},
		{5.0, GradeAPlus},
		{11.4, GradeA},
		{15.0, GradeA},
		{25.8, GradeB},
		{30.0, GradeB},
		{48.2, GradeC},
		{60.0, GradeC},
		{95.0, GradeD},
		{120.0, GradeD},
		{121.0, GradeF},
		{450.0, GradeF},
	}

	for _, tt := range tests {
		got := CalculateGrade(tt.delta)
		if got != tt.want {
			t.Errorf("CalculateGrade(%.1f) = %v, want %v", tt.delta, got, tt.want)
		}
	}
}

func TestParsePingMetrics(t *testing.T) {
	lines := []string{
		"PING 1.1.1.1 (1.1.1.1): 56 data bytes",
		"64 bytes from 1.1.1.1: seq=0 ttl=57 time=14.2 ms",
		"64 bytes from 1.1.1.1: seq=1 ttl=57 time=16.8 ms",
		"64 bytes from 1.1.1.1: seq=2 ttl=57 time=15.1 ms",
		"64 bytes from 1.1.1.1: seq=3 ttl=57 time=14.9 ms",
		"",
		"--- 1.1.1.1 ping statistics ---",
		"4 packets transmitted, 4 packets received, 0% packet loss",
		"round-trip min/avg/max = 14.2/15.2/16.8 ms",
	}

	avg, min, max, jitter := parsePingMetrics(lines)
	if avg < 15.0 || avg > 15.5 {
		t.Errorf("expected avg ~15.2, got %.2f", avg)
	}
	if min != 14.2 {
		t.Errorf("expected min 14.2, got %.2f", min)
	}
	if max != 16.8 {
		t.Errorf("expected max 16.8, got %.2f", max)
	}
	if jitter <= 0 {
		t.Errorf("expected positive jitter, got %.2f", jitter)
	}
}

func TestParseCPUStatLineAndSample(t *testing.T) {
	line1 := "cpu  1000 50 800 50000 200 10 300 0 0 0"
	line2 := "cpu  1500 50 1200 50500 210 10 800 0 0 0"

	s1 := parseCPUStatLine(line1)
	s2 := parseCPUStatLine(line2)

	if len(s1) < 7 || len(s2) < 7 {
		t.Fatalf("failed to parse CPU stat lines")
	}

	sample := computeCPUSample(s1, s2)
	if sample.TotalPct <= 0 || sample.TotalPct > 100 {
		t.Errorf("invalid total CPU %%: %.2f", sample.TotalPct)
	}
	if sample.SoftIRQPct <= 0 {
		t.Errorf("expected softirq > 0, got %.2f", sample.SoftIRQPct)
	}
}

func TestParseSpeedDoctorOutput(t *testing.T) {
	sampleOutput := `
=== IDLE_PING ===
PING 1.1.1.1 (1.1.1.1): 56 data bytes
64 bytes from 1.1.1.1: seq=0 ttl=57 time=10.0 ms
64 bytes from 1.1.1.1: seq=1 ttl=57 time=12.0 ms
round-trip min/avg/max = 10.0/11.0/12.0 ms
=== CPU_START ===
cpu  1000 0 500 20000 100 0 100 0 0 0
=== HTTP_TOOL ===
curl
=== CPU_END ===
cpu  1800 0 1200 20200 110 0 800 0 0 0
=== LOADED_PING ===
64 bytes from 1.1.1.1: seq=0 ttl=57 time=25.0 ms
64 bytes from 1.1.1.1: seq=1 ttl=57 time=29.0 ms
round-trip min/avg/max = 25.0/27.0/29.0 ms
=== DL_RESULT ===
12500000 25000000 2.0
DURATION:2
`

	run := ParseSpeedDoctorOutput(sampleOutput)
	if run == nil {
		t.Fatalf("expected non-nil BenchmarkRun")
	}

	if run.IdlePingAvg < 10.0 || run.IdlePingAvg > 12.0 {
		t.Errorf("expected IdlePingAvg ~11.0, got %.2f", run.IdlePingAvg)
	}
	if run.LoadedPingAvg < 26.0 || run.LoadedPingAvg > 28.0 {
		t.Errorf("expected LoadedPingAvg ~27.0, got %.2f", run.LoadedPingAvg)
	}
	// Delta = 27 - 11 = 16ms -> Grade B (15.0 < delta <= 30.0)
	if run.BufferbloatDelta < 15.0 || run.BufferbloatDelta > 17.0 {
		t.Errorf("expected Delta ~16.0, got %.2f", run.BufferbloatDelta)
	}
	if run.Grade != GradeB {
		t.Errorf("expected Grade B, got %v", run.Grade)
	}

	// Throughput = 12500000 * 8 / 1e6 = 100 Mbps
	if run.SpeedMbps < 95.0 || run.SpeedMbps > 105.0 {
		t.Errorf("expected speed ~100 Mbps, got %.2f", run.SpeedMbps)
	}

	if !run.CPU.Throttled {
		// Softirq is high in sample: (800-100)=700 out of total ~1700 => ~41%
	}
}

func TestBuildRecommendations(t *testing.T) {
	rep := &SpeedDoctorReport{
		Direct: &BenchmarkRun{
			SpeedMbps:        150.0,
			BufferbloatDelta: 180.0,
			Grade:            GradeF,
			CPU: CPUSample{
				TotalPct:   70.0,
				SoftIRQPct: 20.0,
			},
		},
		Tunnel: &BenchmarkRun{
			SpeedMbps:        20.0,
			BufferbloatDelta: 25.0,
			Grade:            GradeB,
			CPU: CPUSample{
				TotalPct:   98.0,
				SoftIRQPct: 55.0,
				Throttled:  true,
			},
		},
		HasTunnel:       true,
		Recommendations: make([]string, 0),
	}

	buildRecommendations(rep)

	if len(rep.Recommendations) == 0 {
		t.Fatalf("expected recommendations for Grade F and throttled tunnel")
	}

	allRecs := strings.Join(rep.Recommendations, " ")
	if !strings.Contains(allRecs, "Bufferbloat") {
		t.Errorf("missing Bufferbloat recommendation")
	}
	if !strings.Contains(allRecs, "steer") {
		t.Errorf("missing steer recommendation for throttled CPU")
	}
}
