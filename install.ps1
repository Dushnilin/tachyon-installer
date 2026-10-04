# Tachyon Installer One-Line Bootstrap for Windows PowerShell
# Usage:
#   irm https://raw.githubusercontent.com/Dushnilin/tachyon-installer/main/install.ps1 | iex
#   irm https://gh-proxy.com/https://raw.githubusercontent.com/Dushnilin/tachyon-installer/main/install.ps1 | iex

[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12 -bor [Net.SecurityProtocolType]::Tls13

Write-Host "==> Initializing Tachyon Express Installer..." -ForegroundColor Cyan

# 1. Detect Architecture
$arch = "amd64"
if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64" -or $env:PROCESSOR_ARCHITEW6432 -eq "ARM64") {
    $arch = "arm64"
}
$binaryName = "tachyon-installer-windows-$arch.exe"
Write-Host "    Platform detected: Windows ($arch)" -ForegroundColor Gray

# 2. Prepare Target Temp Path
$tempDir = [System.IO.Path]::Combine([System.IO.Path]::GetTempPath(), "tachyon-installer")
if (-not (Test-Path $tempDir)) {
    New-Item -ItemType Directory -Path $tempDir -Force | Out-Null
}
$targetPath = [System.IO.Path]::Combine($tempDir, "tachyon-installer.exe")

# 3. Candidate Mirrors for reliable download
$mirrors = @(
    "https://github.com/Dushnilin/tachyon-installer/releases/latest/download/$binaryName",
    "https://gh-proxy.com/https://github.com/Dushnilin/tachyon-installer/releases/latest/download/$binaryName",
    "https://ghfast.top/https://github.com/Dushnilin/tachyon-installer/releases/latest/download/$binaryName",
    "https://gh.ddlc.top/https://github.com/Dushnilin/tachyon-installer/releases/latest/download/$binaryName"
)

$downloaded = $false
foreach ($url in $mirrors) {
    try {
        Write-Host "    Downloading from: $url ..." -ForegroundColor DarkCyan
        $webClient = New-Object System.Net.WebClient
        $webClient.DownloadFile($url, $targetPath)
        if ((Test-Path $targetPath) -and ((Get-Item $targetPath).Length -gt 1048576)) {
            $downloaded = $true
            Write-Host "    [OK] Download completed successfully!" -ForegroundColor Green
            break
        }
    } catch {
        Write-Host "    [Warning] Mirror failed, trying next..." -ForegroundColor Yellow
    }
}

if (-not $downloaded) {
    Write-Host "[ERROR] Could not download installer binary from any mirror." -ForegroundColor Red
    Write-Host "        Please download manually from: https://github.com/Dushnilin/tachyon-installer/releases" -ForegroundColor Red
    return
}

# 4. Launch Installer in Current Console Window
Write-Host "==> Launching Tachyon Installer..." -ForegroundColor Cyan
& "$targetPath" @args
