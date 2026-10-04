package router

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"
)

func TestParseNodeURI(t *testing.T) {
	vlessURI := "vless://12345678-1234-1234-1234-123456789abc@de-fra.example.com:443?security=reality#%F0%9F%87%A9%F0%9F%87%AA%20Frankfurt%2001"
	name, proto, host, port, err := ParseNodeURI(vlessURI)
	if err != nil {
		t.Fatalf("ParseNodeURI failed: %v", err)
	}

	if proto != "VLESS" {
		t.Errorf("expected protocol VLESS, got %s", proto)
	}
	if host != "de-fra.example.com" {
		t.Errorf("expected host de-fra.example.com, got %s", host)
	}
	if port != 443 {
		t.Errorf("expected port 443, got %d", port)
	}
	if name != "🇩🇪 Frankfurt 01" {
		t.Errorf("expected unescaped name '🇩🇪 Frankfurt 01', got %s", name)
	}
}

func TestBenchmarkNodes(t *testing.T) {
	// Start a local mock TCP listener
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer l.Close()

	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	mockPort := l.Addr().(*net.TCPAddr).Port
	nodes := []string{
		fmt.Sprintf("vless://uuid@127.0.0.1:%d#LocalMock", mockPort),
		"vless://uuid@192.0.2.1:12345#UnreachableNode", // TEST-NET-1 (non-routable)
	}

	results := BenchmarkNodes(context.Background(), nodes, 500*time.Millisecond)
	if len(results) == 0 {
		t.Fatalf("expected results")
	}

	hasSuccess := false
	for _, r := range results {
		if r.Success && r.Host == "127.0.0.1" {
			hasSuccess = true
			break
		}
	}
	if !hasSuccess {
		t.Errorf("expected local listener to succeed")
	}
}

func TestResolveNodesFromInput(t *testing.T) {
	// 1. Direct vless link
	input := "vless://uuid@example.com:443#DirectNode"
	nodes, err := ResolveNodesFromInput(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}

	// 2. Base64
	b64Input := "dmxlc3M6Ly91dWlkQGV4YW1wbGUuY29tOjQ0MyNEaXJlY3ROb2Rl" // base64 of vless://uuid@example.com:443#DirectNode
	nodesB64, err := ResolveNodesFromInput(context.Background(), b64Input)
	if err != nil {
		t.Fatalf("unexpected error for base64: %v", err)
	}
	if len(nodesB64) != 1 {
		t.Fatalf("expected 1 node from base64, got %d", len(nodesB64))
	}
}

