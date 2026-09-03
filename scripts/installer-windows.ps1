# VLPM — installation en service sur Windows, via le Planificateur de tâches.
#
# La tâche démarre au boot de la machine et relance le serveur s'il s'arrête.
#
# Usage, dans un PowerShell lancé EN ADMINISTRATEUR :
#   .\installer-windows.ps1
#   .\installer-windows.ps1 -Desinstaller
#
# Windows n'a pas d'équivalent direct de systemd. Le Planificateur suffit ici :
# il sait démarrer au boot et relancer en cas d'échec.

param(
    [switch]$Desinstaller,
    [int]$Port = 8080
)

$ErrorActionPreference = 'Stop'
$NomTache = 'VLPM'

function Test-Administrateur {
    $identite = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = New-Object Security.Principal.WindowsPrincipal($identite)
    return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

if (-not (Test-Administrateur)) {
    Write-Error "Lancez ce script depuis un PowerShell ouvert en tant qu'administrateur."
    exit 1
}

if ($Desinstaller) {
    Unregister-ScheduledTask -TaskName $NomTache -Confirm:$false -ErrorAction SilentlyContinue
    Write-Host ""
    Write-Host "Service desinstalle. Les donnees sont conservees dans :"
    Write-Host "  C:\ProgramData\VLPM"
    exit 0
}

# Le binaire est copie a un emplacement stable : le Planificateur relance un
# chemin absolu, pas le dossier depuis lequel on a lance ce script.
$Source = Join-Path (Split-Path -Parent $PSScriptRoot) 'vlpm-windows-amd64.exe'
if (-not (Test-Path $Source)) {
    $Source = Join-Path (Split-Path -Parent $PSScriptRoot) 'vlpm.exe'
}
if (-not (Test-Path $Source)) {
    Write-Error "Executable introuvable. Placez vlpm-windows-amd64.exe a la racine du dossier."
    exit 1
}

$Dossier = 'C:\Program Files\VLPM'
$Donnees = 'C:\ProgramData\VLPM'
New-Item -ItemType Directory -Force -Path $Dossier, $Donnees | Out-Null
Copy-Item $Source (Join-Path $Dossier 'vlpm.exe') -Force

$Action = New-ScheduledTaskAction `
    -Execute (Join-Path $Dossier 'vlpm.exe') `
    -Argument "--addr 0.0.0.0:$Port --data `"$Donnees`"" `
    -WorkingDirectory $Donnees

# Demarrage au boot, sans attendre l'ouverture d'une session.
$Declencheur = New-ScheduledTaskTrigger -AtStartup

# SYSTEM : la tache tourne sans qu'aucun utilisateur soit connecte.
$Compte = New-ScheduledTaskPrincipal -UserId 'SYSTEM' -LogonType ServiceAccount -RunLevel Highest

# RestartCount / RestartInterval : relance apres un plantage.
# ExecutionTimeLimit 0 : un serveur ne doit jamais etre arrete pour depassement
# de duree, c'est la valeur par defaut qui poserait probleme ici.
$Reglages = New-ScheduledTaskSettingsSet `
    -AllowStartIfOnBatteries `
    -DontStopIfGoingOnBatteries `
    -StartWhenAvailable `
    -RestartCount 999 `
    -RestartInterval (New-TimeSpan -Minutes 1) `
    -ExecutionTimeLimit (New-TimeSpan -Seconds 0) `
    -MultipleInstances IgnoreNew

Unregister-ScheduledTask -TaskName $NomTache -Confirm:$false -ErrorAction SilentlyContinue
Register-ScheduledTask -TaskName $NomTache -Action $Action -Trigger $Declencheur `
    -Principal $Compte -Settings $Reglages `
    -Description 'VLPM - gestion du parc automobile de la police municipale' | Out-Null

Start-ScheduledTask -TaskName $NomTache
Start-Sleep -Seconds 3

Write-Host ""
Write-Host "Service installe et demarre."
Write-Host ""
Write-Host "  Interface : http://localhost:$Port"
Write-Host "  Donnees   : $Donnees"
Write-Host ""
Write-Host "  Etat        : Get-ScheduledTask -TaskName $NomTache"
Write-Host "  Arret       : Stop-ScheduledTask -TaskName $NomTache"
Write-Host "  Desinstaller: .\installer-windows.ps1 -Desinstaller"
Write-Host ""
Write-Host "Le pare-feu Windows demandera peut-etre l'autorisation d'ecouter sur le port $Port."
