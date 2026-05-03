# go-fim Windows Download Script
# Usage: iwr -useb https://raw.githubusercontent.com/Jimzical/go-fim/main/scripts/install.ps1 | iex
# Downloads go-fim.exe to the current directory

param([string]$Version = "latest")

$ErrorActionPreference = "Stop"
$Repo = "Jimzical/go-fim"

# Detect architecture
$arch = $null
try {
    if ((Get-WmiObject Win32_Processor).Architecture -eq 12) {
        $arch = "arm64"
    }
} catch {}

if (-not $arch) {
    if ([Environment]::Is64BitOperatingSystem) {
        $arch = "amd64"
    } else {
        Write-Host "Unsupported Windows architecture: 32-bit Windows is not supported. Available Windows releases are amd64 and arm64." -ForegroundColor Red
        exit 1
    }
}

# Get latest version if needed
if ($Version -eq "latest") {
    try {
        Invoke-WebRequest "https://github.com/$Repo/releases/latest" -MaximumRedirection 0 -EA Stop | Out-Null
    } catch {
        if ($_.Exception.Response.Headers.Location) {
            $Version = $_.Exception.Response.Headers.Location.ToString() -replace '.*/v', ''
        }
    }
    if ($Version -eq "latest") { Write-Host "Failed to fetch version" -ForegroundColor Red; exit 1 }
}

Write-Host "Downloading go-fim v$Version for windows_$arch..." -ForegroundColor Cyan

# Download and extract
$url = "https://github.com/$Repo/releases/download/v$Version/go-fim_${Version}_windows_${arch}.zip"
$zip = "$env:TEMP\go-fim-$([guid]::NewGuid()).zip"
$extractDir = "$env:TEMP\go-fim-extract-$([guid]::NewGuid())"

Invoke-WebRequest $url -OutFile $zip -UseBasicParsing

$checksumUrl = "https://github.com/$Repo/releases/download/v$Version/checksums.txt"
$checksumFile = "$env:TEMP\go-fim-checksums-$([guid]::NewGuid()).txt"
Invoke-WebRequest $checksumUrl -OutFile $checksumFile -UseBasicParsing

$archiveName = "go-fim_${Version}_windows_${arch}.zip"
$expected = (Get-Content $checksumFile | Where-Object {$_ -match $archiveName}) -replace '\s+.*', ''
if (-not $expected) {
    Write-Host "Failed to find checksum for $archiveName" -ForegroundColor Red
    exit 1
}
$actual = (Get-FileHash $zip -Algorithm SHA256).Hash.ToLower()
if ($expected -ne $actual) {
    Write-Host "Checksum mismatch! Expected: $expected, Actual: $actual" -ForegroundColor Red
    exit 1
}

Expand-Archive $zip -DestinationPath $extractDir -Force
Move-Item "$extractDir\go-fim.exe" ".\go-fim.exe" -Force
Remove-Item $zip, $extractDir -Recurse -Force -EA SilentlyContinue

Write-Host "Done! Downloaded: .\go-fim.exe" -ForegroundColor Green
