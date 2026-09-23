# ohmylaya one-shot installer for Windows.
# Usage: irm https://raw.githubusercontent.com/QuBiit0/ohmylaya/main/install.ps1 | iex
# Environment: OHMYLAYA_BIN_DIR (default %LOCALAPPDATA%\ohmylaya\bin), OHMYLAYA_VERSION (default latest),
# OHMYLAYA_INSTALL_ARGS (extra flags for `ohmylaya install`, default "--yes").
$ErrorActionPreference = 'Stop'
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

$repo = 'QuBiit0/ohmylaya'
$binDir = if ($env:OHMYLAYA_BIN_DIR) { $env:OHMYLAYA_BIN_DIR } else { Join-Path $env:LOCALAPPDATA 'ohmylaya\bin' }
$version = if ($env:OHMYLAYA_VERSION) { $env:OHMYLAYA_VERSION } else { 'latest' }
$installArgs = if ($env:OHMYLAYA_INSTALL_ARGS) { $env:OHMYLAYA_INSTALL_ARGS } else { '--yes' }

if (-not [Environment]::Is64BitOperatingSystem) { throw 'ohmylaya requires 64-bit Windows' }
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { throw 'Windows on ARM is not supported by the engine yet' } else { 'amd64' }

if ($version -eq 'latest') {
  $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$repo/releases/latest" -Headers @{ 'User-Agent' = 'ohmylaya-installer' }
  $tag = $release.tag_name
} else {
  $tag = $version
}
$ver = $tag.TrimStart('v')
$archive = "ohmylaya_${ver}_windows_${arch}.zip"
$base = "https://github.com/$repo/releases/download/$tag"

$tmp = Join-Path ([IO.Path]::GetTempPath()) ("ohmylaya-" + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $tmp | Out-Null
try {
  Write-Host "Downloading ohmylaya $tag for windows/$arch"
  Invoke-WebRequest -Uri "$base/$archive" -OutFile (Join-Path $tmp $archive) -UseBasicParsing
  Invoke-WebRequest -Uri "$base/checksums.txt" -OutFile (Join-Path $tmp 'checksums.txt') -UseBasicParsing

  $line = Get-Content (Join-Path $tmp 'checksums.txt') | Where-Object { $_ -match "\s$([regex]::Escape($archive))$" } | Select-Object -First 1
  if (-not $line) { throw "checksums.txt has no entry for $archive" }
  $expected = ($line -split '\s+')[0].ToLower()
  $actual = (Get-FileHash -Algorithm SHA256 (Join-Path $tmp $archive)).Hash.ToLower()
  if ($expected -ne $actual) { throw "checksum mismatch for ${archive}: expected $expected, got $actual" }

  Expand-Archive -Path (Join-Path $tmp $archive) -DestinationPath $tmp -Force
  New-Item -ItemType Directory -Path $binDir -Force | Out-Null
  Copy-Item (Join-Path $tmp 'ohmylaya.exe') (Join-Path $binDir 'ohmylaya.exe') -Force
  Write-Host "Installed $binDir\ohmylaya.exe"

  $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
  if (($userPath -split ';') -notcontains $binDir) {
    [Environment]::SetEnvironmentVariable('Path', "$binDir;$userPath", 'User')
    $env:Path = "$binDir;$env:Path"
    Write-Host "Added $binDir to your user PATH (open a new terminal to pick it up)"
  }
} finally {
  Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}

& (Join-Path $binDir 'ohmylaya.exe') install ($installArgs -split ' ')
exit $LASTEXITCODE
