#!/bin/sh
# Tachyon Installer One-Line Bootstrap for Linux & macOS
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/Dushnilin/tachyon-installer/main/install.sh | bash
#   curl -fsSL https://gh-proxy.com/https://raw.githubusercontent.com/Dushnilin/tachyon-installer/main/install.sh | bash

set -e

printf "\033[1;36m🛰️  Инициализация Tachyon Express Installer...\033[0m\n"

# 1. Detect OS
OS_RAW=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS_RAW" in
    darwin*)
        OS="darwin"
        ;;
    linux*)
        OS="linux"
        ;;
    *)
        printf "\033[1;31m❌ Неподдерживаемая ОС: %s\033[0m\n" "$OS_RAW"
        exit 1
        ;;
esac

# 2. Detect Architecture
ARCH_RAW=$(uname -m)
case "$ARCH_RAW" in
    x86_64|amd64)
        ARCH="amd64"
        ;;
    arm64|aarch64)
        ARCH="arm64"
        ;;
    *)
        printf "\033[1;31m❌ Неподдерживаемая архитектура процессора: %s\033[0m\n" "$ARCH_RAW"
        exit 1
        ;;
esac

BINARY_NAME="tachyon-installer-${OS}-${ARCH}"
printf "\033[0;37m   Обнаружена платформа: %s (%s)\033[0m\n" "$OS" "$ARCH"

# 3. Target Path
TEMP_DIR="/tmp/tachyon-installer"
mkdir -p "$TEMP_DIR"
TARGET_PATH="$TEMP_DIR/tachyon-installer"

# 4. Download with Mirror Fallbacks
MIRRORS="
https://github.com/Dushnilin/tachyon-installer/releases/latest/download/${BINARY_NAME}
https://gh-proxy.com/https://github.com/Dushnilin/tachyon-installer/releases/latest/download/${BINARY_NAME}
https://ghfast.top/https://github.com/Dushnilin/tachyon-installer/releases/latest/download/${BINARY_NAME}
https://gh.ddlc.top/https://github.com/Dushnilin/tachyon-installer/releases/latest/download/${BINARY_NAME}
"

DOWNLOADED=0
for url in $MIRRORS; do
    [ -z "$url" ] && continue
    printf "\033[0;36m⚡ Загрузка с: %s ...\033[0m\n" "$url"
    if command -v curl >/dev/null 2>&1; then
        if curl -fSL --connect-timeout 8 -o "$TARGET_PATH" "$url" 2>/dev/null; then
            DOWNLOADED=1
            break
        fi
    elif command -v wget >/dev/null 2>&1; then
        if wget -q --timeout=8 -O "$TARGET_PATH" "$url" 2>/dev/null; then
            DOWNLOADED=1
            break
        fi
    fi
    printf "\033[0;33m   Сбой зеркала, пробуем следующее...\033[0m\n"
done

if [ "$DOWNLOADED" -ne 1 ] || [ ! -f "$TARGET_PATH" ] || [ $(wc -c < "$TARGET_PATH" 2>/dev/null || stat -c%s "$TARGET_PATH" 2>/dev/null || echo 0) -lt 1048576 ]; then
    printf "\033[1;31m❌ Ошибка: не удалось скачать исполняемый файл ни с одного зеркала.\033[0m\n"
    printf "\033[1;31m   Скачайте файл вручную со страницы: https://github.com/Dushnilin/tachyon-installer/releases\033[0m\n"
    exit 1
fi

chmod +x "$TARGET_PATH"
printf "\033[1;32m✓ Файл успешно загружен!\033[0m\n"
printf "\033[1;36m🚀 Запуск Tachyon Installer...\033[0m\n"

# 5. Execute in place
exec "$TARGET_PATH" "$@"
