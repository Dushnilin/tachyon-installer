// Package discover finds OpenWrt routers with an open SSH port on the local network.
package discover

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Host is a reachable SSH endpoint.
type Host struct {
	IP      string
	Banner  string
	OpenWrt bool // Dropbear is what OpenWrt ships by default
}

// commonRouterIPs are factory addresses of popular routers and OpenWrt defaults.
var commonRouterIPs = []string{
	"192.168.1.1", "192.168.0.1", "192.168.8.1", "192.168.2.1", "192.168.31.1",
	"192.168.100.1", "10.0.0.1", "10.0.1.1", "172.16.0.1",
}

// ProbeHost checks one address and reads the SSH banner.
func ProbeHost(ip string, port int, timeout time.Duration) (Host, bool) {
	addr := net.JoinHostPort(ip, strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return Host{}, false
	}
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(timeout + 500*time.Millisecond))
	banner, _ := bufio.NewReader(conn).ReadString('\n')
	banner = strings.TrimSpace(banner)
	if banner != "" && !strings.HasPrefix(banner, "SSH-") {
		return Host{}, false // some other service on port 22
	}
	return Host{IP: ip, Banner: banner, OpenWrt: strings.Contains(strings.ToLower(banner), "dropbear")}, true
}

// HostsInCIDR lists usable host addresses of an IPv4 network (capped at a /22).
func HostsInCIDR(cidr string) []string {
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil || ip.To4() == nil {
		return nil
	}
	ones, bits := ipnet.Mask.Size()
	if bits-ones > 10 {
		return nil
	}
	var hosts []string
	start := ipnet.IP.To4()
	total := 1 << (bits - ones)
	for i := 1; i < total-1; i++ {
		n := uint32(start[0])<<24 | uint32(start[1])<<16 | uint32(start[2])<<8 | uint32(start[3])
		n += uint32(i)
		hosts = append(hosts, fmt.Sprintf("%d.%d.%d.%d", byte(n>>24), byte(n>>16), byte(n>>8), byte(n)))
	}
	return hosts
}

// localCIDRs returns the IPv4 networks of active non-loopback interfaces.
func localCIDRs() []string {
	var out []string
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok || ipnet.IP.IsLoopback() || ipnet.IP.To4() == nil {
			continue
		}
		ones, bits := ipnet.Mask.Size()
		if bits-ones > 8 { // scan at most a /24 around the host
			ones = bits - 8
		}
		network := ipnet.IP.To4().Mask(net.CIDRMask(ones, bits))
		out = append(out, fmt.Sprintf("%s/%d", network.String(), ones))
	}
	return out
}

// Scan probes the gateway, common router addresses and the local subnets.
// Results are ordered: gateway first, then other OpenWrt hosts, then the rest.
func Scan(ctx context.Context, port int, gateway string) []Host {
	seen := map[string]bool{}
	var candidates []string
	add := func(ip string) {
		if ip != "" && !seen[ip] {
			seen[ip] = true
			candidates = append(candidates, ip)
		}
	}
	add(gateway)
	for _, ip := range commonRouterIPs {
		add(ip)
	}
	for _, c := range localCIDRs() {
		for _, ip := range HostsInCIDR(c) {
			add(ip)
		}
	}
	return scanList(ctx, port, gateway, candidates)
}

func scanList(ctx context.Context, port int, gateway string, candidates []string) []Host {
	var (
		mu    sync.Mutex
		found []Host
		wg    sync.WaitGroup
		sem   = make(chan struct{}, 128)
	)
	for _, ip := range candidates {
		select {
		case <-ctx.Done():
			wg.Wait()
			return sortHosts(found, gateway)
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			defer func() { <-sem }()
			if h, ok := ProbeHost(ip, port, 600*time.Millisecond); ok {
				mu.Lock()
				found = append(found, h)
				mu.Unlock()
			}
		}(ip)
	}
	wg.Wait()
	return sortHosts(found, gateway)
}

func sortHosts(hosts []Host, gateway string) []Host {
	rank := func(h Host) int {
		switch {
		case h.IP == gateway && h.OpenWrt:
			return 0
		case h.OpenWrt:
			return 1
		case h.IP == gateway:
			return 2
		}
		return 3
	}
	sort.SliceStable(hosts, func(i, j int) bool {
		ri, rj := rank(hosts[i]), rank(hosts[j])
		if ri != rj {
			return ri < rj
		}
		return hosts[i].IP < hosts[j].IP
	})
	return hosts
}
