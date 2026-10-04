package router

import (
	"strings"
	"testing"
)

func TestParseLiveStats(t *testing.T) {
	raw := `=== MONITOR ===
0.25 0.15 0.08 1/115 1234
--- MEM ---
MemTotal:         255952 kB
MemFree:           54320 kB
MemAvailable:      89200 kB
--- UPTIME ---
182400.50 150000.00
--- DEV ---
Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo: 1000       10    0    0    0     0          0         0     1000       10    0    0    0     0       0          0
br-lan: 10485760   5000    0    0    0     0          0         0  5242880    4000    0    0    0     0       0          0
--- PROC ---
 1420 root      35420 S    /usr/bin/sing-box run -c /etc/tachyon/config.json
--- CONN ---
145
`

	stats := ParseLiveStats(raw)
	if stats.Load1 != 0.25 || stats.Load5 != 0.15 || stats.Load15 != 0.08 {
		t.Errorf("unexpected loads: %f %f %f", stats.Load1, stats.Load5, stats.Load15)
	}
	if stats.RAMTotalMB < 240 || stats.RAMTotalMB > 260 {
		t.Errorf("unexpected RAMTotalMB: %f", stats.RAMTotalMB)
	}
	if stats.RAMFreeMB < 80 || stats.RAMFreeMB > 95 {
		t.Errorf("unexpected RAMFreeMB: %f", stats.RAMFreeMB)
	}
	if stats.UptimeSec != 182400.50 {
		t.Errorf("unexpected UptimeSec: %f", stats.UptimeSec)
	}
	uptimeStr := stats.UptimeString()
	if !strings.Contains(uptimeStr, "2д") && !strings.Contains(uptimeStr, "2d") {
		t.Errorf("unexpected UptimeString: %s", uptimeStr)
	}
	if stats.EngineName != "sing-box" || stats.EnginePID != "1420" {
		t.Errorf("unexpected engine stats: name=%s pid=%s", stats.EngineName, stats.EnginePID)
	}
	if stats.RXBytes != 10485760 || stats.TXBytes != 5242880 {
		t.Errorf("unexpected RX/TX: rx=%d tx=%d", stats.RXBytes, stats.TXBytes)
	}
	if stats.Connections != 145 {
		t.Errorf("unexpected Connections: %d", stats.Connections)
	}
}
