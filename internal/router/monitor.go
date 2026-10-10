package router

import (
	"fmt"
	"strconv"
	"strings"

	gossh "golang.org/x/crypto/ssh"
	sshutil "tachyon-installer/internal/ssh"
)

// LiveStats contains parsed live performance metrics from OpenWrt.
type LiveStats struct {
	Load1  float64
	Load5  float64
	Load15 float64

	RAMTotalMB float64
	RAMFreeMB  float64
	RAMUsedMB  float64

	UptimeSec float64

	EngineName string
	EnginePID  string
	EngineVSZ  string

	RXBytes int64
	TXBytes int64

	Connections int
}

// UptimeString returns a human-formatted uptime string (e.g. "3d 4h 12m").
func (s LiveStats) UptimeString() string {
	total := int(s.UptimeSec)
	days := total / 86400
	hours := (total % 86400) / 3600
	mins := (total % 3600) / 60

	if days > 0 {
		return fmt.Sprintf("%dд %dч %dм", days, hours, mins)
	}
	if hours > 0 {
		return fmt.Sprintf("%dч %dм", hours, mins)
	}
	return fmt.Sprintf("%dм", mins)
}

// CollectLiveStats gathers real-time CPU, RAM, proxy process, and network metrics from the router.
func CollectLiveStats(client *gossh.Client, execFn sshutil.ExecFunc) (*LiveStats, error) {
	cmd := `
echo "=== MONITOR ==="
cat /proc/loadavg 2>/dev/null
echo "--- MEM ---"
grep -E 'MemTotal|MemFree|MemAvailable' /proc/meminfo 2>/dev/null
echo "--- UPTIME ---"
cat /proc/uptime 2>/dev/null
echo "--- DEV ---"
cat /proc/net/dev 2>/dev/null
echo "--- PROC ---"
ps 2>/dev/null | grep -E 'tachyon-core|sing-box|steer' | grep -v grep | head -n 1
echo "--- CONN ---"
cat /proc/sys/net/netfilter/nf_conntrack_count 2>/dev/null || echo 0
`
	out, err := execFn(client, cmd)
	if err != nil {
		return nil, fmt.Errorf("query live stats: %w", err)
	}

	return ParseLiveStats(out), nil
}

// ParseLiveStats parses the composite monitor output into a LiveStats struct.
func ParseLiveStats(out string) *LiveStats {
	stats := &LiveStats{}

	sections := strings.Split(out, "--- ")
	for _, sec := range sections {
		lines := strings.Split(strings.TrimSpace(sec), "\n")
		if len(lines) == 0 {
			continue
		}
		header := strings.TrimSpace(lines[0])
		content := lines[1:]

		switch {
		case strings.HasPrefix(sec, "=== MONITOR ==="):
			parts := strings.Fields(sec)
			for i, p := range parts {
				if p == "===" && i+3 < len(parts) && parts[i+1] == "MONITOR" && parts[i+2] == "===" {
					loadFields := parts[i+3:]
					if len(loadFields) >= 3 {
						stats.Load1, _ = strconv.ParseFloat(loadFields[0], 64)
						stats.Load5, _ = strconv.ParseFloat(loadFields[1], 64)
						stats.Load15, _ = strconv.ParseFloat(loadFields[2], 64)
					}
					break
				}
			}

		case strings.HasPrefix(header, "MEM"):
			var total, free, avail float64
			for _, l := range content {
				fields := strings.Fields(l)
				if len(fields) >= 2 {
					val, _ := strconv.ParseFloat(fields[1], 64)
					if strings.HasPrefix(fields[0], "MemTotal") {
						total = val / 1024.0
					} else if strings.HasPrefix(fields[0], "MemFree") {
						free = val / 1024.0
					} else if strings.HasPrefix(fields[0], "MemAvailable") {
						avail = val / 1024.0
					}
				}
			}
			stats.RAMTotalMB = total
			if avail > 0 {
				stats.RAMFreeMB = avail
			} else {
				stats.RAMFreeMB = free
			}
			stats.RAMUsedMB = stats.RAMTotalMB - stats.RAMFreeMB

		case strings.HasPrefix(header, "UPTIME"):
			if len(content) > 0 {
				fields := strings.Fields(content[0])
				if len(fields) > 0 {
					stats.UptimeSec, _ = strconv.ParseFloat(fields[0], 64)
				}
			}

		case strings.HasPrefix(header, "DEV"):
			for _, l := range content {
				if strings.Contains(l, ":") {
					parts := strings.Split(l, ":")
					if len(parts) >= 2 {
						ifName := strings.TrimSpace(parts[0])
						if ifName == "br-lan" || ifName == "eth0" || strings.HasPrefix(ifName, "lan") {
							fields := strings.Fields(parts[1])
							if len(fields) >= 9 {
								rx, _ := strconv.ParseInt(fields[0], 10, 64)
								tx, _ := strconv.ParseInt(fields[8], 10, 64)
								stats.RXBytes += rx
								stats.TXBytes += tx
							}
						}
					}
				}
			}

		case strings.HasPrefix(header, "PROC"):
			if len(content) > 0 && strings.TrimSpace(content[0]) != "" {
				fields := strings.Fields(content[0])
				if len(fields) >= 5 {
					stats.EnginePID = fields[0]
					stats.EngineVSZ = fields[2]
					for _, f := range fields {
						if strings.Contains(f, "tachyon-core") {
							stats.EngineName = "tachyon-core"
							break
						} else if strings.Contains(f, "sing-box") {
							stats.EngineName = "sing-box"
							break
						} else if strings.Contains(f, "steer") {
							stats.EngineName = "steer"
							break
						}
					}
				}
			}

		case strings.HasPrefix(header, "CONN"):
			if len(content) > 0 {
				c, _ := strconv.Atoi(strings.TrimSpace(content[0]))
				stats.Connections = c
			}
		}
	}

	return stats
}
