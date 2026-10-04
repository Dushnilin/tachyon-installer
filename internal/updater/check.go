package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ReleaseInfo holds the latest release metadata from GitHub.
type ReleaseInfo struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Name    string `json:"name"`
	Body    string `json:"body"`
}

// CheckForUpdate queries GitHub for the latest release of tachyon-installer.
// It never panics and returns hasUpdate=false on any network or parsing error.
func CheckForUpdate(ctx context.Context, currentVersion string) (hasUpdate bool, latestVersion string, releaseURL string, err error) {
	if currentVersion == "dev" || strings.TrimSpace(currentVersion) == "" {
		return false, "", "", nil
	}

	reqCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	url := "https://api.github.com/repos/Dushnilin/tachyon-installer/releases/latest"
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return false, "", "", err
	}
	req.Header.Set("User-Agent", "tachyon-installer/"+currentVersion)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, "", "", fmt.Errorf("github api returned status %d", resp.StatusCode)
	}

	var rel ReleaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return false, "", "", err
	}

	if isNewerVersion(rel.TagName, currentVersion) {
		return true, rel.TagName, rel.HTMLURL, nil
	}

	return false, rel.TagName, rel.HTMLURL, nil
}

// isNewerVersion compares two semver tags like "v1.3.1" and "v1.3.0".
func isNewerVersion(remote, local string) bool {
	remote = strings.TrimPrefix(strings.TrimSpace(remote), "v")
	local = strings.TrimPrefix(strings.TrimSpace(local), "v")

	rParts := parseSemver(remote)
	lParts := parseSemver(local)

	for i := 0; i < 3; i++ {
		if rParts[i] > lParts[i] {
			return true
		}
		if rParts[i] < lParts[i] {
			return false
		}
	}
	return false
}

func parseSemver(s string) [3]int {
	var parts [3]int
	chunks := strings.Split(s, ".")
	for i := 0; i < len(chunks) && i < 3; i++ {
		// Strip any trailing prerelease tags like "-rc1"
		numStr := strings.Split(chunks[i], "-")[0]
		if val, err := strconv.Atoi(numStr); err == nil {
			parts[i] = val
		}
	}
	return parts
}
