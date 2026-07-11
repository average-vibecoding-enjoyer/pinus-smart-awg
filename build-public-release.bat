@echo off
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0build-public-release.ps1" %*
