$ErrorActionPreference = "Stop"
$projectRoot = $PSScriptRoot
$runDir = Join-Path $projectRoot ".run"
$binDir = Join-Path $runDir "bin"
$pidFile = Join-Path $runDir "pids.json"

if (-not (Test-Path -LiteralPath $pidFile)) {
    Write-Host "No PID record was found; LiteCore may already be stopped."
    exit 0
}

$recorded = Get-Content -LiteralPath $pidFile -Raw | ConvertFrom-Json
$expectedPaths = @{
    upf = Join-Path $binDir "upf.exe"
    smf = Join-Path $binDir "smf.exe"
    amf = Join-Path $binDir "amf.exe"
}

foreach ($name in @("amf", "smf", "upf")) {
    $processId = [int]$recorded.$name
    $process = Get-Process -Id $processId -ErrorAction SilentlyContinue
    if ($null -eq $process) {
        continue
    }

    $actualPath = [System.IO.Path]::GetFullPath($process.Path)
    $expectedPath = [System.IO.Path]::GetFullPath($expectedPaths[$name])
    if ($actualPath -ne $expectedPath) {
        Write-Warning "PID $processId belongs to another executable and was not stopped."
        continue
    }

    Stop-Process -Id $processId
    Write-Host "Stopped $($name.ToUpper()) (PID $processId)"
}

Remove-Item -LiteralPath $pidFile
Write-Host "LiteCore is stopped. In-memory UEs, sessions and rules are cleared."
