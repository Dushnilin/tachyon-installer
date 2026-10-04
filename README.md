<div align="center">

![Tachyon Installer Banner](assets/readme/hero.svg)

[![Go](https://img.shields.io/badge/Go-1.22%20%7C%201.23%20%7C%201.25-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://go.dev/)
[![Releases](https://img.shields.io/github/v/release/Dushnilin/tachyon-installer?style=for-the-badge&color=818CF8)](https://github.com/Dushnilin/tachyon-installer/releases)
[![Platforms](https://img.shields.io/badge/Platforms-Windows%20%7C%20Linux%20%7C%20macOS-38BDF8?style=for-the-badge)](https://github.com/Dushnilin/tachyon-installer/releases)
[![OpenWrt](https://img.shields.io/badge/OpenWrt-23.05%20%7C%2024.10%20%7C%2025.x%20%7C%20SNAPSHOT-10B981?style=for-the-badge&logo=openwrt)](https://openwrt.org/)
[![Telegram](https://img.shields.io/badge/Telegram-Канал-26A5E4?style=for-the-badge&logo=telegram&logoColor=white)](https://t.me/tachyon_proxy)
[![License](https://img.shields.io/badge/License-GPL--3.0-C084FC?style=for-the-badge)](LICENSE)

[**🇷🇺 Русский**](README.md) | [**🇬🇧 English**](README.en.md)

</div>

<p align="center">
  <img src="assets/readme/divider_stream.svg" width="100%" alt="divider" />
</p>

## 🔍 Главные возможности (v1.6)

- **🌐 Управление флотом роутеров (Multi-Router Fleet Manager)**: Автоматическое сканирование локальных подсетей (CIDR), опрос Dropbear SSH и построение интерактивного инвентаря сети в TUI и CLI. Авто-определение архитектур процессоров (`arm64`, `mips`, `x86_64`), проверка версий Tachyon, фильтрация устаревших и чистых роутеров и параллельная пакетная установка/обновление с автоподбором ядра под ОЗУ (кнопка `🌐 Флот роутеров`, клавиша `F` в TUI или флаги `-fleet-scan` / `-fleet-deploy`).
- **✨ Интеллектуальный автопилот (Guided Setup Wizard)**: Полная автоматическая настройка «под ключ» в 1 клик (хоткей `A` или флаг `-wizard`): роутерное тестирование DNS на отравление провайдером, DPI Fuzzer для подбора победной стратегии YouTube/Discord, оптимизация TCP BBR и сквозная валидация.
- **⚡ Мгновенный запуск в 1 строку**: Запуск без предварительного скачивания через PowerShell (`irm ... | iex`) и Linux/macOS (`curl | bash`).
- **🚀 Оптимизация сети и ядра (Network Tuning & BBR)**: Тонкая настройка сетевого стека ядра OpenWrt (TCP BBR + fq_codel, оптимизация буферов rmem/wmem под объем ОЗУ, TCP Fast Open, ускорение сессий conntrack, включение Firewall Flow Offloading) с сохранением в `/etc/sysctl.d/99-tachyon-tune.conf` (флаг `-tune-network` или клавиша `T` в TUI).
- **🔄 Бесшовное самообновление (In-Place Self-Update)**: Автоматическая проверка, скачивание нового релиза, валидация контрольной суммы SHA256 и безопасная замена запущенного бинарника установщика на диске в 1 команду (флаг `-self-update` или клавиша `U` в TUI).
- **🚑 Аварийный сброс сети (Emergency Rescue)**: Снятие «зависших» правил перехвата nftables/iptables, сброс маршрутов TProxy, перезапуск dnsmasq/firewall и моментальное восстановление прямого интернета при сбоях (флаг `-rescue` или кнопка `🚑 Rescue` / клавиша `R` в TUI).
- **🛡️ Авто-устранение сетевых конфликтов (Conflict Auto-Fix)**: Автоматический поиск, корректная остановка и отключение автозапуска конкурирующих прокси/DNS служб (`passwall`, `openclash`, `zapret`, `xray`, `shadowsocksr` и др.) флагом `-fix-conflicts` или клавишей `C` в TUI.
- **📊 Живой мониторинг роутера (Real-Time Live Monitor)**: Отображение текущей нагрузки CPU Loadavg, оперативной памяти RAM (с графическим баром), времени работы, процессов ядра прокси (PID, память), числа активных сессий conntrack и объемов трафика LAN RX/TX в реальном времени (`-monitor` в CLI или клавиша `M` в TUI).
- **💾 Полный снимок системы (Full System Snapshot)**: Резервное копирование всей сетевой конфигурации роутера (`/etc/config/network`, `/etc/config/dhcp`, `/etc/config/firewall`, `/etc/nftables.d/`, `/etc/config/tachyon`) в локальный `.tar.gz` архив на ПК (`-snapshot` в CLI или клавиша `B` в TUI).
- **⚡ Тестирование и пинг нод подписки (Subscription Benchmark & Latency Ping)**: Быстрый параллельный TCP handshake пинг ко всем серверам в подписке (HTTPS-ссылка, прямые ссылки `vless://`, файл или base64) с сортировкой от быстрейшего к медленному (`-test-sub "vless://..."`).
- **📦 Загрузчик оффлайн-пакета (Offline Bundle Downloader)**: Скачивание полного комплекта дистрибутива Tachyon (OPKG + APK, i18n, ядра для популярных архитектур x86_64, arm64, mipsle, mips) в локальную папку для последующей полностью автономной работы на изолированных объектах (`-offline-bundle "offline_pkg/"`).
- **🌐 Мульти-таргетная матрица проверки обхода (Bypass Probes)**: Тестирование YouTube, Discord, Telegram, GitHub прямо с роутера с определением исходящего GeoIP IP/страны и замером времени отклика каждого сервиса.
- **⚡ Горячая смена ядра (Engine Hot-Swap)**: Моментальное переключение ядра прокси (`sing-box-extended`, `sing-box-tiny`, `steer`, `steer-extended`) за считанные секунды без переустановки LuCI и без сброса подписок — флагом `--switch-engine` или клавишей `S` в TUI.
- **🌐 Открытие веб-панели в 1 клик**: Диалог завершения и горячая клавиша `O` мгновенно открывают панель LuCI в браузере по умолчанию.
- **🖥️ Интерактивный TUI и Headless CLI**: Полноценный терминальный интерфейс с поддержкой мыши и стрелок клавиатуры + режим тихой автоматизации (`--yes`, `--ip`, `--pass`, `--engine`, `--zram`).
- **🧠 Интеллектуальный анализ оборудования**: Оценка ресурсов роутера и рекомендации по выбору ядер; опциональная автоматическая активация **zRAM-swap** для защиты от OOM на роутерах с небольшим объемом RAM (<128 МБ).
- **⏱️ Синхронизация времени роутера**: Автоматическая синхронизация системного времени маршрутизатора с ПК по SSH (`date -u -s`) перед установкой для предотвращения ошибок проверки TLS-сертификатов при обращении к HTTPS/репозиториям.
- **🌐 Универсальный парсер подписок**: Поддержка стандартных HTTPS-ссылок, прямых ссылок (`vless://`, `hysteria2://`, `trojan://`, `ss://`, `vmess://`) и base64-пакетов с отображением количества и типов обнаруженных серверов в реальном времени.
- **🛠️ Набор обслуживания**: Команды быстрого чистого удаления (`--uninstall`), отката из локальных архивов (`--restore latest`), просмотра бэкапов (`--list-backups`) и проверки обновлений установщика (`--check-update`).
- **🔍 Глубокая диагностика**: Автоматический запуск после установки и ручной режим (кнопка в меню, горячая клавиша `D` или флаг `--diag`) с экспортом в текстовый файл (`--export-diag`).
- **Поиск роутера в сети**: Автоматическое обнаружение Dropbear SSH в локальной подсети.
- **Вход по SSH-ключу**: Поддержка авторизации по открытому/закрытому ключу; пароль никогда не сохраняется на диск.
- **Проверка файлов**: Сверка хэш-сумм SHA256 и валидация архивов перед передачей на роутер.

## ⚡ О проекте

**Tachyon Express Installer** — это автономная кроссплатформенная десктопная утилита и интерактивный терминальный мастер (TUI) для экспресс-установки и аппаратной диагностики экосистемы **[Tachyon](https://github.com/Dushnilin/tachyon)** на маршрутизаторах под управлением **OpenWrt** (совместимо с 23.05, 24.10, 25.x и SNAPSHOT).

### 🎯 Решаемая проблема
В условиях жестких сетевых ограничений и блокировок роутер зачастую **не имеет прямого доступа к GitHub**, либо интернет на нём нестабилен до настройки средств обхода. Стандартные скрипты установки `curl | sh` на роутере в таких условиях зависают или падают.

**Tachyon Installer решает эту проблему фундаментально:**
1. Установщик запускается **на вашем компьютере** (Windows, Linux, macOS).
2. Компьютер **самостоятельно скачивает все дистрибутивы и ядра** через быстрые проверенные зеркала GitHub.
3. Проверяет хэш-суммы **SHA256**.
4. По локальной сети через SSH проводит глубокую диагностику роутера, делает бэкап настроек и **потоково передает файлы в оперативную память (`/tmp`)**, гарантируя 100% автономную установку без износа флеш-памяти NAND.

<p align="center">
  <img src="assets/readme/divider_stream.svg" width="100%" alt="divider" />
</p>

## 🔄 Архитектура и конвейер установки

<div align="center">

![Tachyon Installer Pipeline](assets/readme/architecture.svg)

</div>

Весь процесс установки разделен на 5 автономных этапов:

1. **🔍 Pre-flight диагностика роутера**:
   - Авто-определение модели, версии OpenWrt и архитектуры процессора (`arm64`, `mipsle`, `mips`, `x86_64`, `armv7`).
   - Анализ пакетного менеджера (`apk` для OpenWrt 25+ / `opkg` для OpenWrt 24 и ниже).
   - Замер свободного места Flash и ОЗУ (предупреждение при дефиците памяти).
   - Проверка поколения файрвола (`fw4` nftables vs `fw3` iptables).
   - Поиск конфликтующих пакетов (`passwall`, `openclash`, `shadowsocksr`, `podkop`, `forkop`, `nextdns`).
2. **🪞 Интеллектуальная загрузка через зеркала (Host-side Download)**:
   - Автоматический параллельный замер задержки зеркал (`gh-proxy.com`, `ghfast.top`, `gh.ddlc.top`, `gh-proxy.org`, direct).
   - Выбор быстрейшего зеркала и автоматический переход на резервные при сбоях.
   - Сверка контрольных сумм по официальному манифесту `sha256sums.txt`.
3. **💾 Резервное копирование (Safe Config Backup)**:
   - Снятие дампа текущей конфигурации `/etc/config/tachyon` и сохранение на ПК перед обновлением.
4. **📦 Потоковая заливка (tar-over-SSH)**:
   - Передача пакетов напрямую в `tmpfs` (`/tmp/tachyon-install`) через защищенный SSH-туннель без промежуточных громоздких архивов.
   - Нулевой паразитный износ постоянной флеш-памяти NAND.
5. **⚡ Активация и валидация службы**:
   - Установка пакетов `tachyon`, `luci-app-tachyon`, языкового пакета и выбранного ядра.
   - Проверка статуса сервиса, правил firewall, работы FakeIP DNS и утечек.
   - Интерактивный ввод и тестирование ссылки на подписку VPN/прокси в 1 клик.

<p align="center">
  <img src="assets/readme/divider_stream.svg" width="100%" alt="divider" />
</p>

## 💎 Главные возможности и преимущества

* **✨ Интеллектуальный мастер настройки (Автопилот под ключ)**: Кнопка `✨ Мастер (Автопилот)` на первом шаге, хоткей `A` или флаг `-wizard`. Автоматически тестирует отравление DNS провайдером, бенчмаркает чистые DoH-резолверы, запускает встроенный **DPI Fuzzer** для подбора победной стратегии обхода YouTube и Discord, настраивает стек TCP BBR и выдает итоговый скоркарт!
* **🚀 Полная независимость от интернета на роутере**: Роутер может быть только что «из коробки» без настроенного WAN — всё необходимое доставляет установщик.
* **🧭 Все варианты на одном экране (Options Dashboard)**:
  * **5 ядер маршрутизации**: `sing-box-extended` (Рекомендуется: xHTTP, Reality, Hy2), `steer-extended` (скоростной), `steer` (ультра-легкий), `sing-box-lx`, либо пропуск ядра.
  * **Динамические версии Tachyon**: Автоматический опрос релизов из GitHub с выбором стабильных тегов или вводом произвольной ветки.
  * **Сетка зеркал GitHub**: Выбор между авто-тестированием, 5 доверенными зеркалами или прямым подключением.
  * **Локализация LuCI**: Опциональная установка русского языкового пакета `luci-i18n-tachyon-ru`.
* **🎨 Премиальный Slate & Sky TUI интерфейс**:
  * Полное отсутствие раздражающих белых инверсий при фокусе.
  * Анимированный логотип Tachyon и двухрядный индикатор прогресса (общий шаг + подзадача).
  * Удобное управление клавишами: `Tab` / `↑↓` (разделы), `←→` или цифры `[1-6]` (выбор), `Space` (флаги), `Enter` (пуск).
* **💻 Мультиплатформенность без внешних зависимостей**:
  * Скомпилирован в единый самодостаточный бинарный файл без необходимости ставить Python, Node.js или Git.

<p align="center">
  <img src="assets/readme/divider_stream.svg" width="100%" alt="divider" />
</p>

## 🚀 Быстрый старт

### ⚡ Запуск в 1 команду (без предварительного скачивания)

По аналогии с Microsoft Activation Scripts (MAS), Tachyon Installer можно запустить мгновенно одной командой в вашем терминале. Скрипт автоматически определит ОС и архитектуру (x64 / arm64 / Apple Silicon), загрузит актуальный бинарник через быстрейшее доступное зеркало и запустит его прямо в вашей консоли:

**Windows (PowerShell):**
```powershell
irm https://raw.githubusercontent.com/Dushnilin/tachyon-installer/main/install.ps1 | iex
```
*(Резервное зеркало при недоступности GitHub:* `irm https://gh-proxy.com/https://raw.githubusercontent.com/Dushnilin/tachyon-installer/main/install.ps1 | iex`*)*

**Linux & macOS (Terminal):**
```bash
curl -fsSL https://raw.githubusercontent.com/Dushnilin/tachyon-installer/main/install.sh | bash
```
*(Резервное зеркало:* `curl -fsSL https://gh-proxy.com/https://raw.githubusercontent.com/Dushnilin/tachyon-installer/main/install.sh | bash`*)*

---

### 1. Скачайте готовую сборку вручную (если нужно)

Загрузите исполняемый файл из раздела **[Releases](https://github.com/Dushnilin/tachyon-installer/releases)**:

| Операционная система | Архитектура | Прямой исполняемый файл |
| :--- | :--- | :--- |
| **Windows** | x86_64 (64-bit) | `tachyon-installer-windows-amd64.exe` |
| **Windows** | ARM64 | `tachyon-installer-windows-arm64.exe` |
| **Linux** | x86_64 (amd64) | `tachyon-installer-linux-amd64` |
| **Linux** | ARM64 (aarch64) | `tachyon-installer-linux-arm64` |
| **macOS** | Apple Silicon (M1/M2/M3/M4) | `tachyon-installer-darwin-arm64` |
| **macOS** | Intel (x86_64) | `tachyon-installer-darwin-amd64` |

### 2. Запуск утилиты

Убедитесь, что ваш компьютер подключен к роутеру по кабелю (LAN) или домашней сети Wi-Fi, и запустите установщик:

**Windows (PowerShell / CMD):**
```powershell
.\tachyon-installer.exe
```

**Linux / macOS (Terminal):**
```bash
chmod +x ./tachyon-installer
./tachyon-installer
```

Следуйте подсказкам мастера:
1. Введите IP-адрес роутера (по умолчанию `192.168.1.1`), порт SSH (`22`) и имя пользователя (`root`).
2. Введите системный пароль роутера.
3. Дождитесь автоматической диагностики оборудования.
4. Выберите ядро, версию и зеркало на интерактивном дашборде (или нажмите `S` для быстрой смены ядра).
5. Нажмите **«Начать установку»** (`Enter`) и наблюдайте за процессом в реальном времени.
6. По завершении нажмите `O`, чтобы открыть веб-панель LuCI прямо в браузере!

### 3. Автоматический режим (Headless CLI)

Для использования в скриптах автоматизации или без запуска графического TUI-интерфейса используйте флаги командной строки:

```bash
# Оптимизация сетевого стека роутера (TCP BBR, fq_codel, буферы, offloading)
./tachyon-installer -ip 192.168.1.1 -pass "secret" -tune-network

# Автоматическое самообновление установщика до последней версии с GitHub
./tachyon-installer -self-update

# 🚀 Интеллектуальный мастер быстрой настройки под ключ (Автопилот)
./tachyon-installer -ip 192.168.1.1 -pass "secret" -wizard

# Аварийный сброс правил перехвата и восстановление прямого интернета
./tachyon-installer -ip 192.168.1.1 -pass "secret" -rescue

# Сканирование сети и вывод инвентаря роутеров с их статусом Tachyon
./tachyon-installer -fleet-scan

# Массовая параллельная установка/обновление Tachyon на все найденные роутеры
./tachyon-installer -pass "secret" -fleet-deploy

# Массовое обновление только тех роутеров, где Tachyon устарел
./tachyon-installer -pass "secret" -fleet-deploy -fleet-only-outdated

# Поиск и отключение конфликтующих прокси-служб (passwall, openclash, zapret)
./tachyon-installer -ip 192.168.1.1 -pass "secret" -fix-conflicts

# Создание полного снимка всей конфигурации (сеть, firewall, dhcp, tachyon)
./tachyon-installer -ip 192.168.1.1 -pass "secret" -snapshot

# Живой мониторинг нагрузки CPU, памяти, аптайма, ядра и сетевых сессий
./tachyon-installer -ip 192.168.1.1 -pass "secret" -monitor

# Тестирование задержки и TCP handshake пинг узлов в подписке (URL / vless / base64)
./tachyon-installer -test-sub "https://example.com/sub"

# Загрузка полного оффлайн-бандла для изолированных объектов
./tachyon-installer -offline-bundle "offline_pack/"

# Горячая смена ядра на sing-box-tiny за 3 секунды (без переустановки LuCI)
./tachyon-installer -ip 192.168.1.1 -pass "secret" -switch-engine sing-box-tiny

# Автоматическая экспресс-установка ядра steer и zRAM-swap
./tachyon-installer -ip 192.168.1.1 -pass "secret" -engine steer -zram -yes

# Экспресс-установка с немедленным добавлением ссылки на подписку
./tachyon-installer -ip 192.168.1.1 -pass "secret" -sub "vless://..." -yes

# Быстрая диагностика роутера с экспортом отчета в файл
./tachyon-installer -ip 192.168.1.1 -pass "secret" -diag -export-diag report.txt

# Просмотр списка локальных бэкапов
./tachyon-installer -list-backups

# Откат настроек из последнего сохраненного бэкапа
./tachyon-installer -ip 192.168.1.1 -pass "secret" -restore latest

# Полное удаление Tachyon с роутера
./tachyon-installer -ip 192.168.1.1 -pass "secret" -uninstall

# Проверка наличия новых версий установщика
./tachyon-installer -check-update
```

<p align="center">
  <img src="assets/readme/divider_stream.svg" width="100%" alt="divider" />
</p>

## 🛠️ Сборка из исходников

Для самостоятельной сборки потребуется установленный **Go 1.22+**:

```bash
# Клонирование репозитория
git clone https://github.com/Dushnilin/tachyon-installer.git
cd tachyon-installer

# Сборка для текущей операционной системы
go build -o tachyon-installer .

# Сборка всех 7 кроссплатформенных релизов сразу
go run scripts/build.go -v 1.0.0
```

Собранные бинарники и архивы с контрольными суммами `sha256sums.txt` будут помещены в каталог `dist/`.

<p align="center">
  <img src="assets/readme/divider_stream.svg" width="100%" alt="divider" />
</p>

## 📜 Лицензия и экосистема

Проект распространяется под свободной лицензией **GNU General Public License v3.0 (GPL-3.0)**.

* 🛰️ Основной репозиторий: **[Tachyon Core & LuCI App](https://github.com/Dushnilin/tachyon)**
* 💬 Сообщество и поддержка: **[Telegram-канал Tachyon](https://t.me/tachyon_proxy)**
