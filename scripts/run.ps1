<#
.SYNOPSIS
Runs the voice assistant on Windows: the "make run" of the mini PC.

.DESCRIPTION
Waits for Docker, starts the voice services (server\compose.yaml), loads
client\.env.<Env> and runs client\bin\voice-client.exe. As with "make run", the
VOICE_* variables of the system are removed first, so the settings come only
from the env file.

.EXAMPLE
.\scripts\run.ps1                      # the assistant with client\.env.prod
.\scripts\run.ps1 devices              # list microphones and speakers
.\scripts\run.ps1 wake-test            # any voice-client command and its flags
.\scripts\run.ps1 -KeepRunning -LogFile logs\voice-client.log   # for autostart
#>
param(
    [Alias('Env')]
    [ValidateSet('dev', 'prod')]
    [string]$Environment = 'prod',
    # Start the client again 5 s after it stops (for autostart).
    [switch]$KeepRunning,
    # Append the client log to this file instead of the console.
    [string]$LogFile,
    # Do not start the Docker voice services.
    [switch]$NoServices,
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$Arguments
)

# Not 'Stop': Windows PowerShell 5.1 then turns each stderr line of a program
# into an error, and voice-client logs to stderr. Failures throw explicitly.
$ErrorActionPreference = 'Continue'
# voice-client writes UTF-8; without this, Spanish accents break in the log file.
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

$exe = Join-Path $root 'client\bin\voice-client.exe'
if (-not (Test-Path $exe)) {
    throw "$exe is missing; build it on the Mac with ""make windows"" and copy it here"
}
$envFile = Join-Path $root "client\.env.$Environment"
if (-not (Test-Path $envFile)) {
    throw "$envFile is missing; copy it from client\.env.prod.example and fill it in"
}

function Wait-Docker {
    $deadline = (Get-Date).AddMinutes(5)
    while ($true) {
        docker info *> $null
        if ($LASTEXITCODE -eq 0) { return }
        if ((Get-Date) -gt $deadline) { throw 'Docker did not start within 5 minutes; start Docker Desktop' }
        Write-Host 'waiting for Docker...'
        Start-Sleep -Seconds 5
    }
}

if (-not $NoServices) {
    Wait-Docker
    Push-Location (Join-Path $root 'server')
    try {
        docker compose up -d --wait
        if ($LASTEXITCODE -ne 0) { throw 'docker compose up failed; see "docker compose logs" in server' }
    } finally {
        Pop-Location
    }
}

# The env file is the whole configuration: KEY=VALUE lines, # comments, and
# optional quotes around the value.
Get-ChildItem Env: | Where-Object Name -like 'VOICE_*' | ForEach-Object { Remove-Item "Env:$($_.Name)" }
foreach ($line in Get-Content $envFile) {
    $line = $line.Trim()
    if ($line -eq '' -or $line.StartsWith('#')) { continue }
    $name, $value = $line -split '=', 2
    if ($null -eq $value) { throw "${envFile}: line ""$line"" is not KEY=VALUE" }
    $value = $value.Trim()
    if ($value.Length -ge 2 -and $value[0] -eq $value[-1] -and '"', "'" -contains $value[0]) {
        $value = $value.Substring(1, $value.Length - 2)
    }
    Set-Item "Env:$($name.Trim())" $value
}

if (-not $Arguments) { $Arguments = @('run') }
if ($LogFile) { New-Item -ItemType Directory -Force (Split-Path -Parent $LogFile) | Out-Null }

while ($true) {
    if ($LogFile) {
        & $exe @Arguments 2>&1 | ForEach-Object { "$_" } | Add-Content -Path $LogFile -Encoding UTF8
    } else {
        & $exe @Arguments
    }
    $code = $LASTEXITCODE
    if (-not $KeepRunning) { exit $code }
    Write-Host "voice-client stopped with code $code; starting again in 5 s"
    Start-Sleep -Seconds 5
}
