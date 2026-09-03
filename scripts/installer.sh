#!/bin/sh
# VLPM — installation sur un serveur Linux (VPS, serveur maison, Raspberry Pi).
#
# Installe le binaire, crée un utilisateur système dédié et enregistre un
# service systemd qui démarre automatiquement au boot.
#
# Usage :  sudo ./installer.sh [chemin-du-binaire]

set -eu

BINAIRE="${1:-./vlpm}"
DESTINATION=/usr/local/bin/vlpm
DOSSIER_DONNEES=/var/lib/vlpm
UTILISATEUR=vlpm
SERVICE=/etc/systemd/system/vlpm.service

if [ "$(id -u)" -ne 0 ]; then
    echo "Ce script doit être lancé avec sudo." >&2
    exit 1
fi

if [ ! -f "$BINAIRE" ]; then
    echo "Binaire introuvable : $BINAIRE" >&2
    echo "Indiquez son chemin : sudo ./installer.sh /chemin/vers/vlpm-linux-amd64" >&2
    exit 1
fi

if ! command -v systemctl >/dev/null 2>&1; then
    echo "systemd est absent de ce système." >&2
    echo "Lancez le binaire à la main, ou utilisez Docker (voir le README)." >&2
    exit 1
fi

echo "Installation du binaire dans $DESTINATION"
install -m 0755 "$BINAIRE" "$DESTINATION"

if ! id "$UTILISATEUR" >/dev/null 2>&1; then
    echo "Création de l'utilisateur système $UTILISATEUR"
    useradd --system --home-dir "$DOSSIER_DONNEES" --shell /usr/sbin/nologin "$UTILISATEUR"
fi

# 0750 : la base contient des données nominatives d'agents, elle ne doit pas
# être lisible par les autres comptes de la machine.
mkdir -p "$DOSSIER_DONNEES"
chown "$UTILISATEUR:$UTILISATEUR" "$DOSSIER_DONNEES"
chmod 0750 "$DOSSIER_DONNEES"

echo "Enregistrement du service systemd"
cat > "$SERVICE" <<SERVICE_EOF
[Unit]
Description=VLPM — gestion du parc automobile
Documentation=https://github.com/assopmf/vlpm
After=network-online.target
Wants=network-online.target

# systemd abandonne par défaut après 5 redémarrages en 10 secondes. Pour un
# poste de police, mieux vaut un service qui s'obstine qu'un service mort :
# la limite est levée. Une boucle de redémarrage reste visible au journal
# (journalctl -u vlpm). Ces deux clés appartiennent à [Unit] : placées dans
# [Service], systemd les ignore.
StartLimitIntervalSec=0
StartLimitBurst=0

[Service]
Type=simple
User=$UTILISATEUR
Group=$UTILISATEUR
ExecStart=$DESTINATION --addr 127.0.0.1:8080 --data $DOSSIER_DONNEES

# always et non on-failure : un serveur qui s'arrête proprement doit repartir
# tout autant qu'un serveur qui plante. Sans cela, une sortie en code 0 --
# arrêt sur signal, cas limite non prévu -- laisserait le service éteint sans
# que personne ne s'en aperçoive avant la prochaine prise de service.
Restart=always
RestartSec=5

# Renseignez l'URL publique pour que les QR codes collés dans les véhicules
# s'ouvrent directement depuis l'appareil photo d'un téléphone.
#Environment=VLPM_BASE_URL=https://vlpm.villeexemple.fr
#Environment=VLPM_DERRIERE_PROXY=true

# Cloisonnement : le service n'accède qu'à son propre dossier de données.
NoNewPrivileges=true
PrivateTmp=true
PrivateDevices=true
ProtectSystem=strict
ProtectHome=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictAddressFamilies=AF_INET AF_INET6
RestrictNamespaces=true
LockPersonality=true
MemoryDenyWriteExecute=true
ReadWritePaths=$DOSSIER_DONNEES

[Install]
WantedBy=multi-user.target
SERVICE_EOF

systemctl daemon-reload
systemctl enable --now vlpm

echo ""
echo "Installation terminée."
echo ""
echo "  Interface     : http://127.0.0.1:8080"
echo "  État          : systemctl status vlpm"
echo "  Journal       : journalctl -u vlpm -f"
echo "  Données       : $DOSSIER_DONNEES"
echo ""
echo "Le mot de passe du compte administrateur figure dans le journal du"
echo "premier démarrage :"
echo ""
echo "  journalctl -u vlpm | grep -A3 'PREMIER DÉMARRAGE'"
echo ""
echo "Le service écoute sur 127.0.0.1 : placez un reverse proxy HTTPS devant"
echo "avant de l'exposer sur Internet. La caméra du téléphone, nécessaire au"
echo "scan des QR codes, exige une connexion HTTPS."
