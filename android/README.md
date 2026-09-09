# Application Android VLPM

Coque native autour de l'interface web de VLPM, distribuée **par APK** et non
par un magasin d'applications : un outil interne de police municipale n'a pas
vocation à figurer sur Google Play.

## Pourquoi une application, et pas simplement le navigateur

Trois choses qu'un navigateur ne peut pas offrir :

- **Le scan des QR codes fonctionne sans HTTPS.** Les navigateurs refusent
  l'accès à la caméra hors connexion chiffrée. Une application native n'a pas
  cette restriction : une commune dont l'instance n'a pas encore de certificat
  dispose malgré tout d'un scanner opérationnel.
- **L'adresse du serveur se saisit une seule fois**, à la première ouverture.
- **Le jeton et l'adresse sont conservés chiffrés** (`EncryptedSharedPreferences`)
  plutôt que dans le stockage du navigateur.

Le reste — prise en compte, restitution, mode hors ligne, photos de constat —
vient de l'interface web. **Il n'y a qu'un seul code métier à maintenir**, ce
qui est déterminant pour une association sans développeur permanent.

## Compiler

Rien à installer si Android Studio est présent : il fournit le JDK et le SDK.

```bash
cd android
JAVA_HOME="/Applications/Android Studio.app/Contents/jbr/Contents/Home" \
  ./gradlew assembleRelease
```

L'APK sort dans `app/build/outputs/apk/release/`.

## Signer pour la distribution

Un APK installé hors magasin doit être signé. Sans trousseau fourni, la
compilation retombe sur la clé de débogage — suffisant pour essayer, à
proscrire pour un déploiement : **une mise à jour signée par une autre clé
oblige à désinstaller l'application**, ce qui efface ses données.

Créez le trousseau une fois pour toutes, et **conservez-le** :

```bash
keytool -genkeypair -v -keystore vlpm.jks -keyalg RSA -keysize 4096 \
        -validity 10000 -alias vlpm
```

`vlpm.jks` ne doit **jamais** être versionné — le `.gitignore` l'exclut déjà.
Perdez-le, et plus aucune mise à jour ne pourra être installée par-dessus.

## Installer sur un terminal

```bash
adb install -r app/build/outputs/apk/release/app-release.apk
```

Ou transmettez le fichier APK aux agents. Android demandera l'autorisation
d'installer depuis une source inconnue : c'est le comportement normal d'une
distribution hors magasin.

## Première ouverture

L'application demande l'adresse de l'instance du service, par exemple
`https://vlpm.villeexemple.fr`. Elle vérifie que l'adresse répond bien à un
serveur VLPM avant de l'enregistrer, plutôt que de laisser l'agent découvrir
son erreur à l'écran de connexion.

Le protocole peut être omis : `vlpm.villeexemple.fr` suffit, HTTPS est
présumé. Une instance en HTTP doit être saisie en toutes lettres, et
l'application avertit alors que la connexion n'est pas chiffrée.

## Limites connues

- **Testée sur émulateur uniquement** (Android 16). Le scan d'un QR code réel
  n'a pas pu être vérifié, faute de caméra physique : le scanner s'ouvre et
  lit correctement, mais le décodage d'une étiquette imprimée reste à
  confirmer sur un vrai terminal.
- **Aucune notification** : les échéances arrivent par courriel, pas par
  notification Android.
- **Pas de mise à jour automatique.** Un nouvel APK doit être distribué et
  installé à la main.
