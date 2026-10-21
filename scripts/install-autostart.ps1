<#
.SYNOPSIS
Starts the voice assistant when you log in to Windows.

.DESCRIPTION
Registers the scheduled task "Jarvis voice assistant", which runs
scripts\run.ps1 -KeepRunning at log-on, in a hidden window. The log is only
in memory: set VOICE_LOG_HTTP_ADDR in client\.env.prod to read it over the
network. It runs in your session, because a Windows service has
no access to the microphone. Turn on the automatic log-on of Windows and the
"Start Docker Desktop when you sign in" setting.

.EXAMPLE
.\scripts\install-autostart.ps1              # install or update the task
.\scripts\install-autostart.ps1 -Uninstall   # remove it
#>
param([switch]$Uninstall)

$ErrorActionPreference = 'Stop'
$name = 'Jarvis voice assistant'

if ($Uninstall) {
    Unregister-ScheduledTask -TaskName $name -Confirm:$false
    Write-Host "removed the task ""$name"""
    exit 0
}

$root = Split-Path -Parent $PSScriptRoot
$script = Join-Path $root 'scripts\run.ps1'
$action = New-ScheduledTaskAction -Execute 'powershell.exe' -WorkingDirectory $root `
    -Argument "-NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File ""$script"" -KeepRunning"
$trigger = New-ScheduledTaskTrigger -AtLogOn -User $env:USERNAME
$settings = New-ScheduledTaskSettingsSet -ExecutionTimeLimit ([TimeSpan]::Zero) -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries
Register-ScheduledTask -TaskName $name -Action $action -Trigger $trigger -Settings $settings -Force | Out-Null
Write-Host "installed the task ""$name""; it starts at the next log-on, or now with:"
Write-Host "  Start-ScheduledTask -TaskName '$name'"
