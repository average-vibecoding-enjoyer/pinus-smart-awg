# Pinus Smart AWG

[![CI](https://github.com/average-vibecoding-enjoyer/pinus-smart-awg/actions/workflows/ci.yml/badge.svg)](https://github.com/average-vibecoding-enjoyer/pinus-smart-awg/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/average-vibecoding-enjoyer/pinus-smart-awg?display_name=tag)](https://github.com/average-vibecoding-enjoyer/pinus-smart-awg/releases/latest)
[![License: MIT](https://img.shields.io/badge/license-MIT-2ea44f.svg)](LICENSE)
[![Windows](https://img.shields.io/badge/Windows-10%20%7C%2011-0078d4.svg)](#скачать)

Windows-клиент AmneziaWG с раздельной маршрутизацией по приложениям,
доменам и IP-адресам. Проект основан на AmneziaWG for Windows и
WireGuard for Windows.

> Статус: публичная beta. Сборка протестирована на Windows x64. ARM64 и x86
> проходят кросс-компиляцию и проверку формата пакета, но требуют отдельного
> тестирования на реальных устройствах соответствующей архитектуры.

> **Нужен VPN-профиль?** Купить доступ и получить готовый конфиг можно через
> Telegram-бота [@pinusvpn_bot](https://t.me/pinusvpn_bot).

## Скачать

Для обычной установки используйте MSI из раздела
[GitHub Releases](https://github.com/average-vibecoding-enjoyer/pinus-smart-awg/releases):

| Windows | Файл | Кому подходит |
| --- | --- | --- |
| x64 | `Pinus-Smart-AWG-*-windows-x64.msi` | Большинство компьютеров с Intel или AMD |
| ARM64 | `Pinus-Smart-AWG-*-windows-arm64.msi` | Windows on ARM, например Snapdragon |
| x86 | `Pinus-Smart-AWG-*-windows-x86.msi` | Старые 32-битные системы |

Архитектура указана в `Параметры -> Система -> О системе -> Тип системы`.
Portable ZIP предназначен для ручного запуска и диагностики. В нём должны
находиться `PinusSmartAWG.exe`, `amnezia-box.exe`, подходящий `wintun.dll` и
лицензионные файлы. Не запускайте EXE прямо из ZIP.

## Возможности

- импорт AmneziaWG-профилей `.conf` и ZIP;
- режим всего трафика через VPN;
- режим только выбранных сервисов, приложений, доменов и IP;
- прямые исключения при включённом режиме всего интернета через VPN;
- готовые правила для Discord, YouTube, Telegram, Instagram и AI-сервисов;
- подписанные Ed25519-обновления пресетов с защитой от отката;
- выбор работающего процесса при создании правила;
- автоматическое переключение профилей без отдельной кнопки применения;
- защищённая служба Windows и проверка SHA-256 сетевых зависимостей;
- локальные журналы и диагностика без экспорта приватных ключей.

## Установка

1. Скачайте MSI для своей архитектуры.
2. Проверьте файл по `SHA256SUMS.txt` из того же GitHub Release.
3. Запустите MSI и подтвердите запрос администратора.
4. Импортируйте профиль на странице `Профили`.

Установщик включает клиент, закреплённую сборку `amnezia-box` и официальную
подписанную DLL Wintun. Дополнительные программы вручную устанавливать не
нужно. При удалении системные службы приложения удаляются, а зашифрованные
профили сохраняются для безопасной переустановки.

## Сборка

Требования:

- Windows 10/11;
- Go версии из файла `.go-version`;
- Git и `curl.exe`.

```powershell
.\build-public-release.bat
```

Скрипт:

1. проверяет тесты клиента и сетевого ядра;
2. скачивает Wintun 0.14.1 и проверяет опубликованный SHA-256;
3. собирает `amnezia-box` из закреплённого commit с AWG 2.0;
4. собирает клиент для x64, ARM64 и x86;
5. подставляет отдельные хэши зависимостей в каждую архитектуру;
6. создаёт portable ZIP, MSI, архив соответствующих исходников движка,
   `release-manifest.json` и `SHA256SUMS.txt`.

Результат появляется в локальной папке `release/`, которая не коммитится в
Git. Для сборки только одной архитектуры:

```powershell
.\build-public-release.bat -Architectures amd64
```

Для подписи задайте `PINUS_SIGN_CERT_THUMBPRINT`; в системе должны быть
сертификат подписи кода и `signtool.exe`. Скрипт подписывает клиент, движок и
MSI. Неподписанные сборки работают, но Windows SmartScreen может показать
предупреждение.

## Проверка

```powershell
cd client
go test ./...
cd ../core
go test ./...
```

Тесты, способные менять реальные адаптеры, DNS, маршруты или сертификаты,
отключены по умолчанию и запускаются только через явно указанные в тестах
переменные `PINUS_RUN_*`.

## Зависимости и лицензии

- Клиент: MIT, см. [LICENSE](LICENSE).
- `amnezia-box`: GPL-3.0-or-later, commit
  `f40548f91a14582975096d0310e3c6afd44656f8`.
- Wintun 0.14.1: официальные неизменённые подписанные DLL и отдельная лицензия
  на предсобранные файлы.

Каждый release содержит архив соответствующих исходников `amnezia-box`,
лицензию движка, лицензию Wintun и third-party notices.

Подробности публикации находятся в [PUBLISHING.md](PUBLISHING.md), сообщения
об уязвимостях принимаются по правилам из [SECURITY.md](SECURITY.md).

История публичных версий находится в [CHANGELOG.md](CHANGELOG.md).

Поддержка: https://t.me/MEN9_HET
