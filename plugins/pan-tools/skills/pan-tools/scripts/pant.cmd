@echo off
setlocal enabledelayedexpansion

if defined PANT_BIN (
  if exist "%PANT_BIN%" (
    "%PANT_BIN%" %*
    exit /b %ERRORLEVEL%
  )
)

where pant >nul 2>&1
if %ERRORLEVEL%==0 (
  pant %*
  exit /b %ERRORLEVEL%
)

set "CACHE_DIR=%LOCALAPPDATA%\pan-tools\bin"
set "CACHED_BIN=%CACHE_DIR%\pant.exe"
if exist "%CACHED_BIN%" (
  "%CACHED_BIN%" %*
  exit /b %ERRORLEVEL%
)

powershell -NoProfile -ExecutionPolicy Bypass -Command ^
  "$ErrorActionPreference='Stop';" ^
  "$arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } elseif ([Environment]::Is64BitOperatingSystem) { 'amd64' } else { '386' };" ^
  "$rel = Invoke-RestMethod -Uri 'https://api.github.com/repos/qwerprog/pan-tools/releases/latest';" ^
  "$asset = $rel.assets | Where-Object { $_.name -match ('^pant-.*-windows-' + $arch + '\.zip$') } | Select-Object -First 1;" ^
  "if (-not $asset) { throw 'No matching Windows release asset found.' };" ^
  "$tmp = Join-Path $env:TEMP ('pant-' + [guid]::NewGuid().ToString('N'));" ^
  "New-Item -ItemType Directory -Force -Path $tmp | Out-Null;" ^
  "try {" ^
  "  $zip = Join-Path $tmp 'pant.zip';" ^
  "  Invoke-WebRequest -Uri $asset.browser_download_url -OutFile $zip;" ^
  "  Expand-Archive -Path $zip -DestinationPath $tmp -Force;" ^
  "  $exe = Get-ChildItem -Path $tmp -Filter 'pant.exe' -Recurse | Select-Object -First 1;" ^
  "  if (-not $exe) { throw 'pant.exe not found inside archive.' };" ^
  "  New-Item -ItemType Directory -Force -Path '%CACHE_DIR%' | Out-Null;" ^
  "  Copy-Item -Path $exe.FullName -Destination '%CACHED_BIN%' -Force;" ^
  "} finally { Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue }"

if exist "%CACHED_BIN%" (
  "%CACHED_BIN%" %*
  exit /b %ERRORLEVEL%
)

echo Error: pant.exe not found and automatic download failed. 1>&2
echo Download from https://github.com/qwerprog/pan-tools/releases 1>&2
exit /b 127
