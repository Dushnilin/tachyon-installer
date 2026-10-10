<div align="center">

![Tachyon Installer Banner](assets/readme/hero.svg)

[![Go](https://img.shields.io/badge/Go-1.22%20%7C%201.23%20%7C%201.25-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://go.dev/)
[![Releases](https://img.shields.io/github/v/release/Dushnilin/tachyon-installer?style=for-the-badge&color=818CF8)](https://github.com/Dushnilin/tachyon-installer/releases)
[![Platforms](https://img.shields.io/badge/Platforms-Windows%20%7C%20Linux%20%7C%20macOS-38BDF8?style=for-the-badge)](https://github.com/Dushnilin/tachyon-installer/releases)
[![OpenWrt](https://img.shields.io/badge/OpenWrt-23.05%20%7C%2024.10%20%7C%2025.x%20%7C%20SNAPSHOT-10B981?style=for-the-badge&logo=openwrt)](https://openwrt.org/)
[![Telegram](https://img.shields.io/badge/Telegram-Channel-26A5E4?style=for-the-badge&logo=telegram&logoColor=white)](https://t.me/tachyon_proxy)
[![License](https://img.shields.io/badge/License-GPL--3.0-C084FC?style=for-the-badge)](LICENSE)

[**🇷🇺 Русский**](README.md) | [**🇬🇧 English**](README.en.md)

</div>

<p align="center">
  <img src="assets/readme/divider_stream.svg" width="100%" alt="divider" />
</p>

## 🔍 Key Highlights (v1.6)

- **⚡ Instant 1-Line Bootstrap**: Launch directly in console without manual downloading via PowerShell (`irm ... | iex`) or Linux/macOS (`curl | bash`).
- **🚀 Kernel & Network Optimization (BBR & Sysctl Tuning)**: Fine-tunes OpenWrt networking stack (TCP BBR + fq_codel queue, adaptive socket buffers based on router RAM, TCP Fast Open, faster conntrack cleanup, and Firewall Flow Offloading) persisted in `/etc/sysctl.d/99-tachyon-tune.conf` (`-tune-network` flag in CLI or `T` hotkey in TUI).
- **🔄 In-Place Self-Update**: Automatically checks, downloads newer releases, verifies SHA256 integrity, and atomically swaps the running installer executable in 1 command (`-self-update` flag or `U` hotkey in TUI).
- **🚑 Emergency Network Rescue**: Safely flushes deadlocked nftables/iptables interception rules, clears TProxy routes, restarts dnsmasq/firewall, and restores native internet access instantly (`-rescue` flag in CLI or `🚑 Rescue` button / `R` hotkey in TUI).
- **🛡️ Conflict Auto-Fix**: Automatically detects, stops, and disables conflicting proxy/DNS tools (`passwall`, `openclash`, `zapret`, `xray`, `shadowsocksr`, etc.) via `-fix-conflicts` flag or `C` hotkey in TUI.
- **📊 Real-Time Live Monitor**: Interactive telemetry dashboard displaying CPU loadavg, RAM usage (with visual bar), system uptime, active proxy core (PID, memory), open conntrack connections, and LAN RX/TX traffic live (`-monitor` flag in CLI or `M` hotkey in TUI).
- **💾 Full System Snapshot**: Backs up entire router network configurations (`/etc/config/network`, `/etc/config/dhcp`, `/etc/config/firewall`, `/etc/nftables.d/`, `/etc/config/tachyon`) into a local `.tar.gz` archive on host PC (`-snapshot` flag or `B` hotkey in TUI).
- **⚡ Subscription Benchmark & Latency Ping**: Concurrently measures TCP handshake latency to all nodes in a subscription (HTTPS URL, raw `vless://`, file, or base64), sorted from lowest to highest ping (`-test-sub "vless://..."`).
- **📦 Offline Bundle Downloader**: Pre-downloads Tachyon releases (OPKG + APK, i18n, and cores for x86_64, arm64, mipsle, mips) into a local bundle directory for air-gapped / offline deployments (`-offline-bundle "offline_pkg/"`).
- **🌐 Multi-Target Bypass Probes**: Verifies router Fake-IP DNS interception and tests response latency across YouTube, Discord, Telegram, and GitHub directly through the router pipeline, reporting egress GeoIP country and IP.
- **⚡ Engine Hot-Swap**: Swap routing engines (`tachyon-core`, `sing-box-extended`, `sing-box-tiny`, `steer`, `steer-extended`) in ~3 seconds without reinstalling LuCI or clearing subscriptions — via `--switch-engine` or hotkey `S` in TUI.
- **🌐 1-Click LuCI Launch**: Completion modal and global hotkey `O` instantly launch the Tachyon LuCI web panel in your default system browser.
- **🖥️ Dual Mode (Interactive TUI + Headless CLI)**: Beautiful zinc terminal dashboard with mouse & arrow keys support, plus automated unattended CLI execution (`--yes`, `--ip`, `--pass`, `--engine`, `--zram`).
- **🧠 Hardware-Aware Intelligence**: Real-time evaluation of router RAM/Flash with smart core recommendations; optional automatic activation of **zRAM-swap** for OOM prevention on constrained routers (<128 MB RAM).
- **⏱️ Router Clock Sync & WAN Health**: Automatically synchronizes router clock with host PC UTC time (`date -u -s`) to prevent TLS/certificate validation failures; tests WAN routing and DNS.
- **🌐 Universal Subscription Parser**: Supports HTTPS subscription URLs, raw configs (`vless://`, `hysteria2://`, `trojan://`, `ss://`, `vmess://`), and base64 bundles with live node counting and protocol breakdown.
- **🛠️ Maintenance Suite**: Clean uninstaller (`--uninstall`), instant rollback from local backup archives (`--restore latest`), backup listing (`--list-backups`), and release update checker (`--check-update`).
- **🔍 Deep Diagnostics**: Automatic post-install audit and standalone mode (dashboard button, `D` hotkey, or `--diag` flag) with export to file (`--export-diag`).
- **LAN Router Discovery**: Automatically probes the local network for Dropbear OpenWrt routers.
- **SSH Key Authentication**: Full support for passwordless or passphrase-protected SSH keys.
- **Checksum Verification**: Validates SHA256 integrity and file formats before streaming to the router.

## ⚡ About The Project

**Tachyon Express Installer** is a standalone, cross-platform desktop utility and interactive Terminal User Interface (TUI) wizard designed for autonomous deployment and pre-flight hardware diagnostics of the **[Tachyon](https://github.com/Dushnilin/tachyon)** ecosystem on **OpenWrt** routers (compatible with 23.05, 24.10, 25.x, and SNAPSHOT).

### 🎯 The Problem It Solves
In restrictive censorship environments, OpenWrt routers frequently **lack direct access to GitHub**, or WAN connectivity is unstable prior to configuring anti-censorship routing. Standard router-side installation scripts (`curl | sh`) often hang or fail under these conditions.

**Tachyon Installer fundamentally eliminates this barrier:**
1. The installer runs locally **on your workstation** (Windows, Linux, macOS).
2. It fetches all necessary packages, LuCI web interfaces, and proxy core binaries through fast, verified GitHub mirrors.
3. Automatically validates integrity using **SHA256 checksums**.
4. Profiles router hardware over the local SSH network, backs up existing configurations, and **streams payloads directly into RAM (`/tmp`)**, ensuring zero NAND flash wear and completely autonomous installation.

<p align="center">
  <img src="assets/readme/divider_stream.svg" width="100%" alt="divider" />
</p>

## 🔄 Deployment Architecture & Pipeline

<div align="center">

![Tachyon Installer Pipeline](assets/readme/architecture.svg)

</div>

The installation executes across 5 autonomous stages:

1. **🔍 Pre-Flight Router Audit**:
   - Hardware detection: Router model, OpenWrt version, CPU architecture normalization (`arm64`, `mipsle`, `mips`, `x86_64`, `armv7`).
   - Package manager detection (`apk` for OpenWrt 25+ / `opkg` for OpenWrt 24 and older).
   - Flash and RAM memory inspection with safety warnings on low-memory routers.
   - Firewall generation check (`fw4` nftables vs legacy `fw3` iptables).
   - Conflicting package detection (`passwall`, `openclash`, `shadowsocksr`, `podkop`, `forkop`, `nextdns`).
2. **🪞 Intelligent Mirror Download (Host-side)**:
   - Automated parallel latency benchmarking across multiple verified GitHub mirrors.
   - Fast failover recovery if an upstream mirror becomes unreachable.
   - Strict SHA256 integrity verification against official release manifests.
3. **💾 Safe Configuration Backup**:
   - Dumps `/etc/config/tachyon` and stores a local backup on the host PC prior to making changes.
4. **📦 tar-over-SSH Streaming**:
   - Streams payload directly into router RAM `tmpfs` (`/tmp/tachyon-install`) via secure SSH pipe.
   - Zero unnecessary flash read/write cycles, preserving NAND storage lifespan.
5. **⚡ Service Activation & Health Verification**:
   - Atomic package installation and placement of the selected routing engine.
   - Validates systemd/procd service status, nftables rules, FakeIP DNS, and leak prevention.
   - Optional 1-click subscription URL testing and configuration.
6. **✨ Intelligent Autopilot Setup Wizard**:
   - Automated DNS poisoning detection and fast DoH upstream benchmarking.
   - Built-in **DPI Fuzzer** testing YouTube & Discord to detect the winning evasion strategy.
   - 1-click TCP BBR network tuning and end-to-end scorecard verification.

<p align="center">
  <img src="assets/readme/divider_stream.svg" width="100%" alt="divider" />
</p>

## 🚀 Quick Start

### ⚡ Instant 1-Line Bootstrap (No Manual Download)

Similar to Microsoft Activation Scripts (MAS), you can launch Tachyon Express Installer with a single command directly inside your terminal. It auto-detects OS and architecture (x64 / arm64 / Apple Silicon), downloads the binary via the nearest reliable mirror, and runs it right in your console:

**Windows (PowerShell):**
```powershell
irm https://raw.githubusercontent.com/Dushnilin/tachyon-installer/main/install.ps1 | iex
```
*(Fallback mirror if GitHub is blocked:* `irm https://gh-proxy.com/https://raw.githubusercontent.com/Dushnilin/tachyon-installer/main/install.ps1 | iex`*)*

**Linux & macOS (Terminal):**
```bash
curl -fsSL https://raw.githubusercontent.com/Dushnilin/tachyon-installer/main/install.sh | bash
```
*(Fallback mirror:* `curl -fsSL https://gh-proxy.com/https://raw.githubusercontent.com/Dushnilin/tachyon-installer/main/install.sh | bash`*)*

---

### 1. Download Pre-Built Binaries Manually (Optional)

Grab the executable for your OS from **[GitHub Releases](https://github.com/Dushnilin/tachyon-installer/releases)**:

| Operating System | Architecture | Direct Executable Binary |
| :--- | :--- | :--- |
| **Windows** | x86_64 (64-bit) | `tachyon-installer-windows-amd64.exe` |
| **Windows** | ARM64 | `tachyon-installer-windows-arm64.exe` |
| **Linux** | x86_64 (amd64) | `tachyon-installer-linux-amd64` |
| **Linux** | ARM64 (aarch64) | `tachyon-installer-linux-arm64` |
| **macOS** | Apple Silicon (M1/M2/M3/M4) | `tachyon-installer-darwin-arm64` |
| **macOS** | Intel (x86_64) | `tachyon-installer-darwin-amd64` |

### 2. Run the Installer

Connect your computer to the router via Ethernet (LAN) or home Wi-Fi, and run:

**Windows (PowerShell / Command Prompt):**
```powershell
.\tachyon-installer.exe
```

**Linux / macOS (Terminal):**
```bash
chmod +x ./tachyon-installer
./tachyon-installer
```

Follow the on-screen TUI wizard:
1. Enter router IP (default `192.168.1.1`), SSH port (`22`), and username (`root`).
2. Provide your router admin password.
3. Review the live pre-flight hardware diagnostics summary.
4. Select your preferred proxy engine, Tachyon version, and mirror on the dashboard (or press `S` to hot-swap engine only).
5. Press **«Start Installation»** (`Enter`) to deploy.
6. Upon completion, press `O` to launch the LuCI panel in your browser!

### 3. Headless Automation (Unattended CLI)

For script-based automation, CI/CD pipelines, or execution without launching the interactive TUI, use command-line flags:

```bash
# Network & kernel optimization (TCP BBR, fq_codel, adaptive socket buffers)
./tachyon-installer -ip 192.168.1.1 -pass "secret" -tune-network

# In-place self-update to the newest release on GitHub
./tachyon-installer -self-update

# Intelligent guided autopilot setup (DNS tester + DPI fuzzer + BBR network tuning)
./tachyon-installer -ip 192.168.1.1 -pass "secret" -wizard

# Emergency rescue: flush interception rules and recover native internet
./tachyon-installer -ip 192.168.1.1 -pass "secret" -rescue

# Auto-fix conflicts: stop and disable passwall, openclash, zapret, etc.
./tachyon-installer -ip 192.168.1.1 -pass "secret" -fix-conflicts

# Create full system snapshot (network, firewall, dhcp, tachyon)
./tachyon-installer -ip 192.168.1.1 -pass "secret" -snapshot

# Real-time live performance monitoring (CPU, RAM, core, traffic, conntrack)
./tachyon-installer -ip 192.168.1.1 -pass "secret" -monitor

# Benchmark and ping subscription nodes concurrently (URL / vless / base64)
./tachyon-installer -test-sub "https://example.com/sub"

# Pre-download complete offline package bundle for air-gapped routers
./tachyon-installer -offline-bundle "offline_pack/"

# Hot-swap routing engine to tachyon-core in ~3 seconds (Rust, 4.2 MB RAM, 0ms GC)
./tachyon-installer -ip 192.168.1.1 -pass "secret" -switch-engine tachyon-core

# Automated express installation with tachyon-core engine and zRAM-swap
./tachyon-installer -ip 192.168.1.1 -pass "secret" -engine tachyon-core -zram -yes

# Express installation with immediate subscription link setup
./tachyon-installer -ip 192.168.1.1 -pass "secret" -sub "vless://..." -yes

# Standalone router diagnostics with report export
./tachyon-installer -ip 192.168.1.1 -pass "secret" -diag -export-diag report.txt

# List all local backup archives
./tachyon-installer -list-backups

# Restore configuration from the newest backup archive
./tachyon-installer -ip 192.168.1.1 -pass "secret" -restore latest

# Completely uninstall Tachyon from the router
./tachyon-installer -ip 192.168.1.1 -pass "secret" -uninstall

# Check for updates on GitHub
./tachyon-installer -check-update
```

<p align="center">
  <img src="assets/readme/divider_stream.svg" width="100%" alt="divider" />
</p>

## 🛠️ Building From Source

Requires **Go 1.22+**:

```bash
# Clone the repository
git clone https://github.com/Dushnilin/tachyon-installer.git
cd tachyon-installer

# Build for current host platform
go build -o tachyon-installer .

# Build all 7 cross-platform desktop releases simultaneously
go run scripts/build.go -v 1.0.0
```

Compiled archives along with `sha256sums.txt` will be generated in `dist/`.

<p align="center">
  <img src="assets/readme/divider_stream.svg" width="100%" alt="divider" />
</p>

## 📜 License

Licensed under the **GNU General Public License v3.0 (GPL-3.0)**.

* 🛰️ Core Repository: **[Tachyon](https://github.com/Dushnilin/tachyon)**
* 💬 Community & Discussion: **[Tachyon Telegram Channel](https://t.me/tachyon_proxy)**
