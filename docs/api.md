# API VLPM v1

Toutes les routes sont préfixées par `/api/v1`. Les corps de requête et de
réponse sont en JSON, encodés en UTF-8.

## Authentification

L'API utilise un jeton porteur. Aucun cookie n'est employé : l'interface web et
une application mobile s'authentifient exactement de la même façon.

```http
POST /api/v1/auth/login
Content-Type: application/json

{"matricule": "1204", "mot_de_passe": "…"}
```

```json
{
  "token": "yBk3…",
  "expire_dans_secondes": 43200,
  "user": {"id": 2, "matricule": "1204", "nom": "Martin", "prenom": "Camille",
           "role": "agent", "actif": true, "must_change_password": true}
}
```

Le jeton accompagne ensuite chaque requête :

```http
Authorization: Bearer yBk3…
```

Il vaut 12 heures. Passé ce délai, l'API répond `401` avec le code
`non_authentifie` ; il faut se reconnecter.

Deux limites encadrent les échecs de connexion, appliquées avant même la
comparaison du mot de passe, et répondant `429 trop_de_tentatives` :

| Axe | Limite | Ce qu'il couvre |
|---|---|---|
| Adresse IP | 10 par quart d'heure | un balayage d'identifiants depuis une machine |
| Matricule | 5 par quart d'heure | le même compte attaqué depuis plusieurs adresses |

Les compteurs sont persistés : un redémarrage du serveur ne les remet pas à
zéro. Le blocage se lève seul au bout de la fenêtre — jamais de verrouillage
définitif, qui permettrait de paralyser un service entier. Une connexion
réussie solde le compteur.

Le matricule est comptabilisé qu'il corresponde ou non à un compte existant :
la réponse ne renseigne donc pas sur l'existence d'un compte.

## Erreurs

Toute erreur renvoie le même format. Le champ `erreur` est un message en
français, rédigé pour être affiché tel quel à l'utilisateur ; `code` est un
identifiant stable, destiné au traitement programmatique.

```json
{"erreur": "Ce véhicule est déjà pris en compte par un autre agent.",
 "code": "deja_en_service"}
```

| Code | HTTP | Signification |
|---|---|---|
| `non_authentifie` | 401 | Jeton absent, invalide ou expiré |
| `identifiants` | 401 | Matricule ou mot de passe incorrect |
| `droits_insuffisants` | 403 | Rôle insuffisant pour cette action |
| `compte_inactif` | 403 | Le compte a été désactivé |
| `introuvable` | 404 | Ressource inexistante |
| `deja_en_service` | 409 | Véhicule déjà pris en compte |
| `deja_detenteur` | 409 | L'agent détient déjà un autre véhicule |
| `agent_introuvable` | 404 | L'agent désigné par `agent_id` n'existe pas |
| `indisponible` | 409 | Véhicule en maintenance ou hors service |
| `doublon` | 409 | Code véhicule ou matricule déjà utilisé |
| `statut_pilote` | 409 | Statut « en service » non modifiable à la main |
| `km_incoherent` | 422 | Kilométrage refusé (voir plus bas) |
| `horodatage_invalide` | 422 | Date déclarée dans le futur ou trop ancienne |
| `date_invalide` | 400 | Date illisible (format RFC 3339 attendu) |
| `mot_de_passe_faible` | 422 | Moins de 10 caractères, ou sans chiffre |
| `trop_de_tentatives` | 429 | Trop d'échecs de connexion |

## Rôles

`agent` < `chef` < `admin`. Chaque route indique le rôle minimum exigé.

---

## Session

| Route | Rôle | Description |
|---|---|---|
| `POST /auth/login` | — | Ouvre une session |
| `POST /auth/logout` | agent | Ferme la session courante |
| `GET /moi` | agent | Profil, et véhicule détenu le cas échéant |
| `POST /moi/mot-de-passe` | agent | Change le mot de passe |
| `GET /moi/sessions` | agent | Liste les appareils connectés au compte |
| `DELETE /moi/sessions/{id}` | agent | Coupe une session précise |
| `POST /moi/sessions/revoquer-autres` | agent | Coupe tout sauf l'appareil courant |

`GET /moi` renvoie le véhicule que l'agent a en main, ce qui permet à un écran
d'accueil de proposer directement la restitution :

```json
{"user": {…}, "checkout_en_cours": {"id": 12, "vehicle_code": "TV1", …}}
```

Changer son mot de passe **ferme toutes les sessions**, y compris celle qui a
émis la requête.

`GET /moi/sessions` renvoie les sessions encore valides, avec une description
de l'appareil déduite de l'en-tête `User-Agent`, l'adresse d'origine et un
drapeau `actuelle`. Le jeton lui-même n'apparaît jamais.

```json
[{"id": 1, "appareil": "Android — Chrome", "ip": "10.0.0.5",
  "created_at": "2026-09-03T18:20:00Z", "expires_at": "2026-09-04T06:20:00Z",
  "actuelle": true}]
```

C'est la réponse au téléphone perdu : l'accès est retiré immédiatement, sans
attendre l'expiration ni l'intervention d'un chef. Une session appartenant à un
autre agent répond `404` — un identifiant deviné ne permet pas de déconnecter
quelqu'un d'autre.

**Note pour une application mobile** : renseignez un `User-Agent` explicite,
par exemple `VLPM-Android/1.0 (Android 14)`. L'agent reconnaîtra son téléphone
dans la liste au lieu d'y lire « Appareil inconnu ».

## Véhicules

| Route | Rôle | Description |
|---|---|---|
| `GET /vehicules` | agent | Liste du parc |
| `GET /vehicules/{id}` | agent | Fiche complète |
| `POST /vehicules` | chef | Crée un véhicule |
| `PATCH /vehicules/{id}` | chef | Modifie un véhicule |
| `GET /vehicules/{id}/qrcode.png` | chef | Étiquette QR (PNG) |
| `GET /scan/{token}` | agent | Résout un QR code |

`GET /vehicules` accepte `?statut=disponible|en_service|maintenance|hors_service`
et `?archives=1`. Chaque véhicule porte sa prise en compte en cours et son
nombre d'incidents ouverts :

```json
[{"id": 1, "code": "TV1", "marque": "Peugeot", "modele": "5008",
  "immatriculation": "AB-123-CD", "statut": "en_service", "km": 45280,
  "prochain_ct": "2026-11-18", "incidents_ouverts": 0,
  "checkout_en_cours": {"id": 12, "user_nom": "Camille Martin",
                        "started_at": "2026-09-02T11:54:03Z", "km_start": 45280}}]
```

`GET /vehicules/{id}` ajoute `historique`, `incidents` et `maintenances`.

Le statut `en_service` découle des prises en compte et **ne peut pas être posé
à la main** : `PATCH` le refuse avec le code `statut_pilote`. Le kilométrage ne
peut jamais être diminué.

`GET /vehicules/{id}/qrcode.png` accepte `?taille=` entre 128 et 1024 pixels.

## Prises en compte

| Route | Rôle | Description |
|---|---|---|
| `GET /prises` | agent | Historique |
| `POST /prises` | agent | Prend un véhicule en compte |
| `POST /prises/{id}/restitution` | agent | Restitue le véhicule |

`GET /prises` accepte `vehicule`, `agent`, `statut` (`en_cours` / `termine`),
`depuis`, `jusqua` (dates RFC 3339), `limite` et `offset`. **Un agent ne voit
que ses propres sorties** ; le filtre `agent` est ignoré pour lui.

### Prendre en compte

```http
POST /api/v1/prises

{"vehicule_id": 1, "km": 45280, "motif": "Patrouille secteur centre",
 "notes": "", "check": {"carburant": true, "proprete": true}, "force": false}
```

`qr_token` remplace `vehicule_id` lorsque la prise se fait par scan. `check`
est un objet JSON libre : le serveur le stocke sans l'interpréter, ce qui permet
de faire évoluer l'état des lieux sans migration.

Refusée si le véhicule n'est pas disponible, ou si l'agent détient déjà un autre
véhicule.

#### Saisir au nom d'un agent

`agent_id` permet à un **chef** d'enregistrer la sortie d'un équipage qui ne peut
pas le faire lui-même — téléphone oublié, saisie au poste avant le départ.

```json
{"vehicule_id": 1, "km": 45280, "agent_id": 7, "motif": "Patrouille secteur centre"}
```

La sortie est alors attribuée à l'agent (`user_id`), et `saisi_par` porte
l'identifiant de celui qui a saisi. Sans cette distinction, la main courante
désignerait le chef comme détenteur du véhicule.

Un agent qui fournit un `agent_id` autre que le sien reçoit `403
droits_insuffisants`. Un compte désactivé ne peut pas se voir confier un
véhicule. La règle du détenteur unique porte sur l'agent, pas sur celui qui
saisit : un chef peut équiper plusieurs équipages d'affilée.

L'agent concerné restitue ensuite normalement, sans que cela compte comme une
clôture par un tiers.

### Restituer

```http
POST /api/v1/prises/12/restitution

{"km": 45412, "notes": "RAS", "check": {…}, "immobilise": false,
 "incidents": [{"type": "dommage", "gravite": "immobilisant",
                "description": "Pare-chocs arrière enfoncé."}]}
```

Un agent ne restitue que sa propre sortie ; un chef peut clôturer celle d'un
autre, et la clôture est alors tracée à son nom. Un incident `immobilisant`
place le véhicule en maintenance.

### Contrôle du kilométrage

Deux règles, à l'origine des réponses `422 km_incoherent` :

1. **Le compteur ne recule jamais.** Refus définitif, `force` n'y change rien.
2. **Au-delà de 1 500 km sur une sortie**, refus par défaut. Renvoyer la même
   requête avec `"force": true` la fait passer.

La seconde règle attrape les fautes de frappe sans bloquer les cas réels — un
véhicule ramené du garage avec plusieurs milliers de kilomètres de plus.

## Opérations différées

Une application mobile qui accepte des saisies sans réseau doit les rejouer
ensuite. Trois champs facultatifs, communs aux prises en compte, aux
restitutions et aux incidents, rendent ce rejeu sûr.

| Champ | Rôle |
|---|---|
| `cle_client` | Identifiant unique de l'opération, généré par le client (un UUID convient) |
| `date_debut` / `date_retour` | Heure réelle de l'opération, au format RFC 3339 |
| `hors_ligne` | Marque l'enregistrement comme saisi sans réseau |

### Rejeu sans doublon

Une requête peut aboutir alors que sa réponse se perd. Le client réessaie ;
sans précaution, une seconde sortie serait créée.

Une `cle_client` déjà reçue fait renvoyer l'enregistrement existant avec un
**`200 OK`** au lieu d'un `201 Created`. Le client traite les deux comme un
succès et retire l'opération de sa file. Rejouer est donc sans danger, et le
nombre de tentatives est sans importance.

Ce contrôle intervient **avant toute validation métier** : un rejeu ne se heurte
pas aux règles qui auraient refusé une nouvelle opération (« vous détenez déjà
un véhicule »), sinon le client resterait bloqué à rejouer indéfiniment.

### Heure déclarée et heure de réception

Sans `date_debut`, le serveur emploie sa propre horloge — c'est le cas d'une
saisie en ligne, où les deux coïncident.

Avec `date_debut`, l'enregistrement conserve les deux :

| Champ renvoyé | Signification |
|---|---|
| `started_at` / `ended_at` | Heure déclarée par l'appareil : celle de la sortie réelle |
| `enregistre_at` / `retour_enregistre_at` | Heure de réception par le serveur |
| `depart_hors_ligne` / `retour_hors_ligne` | Saisie effectuée sans réseau |

Une main courante qui décalerait les sorties de plusieurs heures n'aurait aucune
valeur : c'est l'heure déclarée qui fait foi à l'usage. L'horloge d'un téléphone
n'étant pas une source de confiance, l'heure de réception reste enregistrée à
côté, et l'écart est visible.

Deux bornes encadrent la date déclarée, sous peine de `422 horodatage_invalide` :

- pas plus de 5 minutes dans le futur (tolérance de dérive d'horloge) ;
- pas plus de 7 jours dans le passé.

Une restitution antérieure à la sortie qu'elle clôt est ramenée à l'heure du
serveur plutôt que refusée : l'opération est valide, seule l'horloge est fausse.

### Conflits

Le rejeu peut se heurter à un refus définitif : véhicule pris entre-temps par un
collègue (`409 deja_en_service`), kilométrage devenu incohérent
(`422 km_incoherent`). Réessayer n'y changera rien.

Un client doit distinguer trois situations :

| Situation | Conduite à tenir |
|---|---|
| Échec réseau (aucune réponse) | Garder l'opération, réessayer plus tard |
| `200` ou `201` | Retirer l'opération de la file |
| Toute autre réponse du serveur | Cesser de réessayer, présenter le motif à l'agent |

Une opération refusée ne doit jamais être supprimée en silence : l'agent a
physiquement pris le véhicule, et lui seul peut trancher.

## Incidents

| Route | Rôle | Description |
|---|---|---|
| `GET /incidents` | agent | Liste (`?statut=ouverts` par défaut) |
| `POST /incidents` | agent | Signale un incident |
| `POST /incidents/{id}/resolution` | chef | Marque comme résolu |

`type` : `dommage`, `panne`, `proprete`, `carburant`, `equipement`, `autre`.
`gravite` : `mineur`, `majeur`, `immobilisant`.

## Photos de constat

| Route | Rôle | Description |
|---|---|---|
| `POST /incidents/{id}/photos` | agent | Joint une photo à un incident |
| `GET /photos/{id}` | agent | Sert le fichier |
| `DELETE /photos/{id}` | chef | Supprime la photo et son fichier |

L'envoi se fait en `multipart/form-data`, champ `photo`. Maximum 8 Mo.

Le format est déterminé **en lisant l'en-tête du fichier**, jamais d'après le
type annoncé : un client peut déclarer `image/jpeg` et envoyer autre chose. Les
formats acceptés sont JPEG, PNG et WebP ; tout autre contenu reçoit
`422 format_invalide`.

Les photos sont jointes à chaque incident renvoyé par `GET /incidents` et par
`GET /vehicules/{id}`, dans un tableau `photos` — vide, jamais nul.

`POST /prises/{id}/restitution` renvoie les identifiants des incidents créés,
pour permettre d'y attacher les photos aussitôt :

```json
{"prise": {…}, "incidents_crees": [42]}
```

**Note pour une application mobile** : réduisez et réencodez l'image avant
envoi. Cela retire les métadonnées EXIF, dont les coordonnées GPS, qui n'ont
pas à figurer dans un constat de carrosserie.

## Échéances

| Route | Rôle | Description |
|---|---|---|
| `GET /alertes` | agent | Contrôles techniques et révisions dépassés ou proches |

```json
[{"vehicule_id": 1, "code": "TV1", "modele": "Peugeot 5008",
  "type": "controle_technique", "gravite": "depassee",
  "echeance": "2026-08-26", "jours_restants": -8,
  "message": "Contrôle technique dépassé depuis 8 jours"}]
```

`type` vaut `controle_technique` ou `revision`, `gravite` vaut `depassee` ou
`proche`. Un contrôle technique est signalé 30 jours avant son échéance, une
révision 1 000 km avant le seuil. Les véhicules archivés sont exclus.

## Entretiens

| Route | Rôle | Description |
|---|---|---|
| `GET /entretiens` | agent | Liste (`?vehicule=`) |
| `POST /entretiens` | chef | Enregistre un entretien |

`type` : `revision`, `reparation`, `controle_technique`, `pneus`, `carburant`,
`autre`. Le montant se transmet en euros sous forme de chaîne (`"149,90"` ou
`"149.90"`) et se relit en centimes dans `cout_cents`. `remettre_en_service:
true` fait repasser en `disponible` un véhicule en maintenance.

## Pilotage

| Route | Rôle | Description |
|---|---|---|
| `GET /stats` | agent | Compteurs du tableau de bord |
| `GET /export/prises.csv` | chef | Historique des sorties |
| `GET /export/parc.csv` | chef | Inventaire du parc et échéances |
| `GET /export/incidents.csv` | chef | Signalements et suivi de sinistralité |
| `GET /export/entretiens.csv` | chef | Entretiens et coûts, avec ligne de total |
| `GET /journal` | admin | Journal d'audit |

Tous les CSV sont encodés en UTF-8 avec BOM et séparés par des points-virgules,
pour s'ouvrir directement dans Excel en configuration française. Les montants
emploient la virgule décimale, seule forme qu'Excel y reconnaît comme un nombre.

Filtres : `vehicule` sur les quatre, `statut`, `depuis` et `jusqua` sur
`prises.csv`, `statut` sur `incidents.csv`, `archives=1` sur `parc.csv`.

## Second facteur

Codes TOTP conformes à la RFC 6238 : HMAC-SHA1, pas de 30 secondes, 6
chiffres, tolérance d'un pas de part et d'autre.

| Route | Rôle | Description |
|---|---|---|
| `GET /moi/totp` | agent | État du second facteur pour le compte courant |
| `POST /moi/totp/preparer` | agent | Tire un secret, renvoie l'URI `otpauth://` |
| `GET /moi/totp/qrcode.png` | agent | QR d'inscription (uniquement avant activation) |
| `POST /moi/totp/activer` | agent | Valide un premier code, renvoie les codes de secours |
| `DELETE /moi/totp` | agent | Retire le second facteur (mot de passe exigé) |
| `GET /securite/totp` | admin | Politique du service |
| `PATCH /securite/totp` | admin | Modifie la politique |
| `POST /agents/{id}/totp/reinitialiser` | admin | Débloque un agent ayant tout perdu |

### Connexion

Quand le compte porte un second facteur actif, `POST /auth/login` répond
`401 totp_requis` si `code_totp` est absent. Le mot de passe était alors
correct : redemandez seulement le code, sans réinitialiser le formulaire.

```json
{"matricule": "0801", "mot_de_passe": "…", "code_totp": "123456"}
```

Le champ accepte aussi un **code de secours** (`ABCDE-FGHIJ`), reconnu à son
tiret et consommé définitivement.

Le mot de passe est vérifié **avant** le code : un code valide accompagné d'un
mauvais mot de passe renvoie `identifiants`, sans révéler que le compte porte
un second facteur.

Un `401 totp_requis` **ne compte pas** comme tentative échouée — un agent
mettant dix secondes à sortir son téléphone épuiserait sinon son quota. Un
`401 totp_invalide`, si.

Un code déjà employé est refusé pendant sa fenêtre de validité : le dernier pas
accepté est mémorisé, ce qui interdit de rejouer un code lu par-dessus
l'épaule.

### Politique

`PATCH /securite/totp` accepte `desactive`, `facultatif` ou
`obligatoire_chefs`. Le mode obligatoire est **refusé avec
`409 totp_auteur_manquant`** si l'administrateur qui le demande n'a pas
lui-même de second facteur actif : il se verrouillerait dehors.

Les comptes `agent` ne sont jamais soumis au mode obligatoire.

**Note pour une application mobile** : traitez `totp_requis` comme une seconde
étape du formulaire de connexion, en conservant le mot de passe déjà saisi.

## Conservation des données

Durées au-delà desquelles les données sont supprimées définitivement. Réservé
aux administrateurs.

| Route | Description |
|---|---|
| `GET /conservation` | Durées configurées, et aperçu de ce qu'une purge supprimerait |
| `PATCH /conservation` | Modifie les durées |
| `POST /conservation/simulation` | Chiffre l'effet de durées **sans les enregistrer** |
| `POST /conservation/purger` | Déclenche la purge immédiatement |

```json
{"activite_mois": 24, "journal_mois": 12}
```

`activite_mois` couvre les sorties terminées et les incidents résolus,
`journal_mois` le journal d'audit. `0` conserve indéfiniment, ce qui est la
valeur par défaut : aucune donnée n'est supprimée tant que personne ne l'a
décidé.

Minimums imposés : 12 mois pour l'activité, 6 mois pour le journal. En dessous,
la requête est refusée avec `422 duree_invalide`.

**Les sorties en cours et les incidents ouverts ne sont jamais supprimés**,
quelle que soit leur ancienneté.

`POST /conservation/simulation` accepte les mêmes champs et renvoie le
décompte sans rien modifier — à appeler avant d'enregistrer une durée, la
suppression étant irréversible :

```json
{"a_purger": {"sorties": 2, "incidents": 0, "journal": 2}}
```

La purge automatique s'exécute une fois par jour, immédiatement après la
sauvegarde quotidienne : un instantané récent existe donc toujours au moment où
des enregistrements disparaissent.

## Relevé d'échéances

| Route | Rôle | Description |
|---|---|---|
| `GET /notifications` | chef | Réglages, destinataires effectifs, état du relais |
| `PATCH /notifications` | chef | Modifie fréquence et destinataires |
| `POST /notifications/envoyer` | chef | Expédie un relevé immédiatement |

```json
{"frequence": "hebdomadaire", "destinataires": ["dgs@ville-exemple.fr"]}
```

`frequence` vaut `desactivee`, `quotidienne` ou `hebdomadaire`. Une adresse mal
formée est refusée avec `422 adresse_invalide` plutôt qu'ignorée en silence :
un chef qui croit être destinataire et ne reçoit jamais rien est pire que rien.

`GET /notifications` renvoie `envoi_configure` : à `false`, aucun relais SMTP
n'est configuré sur le serveur et les réglages resteront sans effet. Le relais
se configure au démarrage (voir le README), jamais par l'API — un mot de passe
SMTP n'a pas à être stocké en base.

Les chefs et administrateurs actifs dont le courriel est renseigné sont
destinataires d'office, en plus des adresses saisies. `destinataires_effectifs`
donne la liste consolidée, dédoublonnée.

`POST /notifications/envoyer` sert à vérifier la configuration. Il refuse avec
`422 rien_a_signaler` s'il n'y a aucune échéance : le relevé automatique ne
part jamais à vide non plus.

## Comptes

| Route | Rôle | Description |
|---|---|---|
| `GET /agents` | agent | Liste (`?inactifs=1`) |
| `POST /agents` | chef | Crée un compte |
| `PATCH /agents/{id}` | chef | Modifie un compte |
| `POST /agents/{id}/mot-de-passe` | chef | Réinitialise le mot de passe |

Sans `mot_de_passe` fourni, le serveur en génère un et le renvoie dans
`mot_de_passe_provisoire` — **affiché une seule fois**. Le compte est marqué
`must_change_password`.

Seul un `admin` crée ou modifie un compte `admin`. Le dernier administrateur
actif ne peut pas être désactivé, et personne ne peut désactiver son propre
compte.

## Divers

| Route | Rôle | Description |
|---|---|---|
| `GET /version` | agent | Version de l'application |
| `GET /healthz` | — | Sonde de santé (hors `/api/v1`) |

`/healthz` vérifie l'accès à la base et répond `503` si elle est injoignable.
C'est **la seule route ouverte** avec la connexion : `/version` exige une
session, sa divulgation renseignant un attaquant sur les failles connues d'une
version donnée.

---

## Notes pour une application mobile

- **Jeton** : à conserver dans le stockage chiffré du système (Keystore Android),
  jamais en clair.
- **Expiration** : sur un `401` de code `non_authentifie`, renvoyer vers l'écran
  de connexion.
- **Scan** : décoder le QR localement, puis appeler `GET /scan/{token}`. Le QR
  contient soit une URL complète, soit `vlpm:<jeton>` si le serveur n'a pas
  d'URL publique configurée — ne garder que le dernier segment.
- **Hors ligne** : voir « Opérations différées ». Joindre une `cle_client` à
  chaque opération, mémoriser l'heure réelle de la saisie, et traiter les refus
  du serveur comme définitifs.
