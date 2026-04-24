param(
    [switch]$SkipFull
)

$ErrorActionPreference = "Stop"

$root = Get-Location
$env:GOCACHE = Join-Path $root ".gocache"
$env:APPDATA = Join-Path $root ".appdata"
$env:LOCALAPPDATA = Join-Path $root ".localappdata"
New-Item -ItemType Directory -Force -Path $env:GOCACHE, $env:APPDATA, $env:LOCALAPPDATA | Out-Null

function Invoke-GoTestChecked {
    param(
        [Parameter(Mandatory = $true)]
        [string[]]$Args
    )

    & go test @Args
    if ($LASTEXITCODE -ne 0) {
        throw "go test failed with exit code $LASTEXITCODE"
    }
}

function Invoke-FocusedPackage {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Package
    )

    & go test $Package -count=1 -timeout 300s
    $firstExitCode = $LASTEXITCODE
    if ($firstExitCode -eq 0) {
        return
    }

    Write-Host "!! Focused sandbox suite failed for $Package. Re-running with -v for diagnostics..."
    & go test $Package -count=1 -timeout 300s -v
    $rerunExitCode = $LASTEXITCODE
    if ($rerunExitCode -eq 0) {
        Write-Host "!! Focused sandbox suite for $Package passed on retry (-v). Continuing."
        return
    }

    throw "focused sandbox suite failed: $Package (first exit code $firstExitCode, rerun exit code $rerunExitCode)"
}

Write-Host "==> Running focused sandbox suites..."
@(
    "./internal/tools",
    "./internal/app",
    "./internal/agent",
    "./internal/sandbox",
    "./internal/config"
) | ForEach-Object {
    Invoke-FocusedPackage -Package $_
}

if (-not $SkipFull) {
    Write-Host "==> Running full test suite..."
    Invoke-GoTestChecked -Args @(
        "./...",
        "-count=1",
        "-timeout",
        "300s"
    )
}

Write-Host "Sandbox acceptance checks passed."
