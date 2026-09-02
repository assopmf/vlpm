#!/bin/sh
# VLPM — démarrage sans installation, sur macOS ou Linux.
# Double-cliquez ce fichier, ou lancez-le depuis un terminal.
#
# Les données sont enregistrées dans le sous-dossier « data », à côté du
# binaire : déplacer le dossier déplace toute l'installation.

set -eu
cd "$(dirname "$0")"

SYSTEME=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$(uname -m)" in
    x86_64|amd64) ARCH=amd64 ;;
    arm64|aarch64) ARCH=arm64 ;;
    armv7l|armv6l) ARCH=arm ;;
    *) echo "Architecture non reconnue : $(uname -m)" >&2; exit 1 ;;
esac

BINAIRE="./vlpm-$SYSTEME-$ARCH"
[ -x "$BINAIRE" ] || BINAIRE=./vlpm

if [ ! -x "$BINAIRE" ]; then
    echo "Exécutable introuvable pour $SYSTEME/$ARCH." >&2
    echo "Placez ce script à côté du binaire vlpm correspondant à votre machine." >&2
    exit 1
fi

echo "Démarrage de VLPM…"
echo "Laissez cette fenêtre ouverte tant que l'application doit rester accessible."
echo ""
exec "$BINAIRE" --addr 0.0.0.0:8080 --data ./data
