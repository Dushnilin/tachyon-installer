package discover

import (
	"context"
	"net"
	"testing"
	"time"
)

func listenBanner(t *testing.T, banner string) (string, int, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("cannot listen:", err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Write([]byte(banner))
			c.Close()
		}
	}()
	addr := ln.Addr().(*net.TCPAddr)
	return addr.IP.String(), addr.Port, func() { ln.Close() }
}

func TestProbeDetectsDropbear(t *testing.T) {
	ip, port, stop := listenBanner(t, "SSH-2.0-dropbear_2024.86\r\n")
	defer stop()
	h, ok := ProbeHost(ip, port, time.Second)
	if !ok || !h.OpenWrt || h.Banner != "SSH-2.0-dropbear_2024.86" {
		t.Fatalf("got %+v ok=%v", h, ok)
	}
}

func TestProbeOpenSSHIsNotOpenWrt(t *testing.T) {
	ip, port, stop := listenBanner(t, "SSH-2.0-OpenSSH_9.6\r\n")
	defer stop()
	h, ok := ProbeHost(ip, port, time.Second)
	if !ok || h.OpenWrt {
		t.Fatalf("got %+v ok=%v", h, ok)
	}
}

func TestProbeIgnoresNonSSHService(t *testing.T) {
	ip, port, stop := listenBanner(t, "HTTP/1.1 400 Bad Request\r\n")
	defer stop()
	if _, ok := ProbeHost(ip, port, time.Second); ok {
		t.Fatal("non-SSH service must be ignored")
	}
}

func TestProbeClosedPort(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	if _, ok := ProbeHost("127.0.0.1", port, 300*time.Millisecond); ok {
		t.Fatal("closed port reported as open")
	}
}

func TestHostsInCIDR(t *testing.T) {
	h := HostsInCIDR("192.168.1.0/24")
	if len(h) != 254 || h[0] != "192.168.1.1" || h[253] != "192.168.1.254" {
		t.Fatalf("len=%d first=%v", len(h), h[:1])
	}
	if HostsInCIDR("10.0.0.0/8") != nil {
		t.Fatal("huge networks must not be scanned")
	}
	if HostsInCIDR("garbage") != nil {
		t.Fatal("invalid cidr must give nil")
	}
}

func TestSortHostsOrdersOpenWrtGatewayFirst(t *testing.T) {
	sorted := sortHosts([]Host{
		{IP: "10.0.0.5"},
		{IP: "10.0.0.9", OpenWrt: true},
		{IP: "10.0.0.1", OpenWrt: true},
	}, "10.0.0.1")
	if sorted[0].IP != "10.0.0.1" || sorted[1].IP != "10.0.0.9" || sorted[2].IP != "10.0.0.5" {
		t.Fatalf("bad order: %+v", sorted)
	}
}

func TestScanListFindsListeningHost(t *testing.T) {
	ip, port, stop := listenBanner(t, "SSH-2.0-dropbear\r\n")
	defer stop()
	found := scanList(context.Background(), port, ip, []string{ip})
	if len(found) != 1 || !found[0].OpenWrt {
		t.Fatalf("got %+v", found)
	}
}
