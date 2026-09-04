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

Pour que l'application redémarre toute seule après un plantage ou un
redémarrage de la machine, installez-la en service : voir
« Redémarrage automatique » plus bas.

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

La seconde commande affiche le mot de passe administrateur du premier
démarrage.

Les données vivent dans un volume nommé : mettre à jour l'image ne les efface
pas. Le conteneur tourne sans privilèges, en lecture seule et sous un compte
non privilégié (uid 10001) ; l'image pèse environ 36 Mo.

Pour changer le port publié :

```bash
VLPM_PORT=9000 docker compose up -d
```

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

### Redémarrage automatique

Un serveur qui ne repart pas seul n'est pas exploitable : personne ne surveille
une machine dans un poste de police. Chaque mode d'installation gère à la fois
le plantage du serveur et le redémarrage de la machine.

| Installation | Après un plantage | Après un redémarrage machine |
|---|---|---|
| `scripts/installer.sh` (Linux, systemd) | relance sous 5 s, sans limite | oui, au boot |
| `scripts/installer-macos.sh` | relance sous 10 s, sans limite | à l'ouverture de session, ou au boot avec `--systeme` |
| `scripts/installer-windows.ps1` | relance sous 1 min, 999 fois | oui, au boot |
| `docker compose` | relance immédiate | oui, si le démon Docker démarre au boot |

Deux réglages méritent d'être connus, parce que leur valeur par défaut est
mauvaise pour ce type de service :

- **systemd** abandonne par défaut après 5 relances en 10 secondes, et le
  service reste éteint. La limite est levée (`StartLimitIntervalSec=0`) : mieux
  vaut un service qui s'obstine qu'un service mort découvert à la prise de
  service. Une boucle de plantage reste visible dans `journalctl -u vlpm`.
- **systemd** utilise aussi `Restart=on-failure` dans la plupart des exemples,
  ce qui ne relance pas un processus sorti proprement. Ici c'est
  `Restart=always`.

#### macOS

```bash
make build
./scripts/installer-macos.sh                 # démarre à l'ouverture de session
sudo ./scripts/installer-macos.sh --systeme  # démarre au boot, sans session
```

Données dans `~/Library/Application Support/VLPM` (ou `/usr/local/var/vlpm` en
mode système). Désinstallation : `./scripts/installer-macos.sh --desinstaller`.

#### Windows

Dans un PowerShell **administrateur** :

```powershell
.\scripts\installer-windows.ps1
```

La tâche s'exécute sous le compte SYSTEM, donc sans session ouverte. Données
dans `C:\ProgramData\VLPM`.

#### Ce que la supervision ne couvre pas

Relancer un processus ne répare pas une base corrompue ni un disque plein : le
service repartirait en boucle. Surveillez `/healthz`, qui répond `503` si la
base est injoignable — c'est le point de contrôle à brancher sur votre
supervision si vous en avez une.

### Sauvegardes

Le serveur écrit **une sauvegarde par jour** dans `<data>/sauvegardes`, et
conserve les 14 dernières. C'est automatique, rien à configurer.

```bash
vlpm --sauvegardes 30     # en conserver 30
vlpm --sauvegardes 0      # toutes les conserver
vlpm --sauvegardes -1     # désactiver
```

Sauvegarde manuelle, à tout moment, serveur en marche ou non :

```bash
vlpm sauvegarder --data /var/lib/vlpm
```

La sauvegarde est un instantané cohérent obtenu par `VACUUM INTO` : inutile
d'arrêter le service, et le fichier produit s'ouvre directement comme une base
normale. Une simple copie de `vlpm.db` laisserait des transactions dans le
journal WAL et pourrait donner un fichier tronqué.

**Attention : ces sauvegardes sont sur le même disque que la base.** Elles
protègent d'une corruption ou d'une fausse manœuvre, pas d'une panne de disque
ni d'un vol de la machine. Copiez-les ailleurs :

```bash
rsync -a /var/lib/vlpm/sauvegardes/ sauvegarde@nas.mairie.local:/vlpm/
```

Pour restaurer, remplacez `vlpm.db` par la sauvegarde choisie, service arrêté.

### Conservation des données (RGPD)

L'application enregistre nominativement l'activité d'agents publics. Le RGPD
impose une **durée de conservation définie et justifiée** : elle relève du
délégué à la protection des données de la commune, pas du code. Elle se règle
donc depuis l'application, sans redéploiement.

Un administrateur la fixe dans **Réglages → Conservation des données**, avec
deux durées distinctes :

- **historique d'activité** : sorties terminées et incidents résolus ;
- **journal d'activité** : connexions, créations de comptes, actions sensibles.

**Par défaut, rien n'est supprimé.** Une instance qui effacerait des données
sans que personne l'ait décidé serait un défaut, pas une fonctionnalité.

Trois garde-fous, la suppression étant irréversible :

- l'écran chiffre **avant validation** le nombre exact d'enregistrements que la
  durée choisie supprimerait aujourd'hui ;
- une **sauvegarde est écrite juste avant** chaque purge automatique ;
- les **sorties en cours et les incidents ouverts ne sont jamais supprimés**,
  quelle que soit leur ancienneté. Une sortie ouverte depuis deux ans est une
  anomalie à traiter, pas une donnée à effacer.

La purge s'exécute une fois par jour. Un bouton permet de la déclencher
immédiatement.

Les durées minimales acceptées sont de 12 mois pour l'activité et 6 mois pour
le journal : en dessous, on effacerait des données de l'exercice en cours.

### Exposer l'API sur Internet

Connaître l'adresse de l'API ne donne accès à rien : hors la connexion et la
sonde de santé, toute route exige un jeton obtenu par matricule et mot de
passe. Une application mobile configurée avec la seule URL ne peut rien lire.

Reste que la page de connexion, elle, est joignable. Trois protections :

- **Blocage après échecs répétés**, sur l'adresse IP (10 essais par quart
  d'heure) **et sur le matricule** (5 essais). Cette seconde limite couvre le
  cas qu'une limite par IP laisse passer : le même compte attaqué depuis
  plusieurs adresses. Les compteurs sont en base, donc insensibles à un
  redémarrage du serveur.
- **Blocage temporaire, jamais définitif.** Il se lève seul au bout d'un quart
  d'heure. C'est un compromis assumé : quelqu'un connaissant le matricule d'un
  agent peut le gêner pendant quinze minutes. Un verrouillage définitif serait
  pire — il suffirait de viser tous les agents un dimanche soir pour empêcher
  la prise de service du lundi.
- **Réponse identique** que le matricule existe ou non, avec un temps de
  réponse égalisé : impossible d'énumérer les comptes.

**La mesure la plus efficace n'est pas applicative.** Si les agents disposent
d'un VPN ou d'une carte SIM du réseau de la collectivité, n'exposez l'API que
sur ce réseau : la page de connexion devient injoignable depuis Internet, ce
qui vaut mieux que n'importe quel durcissement.

### Téléphone perdu

Chaque agent voit ses appareils connectés dans **Réglages → Mes appareils
connectés**, et coupe celui qu'il a perdu. L'accès est retiré immédiatement,
sans attendre l'expiration du jeton ni l'intervention d'un chef.

Un chef garde les moyens plus radicaux : réinitialiser le mot de passe ou
désactiver le compte ferment toutes les sessions de l'agent.

### Perte du mot de passe administrateur

Il n'y a **pas d'envoi de courriel** : si le dernier administrateur perd son
mot de passe, personne ne peut le lui rendre depuis l'application. La commande
suivante rétablit l'accès, depuis la machine qui héberge l'instance :

```bash
vlpm reinitialiser-admin --data /var/lib/vlpm
```

Elle affiche un nouveau mot de passe provisoire, réactive et repromeut le
compte si nécessaire, ferme ses sessions ouvertes, et le recrée s'il avait été
supprimé. Le serveur peut rester en marche.

Cette commande n'est pas protégée par mot de passe, et n'a pas à l'être :
quiconque peut la lancer a déjà un accès en écriture au fichier de base, et
pourrait le modifier avec n'importe quel outil SQLite. **La protection réelle
est celle du système de fichiers** — d'où le compte système dédié et les
permissions `0750` posées par les installateurs.

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
| `--sauvegardes` | `VLPM_SAUVEGARDES` | `14` | Sauvegardes quotidiennes conservées (`0` toutes, `-1` désactive) |
| `--smtp-hote` | `VLPM_SMTP_HOTE` | — | Relais SMTP (vide = pas d'envoi) |
| `--smtp-port` | `VLPM_SMTP_PORT` | `587` | Port SMTP |
| `--smtp-utilisateur` | `VLPM_SMTP_UTILISATEUR` | — | Identifiant SMTP (vide pour un relais interne) |
| `--smtp-mot-de-passe` | `VLPM_SMTP_MOT_DE_PASSE` | — | Mot de passe SMTP |
| `--smtp-expediteur` | `VLPM_SMTP_EXPEDITEUR` | — | Adresse d'expédition (obligatoire si un relais est configuré) |
| `--smtp-tls-implicite` | `VLPM_SMTP_TLS_IMPLICITE` | `false` | Chiffrement d'emblée (port 465) |
| `--nom-service` | `VLPM_NOM_SERVICE` | — | Nom du service, affiché comme expéditeur |

Sous-commandes : `vlpm sauvegarder`, `vlpm reinitialiser-admin`, `vlpm aide`.

---

## Utilisation

### Rôles

| Rôle | Peut faire |
|---|---|
| **Agent** | Prendre en compte et restituer un véhicule, signaler un incident, consulter ses propres sorties |
| **Chef de service** | Tout cela, plus : gérer le parc, les agents, les entretiens, enregistrer une sortie au nom d'un agent, clôturer la sortie d'un agent absent, exporter les données |
| **Administrateur** | Tout cela, plus : gérer les comptes administrateurs et consulter le journal d'activité |

### Le parcours quotidien

Le véhicule est enregistré au nom d'**un seul agent : celui qui conduit et qui
répond du véhicule**. Un équipage de deux ou trois agents n'apparaît pas en
entier, et c'est voulu — la responsabilité du véhicule est individuelle.

1. L'agent scanne le QR code collé dans le véhicule, ou le choisit dans la liste.
2. Il relève le kilométrage, coche l'état des lieux, indique le motif de sortie.
3. Au retour, il saisit le kilométrage d'arrivée et signale un éventuel incident.

Un incident déclaré « immobilisant » place automatiquement le véhicule en
maintenance : il ne peut plus être pris en compte tant qu'un chef n'a pas
enregistré la remise en état.

### Équipage sans téléphone

Un chef peut enregistrer la sortie au nom d'un agent depuis le poste, via le
champ « Véhicule confié à » du formulaire de prise en compte. Le véhicule est
attribué à l'agent, qui en répond ; la saisie reste tracée au nom du chef, dans
la fiche comme dans le journal d'audit. L'agent restitue ensuite normalement.

### Exporter les données

Depuis les Réglages, un chef exporte quatre tableaux en CSV : historique des
sorties, état du parc, incidents, entretiens et coûts. Les fichiers s'ouvrent
directement dans Excel ou LibreOffice en configuration française, accents et
montants compris.

### Photos de constat

Un agent joint des photos au signalement d'un incident, au moment de la
restitution ou depuis la fiche du véhicule. Sur téléphone, le bouton ouvre
directement l'appareil photo.

**Les images sont réduites et réencodées dans le navigateur avant envoi.** Cela
supprime les métadonnées EXIF, dont les coordonnées GPS : une photo prise avec
un téléphone de service porte la position exacte de l'intervention, qui n'a
rien à faire dans un constat de carrosserie. Au passage, une photo de 4 Mo
tombe sous 100 Ko, ce qui compte en bord de route.

Le serveur vérifie le format en lisant l'en-tête du fichier, jamais le type
annoncé par le client. Les fichiers sont stockés dans `<data>/photos` sous un
nom aléatoire, en `0600`, et ne sont servis qu'à un utilisateur authentifié.
Seul un chef peut en supprimer une : une pièce de constat ne doit pas
disparaître sur décision d'un seul agent.

Les photos sont **hors des sauvegardes quotidiennes** : recopier plusieurs
gigaoctets d'images chaque jour n'est pas tenable. Elles étant immuables, une
synchronisation du dossier de données suffit — c'est ce que fait la commande
`rsync` donnée plus haut.

### Échéances

Le tableau de bord signale les contrôles techniques et révisions à programmer,
et l'écran **Échéances** les détaille. Un contrôle technique est annoncé 30
jours avant, une révision 1 000 km avant le seuil saisi sur la fiche du
véhicule. Sans date ni seuil renseigné, rien n'est signalé.

Pour ajuster ces seuils, modifiez `PreavisCTJours` et `PreavisRevisionKM` dans
[`internal/store/alertes.go`](internal/store/alertes.go).

#### Recevoir le relevé par courriel

Un service qui n'ouvre pas l'application ne verrait rien passer. Un relevé
périodique peut donc être expédié aux responsables.

Le relais SMTP se configure **au démarrage du serveur**, jamais depuis
l'interface : un mot de passe saisi dans l'application serait stocké en clair
dans la base, alors que les mots de passe des agents n'y sont que sous forme de
condensat. Cette asymétrie serait un piège.

```bash
vlpm --smtp-hote smtp.ville-exemple.fr \
     --smtp-expediteur vlpm@ville-exemple.fr \
     --nom-service "Police municipale de Ville-Exemple"
```

Un relais interne sans authentification, cas fréquent en mairie, fonctionne
tel quel : n'indiquez ni identifiant ni mot de passe. Avec authentification,
préférez les variables d'environnement pour le mot de passe
(`VLPM_SMTP_MOT_DE_PASSE`), afin qu'il n'apparaisse pas dans la liste des
processus. STARTTLS est utilisé dès que le relais l'annonce ; ajoutez
`--smtp-tls-implicite` pour un port 465.

La fréquence et les destinataires se règlent ensuite dans **Réglages → Relevé
d'échéances** : ce sont des choix de service, pas d'installation. Les chefs et
administrateurs dont le courriel est renseigné sont destinataires d'office.

**Aucun message n'est envoyé quand il n'y a rien à signaler.** Un relevé vide
reçu chaque semaine finit par ne plus être lu, et emporte les autres avec lui.

### Étiquettes QR

Depuis la fiche d'un véhicule, un chef génère l'étiquette PNG à imprimer et
coller dans l'habitacle. Le QR encode un jeton opaque, jamais le numéro
d'immatriculation ni l'identifiant interne : une étiquette photographiée
n'apprend rien à qui n'a pas de compte.

### Travailler sans réseau

Les parkings souterrains et les zones blanches sont la règle, pas l'exception.
L'application reste utilisable sans connexion :

- L'interface se charge depuis le cache du navigateur.
- Le dernier état connu du parc reste consultable, daté à l'écran.
- Une prise en compte ou une restitution saisie hors ligne est enregistrée sur
  l'appareil, puis transmise automatiquement au retour du réseau.

**L'heure retenue est celle de la saisie, pas celle de la synchronisation.** Une
sortie faite à 8 h et transmise à midi figure bien à 8 h dans la main courante.
L'heure de réception par le serveur est conservée à côté : l'écart entre les
deux reste visible, l'horloge d'un téléphone n'étant pas une source de
confiance. Une date incohérente — dans le futur, ou vieille de plus de sept
jours — est refusée.

Chaque opération porte une clé unique : si le réseau coupe entre la requête et
sa réponse, l'appareil réessaie sans créer de doublon.

En cas de conflit — deux agents ayant pris le même véhicule chacun de leur côté
— la saisie n'est jamais perdue silencieusement. Elle reste dans la file avec le
motif du refus, à l'écran « Opérations en attente », et l'agent décide.

Pour un usage quotidien, l'application s'ajoute à l'écran d'accueil du téléphone
(« Ajouter à l'écran d'accueil » depuis le navigateur). Le mode hors ligne exige
**HTTPS**, comme le scan des QR codes.

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
web/static/        interface (HTML, CSS, JS, service worker)
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

## Licence

[EUPL-1.2](LICENSE) — Licence Publique de l'Union Européenne.

Ce choix tient à la nature du projet : l'EUPL est conçue pour les
administrations publiques européennes, elle fait foi en français comme dans les
22 autres langues officielles, et son copyleft garantit qu'une commune qui
améliore l'application en fasse profiter les autres. Elle est compatible avec
l'AGPL, la GPL et plusieurs autres licences réciproques, dont la liste figure en
annexe du texte.

## Limites connues

- **Pas encore éprouvé en service réel.** Le parcours complet est testé, mais
  aucune commune ne l'utilise encore au quotidien.
- **Service worker non vérifié.** La mise en cache de l'interface a été écrite
  mais n'a pas pu être testée : le navigateur d'intégration utilisé pendant le
  développement refuse les service workers. La file d'attente hors ligne, elle,
  est testée et fonctionne. À valider sur un vrai téléphone avant déploiement.
- **Obligations RGPD à traiter avant déploiement.** L'application trace
  nominativement l'activité d'agents publics : inscription au registre des
  traitements, information des agents, et consultation des instances
  représentatives du personnel. La durée de conservation, elle, se règle
  désormais dans l'application (voir plus haut) : reste à la faire décider.
