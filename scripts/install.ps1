Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Normalize-Version {
    param([string]$Version)
    if ([string]::IsNullOrWhiteSpace($Version)) {
        return ""
    }
    if ($Version.StartsWith("v")) {
        return $Version
    }
    return "v$Version"
}

function Get-LatestReleaseTag {
    param([string]$Repo)
    $apiUrl = "https://api.github.com/repos/$Repo/releases/latest"
    $response = Invoke-RestMethod -Uri $apiUrl
    if (-not $response.tag_name) {
        throw "Failed to resolve latest release tag from $apiUrl"
    }
    return [string]$response.tag_name
}

function Resolve-Architecture {
    $arch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
    switch ($arch) {
        "X64" { return "amd64" }
        "Arm64" { return "arm64" }
        default { throw "Unsupported architecture: $arch" }
    }
}

function Get-ExpectedChecksum {
    param(
        [string]$ChecksumPath,
        [string]$AssetName
    )
    $line = Get-Content -Path $ChecksumPath | Where-Object { $_ -match (" {2}" + [regex]::Escape($AssetName) + "$") } | Select-Object -First 1
    if (-not $line) {
        throw "Checksum entry not found for $AssetName"
    }
    return ([string]($line -split "\s+")[0]).ToLowerInvariant()
}

$repo = if ($env:BYTEMIND_REPO) { $env:BYTEMIND_REPO } else { "1024XEngineer/bytemind" }
$version = Normalize-Version -Version $env:BYTEMIND_VERSION
if ([string]::IsNullOrWhiteSpace($version)) {
    $version = Get-LatestReleaseTag -Repo $repo
}

$installDir = if ($env:BYTEMIND_INSTALL_DIR) { $env:BYTEMIND_INSTALL_DIR } else { Join-Path $HOME ".bytemind\bin" }
$archName = Resolve-Architecture
$assetName = "bytemind_${version}_windows_${archName}.zip"
$baseUrl = "https://github.com/$repo/releases/download/$version"

$tmpRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("bytemind-install-" + [Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $tmpRoot | Out-Null

try {
    $assetPath = Join-Path $tmpRoot $assetName
    $checksumPath = Join-Path $tmpRoot "checksums.txt"

    Write-Output "Downloading $assetName"
    Invoke-WebRequest -Uri "$baseUrl/$assetName" -OutFile $assetPath
    Invoke-WebRequest -Uri "$baseUrl/checksums.txt" -OutFile $checksumPath

    $expectedHash = Get-ExpectedChecksum -ChecksumPath $checksumPath -AssetName $assetName
    $actualHash = (Get-FileHash -Path $assetPath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actualHash -ne $expectedHash) {
        throw "Checksum verification failed for $assetName`nExpected: $expectedHash`nActual:   $actualHash"
    }

    Expand-Archive -Path $assetPath -DestinationPath $tmpRoot -Force

    $binaryPath = Join-Path $tmpRoot "bytemind_${version}_windows_${archName}\bytemind.exe"
    if (-not (Test-Path -LiteralPath $binaryPath)) {
        throw "Binary not found in archive: $assetName"
    }

    & $binaryPath install -to $installDir
    if ($LASTEXITCODE -ne 0) {
        throw "bytemind install failed with exit code $LASTEXITCODE"
    }

    Write-Output ""
    Write-Output "Bytemind is installed."
    Write-Output "Open a new terminal and run: bytemind chat"
}
finally {
    Remove-Item -LiteralPath $tmpRoot -Recurse -Force -ErrorAction SilentlyContinue
}
