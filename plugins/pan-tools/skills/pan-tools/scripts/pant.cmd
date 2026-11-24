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
  "try { [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12 } catch {};" ^
  "if (-not [Environment]::Is64BitOperatingSystem) { throw 'Only 64-bit (amd64 / arm64) Windows is supported.' };" ^
  "$arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64' -or $env:PROCESSOR_ARCHITEW6432 -eq 'ARM64') { 'arm64' } else { 'amd64' };" ^
  "$tag = $null;" ^
  "try {" ^
  "  $req = [Net.WebRequest]::Create('https://github.com/qwerprog/pan-tools/releases/latest');" ^
  "  $req.AllowAutoRedirect = $false;" ^
  "  $resp = $req.GetResponse();" ^
  "  $loc = $resp.GetResponseHeader('Location');" ^
  "  $resp.Close();" ^
  "  if ($loc -match '/tag/([^/]+)$') { $tag = $Matches[1] }" ^
  "} catch {};" ^
  "if (-not $tag) {" ^
  "  $rel = Invoke-RestMethod -Uri 'https://api.github.com/repos/qwerprog/pan-tools/releases/latest' -Headers @{ 'User-Agent' = 'pan-tools-plugin' };" ^
  "  $tag = $rel.tag_name" ^
  "};" ^
  "if (-not $tag) { throw 'Unable to resolve latest release tag.' };" ^
  "$dlUrl = 'https://github.com/qwerprog/pan-tools/releases/download/' + $tag + '/pant-' + $tag + '-windows-' + $arch + '.zip';" ^
  "$tmp = Join-Path $env:TEMP ('pant-' + [guid]::NewGuid().ToString('N'));" ^
  "New-Item -ItemType Directory -Force -Path $tmp | Out-Null;" ^
  "try {" ^
  "  $zip = Join-Path $tmp 'pant.zip';" ^
  "  Invoke-WebRequest -Uri $dlUrl -OutFile $zip -UseBasicParsing;" ^
  "  Expand-Archive -Path $zip -DestinationPath $tmp -Force;" ^
  "  $exe = Get-ChildItem -Path $tmp -Filter 'pant.exe' -Recurse -File | Select-Object -First 1;" ^
  "  if (-not $exe) { throw 'pant.exe not found inside archive.' };" ^
  "  New-Item -ItemType Directory -Force -Path '%CACHE_DIR%' | Out-Null;" ^
  "  Copy-Item -Path $exe.FullName -Destination '%CACHED_BIN%' -Force;" ^
  "} finally { Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue }"

if exist "%CACHED_BIN%" (
  "%CACHED_BIN%" %*
  exit /b %ERRORLEVEL%
)

echo Error: pant.exe not found and automatic download failed. 1>&2
echo Install via: irm https://raw.githubusercontent.com/qwerprog/pan-tools/main/install.ps1 ^| iex (or: winget install QwerProg.pan-tools) 1>&2
exit /b 127
