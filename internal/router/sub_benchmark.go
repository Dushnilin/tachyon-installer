package router

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ResolveNodesFromInput resolves proxy nodes from a file path, an HTTP/HTTPS URL, or raw text/base64.
func ResolveNodesFromInput(ctx context.Context, rawInput string) ([]string, error) {
	rawInput = strings.TrimSpace(rawInput)
	if rawInput == "" {
		return nil, fmt.Errorf("пустой ввод подписки")
	}

	content := rawInput

	// 1. Check if rawInput is a local file
	if fi, err := os.Stat(rawInput); err == nil && !fi.IsDir() {
		data, err := os.ReadFile(rawInput)
		if err != nil {
			return nil, fmt.Errorf("ошибка чтения файла %s: %w", rawInput, err)
		}
		content = strings.TrimSpace(string(data))
	}

	// 2. Check if content is an HTTP/HTTPS URL
	if strings.HasPrefix(content, "http://") || strings.HasPrefix(content, "https://") {
		req, err := http.NewRequestWithContext(ctx, "GET", content, nil)
		if err != nil {
			return nil, fmt.Errorf("ошибка создания запроса к %s: %w", content, err)
		}
		req.Header.Set("User-Agent", "v2rayN/6.23")
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("ошибка загрузки подписки по ссылке: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("сервер подписки вернул статус HTTP %d", resp.StatusCode)
		}
		bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024))
		if err != nil {
			return nil, fmt.Errorf("ошибка чтения ответа подписки: %w", err)
		}
		content = strings.TrimSpace(string(bodyBytes))
	}

	// 3. Analyze content with AnalyzeSubscription
	analysis := AnalyzeSubscription(content)
	if !analysis.Valid || len(analysis.Nodes) == 0 {
		if analysis.ErrorMsg != "" {
			return nil, fmt.Errorf("%s", analysis.ErrorMsg)
		}
		return nil, fmt.Errorf("не удалось обнаружить валидные узлы в подписке")
	}

	return analysis.Nodes, nil
}


// NodeBenchmarkResult holds the latency test result for a proxy node.
type NodeBenchmarkResult struct {
	Name      string `json:"name"`
	Protocol  string `json:"protocol"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	LatencyMs int64  `json:"latency_ms"`
	Success   bool   `json:"success"`
	RawURI    string `json:"raw_uri"`
	Error     string `json:"error,omitempty"`
}

// ParseNodeURI extracts hostname, port, protocol, and node label from a proxy URI.
func ParseNodeURI(uriStr string) (name, protocol, host string, port int, err error) {
	u, err := url.Parse(uriStr)
	if err != nil {
		return "", "", "", 0, err
	}

	protocol = strings.ToUpper(u.Scheme)
	name = u.Fragment
	if name != "" {
		if unescaped, errUn := url.PathUnescape(name); errUn == nil {
			name = unescaped
		}
	}

	h := u.Hostname()
	pStr := u.Port()
	if pStr == "" {
		switch u.Scheme {
		case "https":
			pStr = "443"
		case "http":
			pStr = "80"
		default:
			pStr = "443"
		}
	}
	p, _ := strconv.Atoi(pStr)

	if name == "" {
		name = fmt.Sprintf("%s:%d", h, p)
	}

	return name, protocol, h, p, nil
}

// BenchmarkNodes concurrently tests TCP handshake latency to proxy nodes.
func BenchmarkNodes(ctx context.Context, nodeURIs []string, timeout time.Duration) []NodeBenchmarkResult {
	if timeout <= 0 {
		timeout = 2500 * time.Millisecond
	}

	var results []NodeBenchmarkResult
	var mu sync.Mutex
	var wg sync.WaitGroup

	sem := make(chan struct{}, 10) // Limit to 10 concurrent probes

	for _, uri := range nodeURIs {
		uri = strings.TrimSpace(uri)
		if uri == "" {
			continue
		}

		name, proto, host, port, err := ParseNodeURI(uri)
		if err != nil || host == "" || port <= 0 {
			continue
		}

		wg.Add(1)
		go func(n, pr, h string, pt int, raw string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			addr := net.JoinHostPort(h, strconv.Itoa(pt))
			start := time.Now()

			d := net.Dialer{Timeout: timeout}
			conn, dialErr := d.DialContext(ctx, "tcp", addr)
			duration := time.Since(start)

			res := NodeBenchmarkResult{
				Name:      n,
				Protocol:  pr,
				Host:      h,
				Port:      pt,
				RawURI:    raw,
				LatencyMs: duration.Milliseconds(),
			}

			if dialErr == nil {
				res.Success = true
				_ = conn.Close()
			} else {
				res.Success = false
				res.Error = dialErr.Error()
			}

			mu.Lock()
			results = append(results, res)
			mu.Unlock()
		}(name, proto, host, port, uri)
	}

	wg.Wait()

	// Sort: successful nodes first (lowest latency to highest), then failed nodes
	sort.Slice(results, func(i, j int) bool {
		if results[i].Success != results[j].Success {
			return results[i].Success
		}
		return results[i].LatencyMs < results[j].LatencyMs
	})

	return results
}
