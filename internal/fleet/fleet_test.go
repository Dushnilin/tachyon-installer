package fleet

import (
	"testing"
)

func TestComputeRecommendations(t *testing.T) {
	// Low-end router
	lowNode := &FleetNode{
		IP:          "192.168.1.10",
		Model:       "TP-Link WR841N",
		RAMTotalMB:  32,
		RAMFreeMB:   8,
		FlashFreeMB: 4,
	}
	lowNode.ComputeRecommendations("1.6.2")
	if lowNode.RecommendedEngine != "steer" {
		t.Errorf("expected steer for 32MB router, got %s", lowNode.RecommendedEngine)
	}
	if !lowNode.RecommendedZRAM {
		t.Errorf("expected zram=true for 32MB router")
	}
	if lowNode.Status != StatusLowResources {
		t.Errorf("expected StatusLowResources, got %s", lowNode.Status)
	}

	// Mid-range router with outdated version
	midNode := &FleetNode{
		IP:               "192.168.1.1",
		Model:            "Xiaomi AX3000T",
		Arch:             "arm64",
		RAMTotalMB:       256,
		RAMFreeMB:        150,
		FlashFreeMB:      60,
		InstalledVersion: "1.4.8",
	}
	midNode.ComputeRecommendations("1.6.2")
	if midNode.Status != StatusOutdated {
		t.Errorf("expected StatusOutdated for v1.4.8 vs 1.6.2, got %s", midNode.Status)
	}
	if midNode.RecommendedEngine != "sing-box-lx" {
		t.Errorf("expected sing-box-lx, got %s", midNode.RecommendedEngine)
	}

	// Router already up-to-date
	upToDateNode := &FleetNode{
		IP:               "192.168.1.205",
		Model:            "Keenetic Giga",
		Arch:             "mips",
		RAMTotalMB:       512,
		RAMFreeMB:        300,
		FlashFreeMB:      100,
		InstalledVersion: "v1.6.2",
	}
	upToDateNode.ComputeRecommendations("1.6.2")
	if upToDateNode.Status != StatusUpToDate {
		t.Errorf("expected StatusUpToDate, got %s", upToDateNode.Status)
	}

	// Clean ready router
	cleanNode := &FleetNode{
		IP:          "192.168.8.1",
		Model:       "GL.iNet MT3000",
		Arch:        "arm64",
		RAMTotalMB:  512,
		RAMFreeMB:   380,
		FlashFreeMB: 120,
	}
	cleanNode.ComputeRecommendations("v1.6.2")
	if cleanNode.Status != StatusClean {
		t.Errorf("expected StatusClean, got %s", cleanNode.Status)
	}
}

func TestSplitSubnets(t *testing.T) {
	input := "192.168.1.0/24, 10.0.0.0/16 , invalid_cidr, 172.16.0.0/24"
	subnets := SplitSubnets(input)
	if len(subnets) != 3 {
		t.Fatalf("expected 3 valid subnets, got %d: %v", len(subnets), subnets)
	}
	if subnets[0] != "192.168.1.0/24" || subnets[1] != "10.0.0.0/16" || subnets[2] != "172.16.0.0/24" {
		t.Errorf("unexpected parsed subnets: %v", subnets)
	}
}

func TestDefaultScanConfig(t *testing.T) {
	cfg := DefaultScanConfig("secret123", "1.6.2")
	if cfg.Port != 22 {
		t.Errorf("expected port 22, got %d", cfg.Port)
	}
	if len(cfg.Passwords) < 2 || cfg.Passwords[0] != "secret123" || cfg.Passwords[1] != "" {
		t.Errorf("expected password sequence starting with [secret123, ''], got %v", cfg.Passwords)
	}
}
