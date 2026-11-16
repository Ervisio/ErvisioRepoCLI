# Installs eg, the ErvisioRepo CLI, from the GitHub releases.
#
#   irm https://raw.githubusercontent.com/Ervisio/ErvisioRepoCLI/main/install.ps1 | iex
#
# Environment:
#   EG_VERSION      version to install (default: latest release), e.g. 0.1.0
#   EG_INSTALL_DIR  target directory (default: %LOCALAPPDATA%\Programs\eg)
$ErrorActionPreference = 'Stop'

$repo = 'Ervisio/ErvisioRepoCLI'
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }

$version = $env:EG_VERSION
if (-not $version) {
  $version = (Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest").tag_name
}
$version = $version.TrimStart('v')

$name = "eg_${version}_windows_$arch"
$base = "https://github.com/$repo/releases/download/v$version"
$tmp = Join-Path ([IO.Path]::GetTempPath()) ([Guid]::NewGuid())
New-Item -ItemType Directory $tmp | Out-Null
try {
  Write-Host "Downloading eg $version for windows/$arch..."
  Invoke-WebRequest "$base/$name.zip" -OutFile "$tmp\$name.zip"
  Invoke-WebRequest "$base/checksums.txt" -OutFile "$tmp\checksums.txt"

  $line = Get-Content "$tmp\checksums.txt" | Where-Object { $_ -match " $name\.zip$" }
  if (-not $line) { throw "no checksum for $name.zip" }
  $want = ($line -split ' ')[0]
  $got = (Get-FileHash "$tmp\$name.zip" -Algorithm SHA256).Hash.ToLower()
  if ($want -ne $got) { throw "checksum mismatch for $name.zip" }

  Expand-Archive "$tmp\$name.zip" -DestinationPath $tmp
  $dir = if ($env:EG_INSTALL_DIR) { $env:EG_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\eg' }
  New-Item -ItemType Directory -Force $dir | Out-Null
  Copy-Item "$tmp\$name\eg.exe" "$dir\eg.exe" -Force

  $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
  if (($userPath -split ';') -notcontains $dir) {
    [Environment]::SetEnvironmentVariable('Path', "$userPath;$dir", 'User')
    Write-Host "Added $dir to your PATH (open a new terminal)."
  }
  Write-Host "Installed $(& "$dir\eg.exe" --version | Select-Object -First 1) to $dir\eg.exe"
  Write-Host 'Next: eg auth login --hostname <your-ervisio-host>'
} finally {
  Remove-Item -Recurse -Force $tmp
}
