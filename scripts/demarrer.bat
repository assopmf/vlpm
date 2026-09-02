@echo off
REM VLPM - demarrage sur Windows. Double-cliquez ce fichier.
REM
REM Les donnees sont enregistrees dans le sous-dossier "data", a cote de
REM l'executable : deplacer le dossier deplace toute l'installation.

cd /d "%~dp0"

if exist "vlpm-windows-amd64.exe" (
    set BINAIRE=vlpm-windows-amd64.exe
) else if exist "vlpm.exe" (
    set BINAIRE=vlpm.exe
) else (
    echo Executable introuvable.
    echo Placez ce fichier a cote de vlpm-windows-amd64.exe
    pause
    exit /b 1
)

echo Demarrage de VLPM...
echo Laissez cette fenetre ouverte tant que l'application doit rester accessible.
echo.
echo Interface : http://localhost:8080
echo.

REM Ouvre le navigateur apres un court delai, le temps que le serveur ecoute.
start /b cmd /c "timeout /t 2 >nul & start http://localhost:8080"

"%BINAIRE%" --addr 0.0.0.0:8080 --data .\data

REM En cas d'arret immediat, la fenetre reste ouverte pour lire l'erreur.
if errorlevel 1 pause
