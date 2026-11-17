package router

import (
	"strings"

	gossh "golang.org/x/crypto/ssh"
)

// GetInstalledVersions queries the router for the installed versions of given packages.
// It returns a map of package name to its installed version string.
func GetInstalledVersions(client *gossh.Client, isAPK bool, packages []string) map[string]string {
	versions := make(map[string]string)
	if len(packages) == 0 {
		return versions
	}

	session, err := client.NewSession()
	if err != nil {
		return versions
	}
	defer session.Close()

	var cmd string
	if isAPK {
		cmd = "apk info -v " + strings.Join(packages, " ") + " 2>/dev/null || true"
	} else {
		cmd = "opkg status " + strings.Join(packages, " ") + " 2>/dev/null || true"
	}

	outBytes, err := session.CombinedOutput(cmd)
	if err == nil {
		out := string(outBytes)
		if isAPK {
			lines := strings.Split(out, "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				for _, pkg := range packages {
					if strings.HasPrefix(line, pkg+"-") {
						ver := strings.TrimPrefix(line, pkg+"-")
						versions[pkg] = ver
						break
					}
				}
			}
		} else {
			lines := strings.Split(out, "\n")
			var currentPkg string
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "Package: ") {
					currentPkg = strings.TrimSpace(strings.TrimPrefix(line, "Package:"))
				} else if strings.HasPrefix(line, "Version: ") && currentPkg != "" {
					ver := strings.TrimSpace(strings.TrimPrefix(line, "Version:"))
					versions[currentPkg] = ver
					currentPkg = ""
				}
			}
		}
	}

	// Check tachyon-core version file or binary
	sessionTC, errTC := client.NewSession()
	if errTC == nil {
		outBytesTC, errTCB := sessionTC.CombinedOutput("cat /etc/tachyon/tachyon-core-version 2>/dev/null | head -1 || /usr/bin/tachyon-core --version 2>/dev/null | head -1 | awk '{print $NF}' || /usr/bin/tachyon-core version 2>/dev/null | head -1 | awk '{print $3}'")
		if errTCB == nil {
			tcVer := strings.TrimSpace(string(outBytesTC))
			if tcVer != "" {
				versions["tachyon-core"] = tcVer
			}
		}
		sessionTC.Close()
	}

	return versions
}
