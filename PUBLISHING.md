# Публикация релиза

## Первый запуск репозитория

1. Создайте пустой публичный репозиторий GitHub без автоматически добавленного
   README или `.gitignore`.
2. Добавьте его как `origin` и отправьте ветку `main`.
3. Убедитесь, что GitHub Actions `CI` завершился успешно.

## Подписанные пресеты маршрутизации

Клиент принимает сетевые пресеты только после проверки встроенным публичным
ключом Ed25519. Приватный ключ не хранится в репозитории: локальный инструмент
шифрует seed через Windows DPAPI, поэтому использовать его может только тот же
пользователь Windows на этом компьютере.

Первичная генерация ключа выполняется один раз:

```powershell
cd client
go run ./cmd/preset-sign -mode generate `
  -key "$env:USERPROFILE\.pinusvpn-signing\preset-ed25519.dpapi"
```

Для обновления увеличьте ревизию, сформируйте payload и подпишите его:

```powershell
go run ./cmd/preset-sign -mode export -revision 3 `
  -published-at "2026-07-11T15:30:00Z" `
  -output "..\presets\catalog.json"
go run ./cmd/preset-sign -mode sign `
  -key "$env:USERPROFILE\.pinusvpn-signing\preset-ed25519.dpapi" `
  -input "..\presets\catalog.json" `
  -output "..\presets\catalog.signed.json"
go test ./smart
```

Коммитьте оба файла из `presets/`. Никогда не добавляйте `.dpapi`-ключ. Клиент
не принимает откат ревизии, неизвестные поля, опасные системные процессы,
некорректные домены, слишком большой payload или неверную подпись.

## Релиз

1. Обновите номер в `client/version/version.go`, `client/versioninfo.json`,
   `client/manifest.xml`, `packaging/README.txt` и добавьте заметки в
   `.github/release-notes/vX.Y.Z.md`.
2. Локально запустите `.\build-public-release.bat`.
3. Проверьте `release/release-manifest.json` и `release/SHA256SUMS.txt`.
4. Для публичного продукта подпишите клиент и MSI сертификатом подписи кода.
5. Создайте и отправьте тег вида `v3.2.0`. Workflow `Release` пересоберёт и
   приложит все артефакты автоматически.

Для подписанного релиза добавьте в GitHub Actions Secrets:

- `WINDOWS_SIGN_CERT_BASE64` — PFX-сертификат в Base64;
- `WINDOWS_SIGN_CERT_PASSWORD` — пароль PFX.

Workflow импортирует сертификат только во временное хранилище runner и удаляет
файл PFX сразу после импорта. Без этих secrets сборка останется рабочей, но
будет опубликована без подписи и может вызвать предупреждение SmartScreen.

Папка `release/` намеренно игнорируется Git. MSI, ZIP, архив исходников
движка, manifest и checksums загружаются как GitHub Release assets, а не
коммитятся в историю исходников.

Перед публикацией нельзя добавлять:

- `.conf`, приватные ключи и runtime JSON;
- содержимое `diagnostics/` и локальные журналы;
- API-токены, пароли серверов и сертификаты подписи;
- тестовые скриншоты с профилями, адресами или именами пользователей.
