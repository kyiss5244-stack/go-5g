param(
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"
$projectRoot = $PSScriptRoot
$goExe = Join-Path $projectRoot ".tools\go\bin\go.exe"
$runDir = Join-Path $projectRoot ".run"
$binDir = Join-Path $runDir "bin"
$logDir = Join-Path $runDir "logs"
$pidFile = Join-Path $runDir "pids.json"

if (-not (Test-Path -LiteralPath $goExe)) {
    throw "Portable Go was not found at $goExe. Read WINDOWS-BEGINNER-GUIDE.md first."
}

# Windows PowerShell can apply filesystem policy relative to the caller's
# current directory. Work from the project root so an absolute invocation
# behaves exactly like an invocation made inside the repository.
Set-Location -LiteralPath $projectRoot

$env:GOCACHE = Join-Path $projectRoot ".cache\go-build"
$env:GOPATH = Join-Path $projectRoot ".cache\gopath"
New-Item -ItemType Directory -Force $binDir, $logDir | Out-Null

function Test-LocalPort([int]$Port) {
    $client = [System.Net.Sockets.TcpClient]::new()
    try {
        $client.Connect("127.0.0.1", $Port)
        return $true
    }
    catch {
        return $false
    }
    finally {
        $client.Dispose()
    }
}

function Wait-LocalPort([int]$Port) {
    for ($attempt = 0; $attempt -lt 40; $attempt++) {
        if (Test-LocalPort $Port) {
            return
        }
        Start-Sleep -Milliseconds 250
    }
    throw "Timed out waiting for port $Port. Check .run\logs."
}

if (-not $SkipBuild) {
    Write-Host "[1/4] Building UPF, SMF, AMF and UE..."
    & $goExe -C $projectRoot build -o (Join-Path $binDir "upf.exe") ./upf
    if ($LASTEXITCODE -ne 0) { throw "UPF build failed" }
    & $goExe -C $projectRoot build -o (Join-Path $binDir "smf.exe") ./smf
    if ($LASTEXITCODE -ne 0) { throw "SMF build failed" }
    & $goExe -C $projectRoot build -o (Join-Path $binDir "amf.exe") ./amf
    if ($LASTEXITCODE -ne 0) { throw "AMF build failed" }
    & $goExe -C $projectRoot build -o (Join-Path $binDir "ue.exe") ./ue
    if ($LASTEXITCODE -ne 0) { throw "UE build failed" }
}

$services = @(
    [PSCustomObject]@{ Name = "upf"; Port = 50053 },
    [PSCustomObject]@{ Name = "smf"; Port = 50052 },
    [PSCustomObject]@{ Name = "amf"; Port = 50051 }
)
$started = [ordered]@{}

try {
    foreach ($service in $services) {
        if (Test-LocalPort $service.Port) {
            throw "Port $($service.Port) is already in use. If LiteCore is running, execute .\stop-windows.ps1 first."
        }

        Write-Host "Starting $($service.Name.ToUpper()) on port $($service.Port)..."
        $process = Start-Process `
            -FilePath (Join-Path $binDir "$($service.Name).exe") `
            -WorkingDirectory $projectRoot `
            -WindowStyle Hidden `
            -RedirectStandardOutput (Join-Path $logDir "$($service.Name).out.log") `
            -RedirectStandardError (Join-Path $logDir "$($service.Name).err.log") `
            -PassThru
        $started[$service.Name] = $process.Id
        Wait-LocalPort $service.Port
    }

    $started | ConvertTo-Json | Set-Content -LiteralPath $pidFile -Encoding UTF8
}
catch {
    foreach ($processId in $started.Values) {
        Stop-Process -Id $processId -ErrorAction SilentlyContinue
    }
    throw
}

Write-Host "[Done] LiteCore is running: AMF=50051, SMF=50052, UPF=50053"
Write-Host "Next: .\demo-windows.ps1"
Write-Host "Stop: .\stop-windows.ps1"
