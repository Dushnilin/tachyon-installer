package downloader

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Known reliable GitHub reverse-proxy mirrors.
var DefaultMirrors = []string{
	"https://gh-proxy.com/",
	"https://ghproxy.net/",
	"https://ghfast.top/",
	"https://gh.ddlc.top/",
	"https://gh-proxy.org/",
	"https://mirror.ghproxy.com/",
	"https://github.moeyy.xyz/",
}

// MirrorPingResult holds the latency and status of a pinged mirror.
type MirrorPingResult struct {
	Mirror  string
	Latency time.Duration
	Err     error
}

// MirrorManager manages mirror selection and URL rewriting for downloads.
type MirrorManager struct {
	selectedMirror string
	fastestMirror  string
	mirrors        []string
	client         *http.Client
	mu             sync.RWMutex
}

// NewMirrorManager creates a new mirror manager.
func NewMirrorManager(selected string) *MirrorManager {
	if selected == "" {
		selected = "auto"
	}
	return &MirrorManager{
		selectedMirror: selected,
		mirrors:        DefaultMirrors,
		client:         &http.Client{Timeout: 5 * time.Second},
	}
}

// SetSelectedMirror updates the active mirror setting ("auto", "direct", or a mirror URL).
func (m *MirrorManager) SetSelectedMirror(mirror string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.selectedMirror = mirror
}

// GetSelectedMirror returns current mirror preference.
func (m *MirrorManager) GetSelectedMirror() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.selectedMirror
}

// GetFastestMirror returns the tested fastest mirror.
func (m *MirrorManager) GetFastestMirror() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.fastestMirror
}

// SetFastestMirror updates the tested fastest mirror.
func (m *MirrorManager) SetFastestMirror(mirror string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fastestMirror = mirror
}

// WrapURL rewrites a direct GitHub URL using a specific mirror prefix.
func WrapURL(mirrorPrefix, rawURL string) string {
	if mirrorPrefix == "" || mirrorPrefix == "direct" {
		return rawURL
	}
	// Avoid double-mirroring
	for _, m := range DefaultMirrors {
		if strings.HasPrefix(rawURL, m) {
			rawURL = strings.TrimPrefix(rawURL, m)
			break
		}
	}
	if !strings.HasSuffix(mirrorPrefix, "/") {
		mirrorPrefix += "/"
	}
	return mirrorPrefix + rawURL
}

// GetCandidateURLs returns an ordered list of URLs to try for a given GitHub URL.
func (m *MirrorManager) GetCandidateURLs(rawURL string) []string {
	m.mu.RLock()
	selected := m.selectedMirror
	fastest := m.fastestMirror
	m.mu.RUnlock()

	var candidates []string
	cleanRaw := rawURL
	for _, mir := range m.mirrors {
		if strings.HasPrefix(cleanRaw, mir) {
			cleanRaw = strings.TrimPrefix(cleanRaw, mir)
			break
		}
	}

	if selected == "direct" {
		candidates = append(candidates, cleanRaw)
		for _, mir := range m.mirrors {
			candidates = append(candidates, WrapURL(mir, cleanRaw))
		}
		return candidates
	}

	if selected != "" && selected != "auto" {
		// Specific mirror requested first
		candidates = append(candidates, WrapURL(selected, cleanRaw))
		for _, mir := range m.mirrors {
			if mir != selected {
				candidates = append(candidates, WrapURL(mir, cleanRaw))
			}
		}
		candidates = append(candidates, cleanRaw)
		return candidates
	}

	// Auto mode: if fastestMirror was measured, put it first!
	isAPI := strings.Contains(cleanRaw, "api.github.com")

	if fastest != "" && fastest != "direct" {
		candidates = append(candidates, WrapURL(fastest, cleanRaw))
	}

	for _, mir := range m.mirrors {
		if mir == fastest {
			continue
		}
		// Some mirrors explicitly reject api.github.com (e.g. 403)
		if isAPI && (mir == "https://ghproxy.net/" || mir == "https://mirror.ghproxy.com/") {
			continue
		}
		candidates = append(candidates, WrapURL(mir, cleanRaw))
	}

	// Add direct GitHub
	candidates = append(candidates, cleanRaw)

	return candidates
}

// TestFastestMirror pings direct GitHub and all mirrors concurrently with a 3s timeout
// and returns the best working mirror URL, or "direct".
func (m *MirrorManager) TestFastestMirror(ctx context.Context) string {
	return m.TestFastestMirrorDetailed(ctx, nil)
}

// TestFastestMirrorDetailed pings direct GitHub and mirrors, notifying onResult for each.
func (m *MirrorManager) TestFastestMirrorDetailed(ctx context.Context, onResult func(MirrorPingResult)) string {
	testTarget := "https://raw.githubusercontent.com/Dushnilin/tachyon/main/README.md"

	// Test candidates: direct ("direct") + each mirror
	candidates := []string{"direct"}
	candidates = append(candidates, m.mirrors...)

	resChan := make(chan MirrorPingResult, len(candidates))
	var wg sync.WaitGroup

	client := &http.Client{
		Timeout: 3500 * time.Millisecond,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return nil
		},
	}

	for _, cand := range candidates {
		wg.Add(1)
		go func(c string) {
			defer wg.Done()
			var target string
			if c == "direct" {
				target = testTarget
			} else {
				target = WrapURL(c, testTarget)
			}

			start := time.Now()
			req, err := http.NewRequestWithContext(ctx, "HEAD", target, nil)
			if err != nil {
				res := MirrorPingResult{Mirror: c, Err: err}
				resChan <- res
				if onResult != nil {
					onResult(res)
				}
				return
			}
			req.Header.Set("User-Agent", "TachyonInstaller/1.0")

			resp, err := client.Do(req)
			if err != nil {
				// Retry with GET if HEAD not supported by mirror
				req, _ = http.NewRequestWithContext(ctx, "GET", target, nil)
				req.Header.Set("User-Agent", "TachyonInstaller/1.0")
				req.Header.Set("Range", "bytes=0-10")
				resp, err = client.Do(req)
			}
			if err != nil {
				res := MirrorPingResult{Mirror: c, Err: err}
				resChan <- res
				if onResult != nil {
					onResult(res)
				}
				return
			}
			resp.Body.Close()

			if resp.StatusCode >= 200 && resp.StatusCode < 400 {
				res := MirrorPingResult{Mirror: c, Latency: time.Since(start)}
				resChan <- res
				if onResult != nil {
					onResult(res)
				}
			} else {
				res := MirrorPingResult{Mirror: c, Err: fmt.Errorf("HTTP %d", resp.StatusCode)}
				resChan <- res
				if onResult != nil {
					onResult(res)
				}
			}
		}(cand)
	}

	wg.Wait()
	close(resChan)

	var bestMirror string
	var bestLatency time.Duration = time.Hour

	for res := range resChan {
		if res.Err == nil && res.Latency < bestLatency {
			bestLatency = res.Latency
			bestMirror = res.Mirror
		}
	}

	if bestMirror == "" {
		bestMirror = "https://gh-proxy.com/"
	}

	m.mu.Lock()
	m.fastestMirror = bestMirror
	m.mu.Unlock()

	return bestMirror
}
