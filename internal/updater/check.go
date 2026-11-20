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

// IsNewerVersion reports whether the remote semver tag is newer than the local one.
func IsNewerVersion(remote, local string) bool { return isNewerVersion(remote, local) }

// isNewerVersion compares two semver tags like "v1.3.1", "v2.0.0-beta.7", and "v1.3.0".
func isNewerVersion(remote, local string) bool {
	rParts, rPre := parseSemverWithPre(strings.TrimPrefix(strings.TrimSpace(remote), "v"))
	lParts, lPre := parseSemverWithPre(strings.TrimPrefix(strings.TrimSpace(local), "v"))

	for i := 0; i < 3; i++ {
		if rParts[i] > lParts[i] {
			return true
		}
		if rParts[i] < lParts[i] {
			return false
		}
	}

	// Same core major.minor.patch: stable > prerelease
	if rPre == "" && lPre != "" {
		return true
	}
	if rPre != "" && lPre == "" {
		return false
	}
	if rPre == lPre {
		return false
	}
	return comparePrerelease(rPre, lPre) > 0
}

func parseSemverWithPre(s string) ([3]int, string) {
	var parts [3]int
	core := s
	pre := ""
	if idx := strings.IndexByte(s, '-'); idx != -1 {
		core = s[:idx]
		pre = s[idx+1:]
	}
	chunks := strings.Split(core, ".")
	for i := 0; i < len(chunks) && i < 3; i++ {
		if val, err := strconv.Atoi(chunks[i]); err == nil {
			parts[i] = val
		}
	}
	return parts, pre
}

func comparePrerelease(a, b string) int {
	ap := strings.Split(a, ".")
	bp := strings.Split(b, ".")
	maxLen := len(ap)
	if len(bp) > maxLen {
		maxLen = len(bp)
	}
	for i := 0; i < maxLen; i++ {
		if i >= len(ap) {
			return -1
		}
		if i >= len(bp) {
			return 1
		}
		ai, aErr := strconv.Atoi(ap[i])
		bi, bErr := strconv.Atoi(bp[i])
		switch {
		case aErr == nil && bErr == nil:
			if ai != bi {
				if ai > bi {
					return 1
				}
				return -1
			}
		case aErr == nil && bErr != nil:
			return -1
		case aErr != nil && bErr == nil:
			return 1
		default:
			if ap[i] != bp[i] {
				if ap[i] > bp[i] {
					return 1
				}
				return -1
			}
		}
	}
	return 0
}
