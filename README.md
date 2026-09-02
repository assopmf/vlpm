# VLPM — gestion du parc automobile d'une police municipale

Application web autonome pour suivre les véhicules de patrouille : qui détient
quel véhicule, depuis quand, à quel kilométrage, dans quel état.

Un seul exécutable, aucune dépendance à installer, aucune base de données à
administrer. L'interface, l'API et le moteur de stockage sont embarqués dans le
binaire.

> **État du projet** — utilisable et testé, mais pas encore éprouvé en service
> réel. Voir « Limites connues » en fin de document.

---

## Installation

### Le plus simple : l'exécutable

Téléchargez le fichier correspondant à votre machine, placez-le dans un dossier,
lancez-le.

| Système | Fichier |
|---|---|
| Linux (VPS, serveur x86) | `vlpm-linux-amd64` |
| Linux ARM (Raspberry Pi 4/5) | `vlpm-linux-arm64` |
| Raspberry Pi plus ancien | `vlpm-linux-arm` |
| macOS (Apple Silicon) | `vlpm-darwin-arm64` |
| macOS (Intel) | `vlpm-darwin-amd64` |
| Windows | `vlpm-windows-amd64.exe` |

```bash
chmod +x vlpm-linux-amd64
./vlpm-linux-amd64
```

Sur Windows, double-cliquez `demarrer.bat`. Sur macOS et Linux, `demarrer.sh`.

L'application démarre sur <http://localhost:8080> et affiche dans la console le
mot de passe du compte administrateur, **généré aléatoirement au premier
démarrage et affiché une seule fois**. Notez-le.

Les données sont écrites dans un sous-dossier `data`, à côté de l'exécutable.
Déplacer ce dossier déplace toute l'installation ; le copier suffit à la
sauvegarder.

### Serveur Linux, en service permanent

```bash
sudo ./scripts/installer.sh ./vlpm-linux-amd64
```

Le script crée un utilisateur système dédié, installe le binaire et enregistre
un service `systemd` qui démarre au boot et redémarre en cas d'incident. Il
place les données dans `/var/lib/vlpm`, en `0750`.

```bash
systemctl status vlpm          # état du service
journalctl -u vlpm -f          # journal en direct
journalctl -u vlpm | grep -A3 'PREMIER DÉMARRAGE'   # retrouver le mot de passe initial
```

### Docker

```bash
docker compose up -d
docker compose logs | grep -A3 'PREMIER DÉMARRAGE'
```

Les données vivent dans un volume nommé : mettre à jour l'image ne les efface
pas.

### Hébergement mutualisé

**Non pris en charge.** Un hébergement mutualisé classique n'exécute que du PHP
et n'autorise pas de processus permanent. VLPM a besoin d'un VPS, d'un serveur
dédié ou d'une machine à demeure — un Raspberry Pi à 60 € suffit largement pour
un parc communal.

---

## Mise en production

### HTTPS : indispensable

Le scan des QR codes utilise la caméra du téléphone, et **les navigateurs
n'autorisent l'accès à la caméra qu'en HTTPS** (hors `localhost`). Sans
certificat, la saisie manuelle du code véhicule reste possible, mais le scan ne
fonctionnera pas.

Exemple avec Caddy, qui obtient le certificat automatiquement :

```
vlpm.villeexemple.fr {
    reverse_proxy 127.0.0.1:8080
}
```

Lancez alors VLPM avec :

```bash
vlpm --addr 127.0.0.1:8080 \
     --base-url https://vlpm.villeexemple.fr \
     --derriere-proxy
```

`--base-url` est ce qui est encodé dans les QR codes. Sans cette option, les
étiquettes restent lisibles depuis l'application mais pas depuis l'appareil
photo du téléphone.

### Sauvegardes

Toute la base tient dans un fichier. Copiez-le, c'est tout :

```bash
sqlite3 /var/lib/vlpm/vlpm.db ".backup '/sauvegardes/vlpm-$(date +%F).db'"
```

À défaut de `sqlite3`, arrêtez le service et copiez `vlpm.db`, `vlpm.db-wal` et
`vlpm.db-shm` ensemble.

---

## Options

Chaque option a son équivalent en variable d'environnement.

| Option | Variable | Défaut | Rôle |
|---|---|---|---|
| `--addr` | `VLPM_ADDR` | `:8080` | Adresse d'écoute |
| `--data` | `VLPM_DATA` | `./data` | Dossier des données |
| `--base-url` | `VLPM_BASE_URL` | — | URL publique, encodée dans les QR codes |
| `--derriere-proxy` | `VLPM_DERRIERE_PROXY` | `false` | Fait confiance à `X-Forwarded-For` |
| `--dev` | `VLPM_DEV` | `false` | Sert l'interface depuis le disque |

---

## Utilisation

### Rôles

| Rôle | Peut faire |
|---|---|
| **Agent** | Prendre en compte et restituer un véhicule, signaler un incident, consulter ses propres sorties |
| **Chef de service** | Tout cela, plus : gérer le parc, les agents, les entretiens, clôturer la sortie d'un agent absent, exporter l'historique |
| **Administrateur** | Tout cela, plus : gérer les comptes administrateurs et consulter le journal d'activité |

### Le parcours quotidien

1. L'agent scanne le QR code collé dans le véhicule, ou le choisit dans la liste.
2. Il relève le kilométrage, coche l'état des lieux, indique le motif de sortie.
3. Au retour, il saisit le kilométrage d'arrivée et signale un éventuel incident.

Un incident déclaré « immobilisant » place automatiquement le véhicule en
maintenance : il ne peut plus être pris en compte tant qu'un chef n'a pas
enregistré la remise en état.

### Étiquettes QR

Depuis la fiche d'un véhicule, un chef génère l'étiquette PNG à imprimer et
coller dans l'habitacle. Le QR encode un jeton opaque, jamais le numéro
d'immatriculation ni l'identifiant interne : une étiquette photographiée
n'apprend rien à qui n'a pas de compte.

### Contrôles de cohérence

Le kilométrage ne peut pas reculer, et un écart de plus de 1 500 km sur une
seule sortie demande une confirmation explicite. Cette règle vient d'un défaut
observé sur le prototype, qui enregistrait sans broncher « + 18 053 km
parcourus » pour une sortie de vingt-quatre minutes.

Pour ajuster ce seuil, modifiez `KMDeltaMax` dans
[`internal/store/checkouts.go`](internal/store/checkouts.go).

---

## API

L'interface web ne consomme que l'API publique : tout ce qu'elle fait, une
application Android pourra le faire. La documentation complète est dans
[`docs/api.md`](docs/api.md).

```bash
# Ouvrir une session
curl -X POST http://localhost:8080/api/v1/auth/login \
     -H 'Content-Type: application/json' \
     -d '{"matricule":"1204","mot_de_passe":"…"}'

# Utiliser le jeton renvoyé
curl http://localhost:8080/api/v1/vehicules \
     -H "Authorization: Bearer <jeton>"
```

L'authentification se fait par jeton porteur, sans cookie de session : le même
mécanisme sert à l'interface web et servira à l'application mobile.

---

## Développement

```bash
make dev        # serveur avec interface rechargée depuis le disque
make verifier   # format, vet et tests
make dist       # exécutables des six plateformes dans dist/
```

Il n'y a **pas d'étape de compilation du front** : `web/static/` contient du
HTML, du CSS et du JavaScript servis tels quels. Corriger un libellé se fait
avec un éditeur de texte, sans installer Node.

```
cmd/vlpm/          point d'entrée
internal/store/    schéma SQLite, requêtes, règles métier
internal/api/      routes HTTP et validation des entrées
internal/auth/     mots de passe et sessions
web/static/        interface (HTML, CSS, JS)
```

Les données transitent par l'API en français (`vehicules`, `prises`, `km`) pour
rester lisibles par les personnes qui reprendront le projet.

---

## Choix techniques

**Go et SQLite.** Le pilote SQLite est écrit en Go pur : le binaire est
statique, se compile pour six plateformes sans chaîne d'outils C, et n'a besoin
de rien sur la machine cible.

**Une instance par commune.** Chaque service installe la sienne et reste seul
responsable de ses données. Pas de convention entre communes à rédiger, pas de
question de responsable de traitement à trancher.

**Aucun cookie d'authentification.** Le jeton voyage dans l'en-tête
`Authorization`, ce qui supprime toute surface CSRF et aligne l'interface web
sur la future application mobile.

**Jetons hachés.** Seul le SHA-256 des jetons de session est stocké : une copie
de la base ne permet pas de rejouer les sessions ouvertes.

---

## Limites connues

- **Pas encore éprouvé en service réel.** Le parcours complet est testé, mais
  aucune commune ne l'utilise encore au quotidien.
- **Image Docker non vérifiée.** Le `Dockerfile` est écrit mais n'a pas pu être
  construit sur la machine de développement. À valider avant tout déploiement
  par ce biais.
- **Pas de mode hors ligne.** Une prise en compte dans un parking souterrain
  sans réseau échouera. C'est le manque le plus gênant à l'usage.
- **Pas de photos.** On ne peut pas joindre de cliché à un constat de dommage.
- **Pas de notifications** d'échéance de contrôle technique ou de révision : les
  dates sont enregistrées et affichées, mais rien ne les rappelle.
- **Sauvegarde manuelle.** Aucune sauvegarde automatique n'est programmée.
