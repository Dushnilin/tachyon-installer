package downloader

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// Client handles robust HTTP downloads using mirrors and fallback strategies.
type Client struct {
	mirrorMgr *MirrorManager
	httpCli   *http.Client
}

// NewClient creates a new downloader client.
func NewClient(mirrorMgr *MirrorManager) *Client {
	if mirrorMgr == nil {
		mirrorMgr = NewMirrorManager("auto")
	}
	return &Client{
		mirrorMgr: mirrorMgr,
		httpCli:   &http.Client{
			// No global timeout so large packages (30+ MB) on slow links don't get abruptly cut off.
			// Instead per-request deadlines or idle connection timeouts apply.
		},
	}
}

// FetchJSON fetches JSON data from a URL trying mirrors in order.
func (c *Client) FetchJSON(ctx context.Context, rawURL string, target any) (string, error) {
	candidates := c.mirrorMgr.GetCandidateURLs(rawURL)

	var lastErr error
	for _, candURL := range candidates {
		req, err := http.NewRequestWithContext(ctx, "GET", candURL, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "TachyonInstaller/1.0")

		client := &http.Client{Timeout: 15 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			lastErr = fmt.Errorf("HTTP %s from %s", resp.Status, candURL)
			continue
		}

		bodyBytes, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}

		if err := json.Unmarshal(bodyBytes, target); err != nil {
			lastErr = fmt.Errorf("json parse error from %s: %w", candURL, err)
			continue
		}

		// Success!
		return candURL, nil
	}

	return "", fmt.Errorf("all mirrors failed for %s (last error: %v)", rawURL, lastErr)
}

// DownloadFile downloads a file from rawURL to destPath using mirrors, reporting progress.
func (c *Client) DownloadFile(ctx context.Context, rawURL string, destPath string, label string, onProgress ProgressCallback) (string, error) {
	candidates := c.mirrorMgr.GetCandidateURLs(rawURL)

	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return "", fmt.Errorf("create destination dir: %w", err)
	}

	var lastErr error
	for _, candURL := range candidates {
		req, err := http.NewRequestWithContext(ctx, "GET", candURL, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("User-Agent", "TachyonInstaller/1.0")

		resp, err := c.httpCli.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			lastErr = fmt.Errorf("HTTP %s from %s", resp.Status, candURL)
			continue
		}

		tmpFile := destPath + ".part"
		out, err := os.Create(tmpFile)
		if err != nil {
			resp.Body.Close()
			return "", fmt.Errorf("create local file %s: %w", tmpFile, err)
		}

		pr := NewProgressReader(resp.Body, resp.ContentLength, onProgress)
		_, copyErr := io.Copy(out, pr)
		out.Close()
		resp.Body.Close()

		if copyErr != nil {
			os.Remove(tmpFile)
			lastErr = fmt.Errorf("download stream error from %s: %w", candURL, copyErr)
			continue
		}

		// Ensure file has content
		st, err := os.Stat(tmpFile)
		if err != nil || st.Size() == 0 {
			os.Remove(tmpFile)
			lastErr = fmt.Errorf("empty file received from %s", candURL)
			continue
		}

		// Rename part file to final file
		_ = os.Remove(destPath) // remove previous if exists
		if err := os.Rename(tmpFile, destPath); err != nil {
			return "", fmt.Errorf("rename temp file to %s: %w", destPath, err)
		}

		return candURL, nil
	}

	return "", fmt.Errorf("failed to download %s: %v", label, lastErr)
}

// ScrapeExpandedAssets parses asset links from GitHub's expanded_assets HTML page as a fallback
// when GitHub API has rate limiting or is inaccessible.
func (c *Client) ScrapeExpandedAssets(ctx context.Context, repo string, tagName string) (map[string]string, error) {
	rawURL := fmt.Sprintf("https://github.com/%s/releases/expanded_assets/%s", repo, tagName)
	candidates := c.mirrorMgr.GetCandidateURLs(rawURL)

	var htmlContent string
	var lastErr error

	client := &http.Client{Timeout: 15 * time.Second}
	for _, candURL := range candidates {
		req, err := http.NewRequestWithContext(ctx, "GET", candURL, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", "TachyonInstaller/1.0")

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			lastErr = fmt.Errorf("HTTP %d from %s", resp.StatusCode, candURL)
			continue
		}

		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err == nil && len(data) > 0 {
			htmlContent = string(data)
			break
		}
	}

	if htmlContent == "" {
		return nil, fmt.Errorf("failed to fetch release assets HTML: %v", lastErr)
	}

	// Regex to extract /owner/repo/releases/download/... links
	re := regexp.MustCompile(`href="(/` + regexp.QuoteMeta(repo) + `/releases/download/[^"]+)"`)
	matches := re.FindAllStringSubmatch(htmlContent, -1)

	assets := make(map[string]string)
	for _, m := range matches {
		if len(m) > 1 {
			relPath := m[1]
			filename := filepath.Base(relPath)
			assets[filename] = "https://github.com" + relPath
		}
	}

	return assets, nil
}
