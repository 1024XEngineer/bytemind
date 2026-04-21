param(
    [switch]$SkipFull
)

$ErrorActionPreference = "Stop"

$root = Get-Location
$env:GOCACHE = Join-Path $root ".gocache"

Write-Host "==> Running focused sandbox suites..."
go test ./internal/tools ./internal/app ./internal/agent ./internal/sandbox ./internal/config -count=1 -timeout 300s

if (-not $SkipFull) {
    Write-Host "==> Running full test suite..."
    go test ./... -count=1 -timeout 300s
}

Write-Host "Sandbox acceptance checks passed."

