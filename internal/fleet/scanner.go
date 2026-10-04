package fleet

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	gossh "golang.org/x/crypto/ssh"

	"tachyon-installer/internal/discover"
	routerpkg "tachyon-installer/internal/router"
	sshpkg "tachyon-installer/internal/ssh"
)

// ScanConfig specifies discovery parameters and credentials for fleet scanning.
type ScanConfig struct {
	Subnets       []string
	Port          int
	Username      string
	Passwords     []string
	KeyPath       string
	Gateway       string
	TargetVersion string
}

// DefaultScanConfig creates sensible defaults for local network probing.
func DefaultScanConfig(userPass string, targetVersion string) ScanConfig {
	passwords := []string{}
	if userPass != "" {
		passwords = append(passwords, userPass)
	}
	// Always try empty password as fallback (standard fresh OpenWrt)
	passwords = append(passwords, "")

	return ScanConfig{
		Port:          22,
		Username:      "root",
		Passwords:     passwords,
		TargetVersion: targetVersion,
	}
}

// ScanFleet discovers and probes all OpenWrt routers across target subnets.
func ScanFleet(ctx context.Context, cfg ScanConfig, progressCb func(done, total int, currentIP string)) ([]*FleetNode, error) {
	if cfg.Port <= 0 {
		cfg.Port = 22
	}
	if cfg.Username == "" {
		cfg.Username = "root"
	}

	// 1. Gather all candidate IP addresses to scan
	var candidateIPs []string
	seen := map[string]bool{}

	addIP := func(ip string) {
		ip = strings.TrimSpace(ip)
		if ip != "" && !seen[ip] {
			seen[ip] = true
			candidateIPs = append(candidateIPs, ip)
		}
	}

	if cfg.Gateway != "" {
		addIP(cfg.Gateway)
	}

	// Add custom subnets if specified
	for _, cidr := range cfg.Subnets {
		for _, ip := range discover.HostsInCIDR(cidr) {
			addIP(ip)
		}
	}

	// If no custom subnets, use auto-discovery from local network
	if len(cfg.Subnets) == 0 {
		discoveredHosts := discover.Scan(ctx, cfg.Port, cfg.Gateway)
		for _, h := range discoveredHosts {
			addIP(h.IP)
		}
	}

	total := len(candidateIPs)
	if total == 0 {
		return nil, nil
	}

	// 2. Parallel probing of discovered hosts
	var (
		mu      sync.Mutex
		results []*FleetNode
		wg      sync.WaitGroup
		sem     = make(chan struct{}, 8) // Limit concurrent SSH handshakes
		doneCnt int
	)

	for _, ip := range candidateIPs {
		select {
		case <-ctx.Done():
			break
		case sem <- struct{}{}:
		}

		wg.Add(1)
		go func(targetIP string) {
			defer wg.Done()
			defer func() { <-sem }()

			node := probeNode(ctx, targetIP, cfg)

			mu.Lock()
			doneCnt++
			if node != nil {
				results = append(results, node)
			}
			if progressCb != nil {
				progressCb(doneCnt, total, targetIP)
			}
			mu.Unlock()
		}(ip)
	}

	wg.Wait()

	// 3. Sort nodes by priority: Outdated -> Clean -> UpToDate -> AuthFailed -> Others
	sort.SliceStable(results, func(i, j int) bool {
		rank := func(s RouterStatus) int {
			switch s {
			case StatusOutdated:
				return 0
			case StatusClean:
				return 1
			case StatusUpToDate:
				return 2
			case StatusLowResources:
				return 3
			case StatusAuthFailed:
				return 4
			default:
				return 5
			}
		}
		ri, rj := rank(results[i].Status), rank(results[j].Status)
		if ri != rj {
			return ri < rj
		}
		return results[i].IP < results[j].IP
	})

	return results, nil
}

// probeNode tests port 22 and attempts SSH authentication on a single host.
func probeNode(ctx context.Context, ip string, cfg ScanConfig) *FleetNode {
	// First check if port is open with quick timeout
	_, ok := discover.ProbeHost(ip, cfg.Port, 800*time.Millisecond)
	if !ok {
		return nil
	}

	node := &FleetNode{
		IP:       ip,
		Port:     cfg.Port,
		Username: cfg.Username,
	}

	// Attempt authentication using configured passwords and keys
	var (
		client       *gossh.Client
		resolvedPass string
		authErr      error
	)

	// Try passwords (sshpkg.Connect already incorporates loaded private keys)
	if cfg.KeyPath != "" {
		sshpkg.PrivateKeyPath = cfg.KeyPath
	}

	for _, pass := range cfg.Passwords {
		c, err := sshpkg.Connect(ip, cfg.Port, cfg.Username, pass)
		if err == nil {
			client = c
			resolvedPass = pass
			authErr = nil
			break
		}
		authErr = err
	}

	if client == nil {
		// Reachable host on port 22, but SSH authentication rejected
		node.Status = StatusAuthFailed
		node.StatusText = "Требуется пароль SSH"
		node.Model = "OpenWrt Router"
		if authErr != nil && strings.Contains(authErr.Error(), "handshake") {
			node.StatusText = "Ошибка SSH Handshake"
		}
		return node
	}
	defer client.Close()

	node.Password = resolvedPass

	// Collect complete hardware & software profile
	profile, err := routerpkg.RunPreConnectionCheck(client)
	if err != nil {
		node.Status = StatusAuthFailed
		node.StatusText = fmt.Sprintf("Ошибка профилирования: %v", err)
		return node
	}

	node.Model = profile.Model
	node.Version = profile.Version
	node.Arch = profile.Arch
	node.DistribArch = profile.DistribArch
	node.RAMTotalMB = profile.RAMTotal
	node.RAMFreeMB = profile.RAMFree
	node.FlashFreeMB = profile.FlashFree
	node.IsAPK = profile.IsAPK
	node.Firewall = profile.Firewall
	node.InstalledVersion = profile.InstalledTachyonVer
	node.ActiveEngine = profile.ActiveEngine

	// Compute recommendations and final status
	node.ComputeRecommendations(cfg.TargetVersion)

	// By default, select routers that are ready for upgrade or initial install
	if node.Status == StatusOutdated || node.Status == StatusClean {
		node.Selected = true
	}

	return node
}

// SplitSubnets parses comma-separated CIDR strings.
func SplitSubnets(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		// Validate CIDR format
		if _, _, err := net.ParseCIDR(part); err == nil {
			out = append(out, part)
		}
	}
	return out
}
