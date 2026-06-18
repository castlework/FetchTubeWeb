# FetchTubeWeb Release Build Script
# Usage: .\build_release.ps1
# Output: .\dist\

$ErrorActionPreference = "Stop"
$goDir = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $goDir
$releaseDir = Join-Path $goDir "dist"

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  FetchTubeWeb Release Build" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

# 1. Build Go binary
Write-Host "[1/4] Building Go binary..." -ForegroundColor Yellow
go build -ldflags="-s -w" -o FetchTubeWeb.exe .
if ($LASTEXITCODE -ne 0) {
    Write-Host "ERROR: Build failed" -ForegroundColor Red
    exit 1
}
Write-Host "  [OK] FetchTubeWeb.exe built" -ForegroundColor Green

# 2. Create release directory
Write-Host "[2/4] Creating release directory: $releaseDir" -ForegroundColor Yellow
if (Test-Path $releaseDir) {
    Remove-Item -Recurse -Force $releaseDir
}
New-Item -ItemType Directory -Force $releaseDir | Out-Null
Write-Host "  [OK] Directory created" -ForegroundColor Green

# 3. Copy main binary
Write-Host "[3/4] Copying files..." -ForegroundColor Yellow
Copy-Item FetchTubeWeb.exe $releaseDir\
Write-Host "  [OK] FetchTubeWeb.exe" -ForegroundColor Green

# 4. Find and copy dependency executables
Write-Host "[4/4] Finding dependencies..." -ForegroundColor Yellow

$missing = @()

$deps = @{
    "yt-dlp.exe" = "yt-dlp (video download engine)"
    "ffmpeg.exe" = "ffmpeg (audio/video merge)"
    "node.exe"   = "node   (JS runtime for YouTube)"
}

foreach ($exe in $deps.Keys) {
    $desc = $deps[$exe]
    $found = $null

    # Search: current directory first, then PATH
    $localPath = Join-Path $goDir $exe
    if (Test-Path $localPath) {
        $found = $localPath
    } else {
        $pathExe = (Get-Command $exe -ErrorAction SilentlyContinue).Source
        if ($pathExe) {
            $found = $pathExe
        }
    }

    if ($found) {
        Copy-Item $found $releaseDir\
        $sizeMB = (Get-Item $found).Length / 1MB
        Write-Host "  [OK] $exe <- $found ($([int]$sizeMB) MB)" -ForegroundColor Green

        # yt-dlp.exe from pip is only a launcher (~0.1MB), needs Python installed.
        # Standalone yt-dlp.exe from GitHub is ~15MB, bundles its own Python.
        if ($exe -eq "yt-dlp.exe" -and $sizeMB -lt 1) {
            Write-Host "       WARNING: This yt-dlp.exe is the pip launcher (requires Python)." -ForegroundColor Yellow
            Write-Host "       For distribution, download the standalone exe from:" -ForegroundColor Yellow
            Write-Host "       https://github.com/yt-dlp/yt-dlp/releases" -ForegroundColor Yellow
        }
    } else {
        $missing += "  - $exe  $desc"
        Write-Host "  [MISSING] $exe" -ForegroundColor Red
    }
}

Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  Release: $releaseDir" -ForegroundColor White
Write-Host "========================================" -ForegroundColor Cyan
Get-ChildItem $releaseDir | ForEach-Object {
    $size = "{0:N1} MB" -f ($_.Length / 1MB)
    Write-Host "  $($_.Name)  ($size)"
}

if ($missing.Count -gt 0) {
    Write-Host ""
    Write-Host "WARNING: Missing dependencies, add manually:" -ForegroundColor Yellow
    foreach ($m in $missing) {
        Write-Host $m -ForegroundColor Yellow
    }
    Write-Host ""
    Write-Host "Download links:" -ForegroundColor White
    Write-Host "  yt-dlp : https://github.com/yt-dlp/yt-dlp/releases" -ForegroundColor Gray
    Write-Host "  ffmpeg : https://www.gyan.dev/ffmpeg/builds/ (essentials build)" -ForegroundColor Gray
    Write-Host "  node   : https://nodejs.org/dist/v22.11.0/win-x64/node.exe" -ForegroundColor Gray
}

Write-Host ""
Write-Host "Done. Zip the dist folder to distribute." -ForegroundColor Cyan
