param(
    [int]$Count = 100,
    [int]$Concurrency = 20,
    [string]$Scenario = "mixed"
)

$ErrorActionPreference = "Stop"
$projectRoot = $PSScriptRoot
$ueExe = Join-Path $projectRoot ".run\bin\ue.exe"
$resultDir = Join-Path $projectRoot "results"

if (-not (Test-Path -LiteralPath $ueExe)) {
    throw "UE executable was not found. Run .\start-windows.ps1 first."
}

$client = [System.Net.Sockets.TcpClient]::new()
try {
    $client.Connect("127.0.0.1", 50051)
}
catch {
    throw "AMF is not responding on port 50051. Run .\start-windows.ps1 first."
}
finally {
    $client.Dispose()
}

Write-Host "`n[1/6] Stable-channel registration (expected: success)"
& $ueExe -action register -id UE-DEMO -scenario stable
if ($LASTEXITCODE -ne 0) { throw "Stable-channel registration failed" }

Write-Host "`n[2/6] Duplicate registration (expected: idempotent response)"
& $ueExe -action register -id UE-DEMO -scenario stable
if ($LASTEXITCODE -ne 0) { throw "Duplicate registration failed" }

Write-Host "`n[3/6] Query UE status (expected: REGISTERED with session and IP)"
& $ueExe -action get -id UE-DEMO
if ($LASTEXITCODE -ne 0) { throw "UE status query failed" }

Write-Host "`n[4/6] Deregistration (expected: resources released)"
& $ueExe -action deregister -id UE-DEMO
if ($LASTEXITCODE -ne 0) { throw "Deregistration failed" }

Write-Host "`n[5/6] Degrading-channel registration (expected: rejection)"
& $ueExe -action register -id UE-BAD -scenario degrading
if ($LASTEXITCODE -eq 0) { throw "The degrading-channel request unexpectedly succeeded" }
Write-Host "The registration failure above is the expected result."

New-Item -ItemType Directory -Force $resultDir | Out-Null
$timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
$csvPath = Join-Path $resultDir "benchmark-$timestamp.csv"
Write-Host "`n[6/6] Benchmark: count=$Count, concurrency=$Concurrency, scenario=$Scenario"
& $ueExe -action benchmark -count $Count -concurrency $Concurrency -scenario $Scenario -seed 42 -output $csvPath
if ($LASTEXITCODE -ne 0) { throw "Benchmark failed" }

Write-Host "`n[Done] CSV details: $csvPath"
Write-Host "Logs: Get-Content .\.run\logs\amf.err.log -Tail 20"
Write-Host "Stop after the demo: .\stop-windows.ps1"
