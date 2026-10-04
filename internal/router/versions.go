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

	// Also check binary version for sing-box
	sessionSB, err := client.NewSession()
	if err == nil {
		outBytesSB, errSB := sessionSB.CombinedOutput("GOMEMLIMIT=15MiB /usr/bin/sing-box version 2>/dev/null | head -1 | awk '{print $3}'")
		if errSB == nil {
			sbVer := strings.TrimSpace(string(outBytesSB))
			if sbVer != "" {
				versions["sing-box-extended"] = sbVer
				versions["sing-box"] = sbVer
			}
		}
		sessionSB.Close()
	}

	// Also check steer
	sessionSteer, err := client.NewSession()
	if err == nil {
		outBytesSteer, errSteer := sessionSteer.CombinedOutput("/usr/bin/steer version 2>/dev/null || /usr/bin/steer -v 2>/dev/null || echo ''")
		if errSteer == nil {
			stVer := strings.TrimSpace(string(outBytesSteer))
			if stVer != "" {
				versions["steer"] = stVer
				versions["steer-extended"] = stVer
			}
		}
		sessionSteer.Close()
	}

	return versions
}
