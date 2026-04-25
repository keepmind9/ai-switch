# ai-switch Windows Installation Script
# Downloads latest release from GitHub and installs to ~/.local/bin

$ErrorActionPreference = "Stop"

$Repo = "keepmind9/ai-switch"
$Binary = "ai-switch"
$InstallDir = "$env:USERPROFILE\.local\bin"

Write-Host "Checking ai-switch installation..."

# Get latest release info
Write-Host "Fetching latest release..."
$releaseInfo = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -TimeoutSec 30

if (-not $releaseInfo.tag_name) {
    Write-Host "No releases found. Install manually:" -ForegroundColor Red
    Write-Host "  https://github.com/$Repo/releases"
    exit 1
}

$latestVersion = $releaseInfo.tag_name

if (Get-Command $Binary -ErrorAction SilentlyContinue) {
    try {
        $currentOutput = & $Binary version 2>$null
        $currentVersion = ($currentOutput | Select-String "Version:\s+(\S+)").Matches.Groups[1].Value
        if ($currentVersion -eq $latestVersion.TrimStart("v")) {
            Write-Host "ai-switch is already up to date ($latestVersion)."
            exit 0
        }
        if ($currentVersion) {
            Write-Host "ai-switch $currentVersion installed, upgrading to $latestVersion..."
        } else {
            Write-Host "ai-switch installed, upgrading to $latestVersion..."
        }
    } catch {
        Write-Host "ai-switch installed but broken, reinstalling $latestVersion..."
    }
} else {
    Write-Host "ai-switch not found. Installing $latestVersion..."
}

# Check if ai-switch is currently running (cannot replace a running binary)
$runningProcess = Get-Process -Name $Binary -ErrorAction SilentlyContinue
if ($runningProcess) {
    Write-Host ""
    Write-Host "Error: ai-switch is currently running and cannot be replaced." -ForegroundColor Red
    Write-Host ""
    Write-Host "Please stop it first, then re-run this script:" -ForegroundColor Yellow
    Write-Host "  ai-switch stop"
    Write-Host ""
    Write-Host "Or close the process manually:" -ForegroundColor Yellow
    Write-Host "  Stop-Process -Name `"$Binary`" -Force"
    exit 1
}

# Find matching asset for windows-amd64
try {
    $version = $releaseInfo.tag_name
    $asset = $releaseInfo.assets | Where-Object { $_.name -like "*windows-amd64*" } | Select-Object -First 1

    if (-not $asset) {
        Write-Host "No matching release found for windows-amd64." -ForegroundColor Red
        Write-Host "Available assets:"
        $releaseInfo.assets | ForEach-Object { Write-Host "  $($_.name)" }
        exit 1
    }

    Write-Host "Downloading ai-switch $version for Windows..."

    $tmpDir = [System.IO.Path]::GetTempPath() + "ai-switch-install"
    New-Item -ItemType Directory -Path $tmpDir -Force | Out-Null

    $downloadPath = Join-Path $tmpDir $asset.name
    Invoke-WebRequest -Uri $asset.browser_download_url -OutFile $downloadPath -TimeoutSec 120

    Write-Host "Extracting..."

    if ($asset.name -like "*.zip") {
        Expand-Archive -Path $downloadPath -DestinationPath $tmpDir -Force
    } else {
        Write-Host "Unknown archive format: $($asset.name)" -ForegroundColor Red
        exit 1
    }

    # Find the binary
    $binaryPath = Get-ChildItem -Path $tmpDir -Recurse -Filter "$Binary.exe" | Select-Object -First 1

    if (-not $binaryPath) {
        Write-Host "Binary not found in archive." -ForegroundColor Red
        exit 1
    }

    # Install
    if (-not (Test-Path $InstallDir)) {
        New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    }

    Move-Item $binaryPath.FullName "$InstallDir\$Binary.exe" -Force

    # Clean up
    Remove-Item $tmpDir -Recurse -Force

    # Add to PATH if needed
    $pathEnv = [Environment]::GetEnvironmentVariable("PATH", "User")
    if ($pathEnv -notlike "*$InstallDir*") {
        Write-Host "Adding $InstallDir to user PATH..."
        $newPath = $pathEnv + ";$InstallDir"
        [Environment]::SetEnvironmentVariable("PATH", $newPath, "User")
        Write-Host "Added to PATH. Restart your shell for changes to take effect."
    }

    Write-Host ""
    Write-Host "ai-switch $version installed successfully!"
    Write-Host "  Location: $InstallDir\$Binary.exe"
    Write-Host ""
    Write-Host "Verify:"
    Write-Host "  ai-switch version"

} catch {
    Write-Host "Installation failed: $_" -ForegroundColor Red
    Write-Host "Install manually: https://github.com/$Repo/releases"
    exit 1
}
