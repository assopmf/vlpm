# Contribuer

VLPM est développé pour les services de police municipale, par une association
bénévole. Les contributions sont bienvenues, en particulier venant de services
qui l'utilisent : un retour de terrain vaut plus qu'une fonctionnalité de plus.

## Avant d'ouvrir une *pull request*

```bash
make verifier   # format, vet et tests
```

Ces trois-là doivent passer. Le dépôt n'a pas de configuration de formatage
propre : `gofmt` fait autorité.

## Ce que le projet cherche à rester

**Installable sans compétence particulière.** Un service peut n'avoir personne
pour administrer un serveur. C'est pourquoi il n'y a ni base de données
externe, ni conteneur obligatoire, ni étape de compilation du front : un
exécutable et un dossier de données.

Concrètement, deux règles :

- **Pas de dépendance nécessitant cgo.** La compilation croisée vers les six
  plateformes doit rester possible sans chaîne d'outils C.
- **Pas d'outillage Node pour l'interface.** `web/static/` contient du HTML,
  du CSS et du JavaScript servis tels quels. Corriger un libellé doit rester
  possible avec un éditeur de texte.

Toute dépendance nouvelle doit se justifier : l'algorithme TOTP a été écrit
avec la bibliothèque standard plutôt que d'ajouter un paquet.

## Langue

Le code, les commentaires, les messages de commit et l'interface sont **en
français**. Les personnes qui reprendront ce projet dans une commune ne sont
pas nécessairement anglophones, et les noms de champs de l'API sont eux aussi
en français pour rester lisibles.

## Ce qui touche aux données personnelles

L'application trace nominativement l'activité d'agents publics. Une
contribution qui ajoute une donnée collectée, allonge une conservation ou
élargit un accès doit l'expliquer dans son message de commit. En cas de doute,
ouvrez d'abord une issue.

## Licence

En contribuant, vous acceptez que votre travail soit distribué sous
[EUPL-1.2](LICENSE).
