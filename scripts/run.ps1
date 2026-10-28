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
.\scripts\run.ps1 -KeepRunning          # for autostart
.\scripts\run.ps1 -Update               # download the latest voice-client.exe, then run
#>
# No positional parameters: a bare word such as "devices" is a voice-client
# command, not the value of -Env.
[CmdletBinding(PositionalBinding = $false)]
param(
    [Alias('Env')]
    [ValidateSet('dev', 'prod')]
    [string]$Environment = 'prod',
    # Start the client again 5 s after it stops (for autostart).
    [switch]$KeepRunning,
    # Do not start the Docker voice services.
    [switch]$NoServices,
    # Download voice-client.exe from the latest GitHub release first.
    [switch]$Update,
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$Arguments
)

# Not 'Stop': Windows PowerShell 5.1 then turns each stderr line of a program
# into an error, and voice-client logs to stderr. Failures throw explicitly.
$ErrorActionPreference = 'Continue'
# $PSScriptRoot is empty when the script does not run as a file (for example,
# pasted into the console); then the repository is the current folder.
$root = if ($PSScriptRoot) { Split-Path -Parent $PSScriptRoot } else { (Get-Location).Path }
if (-not (Test-Path (Join-Path $root 'client'))) {
    throw "$root is not the jarvis folder; run .\scripts\run.ps1 from the jarvis folder"
}
Set-Location $root

$exe = Join-Path $root 'client\bin\voice-client.exe'

# Downloads voice-client.exe and its SHA-256 from the latest release of the
# repository of "git remote origin", checks it, and replaces the old file.
function Update-Client {
    $remote = git -C $root remote get-url origin 2>$null
    if ($remote -notmatch 'github\.com[:/](?<repo>[^/]+/[^/]+?)(\.git)?$') {
        throw "the git remote ""$remote"" is not a GitHub repository; download voice-client.exe from its releases page"
    }
    $base = "https://github.com/$($Matches.repo)/releases/latest/download"
    # Windows PowerShell 5.1 may default to TLS 1.0, which GitHub refuses, and
    # its progress bar makes downloads very slow.
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
    $ProgressPreference = 'SilentlyContinue'
    New-Item -ItemType Directory -Force (Split-Path -Parent $exe) | Out-Null
    $download = "$exe.download"
    $sumFile = "$exe.sha256"
    Write-Host "downloading $base/voice-client.exe"
    try {
        Invoke-WebRequest "$base/voice-client.exe" -OutFile $download -UseBasicParsing -ErrorAction Stop
        Invoke-WebRequest "$base/voice-client.exe.sha256" -OutFile $sumFile -UseBasicParsing -ErrorAction Stop
    } catch {
        Remove-Item -ErrorAction SilentlyContinue $download
        throw "download failed: $_"
    }
    $want = ((Get-Content $sumFile -Raw).Trim() -split '\s+')[0]
    $got = (Get-FileHash $download -Algorithm SHA256).Hash
    Remove-Item $sumFile
    if ($got -ne $want) {
        Remove-Item $download
        throw "the downloaded voice-client.exe has SHA-256 $got, but the release says $want; nothing was replaced"
    }
    try {
        Move-Item -Force $download $exe -ErrorAction Stop
    } catch {
        Remove-Item -ErrorAction SilentlyContinue $download
        throw "cannot replace $exe; stop the assistant first (Ctrl+C, or Stop-ScheduledTask -TaskName 'Jarvis voice assistant')"
    }
    Write-Host "voice-client.exe updated: $(& $exe version)"
}

if ($Update) { Update-Client }
if (-not (Test-Path $exe)) {
    throw "$exe is missing; run .\scripts\run.ps1 -Update to download it"
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

function Get-LLMModel {
    $model = Join-Path $root 'server\models\llm\qwen2.5-0.5b-instruct-q4_k_m.gguf'
    if (Test-Path $model) { return }
    Write-Host 'downloading the language model (about 470 MB)...'
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
    $ProgressPreference = 'SilentlyContinue'
    New-Item -ItemType Directory -Force (Split-Path -Parent $model) | Out-Null
    $download = "$model.download"
    try {
        Invoke-WebRequest 'https://huggingface.co/Qwen/Qwen2.5-0.5B-Instruct-GGUF/resolve/main/qwen2.5-0.5b-instruct-q4_k_m.gguf' -OutFile $download -UseBasicParsing -ErrorAction Stop
        Move-Item -Force $download $model
    } catch {
        Remove-Item -ErrorAction SilentlyContinue $download
        throw "cannot download the language model: $_"
    }
}

if (-not $NoServices) {
    Wait-Docker
    Get-LLMModel
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
    # An empty value removes the variable, as "make run" treats it as not set.
    [Environment]::SetEnvironmentVariable($name.Trim(), $value, 'Process')
}

if (-not $Arguments) { $Arguments = @('run') }

while ($true) {
    & $exe @Arguments
    $code = $LASTEXITCODE
    if (-not $KeepRunning) { exit $code }
    Write-Host "voice-client stopped with code $code; starting again in 5 s"
    Start-Sleep -Seconds 5
}
