# go-fim Windows Download Script
# Usage: iwr -useb https://raw.githubusercontent.com/Jimzical/go-fim/main/scripts/install.ps1 | iex
# Downloads go-fim.exe to the current directory

param([string]$Version = "latest")

$ErrorActionPreference = "Stop"
$Repo = "Jimzical/go-fim"

# Detect architecture
$arch = if ([Environment]::Is64BitOperatingSystem) { "amd64" } else { "386" }
try {
    if ((Get-WmiObject Win32_Processor).Architecture -eq 12) { $arch = "arm64" }
} catch {}

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

Invoke-WebRequest $url -OutFile $zip -UseBasicParsing
Expand-Archive $zip -DestinationPath $env:TEMP\go-fim-extract -Force
Move-Item "$env:TEMP\go-fim-extract\go-fim.exe" ".\go-fim.exe" -Force
Remove-Item $zip, "$env:TEMP\go-fim-extract" -Recurse -Force -EA SilentlyContinue

Write-Host "Done! Downloaded: .\go-fim.exe" -ForegroundColor Green
