# Tachyon Installer One-Line Bootstrap for Windows PowerShell
# Usage:
#   irm https://raw.githubusercontent.com/Dushnilin/tachyon-installer/main/install.ps1 | iex
#   irm https://gh-proxy.com/https://raw.githubusercontent.com/Dushnilin/tachyon-installer/main/install.ps1 | iex

[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12 -bor [Net.SecurityProtocolType]::Tls13
try { [Console]::OutputEncoding = [System.Text.Encoding]::UTF8 } catch {}

Write-Host "🛰️  Инициализация Tachyon Express Installer..." -ForegroundColor Cyan

# 1. Detect Architecture
$arch = "amd64"
if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64" -or $env:PROCESSOR_ARCHITEW6432 -eq "ARM64") {
    $arch = "arm64"
}
$binaryName = "tachyon-installer-windows-$arch.exe"
Write-Host "   Обнаружена платформа: Windows x64/arm64 ($arch)" -ForegroundColor Gray

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
        Write-Host "⚡ Загрузка с: $url ..." -ForegroundColor DarkCyan
        $webClient = New-Object System.Net.WebClient
        $webClient.DownloadFile($url, $targetPath)
        if ((Test-Path $targetPath) -and ((Get-Item $targetPath).Length -gt 1048576)) {
            $downloaded = $true
            Write-Host "✓ Файл успешно загружен!" -ForegroundColor Green
            break
        }
    } catch {
        Write-Host "   Сбой зеркала, пробуем следующее..." -ForegroundColor Yellow
    }
}

if (-not $downloaded) {
    Write-Host "❌ Ошибка: не удалось скачать исполняемый файл ни с одного зеркала." -ForegroundColor Red
    Write-Host "   Проверьте подключение к сети или скачайте вручную: https://github.com/Dushnilin/tachyon-installer/releases" -ForegroundColor Red
    return
}

# 4. Launch Installer in Current Console Window
Write-Host "🚀 Запуск Tachyon Installer..." -ForegroundColor Cyan
& "$targetPath" @args
