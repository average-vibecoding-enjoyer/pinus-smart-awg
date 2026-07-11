# Публикация релиза

## Первый запуск репозитория

1. Создайте пустой публичный репозиторий GitHub без автоматически добавленного
   README или `.gitignore`.
2. Добавьте его как `origin` и отправьте ветку `main`.
3. Убедитесь, что GitHub Actions `CI` завершился успешно.

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
