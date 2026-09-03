#!/bin/bash
# VLPM — installation en service sur macOS, via launchd.
#
# Le service redémarre tout seul s'il plante, et repart après un redémarrage
# de la machine.
#
# Deux modes :
#   ./installer-macos.sh                 service utilisateur, démarre à l'ouverture
#                                        de session (aucun mot de passe requis)
#   sudo ./installer-macos.sh --systeme  service système, démarre au boot même
#                                        sans session ouverte
#
# Désinstallation :  ./installer-macos.sh --desinstaller  (ajouter sudo si --systeme)

set -euo pipefail

ETIQUETTE=fr.vlpm.serveur
PORT=${VLPM_PORT:-8080}

SYSTEME=0
DESINSTALLER=0
for arg in "$@"; do
    case "$arg" in
        --systeme) SYSTEME=1 ;;
        --desinstaller) DESINSTALLER=1 ;;
        *) echo "Option inconnue : $arg" >&2; exit 1 ;;
    esac
done

if [ "$SYSTEME" = 1 ]; then
    PLIST="/Library/LaunchDaemons/$ETIQUETTE.plist"
    DOMAINE="system"
    if [ "$(id -u)" -ne 0 ]; then
        echo "Le mode --systeme demande sudo :" >&2
        echo "  sudo $0 --systeme" >&2
        exit 1
    fi
    DOSSIER_DONNEES=/usr/local/var/vlpm
    UTILISATEUR_SERVICE=$(stat -f '%Su' /dev/console)
else
    PLIST="$HOME/Library/LaunchAgents/$ETIQUETTE.plist"
    DOMAINE="gui/$(id -u)"
    DOSSIER_DONNEES="$HOME/Library/Application Support/VLPM"
fi

if [ "$DESINSTALLER" = 1 ]; then
    launchctl bootout "$DOMAINE/$ETIQUETTE" 2>/dev/null || true
    rm -f "$PLIST"
    echo "Service désinstallé. Les données restent dans :"
    echo "  $DOSSIER_DONNEES"
    exit 0
fi

# Le binaire doit être à un emplacement stable : launchd relance un chemin
# absolu, pas le dossier depuis lequel on a lancé ce script.
SOURCE="$(cd "$(dirname "$0")/.." && pwd)/vlpm"
if [ ! -x "$SOURCE" ]; then
    echo "Binaire introuvable : $SOURCE" >&2
    echo "Compilez-le d'abord :  make build" >&2
    exit 1
fi

DESTINATION=/usr/local/bin/vlpm
if [ "$SYSTEME" = 1 ]; then
    install -d /usr/local/bin
    install -m 0755 "$SOURCE" "$DESTINATION"
else
    # Sans sudo, /usr/local/bin n'est pas toujours accessible en écriture.
    if install -m 0755 "$SOURCE" "$DESTINATION" 2>/dev/null; then
        :
    else
        DESTINATION="$HOME/.local/bin/vlpm"
        mkdir -p "$(dirname "$DESTINATION")"
        install -m 0755 "$SOURCE" "$DESTINATION"
    fi
fi

mkdir -p "$DOSSIER_DONNEES"
chmod 0750 "$DOSSIER_DONNEES"
JOURNAL="$DOSSIER_DONNEES/vlpm.log"

if [ "$SYSTEME" = 1 ]; then
    chown -R "$UTILISATEUR_SERVICE" "$DOSSIER_DONNEES"
fi

mkdir -p "$(dirname "$PLIST")"
cat > "$PLIST" <<PLIST_EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>$ETIQUETTE</string>

    <key>ProgramArguments</key>
    <array>
        <string>$DESTINATION</string>
        <string>--addr</string>
        <string>0.0.0.0:$PORT</string>
        <string>--data</string>
        <string>$DOSSIER_DONNEES</string>
    </array>

    <!-- RunAtLoad : démarre au boot (daemon) ou à l'ouverture de session (agent). -->
    <key>RunAtLoad</key>
    <true/>

    <!-- KeepAlive inconditionnel : launchd relance le processus quelle que soit
         la raison de son arrêt, plantage comme sortie propre. -->
    <key>KeepAlive</key>
    <true/>

    <!-- Délai minimal entre deux relances. En dessous, une boucle de plantage
         consommerait le processeur sans laisser le temps de lire le journal. -->
    <key>ThrottleInterval</key>
    <integer>10</integer>

    <key>StandardOutPath</key>
    <string>$JOURNAL</string>
    <key>StandardErrorPath</key>
    <string>$JOURNAL</string>

    <key>WorkingDirectory</key>
    <string>$DOSSIER_DONNEES</string>
$( [ "$SYSTEME" = 1 ] && cat <<USER_EOF

    <key>UserName</key>
    <string>$UTILISATEUR_SERVICE</string>
USER_EOF
)
</dict>
</plist>
PLIST_EOF

chmod 0644 "$PLIST"

# bootout puis bootstrap : recharge proprement une version déjà installée.
launchctl bootout "$DOMAINE/$ETIQUETTE" 2>/dev/null || true
launchctl bootstrap "$DOMAINE" "$PLIST"
launchctl enable "$DOMAINE/$ETIQUETTE" 2>/dev/null || true

sleep 2

echo ""
echo "Service installé et démarré."
echo ""
echo "  Interface   : http://localhost:$PORT"
echo "  Données     : $DOSSIER_DONNEES"
echo "  Journal     : $JOURNAL"
echo ""
if [ "$SYSTEME" = 1 ]; then
    echo "  Démarrage   : au boot, sans session ouverte"
else
    echo "  Démarrage   : à l'ouverture de votre session"
    echo "                (pour un démarrage au boot : sudo $0 --systeme)"
fi
echo ""
echo "  État        : launchctl print $DOMAINE/$ETIQUETTE | head -20"
echo "  Journal     : tail -f \"$JOURNAL\""
echo "  Arrêt       : launchctl bootout $DOMAINE/$ETIQUETTE"
echo "  Désinstaller: $0 --desinstaller"
echo ""
echo "Le mot de passe administrateur du premier démarrage figure dans le journal :"
echo "  grep -A3 'PREMIER DÉMARRAGE' \"$JOURNAL\""
