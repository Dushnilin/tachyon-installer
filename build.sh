#!/usr/bin/env bash
# Multiplatform Build Script for Tachyon Installer (Linux / macOS)
set -euo pipefail

VERSION="${1:-1.0.0}"
TARGET_OS="${2:-all}"
TARGET_ARCH="${3:-all}"

echo "==> Starting multi-platform build for Tachyon Installer (v${VERSION})..."
go run scripts/build.go -v "${VERSION}" -os "${TARGET_OS}" -arch "${TARGET_ARCH}"
