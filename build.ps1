# PowerShell Multiplatform Build Script for Tachyon Installer
param(
    [string]$Version = "1.0.0",
    [string]$OS = "all",
    [string]$Arch = "all",
    [switch]$NoArchive
)

$argsList = @("run", "scripts/build.go", "-v", $Version, "-os", $OS, "-arch", $Arch)
if ($NoArchive) {
    $argsList += "-no-archive"
}

Write-Host "==> Starting build with Go..." -ForegroundColor Cyan
& go $argsList
if ($LASTEXITCODE -ne 0) {
    Write-Error "Build failed with exit code $LASTEXITCODE"
    exit $LASTEXITCODE
}
